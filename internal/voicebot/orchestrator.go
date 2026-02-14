package voicebot

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
	"github.com/liuscraft/orion-x/internal/text"
	"github.com/liuscraft/orion-x/internal/tools"
)

// State 表示语音机器人的状态
type State int

const (
	StateIdle State = iota
	StateListening
	StateProcessing
	StateSpeaking
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateListening:
		return "Listening"
	case StateProcessing:
		return "Processing"
	case StateSpeaking:
		return "Speaking"
	default:
		return "Unknown"
	}
}

// Orchestrator 对话编排器，负责状态管理、事件路由、组件协调
type Orchestrator interface {
	Start(ctx context.Context) error
	Stop() error
	GetState() State

	OnASRFinal(text string)
	OnUserSpeakingDetected()
	OnToolCall(tool string, args map[string]interface{})
	OnToolAudioReady(audio io.Reader)
	OnLLMTextChunk(chunk string)
	OnLLMFinished()
}

// OrchestratorObserver 可选观察者，用于外部订阅 LLM/TTS 事件
type OrchestratorObserver interface {
	OnLLMTextChunk(text string, emotion string)
	OnTTSSentence(text string, emotion string)
	OnTTSStart()
	OnTTSStop(isAborted bool)
}

// OrchestratorOptions 可选配置
type OrchestratorOptions struct {
	Observer     OrchestratorObserver
	TTSScheduler TTSSchedulerConfig
	Memory       memory.Service
}

// orchestratorImpl Orchestrator 实现
type orchestratorImpl struct {
	stateMachine *StateMachine
	eventBus     EventBus

	voiceAgent     agent.VoiceAgent
	audioOutPipe   audio.AudioOutPipe
	audioInPipe    audio.AudioInPipe
	toolExecutor   tools.ToolExecutor
	segmenter      *text.Segmenter
	markdownFilter agent.MarkdownFilter

	currentEmotion string
	ctx            context.Context
	cancel         context.CancelFunc

	// Agent context 管理（用于打断时取消 Agent）
	agentCtx    context.Context
	agentCancel context.CancelFunc

	// TTS 播放计数（用于追踪是否有 TTS 正在播放）
	ttsPendingCount int

	// 句子缓存与调度
	sentenceCache   []*SentenceRecord
	sentenceIndex   map[int64]*SentenceRecord
	pendingQueue    []int64
	inFlightQueue   []int64
	nextSentenceID  int64
	currentTurnID   int64
	ttsSchedulerCfg TTSSchedulerConfig
	scheduleMu      sync.Mutex

	wg sync.WaitGroup
	mu sync.Mutex

	observer  OrchestratorObserver
	memorySvc memory.Service

	turnStartedAt      time.Time
	turnUserText       string
	turnAssistantBuf   strings.Builder
	turnAborted        bool
	turnRecorded       bool
	activeAgentStreams int
}

// NewOrchestrator 创建新的Orchestrator
func NewOrchestrator(
	voiceAgent agent.VoiceAgent,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
) Orchestrator {
	return NewOrchestratorWithOptions(voiceAgent, audioOutPipe, audioInPipe, toolExecutor, nil)
}

// NewOrchestratorWithOptions 创建新的Orchestrator（带可选参数）
func NewOrchestratorWithOptions(
	voiceAgent agent.VoiceAgent,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
	opts *OrchestratorOptions,
) Orchestrator {
	var observer OrchestratorObserver
	if opts != nil {
		observer = opts.Observer
	}
	schedulerCfg := defaultTTSSchedulerConfig()
	if opts != nil {
		if opts.TTSScheduler.MaxInFlightSentences > 0 {
			schedulerCfg.MaxInFlightSentences = opts.TTSScheduler.MaxInFlightSentences
		}
		if opts.TTSScheduler.MaxCacheSentences >= 0 {
			schedulerCfg.MaxCacheSentences = opts.TTSScheduler.MaxCacheSentences
		}
	}
	return &orchestratorImpl{
		stateMachine:    NewStateMachine(),
		eventBus:        NewEventBus(),
		voiceAgent:      voiceAgent,
		audioOutPipe:    audioOutPipe,
		audioInPipe:     audioInPipe,
		toolExecutor:    toolExecutor,
		segmenter:       text.NewSegmenter(120),
		markdownFilter:  agent.NewMarkdownFilter(),
		observer:        observer,
		sentenceIndex:   make(map[int64]*SentenceRecord),
		ttsSchedulerCfg: schedulerCfg,
		memorySvc: func() memory.Service {
			if opts != nil {
				return opts.Memory
			}
			return nil
		}(),
	}
}

