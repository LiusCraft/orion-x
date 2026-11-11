// Package realtime contains the wire model for the OpenAI Realtime GA
// protocol. Runtime pipeline construction lives in internal/voice so this
// package remains reusable by transports and protocol adapters.
package realtime

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	// ClientSessionUpdate updates the active Realtime session.
	ClientSessionUpdate = "session.update"
	// ClientInputAudioAppend appends Base64-encoded PCM audio.
	ClientInputAudioAppend = "input_audio_buffer.append"
	// ClientInputAudioCommit commits the current input audio buffer.
	ClientInputAudioCommit = "input_audio_buffer.commit"
	// ClientInputAudioClear clears the current input audio buffer.
	ClientInputAudioClear = "input_audio_buffer.clear"
	// ClientConversationItemCreate adds an input conversation item.
	ClientConversationItemCreate = "conversation.item.create"
	// ClientResponseCreate starts a response for the pending input.
	ClientResponseCreate = "response.create"
	// ClientResponseCancel cancels the active response.
	ClientResponseCancel = "response.cancel"

	// ServerSessionCreated reports the initial session configuration.
	ServerSessionCreated = "session.created"
	// ServerSessionUpdated reports an updated session configuration.
	ServerSessionUpdated = "session.updated"
	// ServerInputAudioBufferCommitted reports a committed input buffer.
	ServerInputAudioBufferCommitted = "input_audio_buffer.committed"
	// ServerInputAudioBufferCleared reports a cleared input buffer.
	ServerInputAudioBufferCleared = "input_audio_buffer.cleared"
	// ServerInputAudioSpeechStarted reports detected speech.
	ServerInputAudioSpeechStarted = "input_audio_buffer.speech_started"
	// ServerInputAudioSpeechStopped reports the end of detected speech.
	ServerInputAudioSpeechStopped = "input_audio_buffer.speech_stopped"
	// ServerConversationItemCreated reports a created conversation item.
	ServerConversationItemCreated = "conversation.item.created"
	// ServerResponseCreated reports the start of a response.
	ServerResponseCreated = "response.created"
	// ServerResponseOutputTextDelta reports an output text fragment.
	ServerResponseOutputTextDelta = "response.output_text.delta"
	// ServerResponseOutputTextDone reports completed output text.
	ServerResponseOutputTextDone = "response.output_text.done"
	// ServerResponseOutputAudioDelta reports a Base64-encoded audio fragment.
	ServerResponseOutputAudioDelta = "response.output_audio.delta"
	// ServerResponseOutputAudioDone reports completed output audio.
	ServerResponseOutputAudioDone = "response.output_audio.done"
	// ServerResponseAudioTranscriptDelta reports an audio transcript fragment.
	ServerResponseAudioTranscriptDelta = "response.output_audio_transcript.delta"
	// ServerResponseAudioTranscriptDone reports a completed audio transcript.
	ServerResponseAudioTranscriptDone = "response.output_audio_transcript.done"
	// ServerResponseDone reports the final response status.
	ServerResponseDone = "response.done"
	// ServerError reports a protocol or processing error.
	ServerError = "error"
)

// ClientEvent is the common envelope for client-to-server events.
type ClientEvent struct {
	Type       string            `json:"type"`
	EventID    string            `json:"event_id,omitempty"`
	Session    *SessionConfig    `json:"session,omitempty"`
	Audio      string            `json:"audio,omitempty"`
	Item       *ConversationItem `json:"item,omitempty"`
	Response   *ResponseCreate   `json:"response,omitempty"`
	ItemID     string            `json:"item_id,omitempty"`
	ResponseID string            `json:"response_id,omitempty"`
}

// SessionConfig describes the subset of GA session configuration supported by
// the local ASR → Agent → TTS backend.
type SessionConfig struct {
	ID                string           `json:"id,omitempty"`
	Object            string           `json:"object,omitempty"`
	Type              string           `json:"type,omitempty"`
	Model             string           `json:"model,omitempty"`
	Instructions      string           `json:"instructions,omitempty"`
	Audio             *AudioConfig     `json:"audio,omitempty"`
	InputAudioFormat  string           `json:"input_audio_format,omitempty"`
	OutputAudioFormat string           `json:"output_audio_format,omitempty"`
	Voice             string           `json:"voice,omitempty"`
	OutputModalities  []string         `json:"output_modalities,omitempty"`
	TurnDetection     *TurnDetection   `json:"turn_detection,omitempty"`
	Tools             []ToolDefinition `json:"tools,omitempty"`
	ToolChoice        any              `json:"tool_choice,omitempty"`
	MaxOutputTokens   any              `json:"max_output_tokens,omitempty"`
}

// AudioConfig is the GA nested audio configuration.
type AudioConfig struct {
	Input  *AudioInputConfig  `json:"input,omitempty"`
	Output *AudioOutputConfig `json:"output,omitempty"`
}

