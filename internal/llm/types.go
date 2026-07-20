package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type BlockType string

const (
	BlockTypeText       BlockType = "text"
	BlockTypeToolCall   BlockType = "tool_call"
	BlockTypeToolResult BlockType = "tool_result"
	BlockTypeRefusal    BlockType = "refusal"
)

type Block struct {
	Type       BlockType   `json:"type"`
	Text       string      `json:"text,omitempty"`
	ToolCall   *ToolCall   `json:"tool_call,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
	Refusal    *Refusal    `json:"refusal,omitempty"`
}

type Refusal struct {
	Reason string `json:"reason"`
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

type ProviderContext struct {
	Adapter string          `json:"adapter"`
	Model   string          `json:"model"`
	Scope   string          `json:"scope"`
	Data    json.RawMessage `json:"data"`
}

// Message keeps the legacy scalar fields during migration. New code should use
// Blocks; Normalize fills Blocks from the legacy representation when needed.
type Message struct {
	Role            string           `json:"role"`
	Blocks          []Block          `json:"blocks,omitempty"`
	ProviderContext *ProviderContext `json:"provider_context,omitempty"`

	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

func (m Message) Normalize() Message {
	if len(m.Blocks) > 0 {
		return m
	}
	if m.Content != "" {
		m.Blocks = append(m.Blocks, Block{Type: BlockTypeText, Text: m.Content})
	}
	for i := range m.ToolCalls {
		call := m.ToolCalls[i]
		m.Blocks = append(m.Blocks, Block{Type: BlockTypeToolCall, ToolCall: &call})
	}
	if m.ToolCallID != "" {
		m.Blocks = append(m.Blocks, Block{
			Type: BlockTypeToolResult,
			ToolResult: &ToolResult{
				ToolCallID: m.ToolCallID,
				Content:    m.Content,
			},
		})
	}
	return m
}

func (m Message) Text() string {
	if len(m.Blocks) == 0 {
		return m.Content
	}
	var out string
	for _, block := range m.Blocks {
		if block.Type == BlockTypeText {
			out += block.Text
		}
	}
	return out
}

func (m Message) Calls() []ToolCall {
	if len(m.Blocks) == 0 {
		return m.ToolCalls
	}
	var calls []ToolCall
	for _, block := range m.Blocks {
		if block.Type == BlockTypeToolCall && block.ToolCall != nil {
			calls = append(calls, *block.ToolCall)
		}
	}
	return calls
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (c ToolCall) Validate() error {
	if c.ID == "" {
		return errors.New("tool call id is required")
	}
	if c.Name == "" {
		return errors.New("tool call name is required")
	}
	if c.Arguments == "" || !json.Valid([]byte(c.Arguments)) {
		return fmt.Errorf("tool call %q arguments must be valid JSON", c.Name)
	}
	return nil
}

func (c ToolCall) ArgumentsJSON() json.RawMessage { return json.RawMessage(c.Arguments) }

type SchemaMode string

const (
	SchemaModeBestEffort SchemaMode = "best_effort"
	SchemaModeStrict     SchemaMode = "strict"
)

type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	SchemaMode  SchemaMode

	// Parameters is retained until all tool registries emit raw JSON schemas.
	Parameters map[string]any
}

func (d ToolDefinition) Schema() (json.RawMessage, error) {
	if len(d.InputSchema) > 0 {
		if !json.Valid(d.InputSchema) {
			return nil, fmt.Errorf("tool %q input schema is invalid JSON", d.Name)
		}
		return d.InputSchema, nil
	}
	if d.Parameters == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`), nil
	}
	data, err := json.Marshal(d.Parameters)
	if err != nil {
		return nil, fmt.Errorf("marshal tool %q input schema: %w", d.Name, err)
	}
	return data, nil
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceFunction ToolChoiceMode = "function"
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

type ThinkingMode string

const (
	ThinkingModeDefault  ThinkingMode = "default"
	ThinkingModeEnabled  ThinkingMode = "enabled"
	ThinkingModeDisabled ThinkingMode = "disabled"
)

type ThinkingEffort string

const (
	ThinkingEffortDefault ThinkingEffort = "default"
	ThinkingEffortMinimal ThinkingEffort = "minimal"
	ThinkingEffortLow     ThinkingEffort = "low"
	ThinkingEffortMedium  ThinkingEffort = "medium"
	ThinkingEffortHigh    ThinkingEffort = "high"
	ThinkingEffortXHigh   ThinkingEffort = "xhigh"
	ThinkingEffortMax     ThinkingEffort = "max"
)

type PreserveMode string

const (
	PreserveModeDefault PreserveMode = "default"
	PreserveModeNone    PreserveMode = "none"
	PreserveModeAll     PreserveMode = "all"
)

type ThinkingConfig struct {
	Mode            ThinkingMode   `json:"mode,omitempty"`
	Effort          ThinkingEffort `json:"effort,omitempty"`
	BudgetTokens    *int           `json:"budget_tokens,omitempty"`
	PreserveHistory PreserveMode   `json:"preserve_history,omitempty"`
	ExposeSummary   bool           `json:"expose_summary,omitempty"`
}

func (c ThinkingConfig) IsDefault() bool {
	return (c.Mode == "" || c.Mode == ThinkingModeDefault) &&
		(c.Effort == "" || c.Effort == ThinkingEffortDefault) &&
		c.BudgetTokens == nil &&
		(c.PreserveHistory == "" || c.PreserveHistory == PreserveModeDefault) &&
		!c.ExposeSummary
}

