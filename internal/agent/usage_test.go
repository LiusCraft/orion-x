package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/liuscraft/orion-x/internal/llm"
	"github.com/liuscraft/orion-x/internal/session"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/pkg/pipeline"
)

func TestUsageCollectorAccumulatesAndTakes(t *testing.T) {
	collector := NewUsageCollector()
	collector.Add(llm.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 60, TotalTokens: 170})
	collector.Add(llm.Usage{InputTokens: 7, OutputTokens: 3, ReasoningTokens: 2, CacheWriteTokens: 5})

	want := Usage{InputTokens: 107, OutputTokens: 13, ReasoningTokens: 2, CacheReadTokens: 60, CacheWriteTokens: 5, Steps: 2}
	if got := collector.Take(); got != want {
		t.Errorf("Take() = %+v, want %+v", got, want)
	}
	if got := collector.Take(); !got.IsZero() {
		t.Errorf("Take() should reset the collector, got %+v", got)
	}
}

// TestUsageCollectorAddIsConcurrencySafe 用 -race 覆盖并行工具调用/子代理同时记账。
func TestUsageCollectorAddIsConcurrencySafe(t *testing.T) {
	collector := NewUsageCollector()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				collector.Add(llm.Usage{InputTokens: 1, OutputTokens: 1})
			}
		}()
	}
	wg.Wait()

	got := collector.Take()
	if got.InputTokens != 400 || got.OutputTokens != 400 || got.Steps != 400 {
		t.Errorf("Take() = %+v, want 400/400/400", got)
	}
}

func TestUsageHelpersAreNilSafe(t *testing.T) {
	var collector *UsageCollector
	collector.Add(llm.Usage{InputTokens: 1}) // 不得 panic
	if got := collector.Take(); !got.IsZero() {
		t.Errorf("nil collector Take() = %+v, want zero", got)
	}

	if UsageCollectorFromContext(context.Background()) != nil {
		t.Error("UsageCollectorFromContext() without a collector should return nil")
	}
	if usage := TakeUsage(context.Background()); usage != nil {
		t.Errorf("TakeUsage() without a collector = %+v, want nil", usage)
	}

	// 计费未启用（没有 collector）时，装着 collector 但本 turn 没有任何调用也返回 nil。
	ctx := WithUsageCollector(context.Background(), NewUsageCollector())
	if usage := TakeUsage(ctx); usage != nil {
		t.Errorf("TakeUsage() on an empty turn = %+v, want nil", usage)
	}

	var nilCtx context.Context
	if usage := TakeUsage(nilCtx); usage != nil {
		t.Errorf("TakeUsage(nil) = %+v, want nil", usage)
	}
}

// fakeGenStream 是 runStep 消费的最小 llm.Stream：按序吐出事件，事件用尽后返回
// err（只返回一次），否则 io.EOF。
type fakeGenStream struct {
	events []llm.Event
	err    error
	idx    int
}

