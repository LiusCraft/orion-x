package wsserver

import "encoding/json"

type BaseMessage struct {
	Type string `json:"type"`
}

type HelloMessage struct {
	Type        string                 `json:"type"`
	DeviceID    string                 `json:"device_id,omitempty"`
	DeviceName  string                 `json:"device_name,omitempty"`
	DeviceMac   string                 `json:"device_mac,omitempty"`
	Token       string                 `json:"token,omitempty"`
	Features    map[string]any         `json:"features,omitempty"`
	AgentParams map[string]any         `json:"agent_params,omitempty"`
	AudioParams *AudioParams           `json:"audio_params,omitempty"`
	Extra       map[string]interface{} `json:"-"`
}

type ListenMessage struct {
	Type         string `json:"type"`
	Mode         string `json:"mode,omitempty"`
	State        string `json:"state,omitempty"`
	Text         string `json:"text,omitempty"`
	TextResponse string `json:"text_response,omitempty"`
}

type AbortMessage struct {
	Type string `json:"type"`
}

type MCPMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type IOTMessage struct {
	Type        string          `json:"type"`
	Descriptors json.RawMessage `json:"descriptors,omitempty"`
	States      json.RawMessage `json:"states,omitempty"`
}

type ServerCommandMessage struct {
	Type    string          `json:"type"`
	Action  string          `json:"action,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
}

type ServerHelloMessage struct {
	Type        string      `json:"type"`
	Version     int         `json:"version"`
	Transport   string      `json:"transport"`
	SessionID   string      `json:"session_id,omitempty"`
	AudioParams AudioParams `json:"audio_params"`
}

type TTSMessage struct {
	Type      string `json:"type"`
	State     string `json:"state"`
	Text      string `json:"text,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	IsAborted bool   `json:"is_aborted,omitempty"`
}

type LLMMessage struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Emotion   string `json:"emotion,omitempty"`
	Action    string `json:"action,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type STTMessage struct {
	Type      string `json:"type"`
	State     string `json:"state,omitempty"`
	Text      string `json:"text"`
	SessionID string `json:"session_id,omitempty"`
	ErrorCode int    `json:"error_code,omitempty"`
}

type NotifyMessage struct {
	Type    string `json:"type"`
	Event   string `json:"event"`
	AgentID string `json:"agent_id,omitempty"`
}

type ServerStatusMessage struct {
	Type    string            `json:"type"`
	Status  string            `json:"status"`
	Message string            `json:"message"`
	Content map[string]string `json:"content,omitempty"`
}
