// Package wsproto defines the WebSocket voice session protocol: JSON text
// control messages (hello/listen/abort/stt/tts) exchanged alongside binary
// audio frames. It intentionally omits the iot/mcp/server message types
// found in device-control-oriented protocols (e.g. xiaozhi-esp32-server) —
// those concern controlling client-side hardware or having the server call
// back into client-local tools, neither of which applies to a plain voice
// conversation session.
package wsproto

import (
	"encoding/json"
	"fmt"
)

// MessageType is the "type" field of a JSON text frame.
type MessageType string

const (
	TypeHello  MessageType = "hello"
	TypeListen MessageType = "listen"
	TypeAbort  MessageType = "abort"
	TypeSTT    MessageType = "stt"
	TypeTTS    MessageType = "tts"
	TypeLLM    MessageType = "llm"
	TypeIoT    MessageType = "iot"
	TypeMCP    MessageType = "mcp"
)

// Mode is the interaction mode negotiated at hello time and fixed for the
// life of the connection (switching modes mid-connection would require
// rebuilding the ASR processor, so it isn't supported).
type Mode string

const (
	// ModeAuto lets the server's VAD decide speech turn boundaries.
	ModeAuto Mode = "auto"
	// ModeManual requires the client to bound each turn with explicit
	// listen start/stop messages; the server disables VAD for the
	// connection's ASR processor.
	ModeManual Mode = "manual"
)

// ListenState is the "state" field of a listen message.
type ListenState string

const (
	// ListenStart begins a turn. In manual mode this starts a recognizer
	// task; in auto mode it's informational (VAD detects speech onset on
	// its own).
	ListenStart ListenState = "start"
	// ListenStop ends a turn (manual mode only; ignored in auto mode).
	ListenStop ListenState = "stop"
	// ListenDetect injects text directly, bypassing ASR (e.g. a
	// text-based client turn).
	ListenDetect ListenState = "detect"
)

// TTSState is the "state" field of a tts message.
type TTSState string

const (
	TTSStateStart         TTSState = "start"
	TTSStateSentenceStart TTSState = "sentence_start"
	TTSStateSentenceEnd   TTSState = "sentence_end"
	TTSStateStop          TTSState = "stop"
)

// AudioParams describes the audio wire format for a connection.
type AudioParams struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	// FrameDuration is the frame size in milliseconds used for encoding and
	// send pacing. Omitted (0) defaults to 60ms. Only a fixed set of values
	// is accepted for opus (see internal/audio/codec); an unsupported value
	// fails the handshake.
	FrameDuration int `json:"frame_duration,omitempty"`
	// BitsPerSample must be 16 if present — the entire pipeline (codec,
	// resampler, ASR/TTS providers) is hardcoded to PCM16LE. Any other
	// value fails the handshake rather than being silently coerced.
	BitsPerSample int `json:"bits_per_sample,omitempty"`
	// PlayBufferDuration is the client's playback buffer size in
	// milliseconds. Larger values let the server safely front-load more
	// audio before switching to steady-rate pacing (see
	// cmd/wsserver's computePreBufferFrames). Omitted (0) uses the server's
	// default pre-buffer window.
	PlayBufferDuration int `json:"play_buffer_duration,omitempty"`
}

// HelloMessage is the handshake message: sent by the client to open a
// session (DeviceID/AudioParams/Mode populated, SessionID/WelcomeMsg empty),
// and echoed back by the server with server-assigned/negotiated values
// (SessionID populated, AudioParams reflecting what the server will
// actually use).
type HelloMessage struct {
	Type        MessageType `json:"type"`
	SessionID   string      `json:"session_id,omitempty"`
	DeviceID    string      `json:"device_id,omitempty"`
	AudioParams AudioParams `json:"audio_params"`
	Mode        Mode        `json:"mode,omitempty"`
	WelcomeMsg  string      `json:"welcome_msg,omitempty"`
	// Features lists optional capabilities the client supports.
	// Currently recognised values: "mcp": true (client acts as device-MCP server).
	Features map[string]bool `json:"features,omitempty"`
}

// NewHelloResponse builds the server's handshake response.
func NewHelloResponse(sessionID string, params AudioParams, mode Mode, welcomeMsg string) HelloMessage {
	return HelloMessage{
		Type:        TypeHello,
		SessionID:   sessionID,
		AudioParams: params,
		Mode:        mode,
		WelcomeMsg:  welcomeMsg,
	}
}

// ListenMessage controls speech capture. Client to server only.
type ListenMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	State     ListenState `json:"state"`
	Text      string      `json:"text,omitempty"`
}

// AbortMessage asks the server to immediately stop the current response
// (LLM generation and/or TTS playback). Client to server only.
type AbortMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
}