func (s *fakeGenStream) Recv() (llm.Event, error) {
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

func (s *fakeGenStream) Close() error { return nil }

// usageGenClient 实现 llm.GenerationClient，因此 runStep 走 Stream 路径
// （provider.Client 也是这样被 AdaptLegacyClient 识别的）。
type usageGenClient struct {
	streams []*fakeGenStream
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

func responseDone(resp *llm.Response) llm.Event {
	return llm.Event{Type: llm.EventResponseDone, Response: resp}
}

func usageRegistry() *tools.Registry {
	return tools.NewRegistry(tools.Spec{
		Name: "noop",
		Execute: func(context.Context, json.RawMessage) (tools.Result, error) {
			return tools.Result{Output: "ok"}, nil
		},
	})
}

// TestRunLoopRecordsUsageIntoFinishedEvent 验证一个 turn 内跨 step 的用量会累加，
// 并在 turn 边界一次性带在 FinishedEvent 上。
func TestRunLoopRecordsUsageIntoFinishedEvent(t *testing.T) {
	client := &usageGenClient{streams: []*fakeGenStream{
		{events: []llm.Event{responseDone(&llm.Response{
			Message: llm.Message{ToolCalls: []llm.ToolCall{{ID: "1", Name: "noop", Arguments: "{}"}}},
			Usage:   llm.Usage{InputTokens: 100, OutputTokens: 10, CacheReadTokens: 60, TotalTokens: 170},
		})}},
		{events: []llm.Event{responseDone(&llm.Response{
			Message: llm.Message{Content: "done"},
			Usage:   llm.Usage{InputTokens: 200, OutputTokens: 30, ReasoningTokens: 5, TotalTokens: 235},
		})}},
	}}
	a := newWithClient(client, usageRegistry(), "test-model", nil, "", "")
	sess := newTestSession("你好")

	collector := NewUsageCollector()
	eventChan, err := a.Run(WithUsageCollector(context.Background(), collector), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected successful FinishedEvent, got %+v", fe)
	}
	want := Usage{InputTokens: 300, OutputTokens: 40, ReasoningTokens: 5, CacheReadTokens: 60, Steps: 2}
	if fe.Usage == nil {
		t.Fatal("FinishedEvent.Usage = nil, want accumulated usage")
	}
	if *fe.Usage != want {
		t.Errorf("FinishedEvent.Usage = %+v, want %+v", *fe.Usage, want)
	}
	if got := collector.Take(); !got.IsZero() {
		t.Errorf("collector should be drained at the turn boundary, got %+v", got)
	}
}

// TestRunLoopErrorPathCarriesUsage 验证响应已到手后本步才失败时，这个 turn 的用量
// 仍然跟着 error 路径带出来（中断的 turn 已经产生了 token）。
func TestRunLoopErrorPathCarriesUsage(t *testing.T) {
	wantErr := errors.New("stream broke after response")
	client := &usageGenClient{streams: []*fakeGenStream{{
		events: []llm.Event{responseDone(&llm.Response{
			Message: llm.Message{Content: "half"},
			Usage:   llm.Usage{InputTokens: 50, OutputTokens: 5, TotalTokens: 55},
		})},
		err: wantErr,
	}}}
	a := newWithClient(client, usageRegistry(), "test-model", nil, "", "")
	sess := newTestSession("你好")

	eventChan, err := a.Run(WithUsageCollector(context.Background(), NewUsageCollector()), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || !errors.Is(fe.Error, wantErr) {
		t.Fatalf("expected FinishedEvent carrying %v, got %+v", wantErr, fe)
	}
	want := Usage{InputTokens: 50, OutputTokens: 5, Steps: 1}
	if fe.Usage == nil || *fe.Usage != want {
		t.Errorf("FinishedEvent.Usage = %+v, want %+v", fe.Usage, want)
	}
}

// TestRunLoopWithoutCollectorKeepsUsageNil 验证没装 collector（计费关闭）时一切照旧。
func TestRunLoopWithoutCollectorKeepsUsageNil(t *testing.T) {
	client := &usageGenClient{streams: []*fakeGenStream{{
		events: []llm.Event{responseDone(&llm.Response{
			Message: llm.Message{Content: "done"},
			Usage:   llm.Usage{InputTokens: 50, OutputTokens: 5, TotalTokens: 55},
		})},
	}}}
	a := newWithClient(client, usageRegistry(), "test-model", nil, "", "")
	sess := newTestSession("你好")

	eventChan, err := a.Run(context.Background(), sess)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	fe := lastFinished(collectEvents(t, eventChan))
	if fe == nil || fe.Error != nil {
		t.Fatalf("expected successful FinishedEvent, got %+v", fe)
	}
	if fe.Usage != nil {
		t.Errorf("FinishedEvent.Usage = %+v, want nil without a collector", *fe.Usage)
	}
}

// TestAgentStageFinishedMessageCarriesUsage 验证 Stage 会为每个 turn 装 collector，
// 并把用量放进 pipeline 消息元数据（同时不动 source extra、不与入参共享 map）。
func TestAgentStageFinishedMessageCarriesUsage(t *testing.T) {
	runner := &mockAgentRunner{
		processFunc: func(ctx context.Context, _ *session.Session) (<-chan AgentEvent, error) {
			collector := UsageCollectorFromContext(ctx)
			if collector == nil {
				return nil, errors.New("usage collector is not installed for the turn")
			}
			collector.Add(llm.Usage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 60, TotalTokens: 180})

			ch := make(chan AgentEvent, 2)
			ch <- &TextChunkEvent{Chunk: "你好"}
			ch <- &FinishedEvent{Usage: TakeUsage(ctx)}
			close(ch)
			return ch, nil
		},
	}

	stage := NewAgentStage(runner, session.New(session.SessionMeta{Model: "test"}))
	input := make(chan pipeline.Message, 1)
	output := stage.Process(context.Background(), input)
	defer close(input)

	textMsg := pipeline.NewMessage(pipeline.MessageTypeData, "hello")
	textMsg.Metadata.Extra = map[string]interface{}{"source": "llm"}
	input <- textMsg

	finished := <-output
	for finished.Type != pipeline.MessageTypeFinished {
		finished = <-output
	}

	usage, ok := UsageFromMetadata(finished.Metadata)
	if !ok {
		t.Fatalf("finished message has no usage metadata: %#v", finished.Metadata.Extra)
	}
	// collector 只做累加；字段的互斥口径由 adapter 写入时保证（见 llm.Usage 注释）。
	want := Usage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 60, Steps: 1}
	if usage != want {
		t.Errorf("usage from metadata = %+v, want %+v", usage, want)
	}
	if source, _ := finished.Metadata.Extra["source"].(string); source != "llm" {
		t.Errorf("existing source extra = %q, want %q", source, "llm")
	}
	if _, leaked := textMsg.Metadata.Extra[UsageMetadataKey]; leaked {
		t.Error("incoming message metadata was mutated with the usage key")
	}
}

func TestUsageFromMetadataWithoutUsage(t *testing.T) {
	if _, ok := UsageFromMetadata(pipeline.Metadata{}); ok {
		t.Error("UsageFromMetadata() on empty metadata should report false")
	}
	md := pipeline.Metadata{Extra: map[string]interface{}{"source": "llm"}}
	if _, ok := UsageFromMetadata(md); ok {
		t.Error("UsageFromMetadata() without the usage key should report false")
	}
	// 契约是存值（不是指针）；别的类型一律当作没有。
	md.Extra[UsageMetadataKey] = &Usage{Steps: 1}
	if _, ok := UsageFromMetadata(md); ok {
		t.Error("UsageFromMetadata() should reject a non-Usage value")
	}
}
