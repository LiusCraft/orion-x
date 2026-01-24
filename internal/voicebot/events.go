package voicebot

import (
	"io"
	"time"
)

// Event 事件实现
type BaseEvent struct {
	eventType EventType
	timestamp time.Time
}

func (e *BaseEvent) Type() EventType {
	return e.eventType
}

func (e *BaseEvent) Timestamp() time.Time {
	return e.timestamp
}

// UserSpeakingDetectedEvent 用户说话事件（触发中断）
type UserSpeakingDetectedEvent struct {
	BaseEvent
}

func NewUserSpeakingDetectedEvent() *UserSpeakingDetectedEvent {
	return &UserSpeakingDetectedEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeUserSpeakingDetected,
			timestamp: time.Now(),
		},
	}
}

// ASRFinalEvent ASR识别完成事件
type ASRFinalEvent struct {
	BaseEvent
	Text string
}

func NewASRFinalEvent(text string) *ASRFinalEvent {
	return &ASRFinalEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeASRFinal,
			timestamp: time.Now(),
		},
		Text: text,
	}
}

// ToolCallRequestedEvent 工具调用请求事件
type ToolCallRequestedEvent struct {
	BaseEvent
	Tool string
	Args map[string]interface{}
}

func NewToolCallRequestedEvent(tool string, args map[string]interface{}) *ToolCallRequestedEvent {
	return &ToolCallRequestedEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeToolCallRequested,
			timestamp: time.Now(),
		},
		Tool: tool,
		Args: args,
	}
}

// ToolAudioReadyEvent 工具返回音频事件
type ToolAudioReadyEvent struct {
	BaseEvent
	Audio io.Reader
}

func NewToolAudioReadyEvent(audio io.Reader) *ToolAudioReadyEvent {
	return &ToolAudioReadyEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeToolAudioReady,
			timestamp: time.Now(),
		},
		Audio: audio,
	}
}

// LLMEmotionChangedEvent LLM情绪变化事件
type LLMEmotionChangedEvent struct {
	BaseEvent
	Emotion string
}

func NewLLMEmotionChangedEvent(emotion string) *LLMEmotionChangedEvent {
	return &LLMEmotionChangedEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeLLMEmotionChanged,
			timestamp: time.Now(),
		},
		Emotion: emotion,
	}
}

// TTSInterruptEvent TTS播放中断事件
type TTSInterruptEvent struct {
	BaseEvent
}

func NewTTSInterruptEvent() *TTSInterruptEvent {
	return &TTSInterruptEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeTTSInterrupt,
			timestamp: time.Now(),
		},
	}
}

// StateChangedEvent 状态变化事件
type StateChangedEvent struct {
	BaseEvent
	OldState State
	NewState State
}

func NewStateChangedEvent(oldState, newState State) *StateChangedEvent {
	return &StateChangedEvent{
		BaseEvent: BaseEvent{
			eventType: EventTypeStateChanged,
			timestamp: time.Now(),
		},
		OldState: oldState,
		NewState: newState,
	}
}