// Start 启动Orchestrator
func (o *orchestratorImpl) Start(ctx context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	logging.Infof("Orchestrator: starting...")
	o.ctx, o.cancel = context.WithCancel(ctx)

	o.eventBus.Subscribe(EventTypeStateChanged, o.handleStateChanged)
	o.eventBus.Subscribe(EventTypeUserSpeakingDetected, o.handleUserSpeakingDetected)
	o.eventBus.Subscribe(EventTypeASRFinal, o.handleASRFinal)
	o.eventBus.Subscribe(EventTypeToolCallRequested, o.handleToolCallRequested)
	o.eventBus.Subscribe(EventTypeToolAudioReady, o.handleToolAudioReady)
	o.eventBus.Subscribe(EventTypeLLMEmotionChanged, o.handleLLMEmotionChanged)

	logging.Infof("Orchestrator: event handlers registered")

	if o.audioInPipe != nil {
		logging.Infof("Orchestrator: starting AudioInPipe...")
		if err := o.audioInPipe.Start(o.ctx); err != nil {
			logging.Errorf("Orchestrator: failed to start AudioInPipe: %v", err)
			return err
		}
		logging.Infof("Orchestrator: AudioInPipe started")

		o.audioInPipe.OnASRResult(func(text string, isFinal bool) {
			if isFinal {
				// ASR final 表示用户说完了，直接处理，不触发打断
				logging.Infof("Orchestrator: ASR final result: %s", text)
				o.OnASRFinal(text)
			} else if text != "" {
				// 只有非 final 的中间结果才触发打断（用户正在说话）
				logging.Infof("Orchestrator: user speaking detected (interim): %s", text)
				o.OnUserSpeakingDetected()
			}
		})
		o.audioInPipe.OnUserSpeakingDetected(func() {
			logging.Infof("Orchestrator: VAD user speaking detected")
			o.OnUserSpeakingDetected()
		})
	}

	if o.audioOutPipe != nil {
		logging.Infof("Orchestrator: starting AudioOutPipe...")
		// 设置播放完成回调
		o.audioOutPipe.SetOnPlaybackFinished(o.onTTSPlaybackFinished)
		if err := o.audioOutPipe.Start(o.ctx); err != nil {
			logging.Errorf("Orchestrator: failed to start AudioOutPipe: %v", err)
			return err
		}
		logging.Infof("Orchestrator: AudioOutPipe started")
	}

	logging.Infof("Orchestrator: started successfully, current state: %s", o.stateMachine.GetCurrentState())
	return nil
}

// Stop 停止Orchestrator
func (o *orchestratorImpl) Stop() error {
	o.mu.Lock()

	logging.Infof("Orchestrator: stopping...")

	// 取消 Agent（如果正在运行）
	if o.agentCancel != nil {
		o.agentCancel()
		o.agentCancel = nil
	}

	if o.cancel != nil {
		o.cancel()
	}

	// 获取组件引用后释放锁，避免死锁
	// 因为子组件的 Stop 可能会触发回调，回调中需要获取锁
	audioInPipe := o.audioInPipe
	audioOutPipe := o.audioOutPipe
	o.mu.Unlock()

	// 在锁外调用子组件的 Stop 方法
	if audioInPipe != nil {
		logging.Infof("Orchestrator: stopping AudioInPipe...")
		audioInPipe.Stop()
	}

	if audioOutPipe != nil {
		logging.Infof("Orchestrator: stopping AudioOutPipe...")
		audioOutPipe.Stop()
	}

	logging.Infof("Orchestrator: waiting for goroutines to finish...")
	o.wg.Wait()

	logging.Infof("Orchestrator: stopped, final state: %s", o.stateMachine.GetCurrentState())
	return nil
}

