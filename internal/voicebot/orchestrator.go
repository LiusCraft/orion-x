package voicebot

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/memory"
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
	Observer OrchestratorObserver
	Memory   memory.Service
}

// AgentRunner 是 Orchestrator 对 agent 运行体的最小依赖。
type AgentRunner interface {
	Process(ctx context.Context, text string) (<-chan agent.AgentEvent, error)
	SummarizeToolResult(ctx context.Context, tool string, args map[string]interface{}, result interface{}) (<-chan agent.AgentEvent, error)
}

// orchestratorImpl Orchestrator 实现
type orchestratorImpl struct {
	stateMachine *StateMachine
	eventBus     EventBus

	agentRunner  AgentRunner
	audioOutPipe audio.AudioOutPipe
	audioInPipe  audio.AudioInPipe
	toolExecutor tools.ToolExecutor
	textFilter   TextFilterNode
	metadataNode OutputMetadataNode

	currentEmotion string
	ctx            context.Context
	cancel         context.CancelFunc

	// Agent context 管理（用于打断时取消 Agent）
	agentCtx    context.Context
	agentCancel context.CancelFunc

	// 流式 TTS session 状态
	ttsStreamActive   bool // 当前是否有 TTS stream 在接收文本
	ttsPendingStreams  int  // 已建立但尚未播放完毕的 TTS stream 数量
	currentTurnID     int64

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
	agentRunner AgentRunner,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
) Orchestrator {
	return NewOrchestratorWithOptions(agentRunner, audioOutPipe, audioInPipe, toolExecutor, nil)
}

