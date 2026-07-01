package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/tools"
)

type fakeClient struct {
	chatFunc func(ctx context.Context, req llm.Request) (*llm.StreamReader, error)
}

func (f *fakeClient) Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
	return f.chatFunc(ctx, req)
}

func (f *fakeClient) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
	return llm.Message{}, nil
}

func collectEmitted(events *[]AgentEvent) func(AgentEvent) bool {
	return func(e AgentEvent) bool {
		*events = append(*events, e)
		return true
	}
}

func TestRunStepReturnsTextWithoutToolCalls(t *testing.T) {
	a := &Agent{
		client: &fakeClient{
			chatFunc: func(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
				sr := llm.NewStreamReader(func() {})
				sr.Send(llm.Message{Content: "你好"})
				sr.Send(llm.Message{Content: "，世界"})
				sr.Close()
				return sr, nil
			},
		},
		registry: tools.NewRegistry(),
	}

	var events []AgentEvent
	result, err := a.runStep(context.Background(), nil, collectEmitted(&events))
	if err != nil {
		t.Fatalf("runStep() error = %v", err)
	}
	if result.text != "你好，世界" {
		t.Errorf("expected accumulated text %q, got %q", "你好，世界", result.text)
	}
	if len(result.toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %v", result.toolCalls)
	}
	if len(events) == 0 {
		t.Error("expected at least one TextChunkEvent")
	}
}

func TestRunStepCollectsToolCalls(t *testing.T) {
	a := &Agent{
		client: &fakeClient{
			chatFunc: func(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
				sr := llm.NewStreamReader(func() {})
				sr.Send(llm.Message{ToolCalls: []llm.ToolCall{{ID: "1", Name: "getTime", Arguments: "{}"}}})
				sr.Close()
				return sr, nil
			},
		},
		registry: tools.NewRegistry(),
	}

	var events []AgentEvent
	result, err := a.runStep(context.Background(), nil, collectEmitted(&events))
	if err != nil {
		t.Fatalf("runStep() error = %v", err)
	}
	if len(result.toolCalls) != 1 || result.toolCalls[0].Name != "getTime" {
		t.Errorf("expected 1 tool call named getTime, got %v", result.toolCalls)
	}
}

func TestRunStepReturnsStreamError(t *testing.T) {
	wantErr := errors.New("stream broke")
	a := &Agent{
		client: &fakeClient{
			chatFunc: func(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
				sr := llm.NewStreamReader(func() {})
				sr.SendError(wantErr)
				sr.Close()
				return sr, nil
			},
		},
		registry: tools.NewRegistry(),
	}

	var events []AgentEvent
	_, err := a.runStep(context.Background(), nil, collectEmitted(&events))
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}

func TestRunStepStopsWhenEmitReturnsFalse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &Agent{
		client: &fakeClient{
			chatFunc: func(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
				sr := llm.NewStreamReader(func() {})
				sr.Send(llm.Message{Content: "hello"})
				return sr, nil
			},
		},
		registry: tools.NewRegistry(),
	}

	emit := func(e AgentEvent) bool { return false }
	_, err := a.runStep(ctx, nil, emit)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