// AudioInputConfig describes input audio format and turn detection.
type AudioInputConfig struct {
	Format           *AudioFormat   `json:"format,omitempty"`
	TurnDetection    *TurnDetection `json:"turn_detection,omitempty"`
	turnDetectionSet bool
}

// TurnDetectionSpecified reports whether the input configuration explicitly
// included a turn_detection field, including an explicit null value.
func (c *AudioInputConfig) TurnDetectionSpecified() bool {
	return c != nil && (c.turnDetectionSet || c.TurnDetection != nil)
}

// UnmarshalJSON preserves whether turn_detection was omitted or set to null.
func (c *AudioInputConfig) UnmarshalJSON(data []byte) error {
	var payload struct {
		Format        *AudioFormat    `json:"format"`
		TurnDetection json.RawMessage `json:"turn_detection"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	c.Format = payload.Format
	c.TurnDetection = nil
	c.turnDetectionSet = payload.TurnDetection != nil
	if !c.turnDetectionSet || bytes.Equal(bytes.TrimSpace(payload.TurnDetection), []byte("null")) {
		return nil
	}
	return json.Unmarshal(payload.TurnDetection, &c.TurnDetection)
}

// AudioOutputConfig describes output audio format and voice.
type AudioOutputConfig struct {
	Format *AudioFormat `json:"format,omitempty"`
	Voice  string       `json:"voice,omitempty"`
}

// AudioFormat identifies a realtime audio codec and sample rate.
type AudioFormat struct {
	Type       string `json:"type,omitempty"`
	Rate       int    `json:"rate,omitempty"`
	Encoding   string `json:"encoding,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
}

// TurnDetection controls server-side VAD and automatic responses.
type TurnDetection struct {
	Type              string  `json:"type,omitempty"`
	Threshold         float64 `json:"threshold,omitempty"`
	PrefixPaddingMs   int     `json:"prefix_padding_ms,omitempty"`
	SilenceDurationMs int     `json:"silence_duration_ms,omitempty"`
	CreateResponse    *bool   `json:"create_response,omitempty"`
	InterruptResponse *bool   `json:"interrupt_response,omitempty"`
}

// ConversationItem is a conversation item accepted by the minimal server.
type ConversationItem struct {
	ID        string        `json:"id,omitempty"`
	Type      string        `json:"type,omitempty"`
	Object    string        `json:"object,omitempty"`
	Status    string        `json:"status,omitempty"`
	Role      string        `json:"role,omitempty"`
	Content   []ContentPart `json:"content,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Output    string        `json:"output,omitempty"`
	Name      string        `json:"name,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
}

// ContentPart is a text or audio content part in a conversation item.
type ContentPart struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	Audio      string `json:"audio,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

// ResponseCreate controls one response generation.
type ResponseCreate struct {
	Modalities       []string `json:"modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	Instructions     string   `json:"instructions,omitempty"`
	Voice            string   `json:"voice,omitempty"`
}

// ToolDefinition is kept for session compatibility. Tool execution remains
// owned by the existing Agent pipeline in this first GA implementation.
type ToolDefinition struct {
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ServerEvent is the common envelope for server-to-client events.
type ServerEvent struct {
	Type           string            `json:"type"`
	EventID        string            `json:"event_id,omitempty"`
	Session        *SessionConfig    `json:"session,omitempty"`
	Item           *ConversationItem `json:"item,omitempty"`
	Response       any               `json:"response,omitempty"`
	ResponseID     string            `json:"response_id,omitempty"`
	ItemID         string            `json:"item_id,omitempty"`
	OutputIndex    int               `json:"output_index"`
	ContentIndex   int               `json:"content_index"`
	AudioStartMS   int               `json:"audio_start_ms"`
	AudioEndMS     int               `json:"audio_end_ms"`
	PreviousItemID string            `json:"previous_item_id,omitempty"`
	Delta          string            `json:"delta,omitempty"`
	Text           string            `json:"text,omitempty"`
	Transcript     string            `json:"transcript,omitempty"`
	Audio          string            `json:"audio,omitempty"`
	Error          *Error            `json:"error,omitempty"`
}

// Error is the GA protocol error payload.
type Error struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
	EventID string `json:"event_id,omitempty"`
}

// ParseClientEvent decodes and validates a client event envelope.
func ParseClientEvent(data []byte) (ClientEvent, error) {
	var event ClientEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return ClientEvent{}, fmt.Errorf("decode realtime event: %w", err)
	}
	if event.Type == "" {
		return ClientEvent{}, fmt.Errorf("realtime event type is required")
	}
	return event, nil
}

// MarshalEvent serializes a server event.
func MarshalEvent(event ServerEvent) ([]byte, error) {
	return json.Marshal(event)
}