// NewOrchestratorWithOptions 创建新的Orchestrator（带可选参数）
func NewOrchestratorWithOptions(
	agentRunner AgentRunner,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
	opts *OrchestratorOptions,
) Orchestrator {
	var observer OrchestratorObserver
	if opts != nil {
		observer = opts.Observer
	}
	return &orchestratorImpl{
		stateMachine:   NewStateMachine(),
		eventBus:       NewEventBus(),
		agentRunner:    agentRunner,
		audioOutPipe:   audioOutPipe,
		audioInPipe:    audioInPipe,
		toolExecutor:   toolExecutor,
		textFilter:     NewTextFilterNode(),
		metadataNode:   NewOutputMetadataNode(),
		currentEmotion: "default",
		observer:       observer,
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
	o.eventBus.Subscribe(EventTypeOutputEmotionChanged, o.handleOutputEmotionChanged)

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

	o.mu.Lock()
	ttsActive := o.ttsStreamActive
	o.mu.Unlock()

	// 只在 Processing、Speaking 状态或有 TTS 活跃时才需要打断
	needInterrupt := currentState == StateSpeaking || currentState == StateProcessing || ttsActive
	if needInterrupt {
		o.mu.Lock()
		o.turnAborted = true
		o.ttsStreamActive = false
		o.ttsPendingStreams = 0
		o.mu.Unlock()
		logging.Infof("Orchestrator: UserSpeakingDetected - interrupting (state=%s, ttsActive=%v)", currentState, ttsActive)

		// 1. 取消 Agent（停止 LLM 生成）
		o.mu.Lock()
		if o.agentCancel != nil {
			logging.Infof("Orchestrator: cancelling Agent...")
			o.agentCancel()
			o.agentCancel = nil
		}
		o.mu.Unlock()

		// 2. 中断 TTS Pipeline（停止播放、关闭 stream）
		if o.audioOutPipe != nil {
			logging.Infof("Orchestrator: interrupting AudioOutPipe...")
			o.audioOutPipe.Interrupt()
		}

		if o.observer != nil && ttsActive {
			o.observer.OnTTSStop(true)
		}

		// 3. 状态转换
		o.transitionTo(StateListening)
	}
}

// onTTSPlaybackFinished TTS 播放完成回调（由 TTSPipeline 调用）
func (o *orchestratorImpl) onTTSPlaybackFinished() {
	o.mu.Lock()
	if o.ttsPendingStreams > 0 {
		o.ttsPendingStreams--
	}
	pending := o.ttsPendingStreams
	aborted := o.turnAborted
	o.mu.Unlock()

	logging.Infof("Orchestrator: TTS playback finished, pending streams: %d", pending)

	if pending > 0 {
		// 还有其他 TTS stream 未播放完（如 tool summary），继续等待
		return
	}

	currentState := o.stateMachine.GetCurrentState()
	if currentState == StateSpeaking {
		logging.Infof("Orchestrator: All TTS finished, transitioning to Idle")
		o.transitionTo(StateIdle)
	}
	if o.observer != nil && !aborted {
		o.observer.OnTTSStop(false)
	}
	if !aborted {
		o.maybeFinalizeTurn()
	}
}

func (o *orchestratorImpl) handleASRFinal(event Event) {
	asrEvent, ok := event.(*ASRFinalEvent)
	if !ok {
		return
	}

	// 如果之前有 Agent 在运行，先取消
	o.mu.Lock()
	if o.agentCancel != nil {
		logging.Infof("Orchestrator: cancelling previous Agent before starting new one...")
		o.agentCancel()
	}
	o.currentTurnID++
	o.turnStartedAt = time.Now()
	o.turnUserText = asrEvent.Text
	o.turnAssistantBuf.Reset()
	o.turnAborted = false
	o.turnRecorded = false
	o.activeAgentStreams = 0
	o.ttsStreamActive = false
	o.ttsPendingStreams = 0

	// 为新的 Agent 调用创建独立的 context
	o.agentCtx, o.agentCancel = context.WithCancel(o.ctx)
	agentCtx := o.agentCtx
	o.mu.Unlock()

	logging.StartTurn()
	logging.Infof("Orchestrator: ASR final event received: %s", asrEvent.Text)
	o.transitionTo(StateProcessing)

	o.wg.Add(1)
	go func() {
		o.incAgentStreams()
		defer o.wg.Done()
		defer o.decAgentStreams()

		// 使用 agentCtx 调用 Agent（可被打断）
		eventChan, err := o.agentRunner.Process(agentCtx, asrEvent.Text)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				logging.Infof("Orchestrator: Agent process cancelled (normal interruption)")
			} else {
				logging.Errorf("Orchestrator: Agent process error: %v", err)
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

		if o.agentRunner == nil {
			return
		}

		summaryChan, err := o.agentRunner.SummarizeToolResult(o.ctx, toolEvent.Tool, toolEvent.Args, result)
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

func (o *orchestratorImpl) handleOutputEmotionChanged(event Event) {
	emotionEvent, ok := event.(*OutputEmotionChangedEvent)
	if !ok {
		return
	}

	o.currentEmotion = emotionEvent.Emotion
	logging.Infof("Orchestrator: output emotion changed to: %s", emotionEvent.Emotion)
}

func (o *orchestratorImpl) handleAgentEvent(event agent.AgentEvent) {
	switch e := event.(type) {
	case *agent.TextChunkEvent:
		metadata := o.metadataNode.Process(e.Chunk)
		if metadata.Emotion != "" && metadata.Emotion != o.currentEmotion {
			o.currentEmotion = metadata.Emotion
			o.eventBus.Publish(NewOutputEmotionChangedEvent(metadata.Emotion))
		}

		o.appendAssistantText(e.Chunk)
		if o.observer != nil {
			o.observer.OnLLMTextChunk(e.Chunk, o.currentEmotion)
		}
		o.OnLLMTextChunk(e.Chunk)

		// 第一个 chunk 时建立 TTS stream，立即开始接收音频
		o.mu.Lock()
		if !o.ttsStreamActive {
			o.ttsStreamActive = true
			o.ttsPendingStreams++
			emotion := o.currentEmotion
			o.mu.Unlock()
			if err := o.audioOutPipe.BeginTTSStream(emotion); err != nil {
				if !errors.Is(err, context.Canceled) {
					logging.Errorf("Orchestrator: BeginTTSStream error: %v", err)
				}
				o.mu.Lock()
				o.ttsStreamActive = false
				o.ttsPendingStreams--
				o.mu.Unlock()
			} else {
				if o.observer != nil {
					o.observer.OnTTSStart()
				}
				o.transitionTo(StateSpeaking)
			}
		} else {
			o.mu.Unlock()
		}

		// 过滤后直接写入 TTS stream
		filtered := o.textFilter.Process(e.Chunk)
		if filtered != "" {
			if err := o.audioOutPipe.WriteTTSChunk(filtered); err != nil {
				if !errors.Is(err, context.Canceled) {
					logging.Errorf("Orchestrator: WriteTTSChunk error: %v", err)
				}
			}
		}
	case *agent.ToolCallRequestedEvent:
		o.OnToolCall(e.Tool, e.Args)
	case *agent.FinishedEvent:
		o.mu.Lock()
		wasActive := o.ttsStreamActive
		// 重置 ttsStreamActive，允许 tool summary 等后续 agent 建立新 session
		o.ttsStreamActive = false
		o.mu.Unlock()

		if wasActive {
			if err := o.audioOutPipe.EndTTSStream(); err != nil {
				if !errors.Is(err, context.Canceled) {
					logging.Errorf("Orchestrator: EndTTSStream error: %v", err)
				}
			}
		}
		logging.Infof("Orchestrator: Agent finished")
		// 如果 LLM 无文本输出（没有 TTS），直接结束 turn
		if !wasActive {
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
	EventTypeOutputEmotionChanged
	EventTypeTTSInterrupt
	EventTypeStateChanged
)

// EventHandler 事件处理器
type EventHandler func(event Event)