// GetState 获取当前状态
func (o *orchestratorImpl) GetState() State {
	return o.stateMachine.GetCurrentState()
}

// OnASRFinal 处理ASR识别完成
func (o *orchestratorImpl) OnASRFinal(text string) {
	o.eventBus.Publish(NewASRFinalEvent(text))
}

// OnUserSpeakingDetected 处理用户说话检测
func (o *orchestratorImpl) OnUserSpeakingDetected() {
	// Handle synchronously to preserve ordering vs ASR final and avoid self-cancel.
	o.handleUserSpeakingDetected(NewUserSpeakingDetectedEvent())
}

// OnToolCall 处理工具调用
func (o *orchestratorImpl) OnToolCall(tool string, args map[string]interface{}) {
	o.eventBus.Publish(NewToolCallRequestedEvent(tool, args))
}

// OnToolAudioReady 处理工具返回音频
func (o *orchestratorImpl) OnToolAudioReady(audio io.Reader) {
	o.eventBus.Publish(NewToolAudioReadyEvent(audio))
}

// OnLLMTextChunk 处理LLM文本流
func (o *orchestratorImpl) OnLLMTextChunk(chunk string) {
	// logging.Infof("LLM chunk: %s", chunk)
}

// OnLLMFinished 处理LLM完成
func (o *orchestratorImpl) OnLLMFinished() {
	logging.Infof("LLM finished")
}

func (o *orchestratorImpl) handleStateChanged(event Event) {
	stateChangedEvent, ok := event.(*StateChangedEvent)
	if !ok {
		return
	}
	logging.Infof("State changed: %s -> %s", stateChangedEvent.OldState, stateChangedEvent.NewState)
}

func (o *orchestratorImpl) handleUserSpeakingDetected(event Event) {
	currentState := o.stateMachine.GetCurrentState()

	// 检查是否有 TTS 正在播放
	o.mu.Lock()
	ttsPending := o.ttsPendingCount > 0 || len(o.pendingQueue) > 0 || len(o.inFlightQueue) > 0
	o.mu.Unlock()

	// 只在 Processing、Speaking 状态或有 TTS pending 时才需要打断
	needInterrupt := currentState == StateSpeaking || currentState == StateProcessing || ttsPending
	if needInterrupt {
		o.mu.Lock()
		o.turnAborted = true
		pendingCount := o.ttsPendingCount
		o.mu.Unlock()
		logging.Infof("Orchestrator: UserSpeakingDetected - interrupting (state=%s, ttsPending=%d)", currentState, pendingCount)

		// 1. 取消 Agent（停止 LLM 生成）
		o.mu.Lock()
		if o.agentCancel != nil {
			logging.Infof("Orchestrator: cancelling Agent...")
			o.agentCancel()
			o.agentCancel = nil
		}
		o.mu.Unlock()

		// 2. 中断 TTS Pipeline（清空队列、停止播放）
		if o.audioOutPipe != nil {
			logging.Infof("Orchestrator: interrupting AudioOutPipe...")
			o.audioOutPipe.Interrupt()
		}

		// 3. 重置分句器
		o.segmenter.Flush()

		// 4. 清理 TTS 调度队列与计数
		o.abortPendingSentences()

		if o.observer != nil && ttsPending {
			o.observer.OnTTSStop(true)
		}

		// 5. 状态转换
		o.transitionTo(StateListening)
	}
}

