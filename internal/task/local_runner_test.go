package task

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/tools"
)

// scriptedLLM 依次返回预设的响应流，超出序列后重复最后一个。
type scriptedLLM struct {
	streams []func() *llm.StreamReader
	calls   int
}

func (c *scriptedLLM) Chat(context.Context, llm.Request) (*llm.StreamReader, error) {
	idx := c.calls
	if idx >= len(c.streams) {
		idx = len(c.streams) - 1
	}
	c.calls++
	return c.streams[idx](), nil
}

func (c *scriptedLLM) ChatSync(context.Context, llm.Request) (llm.Message, error) {
	return llm.Message{}, nil
}

func textStream(text string) func() *llm.StreamReader {
	return func() *llm.StreamReader {
		sr := llm.NewStreamReader(func() {})
		sr.Send(llm.Message{Content: text})
		sr.Close()
		return sr
	}
}

func toolCallStream(calls ...llm.ToolCall) func() *llm.StreamReader {
	return func() *llm.StreamReader {
		sr := llm.NewStreamReader(func() {})
		sr.Send(llm.Message{ToolCalls: calls})
		sr.Close()
		return sr
	}
}

// alwaysToolCallLLM 每轮都要求调用 loopTool，用于触发步数上限。
type alwaysToolCallLLM struct{}

func (alwaysToolCallLLM) Chat(context.Context, llm.Request) (*llm.StreamReader, error) {
	sr := llm.NewStreamReader(func() {})
	sr.Send(llm.Message{ToolCalls: []llm.ToolCall{{ID: "1", Name: "loopTool", Arguments: "{}"}}})
	sr.Close()
	return sr, nil
}

func (alwaysToolCallLLM) ChatSync(context.Context, llm.Request) (llm.Message, error) {
	return llm.Message{}, nil
}

// stallingLLM 发出一个文本增量后挂起，直到 ctx 取消才关闭流。
type stallingLLM struct{}

func (stallingLLM) Chat(ctx context.Context, _ llm.Request) (*llm.StreamReader, error) {
	sr := llm.NewStreamReader(func() {})
	sr.Send(llm.Message{Content: "partial"})
	go func() {
		<-ctx.Done()
		sr.Close()
	}()
	return sr, nil
}

func (stallingLLM) ChatSync(context.Context, llm.Request) (llm.Message, error) {
	return llm.Message{}, nil
}

func collectRunEvents(t *testing.T, run Run) []Event {
	t.Helper()
	var events []Event
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event, ok := <-run.Events():
			if !ok {
				return events
			}
			events = append(events, event)
		case <-timeout:
			t.Fatal("timed out waiting for task events")
		}
	}
}

func TestLocalRunnerToolRoundtrip(t *testing.T) {
	var gotArgs string
	registry := tools.NewRegistry(tools.Spec{
		Name: "getTime",
		Execute: func(_ context.Context, args json.RawMessage) (tools.Result, error) {
			gotArgs = string(args)
			return tools.Result{Output: "12:00"}, nil
		},
	})
	client := &scriptedLLM{streams: []func() *llm.StreamReader{
		toolCallStream(llm.ToolCall{ID: "1", Name: "getTime", Arguments: "{}"}),
		textStream("现在是 12:00"),
	}}
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client: client,
		Tools:  func() *tools.Registry { return registry },
	})

	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "现在几点"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if run.Ref() != "run:task-1" {
		t.Fatalf("Ref() = %q", run.Ref())
	}
	events := collectRunEvents(t, run)

	terminal := events[len(events)-1]
	if terminal.Type != EventDone {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if terminal.Usage == nil || terminal.Usage.Steps != 2 {
		t.Fatalf("usage = %+v", terminal.Usage)
	}
	var text string
	sawTool := false
	for _, event := range events {
		switch event.Type {
		case EventText:
			text += event.Text
		case EventToolUse:
			sawTool = event.Tool != nil && event.Tool.Name == "getTime"
		}
	}
	if !sawTool {
		t.Error("expected a task:tool:use event for getTime")
	}
	if text != "现在是 12:00" {
		t.Errorf("text = %q, want %q", text, "现在是 12:00")
	}
	if gotArgs != "{}" {
		t.Errorf("tool args = %q, want {}", gotArgs)
	}
}

func TestLocalRunnerUnknownToolIsRecoverable(t *testing.T) {
	client := &scriptedLLM{streams: []func() *llm.StreamReader{
		toolCallStream(llm.ToolCall{ID: "1", Name: "doesNotExist", Arguments: "{}"}),
		textStream("尽力了"),
	}}
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client: client,
		Tools:  func() *tools.Registry { return tools.NewRegistry() },
	})

	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "帮我做件怪事"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := collectRunEvents(t, run)
	if terminal := events[len(events)-1]; terminal.Type != EventDone {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if client.calls != 2 {
		t.Errorf("llm calls = %d, want 2 (retry after unknown tool)", client.calls)
	}
}

func TestLocalRunnerReachesMaxSteps(t *testing.T) {
	registry := tools.NewRegistry(tools.Spec{
		Name: "loopTool",
		Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "again"}, nil
		},
	})
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client:   alwaysToolCallLLM{},
		Tools:    func() *tools.Registry { return registry },
		MaxSteps: 2,
	})

	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "死循环"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := collectRunEvents(t, run)
	terminal := events[len(events)-1]
	if terminal.Type != EventError || terminal.Err == nil {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if want := "reached max steps (2)"; terminal.Err.Error() != want {
		t.Errorf("error = %q, want %q", terminal.Err.Error(), want)
	}
}

func TestLocalRunnerCloseStopsRun(t *testing.T) {
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client: stallingLLM{},
		Tools:  func() *tools.Registry { return tools.NewRegistry() },
	})
	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "挂着"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case event := <-run.Events():
		if event.Type != EventText {
			t.Fatalf("first event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the first text event")
	}
	if err := run.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case _, ok := <-run.Events():
		if ok {
			t.Fatal("expected the events channel to be closed after Close()")
		}
	case <-time.After(time.Second):
		t.Fatal("events channel was not closed after Close()")
	}
}
