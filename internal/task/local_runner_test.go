package task

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/agent"
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

// usageGenStream 是 llm.Stream 的最小实现：按序吐出事件，事件用尽后返回 err
// （只返回一次），否则 io.EOF。
type usageGenStream struct {
	events []llm.Event
	err    error
	idx    int
}

func (s *usageGenStream) Recv() (llm.Event, error) {
	if s.idx < len(s.events) {
		event := s.events[s.idx]
		s.idx++
		return event, nil
	}
	if s.err != nil {
		err := s.err
		s.err = nil
		return llm.Event{}, err
	}
	return llm.Event{}, io.EOF
}

func (s *usageGenStream) Close() error { return nil }

// usageGenClient 实现 llm.GenerationClient，因此 local runner 走 Stream 路径，
// 能带上真实用量——生产用的 provider.Client 也是这样被识别的。
type usageGenClient struct {
	streams []*usageGenStream
	calls   int
}

func (c *usageGenClient) Chat(context.Context, llm.Request) (*llm.StreamReader, error) {
	return nil, errors.New("Chat should not be called")
}

func (c *usageGenClient) ChatSync(context.Context, llm.Request) (llm.Message, error) {
	return llm.Message{}, errors.New("ChatSync should not be called")
}

func (c *usageGenClient) Generate(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, errors.New("Generate should not be called")
}

func (c *usageGenClient) Stream(context.Context, llm.Request) (llm.Stream, error) {
	idx := c.calls
	if idx >= len(c.streams) {
		idx = len(c.streams) - 1
	}
	c.calls++
	return c.streams[idx], nil
}

func responseDoneEvent(resp *llm.Response) llm.Event {
	return llm.Event{Type: llm.EventResponseDone, Response: resp}
}

// TestLocalRunnerAggregatesUsageAcrossSteps 验证多步执行的用量会累加进终态事件。
// 这是本 PR 修复的核心缺口（任务用量此前从未上报计费），必须有回归兜底。
func TestLocalRunnerAggregatesUsageAcrossSteps(t *testing.T) {
	registry := tools.NewRegistry(tools.Spec{
		Name: "noop",
		Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "ok"}, nil
		},
	})
	client := &usageGenClient{streams: []*usageGenStream{
		{events: []llm.Event{responseDoneEvent(&llm.Response{
			Message: llm.Message{ToolCalls: []llm.ToolCall{{ID: "1", Name: "noop", Arguments: "{}"}}},
			Usage:   llm.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 60},
		})}},
		{events: []llm.Event{responseDoneEvent(&llm.Response{
			Message: llm.Message{Content: "done"},
			Usage:   llm.Usage{InputTokens: 200, OutputTokens: 30, ReasoningTokens: 5},
		})}},
	}}
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client: client,
		Tools:  func() *tools.Registry { return registry },
	})

	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "算一下"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := collectRunEvents(t, run)
	terminal := events[len(events)-1]
	if terminal.Type != EventDone {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if terminal.Usage == nil {
		t.Fatal("EventDone.Usage = nil, want accumulated usage")
	}
	want := agent.Usage{
		InputTokens:     300,
		OutputTokens:    40,
		ReasoningTokens: 5,
		CacheReadTokens: 60,
		Steps:           2,
	}
	if *terminal.Usage != want {
		t.Errorf("EventDone.Usage = %+v, want %+v", *terminal.Usage, want)
	}
}

// TestLocalRunnerKeepsUsageOnStreamError 验证流中途报错时，已经发生的用量不会
// 随错误一起丢掉：厂商已经收过钱，这部分 token 仍要计费。
func TestLocalRunnerKeepsUsageOnStreamError(t *testing.T) {
	wantErr := errors.New("stream broke after response")
	client := &usageGenClient{streams: []*usageGenStream{{
		events: []llm.Event{responseDoneEvent(&llm.Response{
			Message: llm.Message{Content: "half"},
			Usage:   llm.Usage{InputTokens: 50, OutputTokens: 5},
		})},
		err: wantErr,
	}}}
	runner := NewLocalRunner(context.Background(), LocalConfig{
		Client: client,
		Tools:  func() *tools.Registry { return tools.NewRegistry() },
	})

	run, err := runner.Start(RunRequest{TaskID: "task-1", Title: "算一半"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	events := collectRunEvents(t, run)
	terminal := events[len(events)-1]
	if terminal.Type != EventError {
		t.Fatalf("terminal event = %+v", terminal)
	}
	if terminal.Err == nil || !errors.Is(terminal.Err, wantErr) {
		t.Errorf("terminal.Err = %v, want %v", terminal.Err, wantErr)
	}
	if terminal.Usage == nil {
		t.Fatal("EventError.Usage = nil, want the usage already spent")
	}
	if terminal.Usage.InputTokens != 50 || terminal.Usage.OutputTokens != 5 {
		t.Errorf("EventError.Usage = %+v, want input=50 output=5", *terminal.Usage)
	}
}