func (o *orchestratorImpl) cacheSentence(text, emotion string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return
	}

	o.mu.Lock()
	o.nextSentenceID++
	id := o.nextSentenceID
	rec := &SentenceRecord{
		ID:      id,
		TurnID:  o.currentTurnID,
		Text:    trimmed,
		Emotion: emotion,
		Status:  SentencePending,
	}
	o.sentenceCache = append(o.sentenceCache, rec)
	o.sentenceIndex[id] = rec
	skipTTS := shouldSkipTTSSentence(trimmed)
	if skipTTS {
		rec.Status = SentenceSkipped
	} else {
		o.pendingQueue = append(o.pendingQueue, id)
	}
	o.enforceCacheLimitLocked()
	observer := o.observer
	o.mu.Unlock()

	if skipTTS {
		if observer != nil {
			observer.OnTTSSentence(trimmed, emotion)
		}
		return
	}

	o.tryScheduleTTS()
}

func shouldSkipTTSSentence(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

func (o *orchestratorImpl) tryScheduleTTS() {
	if o.audioOutPipe == nil {
		return
	}

	o.scheduleMu.Lock()
	defer o.scheduleMu.Unlock()

	for {
		o.mu.Lock()
		maxInFlight := o.ttsSchedulerCfg.MaxInFlightSentences
		if maxInFlight <= 0 {
			maxInFlight = 1
		}
		if o.ttsPendingCount >= maxInFlight || len(o.pendingQueue) == 0 {
			o.mu.Unlock()
			return
		}

		id := o.pendingQueue[0]
		o.pendingQueue = o.pendingQueue[1:]
		rec := o.sentenceIndex[id]
		if rec == nil || rec.Status != SentencePending || rec.TurnID != o.currentTurnID {
			o.mu.Unlock()
			continue
		}

		rec.Status = SentenceEnqueued
		o.inFlightQueue = append(o.inFlightQueue, id)
		shouldStart := o.ttsPendingCount == 0
		o.ttsPendingCount++
		text := rec.Text
		emotion := rec.Emotion
		observer := o.observer
		o.mu.Unlock()

		if shouldStart && observer != nil {
			observer.OnTTSStart()
		}
		if observer != nil {
			observer.OnTTSSentence(text, emotion)
		}

		if err := o.audioOutPipe.PlayTTS(text, emotion); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				logging.Infof("Orchestrator: PlayTTS cancelled (normal interruption)")
			} else {
				logging.Errorf("Orchestrator: PlayTTS error: %v", err)
			}

			o.mu.Lock()
			o.markSentenceStatusLocked(id, SentenceAborted)
			o.removeInFlightLocked(id)
			if o.ttsPendingCount > 0 {
				o.ttsPendingCount--
			}
			pending := o.ttsPendingCount
			hasPendingQueue := len(o.pendingQueue) > 0
			obs := o.observer
			o.mu.Unlock()

			if pending <= 0 && !hasPendingQueue && obs != nil {
				obs.OnTTSStop(false)
			}
			continue
		}

		o.transitionTo(StateSpeaking)
	}
}

func (o *orchestratorImpl) removeInFlightLocked(id int64) {
	for i, val := range o.inFlightQueue {
		if val == id {
			o.inFlightQueue = append(o.inFlightQueue[:i], o.inFlightQueue[i+1:]...)
			return
		}
	}
}

func (o *orchestratorImpl) markSentenceStatusLocked(id int64, status SentenceStatus) {
	if rec := o.sentenceIndex[id]; rec != nil {
		rec.Status = status
	}
}

func (o *orchestratorImpl) abortPendingSentencesLocked() {
	for _, id := range o.pendingQueue {
		o.markSentenceStatusLocked(id, SentenceAborted)
	}
	for _, id := range o.inFlightQueue {
		o.markSentenceStatusLocked(id, SentenceAborted)
	}
	o.pendingQueue = nil
	o.inFlightQueue = nil
	o.ttsPendingCount = 0
}

