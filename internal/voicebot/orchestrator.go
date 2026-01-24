package voicebot

import (
	"context"
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

	voiceAgent   agent.VoiceAgent
	audioOutPipe audio.AudioOutPipe
	audioInPipe  audio.AudioInPipe
	toolExecutor tools.ToolExecutor
	segmenter    *text.Segmenter

	currentEmotion string
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex
}

// NewOrchestrator 创建新的Orchestrator
func NewOrchestrator(
	voiceAgent agent.VoiceAgent,
	audioOutPipe audio.AudioOutPipe,
	audioInPipe audio.AudioInPipe,
	toolExecutor tools.ToolExecutor,
) Orchestrator {
	return &orchestratorImpl{
		stateMachine: NewStateMachine(),
		eventBus:     NewEventBus(),
		voiceAgent:   voiceAgent,
		audioOutPipe: audioOutPipe,
		audioInPipe:  audioInPipe,
		toolExecutor: toolExecutor,
		segmenter:    text.NewSegmenter(120),
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
			if text != "" {
				logging.Infof("Orchestrator: user speaking detected: %s", text)
				o.OnUserSpeakingDetected()
			}
			if isFinal {
				logging.Infof("Orchestrator: ASR final result: %s", text)
				o.OnASRFinal(text)
			}
		})
		o.audioInPipe.OnUserSpeakingDetected(func() {
			logging.Infof("Orchestrator: VAD user speaking detected")
			o.OnUserSpeakingDetected()
		})
	}

	if o.audioOutPipe != nil {
		logging.Infof("Orchestrator: starting AudioOutPipe...")
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
	defer o.mu.Unlock()

	logging.Infof("Orchestrator: stopping...")

	if o.cancel != nil {
		o.cancel()
	}

	if o.audioInPipe != nil {
		logging.Infof("Orchestrator: stopping AudioInPipe...")
		o.audioInPipe.Stop()
	}

	if o.audioOutPipe != nil {
		logging.Infof("Orchestrator: stopping AudioOutPipe...")
		o.audioOutPipe.Stop()
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
	logging.Infof("Orchestrator: UserSpeakingDetected received, current state: %s", o.stateMachine.GetCurrentState())
	if o.stateMachine.GetCurrentState() == StateSpeaking {
		logging.Infof("Orchestrator: interrupting current playback...")
		o.transitionTo(StateListening)
		o.audioOutPipe.Interrupt()
	}
}

func (o *orchestratorImpl) handleASRFinal(event Event) {
	asrEvent, ok := event.(*ASRFinalEvent)
	if !ok {
		return
	}

	logging.StartTurn()
	logging.Infof("Orchestrator: ASR final event received: %s", asrEvent.Text)
	o.transitionTo(StateProcessing)

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

		eventChan, err := o.voiceAgent.Process(o.ctx, asrEvent.Text)
		if err != nil {
			logging.Errorf("Orchestrator: VoiceAgent process error: %v", err)
			o.transitionTo(StateIdle)
			return
		}

		for agentEvent := range eventChan {
			o.handleAgentEvent(agentEvent)
		}
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
				logging.Infof("Orchestrator: playing TTS for sentence: %s", sentence)
				err := o.audioOutPipe.PlayTTS(sentence, o.currentEmotion)
				if err != nil {
					logging.Errorf("Orchestrator: PlayTTS error: %v", err)
				}
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
			logging.Infof("Orchestrator: playing final TTS sentence: %s", last)
			err := o.audioOutPipe.PlayTTS(last, o.currentEmotion)
			if err != nil {
				logging.Errorf("Orchestrator: PlayTTS error: %v", err)
			}
			o.transitionTo(StateSpeaking)
		}
		logging.Infof("Orchestrator: VoiceAgent finished")
		o.transitionTo(StateIdle)
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