func (c ThinkingConfig) HasEffort() bool {
	return c.Effort != "" && c.Effort != ThinkingEffortDefault
}

type JSONSchemaFormat struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Strict      bool
}

type TextBlock struct {
	Text string
}

type Request struct {
	Instructions    []TextBlock
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      ToolChoice
	ParallelTools   *bool
	OutputFormat    *JSONSchemaFormat
	MaxOutputTokens *int
	Temperature     *float64
	StopSequences   []string
	Thinking        ThinkingConfig
	ProviderOptions json.RawMessage
}

type StopReason string

const (
	StopReasonStop          StopReason = "stop"
	StopReasonToolCalls     StopReason = "tool_calls"
	StopReasonLength        StopReason = "length"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonPause         StopReason = "pause"
	StopReasonError         StopReason = "error"
	StopReasonUnknown       StopReason = "unknown"
)

type Usage struct {
	InputTokens       int64           `json:"input_tokens"`
	OutputTokens      int64           `json:"output_tokens"`
	ReasoningTokens   int64           `json:"reasoning_tokens,omitempty"`
	CacheReadTokens   int64           `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens  int64           `json:"cache_write_tokens,omitempty"`
	TotalTokens       int64           `json:"total_tokens"`
	ProviderBreakdown json.RawMessage `json:"provider_breakdown,omitempty"`
}

type Response struct {
	ID         string
	Model      string
	Message    Message
	StopReason StopReason
	StopDetail string
	Usage      Usage
	RequestID  string
}

type EventType string

const (
	EventResponseStart         EventType = "response_start"
	EventTextDelta             EventType = "text_delta"
	EventToolCallStart         EventType = "tool_call_start"
	EventToolCallDelta         EventType = "tool_call_delta"
	EventToolCallDone          EventType = "tool_call_done"
	EventReasoningSummaryDelta EventType = "reasoning_summary_delta"
	EventResponseDone          EventType = "response_done"
)

type ToolCallDelta struct {
	ID             string
	Name           string
	ArgumentsDelta string
	Done           bool
}

type Event struct {
	Type      EventType
	Index     int
	TextDelta string
	ToolCall  *ToolCallDelta
	Reasoning string
	Response  *Response
}

type Stream interface {
	Recv() (Event, error)
	Close() error
}

type GenerationClient interface {
	Generate(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request) (Stream, error)
}

// Client is the legacy interface kept while Agent and memory callers migrate.
type Client interface {
	Chat(ctx context.Context, req Request) (*StreamReader, error)
	ChatSync(ctx context.Context, req Request) (Message, error)
}

type EventStream struct {
	ch       chan streamEventItem
	done     chan struct{}
	cancel   func()
	closeOne sync.Once
}

type streamEventItem struct {
	event Event
	err   error
}

func NewEventStream(cancel func()) *EventStream {
	return &EventStream{
		ch:     make(chan streamEventItem, 32),
		done:   make(chan struct{}),
		cancel: cancel,
	}
}

func (s *EventStream) Send(event Event) bool {
	select {
	case <-s.done:
		return false
	case s.ch <- streamEventItem{event: event}:
		return true
	}
}

func (s *EventStream) SendError(err error) bool {
	select {
	case <-s.done:
		return false
	case s.ch <- streamEventItem{err: err}:
		return true
	}
}

// Finish is producer-only and closes the receive side.
func (s *EventStream) Finish() {
	s.closeOne.Do(func() {
		close(s.done)
		close(s.ch)
	})
}

func (s *EventStream) Recv() (Event, error) {
	item, ok := <-s.ch
	if !ok {
		return Event{}, io.EOF
	}
	return item.event, item.err
}

func (s *EventStream) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// StreamReader is the compatibility stream used by legacy callers.
type StreamReader struct {
	ch     chan streamItem
	done   bool
	mu     sync.Mutex
	cancel func()
}

type streamItem struct {
	msg Message
	err error
}

func NewStreamReader(cancel func()) *StreamReader {
	return &StreamReader{
		ch:     make(chan streamItem, 32),
		cancel: cancel,
	}
}

func (sr *StreamReader) Send(msg Message) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.done {
		return
	}
	sr.ch <- streamItem{msg: msg}
}

func (sr *StreamReader) SendError(err error) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.done {
		return
	}
	sr.ch <- streamItem{err: err}
}

func (sr *StreamReader) Recv() (Message, error) {
	item, ok := <-sr.ch
	if !ok {
		return Message{}, io.EOF
	}
	return item.msg, item.err
}

func (sr *StreamReader) Close() {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.done {
		return
	}
	sr.done = true
	if sr.cancel != nil {
		sr.cancel()
	}
	close(sr.ch)
}

var ErrStreamClosed = errors.New("stream closed")

type APIError struct {
	Adapter    string
	StatusCode int
	Type       string
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	Cause      error
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Adapter, e.Message, e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Adapter, e.Message)
}

func (e *APIError) Unwrap() error { return e.Cause }

type UnsupportedOptionError struct {
	Adapter string
	Option  string
	Reason  string
}

func (e *UnsupportedOptionError) Error() string {
	return fmt.Sprintf("%s does not support %s: %s", e.Adapter, e.Option, e.Reason)
}