func (o *orchestratorImpl) abortPendingSentences() {
	o.scheduleMu.Lock()
	o.mu.Lock()
	o.abortPendingSentencesLocked()
	o.mu.Unlock()
	o.scheduleMu.Unlock()
}

func (o *orchestratorImpl) enforceCacheLimitLocked() {
	maxCache := o.ttsSchedulerCfg.MaxCacheSentences
	if maxCache <= 0 {
		return
	}
	for len(o.sentenceCache) > maxCache {
		dropIndex := -1
		for i, rec := range o.sentenceCache {
			if rec.Status == SentenceDone || rec.Status == SentenceAborted {
				dropIndex = i
				break
			}
		}
		if dropIndex == -1 {
			return
		}
		rec := o.sentenceCache[dropIndex]
		delete(o.sentenceIndex, rec.ID)
		o.sentenceCache = append(o.sentenceCache[:dropIndex], o.sentenceCache[dropIndex+1:]...)
	}
}

// onTTSPlaybackFinished TTS 播放完成回调（由 TTSPipeline 调用）
func (o *orchestratorImpl) onTTSPlaybackFinished() {
	o.mu.Lock()
	if o.ttsPendingCount > 0 {
		o.ttsPendingCount--
	}
	pending := o.ttsPendingCount
	var finishedID int64
	if len(o.inFlightQueue) > 0 {
		finishedID = o.inFlightQueue[0]
		o.inFlightQueue = o.inFlightQueue[1:]
	}
	aborted := o.turnAborted
	if finishedID != 0 {
		if rec := o.sentenceIndex[finishedID]; rec != nil && rec.Status == SentenceEnqueued {
			rec.Status = SentenceDone
		}
	}
	o.mu.Unlock()

	if finishedID == 0 {
		logging.Infof("Orchestrator: ignoring stale TTS playback finished callback")
		return
	}

	logging.Infof("Orchestrator: TTS playback finished, pending count: %d", pending)

	// 尝试补位调度下一句
	o.tryScheduleTTS()

	// 如果所有 TTS 都播放完成且没有待调度，转为 Idle
	o.mu.Lock()
	pending = o.ttsPendingCount
	hasPendingQueue := len(o.pendingQueue) > 0
	aborted = o.turnAborted
	o.mu.Unlock()
	if pending <= 0 && !hasPendingQueue {
		currentState := o.stateMachine.GetCurrentState()
		if currentState == StateSpeaking {
			logging.Infof("Orchestrator: All TTS finished, transitioning to Idle")
			o.transitionTo(StateIdle)
		}
		if o.observer != nil && !aborted {
			o.observer.OnTTSStop(false)
		}
		o.maybeFinalizeTurn()
	}
}

func (o *orchestratorImpl) handleASRFinal(event Event) {
	asrEvent, ok := event.(*ASRFinalEvent)
	if !ok {
		return
	}

	// 如果之前有 Agent 在运行，先取消
	o.scheduleMu.Lock()
	o.mu.Lock()
	if o.agentCancel != nil {
		logging.Infof("Orchestrator: cancelling previous Agent before starting new one...")
		o.agentCancel()
	}
	// 新 turn：清理 pending/inFlight，避免旧句子继续调度
	o.currentTurnID++
	o.turnStartedAt = time.Now()
	o.turnUserText = asrEvent.Text
	o.turnAssistantBuf.Reset()
	o.turnAborted = false
	o.turnRecorded = false
	o.activeAgentStreams = 0
	o.abortPendingSentencesLocked()

	// 为新的 Agent 调用创建独立的 context
	o.agentCtx, o.agentCancel = context.WithCancel(o.ctx)
	agentCtx := o.agentCtx
	o.mu.Unlock()
	o.scheduleMu.Unlock()

	logging.StartTurn()
	logging.Infof("Orchestrator: ASR final event received: %s", asrEvent.Text)
	o.transitionTo(StateProcessing)

	o.wg.Add(1)
	go func() {
		o.incAgentStreams()
		defer o.wg.Done()
		defer o.decAgentStreams()

		// 使用 agentCtx 调用 Agent（可被打断）
		eventChan, err := o.voiceAgent.Process(agentCtx, asrEvent.Text)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logging.Infof("Orchestrator: VoiceAgent process cancelled (normal interruption)")
			} else {
				logging.Errorf("Orchestrator: VoiceAgent process error: %v", err)
			}
			o.transitionTo(StateIdle)
			return
		}

		for agentEvent := range eventChan {
			// 检查是否被取消
			select {
			case <-agentCtx.Done():
				logging.Infof("Orchestrator: Agent cancelled, stopping event processing")
				return
			default:
			}

			o.handleAgentEvent(agentEvent)
		}

		// Agent 完成后清理
		o.mu.Lock()
		if o.agentCtx == agentCtx {
			o.agentCancel = nil
		}
		o.mu.Unlock()
	}()
}

