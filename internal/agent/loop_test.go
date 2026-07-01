package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
)

// sequentialClient 依次按调用次数返回不同的响应流，用于模拟多轮回合。
// 超出序列长度后重复返回最后一个响应。
type sequentialClient struct {
	responses []func() *llm.StreamReader
	calls     int
}

func (c *sequentialClient) Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
	idx := c.calls
	if idx >= len(c.responses) {
		idx = len(c.responses) - 1
	}
	c.calls++
	return c.responses[idx](), nil
}

func (c *sequentialClient) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
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

func collectEvents(t *testing.T, eventChan <-chan AgentEvent) []AgentEvent {
	t.Helper()
	var events []AgentEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-eventChan:
			if !ok {
				return events
			}
			events = append(events, e)
		case <-timeout:
			t.Fatal("timed out waiting for agent events")
		}
	}
}

func lastFinished(events []AgentEvent) *FinishedEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if fe, ok := events[i].(*FinishedEvent); ok {
			return fe
		}
	}
	return nil
}

func newTestSession(userText string) *session.Session {
	sess := session.New(session.SessionMeta{Model: "test"})
	sess.Add(session.Message{Role: session.RoleUser, Content: userText})
	return sess
}

func TestRunLoopSingleStepNoToolCalls(t *testing.T) {
	client := &sequentialClient{responses: []func() *llm.StreamReader{textStream("你好")}}
	a := newWithClient(client, tools.NewRegistry(), "test-model", nil)
	sess := newTestSession("你好")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected successful FinishedEvent, got %+v", fe)
	}
}

func TestRunLoopSingleToolCallRoundtrip(t *testing.T) {
	client := &sequentialClient{responses: []func() *llm.StreamReader{
		toolCallStream(llm.ToolCall{ID: "1", Name: "getTime", Arguments: "{}"}),
		textStream("现在是中午"),
	}}
	registry := tools.NewRegistry(tools.Spec{
		Name: "getTime",
		Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "12:00"}, nil
		},
	})
	a := newWithClient(client, registry, "test-model", nil)
	sess := newTestSession("现在几点")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected successful FinishedEvent, got %+v", fe)
	}

	found := false
	for _, msg := range sess.Messages {
		if msg.Role == session.RoleTool && msg.Content == "12:00" {
			found = true
		}
	}
	if !found {
		t.Error("expected tool result message written back to session")
	}
}

func TestRunLoopParallelToolCallRoundtrip(t *testing.T) {
	client := &sequentialClient{responses: []func() *llm.StreamReader{
		toolCallStream(
			llm.ToolCall{ID: "1", Name: "toolA", Arguments: "{}"},
			llm.ToolCall{ID: "2", Name: "toolB", Arguments: "{}"},
		),
		textStream("完成"),
	}}
	registry := tools.NewRegistry(
		tools.Spec{Name: "toolA", ParallelSafe: true, Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "A"}, nil
		}},
		tools.Spec{Name: "toolB", ParallelSafe: true, Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "B"}, nil
		}},
	)
	a := newWithClient(client, registry, "test-model", nil)
	sess := newTestSession("并行测试")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected successful FinishedEvent, got %+v", fe)
	}

	var toolContents []string
	for _, msg := range sess.Messages {
		if msg.Role == session.RoleTool {
			toolContents = append(toolContents, msg.Content)
		}
	}
	if len(toolContents) != 2 || toolContents[0] != "A" || toolContents[1] != "B" {
		t.Fatalf("expected ordered tool results [A B], got %v", toolContents)
	}
}

func TestRunLoopUnknownToolIsRecoverable(t *testing.T) {
	client := &sequentialClient{responses: []func() *llm.StreamReader{
		toolCallStream(llm.ToolCall{ID: "1", Name: "doesNotExist", Arguments: "{}"}),
		textStream("抱歉，我做不到"),
	}}
	a := newWithClient(client, tools.NewRegistry(), "test-model", nil)
	sess := newTestSession("帮我做件怪事")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected loop to continue and finish successfully, got %+v", fe)
	}
	if client.calls != 2 {
		t.Errorf("expected LLM to be called twice (retry after unknown tool), got %d", client.calls)
	}
}

func TestRunLoopToolExecuteErrorTerminates(t *testing.T) {
	wantErr := errors.New("tool framework failure")
	client := &sequentialClient{responses: []func() *llm.StreamReader{
		toolCallStream(llm.ToolCall{ID: "1", Name: "broken", Arguments: "{}"}),
	}}
	registry := tools.NewRegistry(tools.Spec{
		Name: "broken",
		Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{}, wantErr
		},
	})
	a := newWithClient(client, registry, "test-model", nil)
	sess := newTestSession("触发错误")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error == nil {
		t.Fatalf("expected FinishedEvent with error, got %+v", fe)
	}
}

type alwaysToolCallClient struct{}

func (c *alwaysToolCallClient) Chat(ctx context.Context, req llm.Request) (*llm.StreamReader, error) {
	sr := llm.NewStreamReader(func() {})
	sr.Send(llm.Message{ToolCalls: []llm.ToolCall{{ID: "1", Name: "loopTool", Arguments: "{}"}}})
	sr.Close()
	return sr, nil
}

func (c *alwaysToolCallClient) ChatSync(ctx context.Context, req llm.Request) (llm.Message, error) {
	return llm.Message{}, nil
}

func TestRunLoopReachesMaxSteps(t *testing.T) {
	registry := tools.NewRegistry(tools.Spec{
		Name: "loopTool",
		Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "again"}, nil
		},
	})
	a := newWithClient(&alwaysToolCallClient{}, registry, "test-model", nil)
	a.SetMaxSteps(2)
	sess := newTestSession("死循环")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error == nil {
		t.Fatalf("expected FinishedEvent with max steps error, got %+v", fe)
	}
	wantMsg := fmt.Sprintf("reached max steps (%d)", 2)
	if fe.Error.Error() != wantMsg {
		t.Errorf("expected error %q, got %q", wantMsg, fe.Error.Error())
	}
}
