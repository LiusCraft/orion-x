package voicebot

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/liuscraft/orion-x/internal/agent"
	"github.com/liuscraft/orion-x/internal/audio"
	"github.com/liuscraft/orion-x/internal/logging"
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

	wg sync.WaitGroup
	mu sync.Mutex
}

// NewOrchestrator 创建新的Orchestrator
func NewOrchestrator(
	voiceAgent agent.VoiceAgent,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
) Orchestrator {
	return &orchestratorImpl{
		stateMachine:   NewStateMachine(),
		eventBus:       NewEventBus(),
		voiceAgent:     voiceAgent,
		audioOutPipe:   audioOutPipe,
		audioInPipe:    audioInPipe,
		toolExecutor:   toolExecutor,
		segmenter:      text.NewSegmenter(120),
		markdownFilter: agent.NewMarkdownFilter(),
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
	o.eventBus.Publish(NewUserSpeakingDetectedEvent())
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
	logging.Infof("LLM chunk: %s", chunk)
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
	ttsPending := o.ttsPendingCount > 0
	o.mu.Unlock()

	// 只在 Processing、Speaking 状态或有 TTS pending 时才需要打断
	needInterrupt := currentState == StateSpeaking || currentState == StateProcessing || ttsPending
	if needInterrupt {
		logging.Infof("Orchestrator: UserSpeakingDetected - interrupting (state=%s, ttsPending=%d)", currentState, o.ttsPendingCount)

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

		// 4. 重置 TTS 计数
		o.mu.Lock()
		o.ttsPendingCount = 0
		o.mu.Unlock()

		// 5. 状态转换
		o.transitionTo(StateListening)
	}
}

// onTTSPlaybackFinished TTS 播放完成回调（由 TTSPipeline 调用）
func (o *orchestratorImpl) onTTSPlaybackFinished() {
	o.mu.Lock()
	o.ttsPendingCount--
	pending := o.ttsPendingCount
	o.mu.Unlock()

	logging.Infof("Orchestrator: TTS playback finished, pending count: %d", pending)

	// 如果所有 TTS 都播放完成，转为 Idle
	if pending <= 0 {
		currentState := o.stateMachine.GetCurrentState()
		if currentState == StateSpeaking {
			logging.Infof("Orchestrator: All TTS finished, transitioning to Idle")
			o.transitionTo(StateIdle)
		}
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

	// 为新的 Agent 调用创建独立的 context
	o.agentCtx, o.agentCancel = context.WithCancel(o.ctx)
	agentCtx := o.agentCtx
	o.mu.Unlock()

	logging.StartTurn()
	logging.Infof("Orchestrator: ASR final event received: %s", asrEvent.Text)
	o.transitionTo(StateProcessing)

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

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
		defer o.wg.Done()

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
				logging.Infof("Orchestrator: enqueuing TTS for sentence: %s", sentence)
				// PlayTTS 现在是异步的，立即返回
				err := o.audioOutPipe.PlayTTS(sentence, o.currentEmotion)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						logging.Infof("Orchestrator: PlayTTS cancelled (normal interruption)")
						return // 被打断，停止处理
					} else {
						logging.Errorf("Orchestrator: PlayTTS error: %v", err)
					}
				}
				// 增加 TTS 计数
				o.mu.Lock()
				o.ttsPendingCount++
				o.mu.Unlock()
				o.transitionTo(StateSpeaking)
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
			logging.Infof("Orchestrator: enqueuing final TTS sentence: %s", last)
			// PlayTTS 现在是异步的，立即返回
			err := o.audioOutPipe.PlayTTS(last, o.currentEmotion)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					logging.Infof("Orchestrator: PlayTTS cancelled (normal interruption)")
				} else {
					logging.Errorf("Orchestrator: PlayTTS error: %v", err)
				}
			}
			// 增加 TTS 计数
			o.mu.Lock()
			o.ttsPendingCount++
			o.mu.Unlock()
			o.transitionTo(StateSpeaking)
		}
		logging.Infof("Orchestrator: VoiceAgent finished (TTS pending: %d)", o.ttsPendingCount)
		// 注意：不转为 Idle，保持 Speaking 状态直到所有 TTS 播放完成
		// onTTSPlaybackFinished 会在每个 TTS 播放完成时被调用
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