func (o *orchestratorImpl) handleToolCallRequested(event Event) {
	toolEvent, ok := event.(*ToolCallRequestedEvent)
	if !ok {
		return
	}

	logging.Infof("Orchestrator: ToolCallRequested event - tool: %s, args: %v", toolEvent.Tool, toolEvent.Args)

	o.wg.Add(1)
	go func() {
		o.incAgentStreams()
		defer o.wg.Done()
		defer o.decAgentStreams()

		result, audioReader, err := o.toolExecutor.Execute(toolEvent.Tool, toolEvent.Args)
		if err != nil {
			logging.Errorf("Orchestrator: Tool execution error: %v", err)
			return
		}

		if audioReader != nil {
			logging.Infof("Orchestrator: tool returned audio, playing...")
			o.OnToolAudioReady(audioReader)
		}

		logging.Infof("Orchestrator: Tool execution result: %v", result)

		if o.voiceAgent == nil {
			return
		}
		if o.voiceAgent.GetToolType(toolEvent.Tool) != agent.ToolTypeQuery {
			return
		}

		summaryChan, err := o.voiceAgent.SummarizeToolResult(o.ctx, toolEvent.Tool, toolEvent.Args, result)
		if err != nil {
			logging.Errorf("Orchestrator: Tool summary error: %v", err)
			return
		}
		for agentEvent := range summaryChan {
			o.handleAgentEvent(agentEvent)
		}
	}()
}

func (o *orchestratorImpl) handleToolAudioReady(event Event) {
	audioEvent, ok := event.(*ToolAudioReadyEvent)
	if !ok {
		return
	}

	logging.Infof("Orchestrator: ToolAudioReady event, playing resource audio...")
	err := o.audioOutPipe.PlayResource(audioEvent.Audio)
	if err != nil {
		logging.Errorf("Orchestrator: Play resource error: %v", err)
	}
}

func (o *orchestratorImpl) handleLLMEmotionChanged(event Event) {
	emotionEvent, ok := event.(*LLMEmotionChangedEvent)
	if !ok {
		return
	}

	o.currentEmotion = emotionEvent.Emotion
	logging.Infof("Orchestrator: LLM emotion changed to: %s", emotionEvent.Emotion)
}

