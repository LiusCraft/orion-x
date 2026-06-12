package llm

import (
	"context"
	"errors"
	"io"
	"sync"
)

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Request struct {
	Messages []Message
	Tools    []ToolDefinition
}

type Client interface {
	Chat(ctx context.Context, req Request) (*StreamReader, error)
	ChatSync(ctx context.Context, req Request) (Message, error)
}

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