// STTMessage carries a recognized utterance. Server to client only.
type STTMessage struct {
	Type      MessageType `json:"type"`
	Text      string      `json:"text"`
	SessionID string      `json:"session_id,omitempty"`
}

// NewSTTMessage builds a server-to-client STT message.
func NewSTTMessage(sessionID, text string) STTMessage {
	return STTMessage{Type: TypeSTT, SessionID: sessionID, Text: text}
}

// TTSMessage carries TTS playback state. Server to client only.
type TTSMessage struct {
	Type      MessageType `json:"type"`
	State     TTSState    `json:"state"`
	Text      string      `json:"text,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}

// NewTTSMessage builds a server-to-client TTS state message.
func NewTTSMessage(sessionID string, state TTSState, text string) TTSMessage {
	return TTSMessage{Type: TypeTTS, SessionID: sessionID, State: state, Text: text}
}

// LLMMessage carries a streaming LLM text chunk with an optional emotion tag.
// Server to client only. Emotion is the leading emoji extracted from the LLM
// output (e.g. "😊"); it is omitted when the current turn has no emotion.
type LLMMessage struct {
	Type      MessageType `json:"type"`
	Text      string      `json:"text"`
	Emotion   string      `json:"emotion,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}

// NewLLMMessage builds a server-to-client LLM text chunk message.
func NewLLMMessage(sessionID, text, emotion string) LLMMessage {
	return LLMMessage{Type: TypeLLM, SessionID: sessionID, Text: text, Emotion: emotion}
}

// IoTProperty describes a single property of an IoT device.
type IoTProperty struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

// IoTMethod describes a callable method on an IoT device, including its parameters.
type IoTMethod struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]IoTProperty `json:"parameters,omitempty"`
}

// IoTDescriptor is the device capability declaration sent by the client.
type IoTDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Properties  map[string]IoTProperty `json:"properties,omitempty"`
	Methods     map[string]IoTMethod   `json:"methods,omitempty"`
}

// IoTState is a device state snapshot sent by the client.
type IoTState struct {
	Name  string         `json:"name"`
	State map[string]any `json:"state"`
}

// IoTCommand is a single device command sent by the server.
type IoTCommand struct {
	Name       string         `json:"name"`
	Method     string         `json:"method"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// IoTMessage is a bidirectional IoT frame.
// Client→server: carries Descriptors (capability declaration) and/or States (current values).
// Server→client: carries Commands (control instructions).
type IoTMessage struct {
	Type        MessageType      `json:"type"`
	SessionID   string           `json:"session_id,omitempty"`
	Descriptors []IoTDescriptor  `json:"descriptors,omitempty"`
	States      []IoTState       `json:"states,omitempty"`
	Commands    []IoTCommand     `json:"commands,omitempty"`
}

// NewIoTCommandMessage builds a server→client IoT control frame.
func NewIoTCommandMessage(sessionID string, cmds []IoTCommand) IoTMessage {
	return IoTMessage{Type: TypeIoT, SessionID: sessionID, Commands: cmds}
}

// MCPMessage wraps a JSON-RPC 2.0 payload for device-side MCP.
// Server→client: method calls (initialize, tools/list, tools/call).
// Client→server: result/error responses.
type MCPMessage struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	Payload   any         `json:"payload"`
}

// NewMCPMessage builds a server→client MCP frame.
func NewMCPMessage(sessionID string, payload any) MCPMessage {
	return MCPMessage{Type: TypeMCP, SessionID: sessionID, Payload: payload}
}

// envelope is decoded first to sniff the "type" field before parsing into a
// concrete message type.
type envelope struct {
	Type MessageType `json:"type"`
}

// ParseClientMessage decodes a text frame from a client into a concrete
// message type: *HelloMessage, *ListenMessage, or *AbortMessage. Returns an
// error for malformed JSON or a type this protocol doesn't support
// server-side (including the intentionally-unimplemented iot/mcp/server
// types).
func ParseClientMessage(data []byte) (any, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("wsproto: invalid message: %w", err)
	}

	switch env.Type {
	case TypeHello:
		var msg HelloMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("wsproto: invalid hello message: %w", err)
		}
		return &msg, nil
	case TypeListen:
		var msg ListenMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("wsproto: invalid listen message: %w", err)
		}
		return &msg, nil
	case TypeAbort:
		var msg AbortMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("wsproto: invalid abort message: %w", err)
		}
		return &msg, nil
	case TypeIoT:
		var msg IoTMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("wsproto: invalid iot message: %w", err)
		}
		return &msg, nil
	case TypeMCP:
		var msg MCPMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, fmt.Errorf("wsproto: invalid mcp message: %w", err)
		}
		return &msg, nil
	default:
		return nil, fmt.Errorf("wsproto: unsupported message type %q", env.Type)
	}
}