func (o *orchestratorImpl) handleAgentEvent(event agent.AgentEvent) {
	switch e := event.(type) {
	case *agent.TextChunkEvent:
		o.appendAssistantText(e.Chunk)
		if o.observer != nil {
			o.observer.OnLLMTextChunk(e.Chunk, e.Emotion)
		}
		o.OnLLMTextChunk(e.Chunk)
		if e.Emotion != "" && e.Emotion != o.currentEmotion {
			o.currentEmotion = e.Emotion
			o.eventBus.Publish(NewLLMEmotionChangedEvent(e.Emotion))
		}

		sentences := o.segmenter.Feed(e.Chunk)
		for _, sentence := range sentences {
			if sentence != "" {
				// 移除 Markdown 格式，避免 TTS 播放特殊符号
				sentence = o.markdownFilter.Filter(sentence)
				logging.Infof("Orchestrator: caching TTS sentence: %s", sentence)
				o.cacheSentence(sentence, o.currentEmotion)
			}
		}
	case *agent.EmotionChangedEvent:
		o.currentEmotion = e.Emotion
		o.eventBus.Publish(NewLLMEmotionChangedEvent(e.Emotion))
	case *agent.ToolCallRequestedEvent:
		o.OnToolCall(e.Tool, e.Args)
	case *agent.FinishedEvent:
		if last := o.segmenter.Flush(); last != "" {
			// 移除 Markdown 格式，避免 TTS 播放特殊符号
			last = o.markdownFilter.Filter(last)
			logging.Infof("Orchestrator: caching final TTS sentence: %s", last)
			o.cacheSentence(last, o.currentEmotion)
		}
		o.mu.Lock()
		pending := o.ttsPendingCount
		hasPendingQueue := len(o.pendingQueue) > 0
		o.mu.Unlock()
		logging.Infof("Orchestrator: VoiceAgent finished (TTS pending: %d)", pending)
		// 注意：不转为 Idle，保持 Speaking 状态直到所有 TTS 播放完成
		// onTTSPlaybackFinished 会在每个 TTS 播放完成时被调用
		if pending <= 0 && !hasPendingQueue {
			o.maybeFinalizeTurn()
		}
	}
}

func (o *orchestratorImpl) appendAssistantText(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turnRecorded {
		return
	}
	o.turnAssistantBuf.WriteString(text)
}

func (o *orchestratorImpl) incAgentStreams() {
	o.mu.Lock()
	o.activeAgentStreams++
	o.mu.Unlock()
}

func (o *orchestratorImpl) decAgentStreams() {
	o.mu.Lock()
	if o.activeAgentStreams > 0 {
		o.activeAgentStreams--
	}
	o.mu.Unlock()
	o.maybeFinalizeTurn()
}

func (o *orchestratorImpl) maybeFinalizeTurn() {
	if o.memorySvc == nil {
		return
	}
	o.mu.Lock()
	if o.turnRecorded || o.activeAgentStreams > 0 {
		o.mu.Unlock()
		return
	}
	if strings.TrimSpace(o.turnUserText) == "" {
		o.turnRecorded = true
		o.mu.Unlock()
		return
	}
	turn := memory.Turn{
		TurnID:        o.currentTurnID,
		UserText:      o.turnUserText,
		AssistantText: strings.TrimSpace(o.turnAssistantBuf.String()),
		StartedAt:     o.turnStartedAt,
		EndedAt:       time.Now(),
		Aborted:       o.turnAborted,
	}
	o.turnRecorded = true
	o.mu.Unlock()

	if err := o.memorySvc.RecordTurn(o.ctx, turn); err != nil {
		logging.Warnf("Orchestrator: record memory failed: %v", err)
	}
}

func (o *orchestratorImpl) transitionTo(newState State) bool {
	oldState := o.stateMachine.GetCurrentState()
	if o.stateMachine.Transition(newState) {
		o.eventBus.Publish(NewStateChangedEvent(oldState, newState))
		return true
	}
	return false
}

// EventBus 事件总线，负责组件间异步通信
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler)
}

// Event 事件接口
type Event interface {
	Type() EventType
}

// EventType 事件类型
type EventType int

const (
	EventTypeUserSpeakingDetected EventType = iota
	EventTypeASRFinal
	EventTypeToolCallRequested
	EventTypeToolAudioReady
	EventTypeLLMEmotionChanged
	EventTypeTTSInterrupt
	EventTypeStateChanged
)

// EventHandler 事件处理器
type EventHandler func(event Event)
