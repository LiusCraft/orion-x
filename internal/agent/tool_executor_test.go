package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/tools"
)

func TestRunOneToolCallFillsEmptyOutputFromResultError(t *testing.T) {
	registry := tools.NewRegistry(tools.Spec{
		Name: "failing",
		Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Error: errors.New("mcp tool error: timeout")}, nil
		},
	})
	a := &Agent{registry: registry}

	outcome := a.runOneToolCall(context.Background(), llm.ToolCall{ID: "1", Name: "failing"})

	if outcome.fatal != nil {
		t.Fatalf("expected no fatal error, got %v", outcome.fatal)
	}
	if outcome.message.Content != "mcp tool error: timeout" {
		t.Errorf("expected content filled from Result.Error, got %q", outcome.message.Content)
	}
}

func TestExecuteToolCallsRunsParallelWhenAllSafe(t *testing.T) {
	const sleepDur = 40 * time.Millisecond
	makeSpec := func(name, output string) tools.Spec {
		return tools.Spec{
			Name:         name,
			ParallelSafe: true,
			Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
				time.Sleep(sleepDur)
				return tools.Result{Output: output}, nil
			},
		}
	}
	registry := tools.NewRegistry(makeSpec("first", "first-done"), makeSpec("second", "second-done"))
	a := &Agent{registry: registry}

	calls := []llm.ToolCall{{ID: "1", Name: "first"}, {ID: "2", Name: "second"}}
	start := time.Now()
	messages, err := a.executeToolCalls(context.Background(), calls)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("executeToolCalls() error = %v", err)
	}
	if elapsed >= sleepDur*3/2 {
		t.Errorf("expected concurrent execution (~%v), took %v", sleepDur, elapsed)
	}
	if len(messages) != 2 || messages[0].ToolCallID != "1" || messages[1].ToolCallID != "2" {
		t.Fatalf("expected results in original order, got %+v", messages)
	}
	if messages[0].Content != "first-done" || messages[1].Content != "second-done" {
		t.Fatalf("unexpected content: %+v", messages)
	}
}

func TestExecuteToolCallsFallsBackToSerialWhenNotAllSafe(t *testing.T) {
	registry := tools.NewRegistry(
		tools.Spec{Name: "safe", ParallelSafe: true, Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "safe-done"}, nil
		}},
		tools.Spec{Name: "unsafe", ParallelSafe: false, Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "unsafe-done"}, nil
		}},
	)
	a := &Agent{registry: registry}

	calls := []llm.ToolCall{{ID: "1", Name: "safe"}, {ID: "2", Name: "unsafe"}}
	messages, err := a.executeToolCalls(context.Background(), calls)
	if err != nil {
		t.Fatalf("executeToolCalls() error = %v", err)
	}
	if len(messages) != 2 || messages[0].Content != "safe-done" || messages[1].Content != "unsafe-done" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestExecuteToolCallsFatalErrorStopsButReturnsPartialMessages(t *testing.T) {
	wantErr := errors.New("framework failure")
	registry := tools.NewRegistry(
		tools.Spec{Name: "ok", Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "ok-done"}, nil
		}},
		tools.Spec{Name: "broken", Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{}, wantErr
		}},
	)
	a := &Agent{registry: registry}

	calls := []llm.ToolCall{{ID: "1", Name: "ok"}, {ID: "2", Name: "broken"}}
	messages, err := a.executeToolCalls(context.Background(), calls)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected fatal error %v, got %v", wantErr, err)
	}
	if len(messages) != 1 || messages[0].Content != "ok-done" {
		t.Fatalf("expected only the successful message before the fatal one, got %+v", messages)
	}
}
