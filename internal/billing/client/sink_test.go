package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/liuscraft/orion-x/internal/billing"
)

// stubControlPlane 是一个假的控制面：记录收到的每个请求，按脚本回复。
type stubControlPlane struct {
	*httptest.Server

	mu       sync.Mutex
	sequence []string
	usage    []billing.UsageBatchRequest
	settles  []billing.SettleRequest

	usageHandler func(call int, req billing.UsageBatchRequest) (int, string)

	gate     chan struct{}
	gateOpen bool

	entered     chan struct{}
	enteredOnce sync.Once
}

func newStubControlPlane(t *testing.T) *stubControlPlane {
	t.Helper()

	stub := &stubControlPlane{entered: make(chan struct{})}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}

		stub.mu.Lock()
		stub.sequence = append(stub.sequence, r.URL.Path)
		gate := stub.gate
		stub.mu.Unlock()

		switch r.URL.Path {
		case billing.PathUsageEvents:
			var req billing.UsageBatchRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("decode usage batch: %v", err)
			}
			stub.mu.Lock()
			stub.usage = append(stub.usage, req)
			call := len(stub.usage)
			handler := stub.usageHandler
			stub.mu.Unlock()

			if call == 1 {
				stub.enteredOnce.Do(func() { close(stub.entered) })
			}
			if gate != nil {
				<-gate
			}
			status, payload := http.StatusOK, fmt.Sprintf(`{"accepted":%d}`, len(req.Events))
			if handler != nil {
				status, payload = handler(call, req)
			}
			writeStubResponse(w, status, payload)
		case billing.PathSettle:
			var req billing.SettleRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("decode settle request: %v", err)
			}
			stub.mu.Lock()
			stub.settles = append(stub.settles, req)
			stub.mu.Unlock()
			writeStubResponse(w, http.StatusOK, `{"charged_micro":12345,"released_micro":487655,"balance_micro":7987655,"settled":true}`)
		default:
			writeStubResponse(w, http.StatusNotFound, `{"error":"unknown path"}`)
		}
	}))
	t.Cleanup(func() {
		stub.release()
		stub.Close()
	})
	return stub
}

func writeStubResponse(w http.ResponseWriter, status int, payload string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, payload)
}

// onUsage 替换 usage-events 的回复逻辑。
func (s *stubControlPlane) onUsage(handler func(call int, req billing.UsageBatchRequest) (int, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usageHandler = handler
}

// holdUsage 让后续的 usage 请求挂住，直到 release 被调用。
func (s *stubControlPlane) holdUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate == nil {
		s.gate = make(chan struct{})
	}
}

// release 放行挂住的请求；重复调用安全。
func (s *stubControlPlane) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate != nil && !s.gateOpen {
		close(s.gate)
		s.gateOpen = true
	}
}

func (s *stubControlPlane) usageCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.usage)
}

func (s *stubControlPlane) usageRequests() []billing.UsageBatchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]billing.UsageBatchRequest(nil), s.usage...)
}

func (s *stubControlPlane) settleRequests() []billing.SettleRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]billing.SettleRequest(nil), s.settles...)
}

func (s *stubControlPlane) requestSequence() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sequence...)
}

// waitForUsageCalls 等到至少收到 want 个上报请求，用来盯后台 flush。
func (s *stubControlPlane) waitForUsageCalls(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.usageCalls() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("usage calls = %d, want at least %d", s.usageCalls(), want)
}

// waitUsageEntered 等到第一个上报请求真的进了服务端，用来确认网络调用正在进行。
func (s *stubControlPlane) waitUsageEntered(t *testing.T) {
	t.Helper()
	select {
	case <-s.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("no usage request reached the control plane in time")
	}
}

// waitFor 轮询等一个条件成立，用来盯后台 goroutine 的效果。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// statusScript 按序返回状态码，脚本用完后重复最后一个；200 回复空结果体。
func statusScript(statuses ...int) func(call int, _ billing.UsageBatchRequest) (int, string) {
	return func(call int, _ billing.UsageBatchRequest) (int, string) {
		status := statuses[len(statuses)-1]
		if call-1 < len(statuses) {
			status = statuses[call-1]
		}
		if status == http.StatusOK {
			return status, "{}"
		}
		return status, fmt.Sprintf(`{"error":"status %d"}`, status)
	}
}

// newTestSink 构造测试用的 Sink：短退避，未指定的字段给一组安全默认。
func newTestSink(t *testing.T, stub *stubControlPlane, cfg SinkConfig) *Sink {
	t.Helper()
	if cfg.Client == nil {
		cfg.Client = New(Config{BaseURL: stub.URL, Token: "test-token"})
	}
	if cfg.SessionID == "" {
		cfg.SessionID = "sess-1"
	}
	if cfg.DeviceID == "" {
		cfg.DeviceID = "dev-1"
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 1
	}
	if cfg.RetryBase == 0 {
		cfg.RetryBase = time.Millisecond
	}
	if cfg.RetryMax == 0 {
		cfg.RetryMax = 2 * time.Millisecond
	}
	sink := NewSink(cfg)
	t.Cleanup(sink.Close)
	return sink
}

// priceSnapshot 造一份可估算的快照。
func priceSnapshot(itemCode string, unitPriceMicro, unitSize int64) map[string]billing.PriceSnapshot {
	return map[string]billing.PriceSnapshot{
		itemCode: {ItemCode: itemCode, Billable: true, UnitPriceMicro: unitPriceMicro, UnitSize: unitSize},
	}
}

// eventView 是一条上报事件里测试关心的部分（event_id 是随机的，occurred_at 由时间断言
// 单独盯）。
type eventView struct {
	ItemCode   string
	Quantity   int64
	TurnIndex  int64
	Dimensions map[string]any
}

func eventViews(events []billing.UsageEventReport) []eventView {
	out := make([]eventView, 0, len(events))
	for _, event := range events {
		out = append(out, eventView{
			ItemCode:   event.ItemCode,
			Quantity:   event.Quantity,
			TurnIndex:  event.TurnIndex,
			Dimensions: event.Dimensions,
		})
	}
	return out
}

// allEvents 把多次请求里的事件拼成一串。
func allEvents(t *testing.T, stub *stubControlPlane) []billing.UsageEventReport {
	t.Helper()
	var out []billing.UsageEventReport
	for _, req := range stub.usageRequests() {
		out = append(out, req.Events...)
	}
	return out
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// TestSinkRecordAccumulatesByKey 缓冲按 (item_code, turn_index, 维度) 合并。
func TestSinkRecordAccumulatesByKey(t *testing.T) {
	tests := []struct {
		name    string
		records []recordCall
		want    []eventView
	}{
		{
			name: "same item, turn and dims merge",
			records: []recordCall{
				{item: billing.ItemTTSCharacters, quantity: 3, dims: map[string]any{"source": "tts"}},
				{item: billing.ItemTTSCharacters, quantity: 4, dims: map[string]any{"source": "tts"}},
				{item: billing.ItemTTSCharacters, quantity: 5, dims: map[string]any{"source": "tts"}},
			},
			want: []eventView{
				{ItemCode: billing.ItemTTSCharacters, Quantity: 12, TurnIndex: 1, Dimensions: map[string]any{"source": "tts"}},
			},
		},
		{
			name: "different dims stay apart",
			records: []recordCall{
				{item: billing.ItemTTSCharacters, quantity: 3, dims: map[string]any{"source": "main"}},
				{item: billing.ItemTTSCharacters, quantity: 4, dims: map[string]any{"source": "tts"}},
			},
			want: []eventView{
				{ItemCode: billing.ItemTTSCharacters, Quantity: 3, TurnIndex: 1, Dimensions: map[string]any{"source": "main"}},
				{ItemCode: billing.ItemTTSCharacters, Quantity: 4, TurnIndex: 1, Dimensions: map[string]any{"source": "tts"}},
			},
		},
		{
			name: "dims key order does not matter",
			records: []recordCall{
				{item: billing.ItemTTSCharacters, quantity: 3, dims: map[string]any{"a": "1", "b": "2"}},
				{item: billing.ItemTTSCharacters, quantity: 4, dims: map[string]any{"b": "2", "a": "1"}},
			},
			want: []eventView{
				{ItemCode: billing.ItemTTSCharacters, Quantity: 7, TurnIndex: 1, Dimensions: map[string]any{"a": "1", "b": "2"}},
			},
		},
		{
			name: "different turns stay apart",
			records: []recordCall{
				{item: billing.ItemTTSCharacters, quantity: 3},
				{turn: turnPtr(2), item: billing.ItemTTSCharacters, quantity: 4},
			},
			want: []eventView{
				{ItemCode: billing.ItemTTSCharacters, Quantity: 3, TurnIndex: 1},
				{ItemCode: billing.ItemTTSCharacters, Quantity: 4, TurnIndex: 2},
			},
		},
		{
			name: "different items stay apart",
			records: []recordCall{
				{item: billing.ItemLLMInput, quantity: 10},
				{item: billing.ItemLLMOutput, quantity: 20},
				{item: billing.ItemLLMInput, quantity: 5},
			},
			want: []eventView{
				{ItemCode: billing.ItemLLMInput, Quantity: 15, TurnIndex: 1},
				{ItemCode: billing.ItemLLMOutput, Quantity: 20, TurnIndex: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubControlPlane(t)
			sink := newTestSink(t, stub, SinkConfig{})
			sink.SetTurn(1)

			for _, record := range tt.records {
				if record.turn != nil {
					sink.SetTurn(*record.turn)
				}
				sink.Record(record.item, record.quantity, record.dims)
			}

			if got := sink.Pending(); got != len(tt.want) {
				t.Fatalf("pending = %d, want %d", got, len(tt.want))
			}
			if err := sink.Flush(context.Background()); err != nil {
				t.Fatalf("flush: %v", err)
			}
			if got := eventViews(allEvents(t, stub)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("events = %+v, want %+v", got, tt.want)
			}
			if got := sink.Pending(); got != 0 {
				t.Fatalf("pending after flush = %d, want 0", got)
			}
		})
	}
}

type recordCall struct {
	turn     *int64
	item     string
	quantity int64
	dims     map[string]any
}

func turnPtr(turn int64) *int64 { return &turn }

// TestSinkRecordIgnoresEmptyAndNonPositive 非正的量与空 item_code 直接忽略。
func TestSinkRecordIgnoresEmptyAndNonPositive(t *testing.T) {
	tests := []struct {
		name     string
		item     string
		quantity int64
	}{
		{name: "zero", item: billing.ItemTTSCharacters, quantity: 0},
		{name: "negative", item: billing.ItemTTSCharacters, quantity: -3},
		{name: "empty item", item: "", quantity: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubControlPlane(t)
			sink := newTestSink(t, stub, SinkConfig{ReservedMicro: 1000, Snapshot: priceSnapshot(billing.ItemTTSCharacters, 1, 1)})

			sink.Record(tt.item, tt.quantity, nil)

			if got := sink.Pending(); got != 0 {
				t.Errorf("pending = %d, want 0", got)
			}
			if got := sink.Budget().EstimatedMicro; got != 0 {
				t.Errorf("estimated = %d, want 0", got)
			}
			if err := sink.Flush(context.Background()); err != nil {
				t.Errorf("flush: %v", err)
			}
			if got := stub.usageCalls(); got != 0 {
				t.Errorf("usage calls = %d, want 0", got)
			}
		})
	}
}

// TestSinkRecordCopiesDimensions 调用方之后改自己的 map 不能影响缓冲里的事实。
func TestSinkRecordCopiesDimensions(t *testing.T) {
	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{})

	dims := map[string]any{"source": "tts"}
	sink.Record(billing.ItemTTSCharacters, 3, dims)
	dims["source"] = "mutated"
	sink.Record(billing.ItemTTSCharacters, 3, dims)

	if got := sink.Pending(); got != 2 {
		t.Fatalf("pending = %d, want 2 (dims must be copied, not aliased)", got)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := eventViews(allEvents(t, stub))
	want := []eventView{
		{ItemCode: billing.ItemTTSCharacters, Quantity: 3, TurnIndex: 0, Dimensions: map[string]any{"source": "mutated"}},
		{ItemCode: billing.ItemTTSCharacters, Quantity: 3, TurnIndex: 0, Dimensions: map[string]any{"source": "tts"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

// TestSinkOccurredAtKeepsFirstRecord occurred_at 是量产生的时间，后续累加不重写它。
func TestSinkOccurredAtKeepsFirstRecord(t *testing.T) {
	first := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(first)

	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{Now: clock.Now})

	sink.Record(billing.ItemASRAudioSecond, 5, nil)
	clock.Advance(2 * time.Second)
	sink.Record(billing.ItemASRAudioSecond, 5, nil)

	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	events := allEvents(t, stub)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 merged event", len(events))
	}
	if events[0].Quantity != 10 {
		t.Errorf("quantity = %d, want 10", events[0].Quantity)
	}
	if !events[0].OccurredAt.Equal(first) {
		t.Errorf("occurred_at = %s, want the first record time %s", events[0].OccurredAt, first)
	}
}

// TestSinkFlushChunksByBatchLimit 超过单批上限要分批发，不能整批被 413 打回。
func TestSinkFlushChunksByBatchLimit(t *testing.T) {
	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{})

	const total = billing.MaxUsageEventsPerBatch + 2
	for i := 0; i < total; i++ {
		sink.Record(billing.ItemTTSCharacters, 1, map[string]any{"sentence": fmt.Sprintf("s%d", i)})
	}

	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	requests := stub.usageRequests()
	if len(requests) != 2 {
		t.Fatalf("batches = %d, want 2", len(requests))
	}
	if got := len(requests[0].Events); got != billing.MaxUsageEventsPerBatch {
		t.Errorf("first batch = %d events, want %d", got, billing.MaxUsageEventsPerBatch)
	}
	if got := len(requests[1].Events); got != 2 {
		t.Errorf("second batch = %d events, want 2", got)
	}
	for _, req := range requests {
		if req.SessionID != "sess-1" {
			t.Errorf("session_id = %q, want sess-1", req.SessionID)
		}
	}
	if got := sink.Pending(); got != 0 {
		t.Errorf("pending after flush = %d, want 0", got)
	}
}

// TestSinkFlushRetriesUntilSuccess 5xx 之后退避重试，成功则缓冲清空。
func TestSinkFlushRetriesUntilSuccess(t *testing.T) {
	stub := newStubControlPlane(t)
	stub.onUsage(statusScript(http.StatusInternalServerError, http.StatusServiceUnavailable, http.StatusOK))
	sink := newTestSink(t, stub, SinkConfig{MaxRetries: 3})

	sink.Record(billing.ItemLLMInput, 1234, nil)
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := stub.usageCalls(); got != 3 {
		t.Fatalf("usage calls = %d, want 3", got)
	}
	requests := stub.usageRequests()
	if requests[0].Events[0].EventID != requests[1].Events[0].EventID {
		t.Fatalf("event_id changed across retries: %q vs %q", requests[0].Events[0].EventID, requests[1].Events[0].EventID)
	}
	if got := sink.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

// TestSinkFlushKeepsBufferWhenRetriesExhausted 退避耗尽后事件留在缓冲里，下一轮还能补报。
func TestSinkFlushKeepsBufferWhenRetriesExhausted(t *testing.T) {
	stub := newStubControlPlane(t)
	stub.onUsage(statusScript(http.StatusInternalServerError))
	sink := newTestSink(t, stub, SinkConfig{MaxRetries: 2})

	sink.Record(billing.ItemLLMInput, 1234, nil)

	err := sink.Flush(context.Background())
	if err == nil {
		t.Fatal("flush: want error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("err = %q, want it to carry the status", err)
	}
	if got := stub.usageCalls(); got != 3 {
		t.Fatalf("usage calls = %d, want 1 attempt + 2 retries", got)
	}
	if got := sink.Pending(); got != 1 {
		t.Fatalf("pending = %d, want the event kept in the buffer", got)
	}

	// 控制面恢复后再 flush：同一份量补报上去，用的是新生成的 event_id。
	stub.onUsage(statusScript(http.StatusOK))
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if got := sink.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
	requests := stub.usageRequests()
	if len(requests) != 4 {
		t.Fatalf("usage calls = %d, want 4", len(requests))
	}
	if requests[3].Events[0].Quantity != 1234 {
		t.Errorf("补报 quantity = %d, want 1234", requests[3].Events[0].Quantity)
	}
	if requests[3].Events[0].EventID == requests[0].Events[0].EventID {
		t.Errorf("event_id = %q, want a fresh id on a new flush", requests[3].Events[0].EventID)
	}
}

// TestSinkFlushRejectedIsTerminal 逐条拒绝和 duplicated 都不是重试理由，只有传输错误才是。
func TestSinkFlushRejectedIsTerminal(t *testing.T) {
	tests := []struct {
		name      string
		respond   func(req billing.UsageBatchRequest) string
		wantCalls int
	}{
		{
			name:      "duplicated counts as success",
			respond:   func(billing.UsageBatchRequest) string { return `{"accepted":0,"duplicated":1}` },
			wantCalls: 1,
		},
		{
			name: "rejected entries are terminal",
			respond: func(req billing.UsageBatchRequest) string {
				return fmt.Sprintf(`{"accepted":0,"duplicated":0,"rejected":[{"event_id":%q,"reason":"unknown_session"}]}`, req.Events[0].EventID)
			},
			wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubControlPlane(t)
			stub.onUsage(func(_ int, req billing.UsageBatchRequest) (int, string) {
				return http.StatusOK, tt.respond(req)
			})
			sink := newTestSink(t, stub, SinkConfig{MaxRetries: 3})

			sink.Record(billing.ItemLLMInput, 7, nil)
			if err := sink.Flush(context.Background()); err != nil {
				t.Fatalf("flush: %v", err)
			}
			if got := stub.usageCalls(); got != tt.wantCalls {
				t.Fatalf("usage calls = %d, want %d", got, tt.wantCalls)
			}
			if got := sink.Pending(); got != 0 {
				t.Fatalf("pending = %d, want 0", got)
			}
		})
	}
}

// TestSinkFlushStopsOnContextCancel 退避期间 ctx 取消要立刻返回，不能被会话释放卡住。
func TestSinkFlushStopsOnContextCancel(t *testing.T) {
	stub := newStubControlPlane(t)
	stub.onUsage(statusScript(http.StatusInternalServerError))
	sink := newTestSink(t, stub, SinkConfig{MaxRetries: 5, RetryBase: 200 * time.Millisecond})

	sink.Record(billing.ItemLLMInput, 7, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	err := sink.Flush(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("flush: want error, got nil")
	}
	if elapsed > time.Second {
		t.Fatalf("flush took %s, want it to stop on ctx cancellation", elapsed)
	}
	if got := stub.usageCalls(); got != 1 {
		t.Fatalf("usage calls = %d, want 1 before cancellation", got)
	}
	if got := sink.Pending(); got != 1 {
		t.Fatalf("pending = %d, want the event kept", got)
	}
}

// TestSinkBreakerBoundaries 本地熔断的两个阈值：90% 与 100%，降级会话恒不触发。
func TestSinkBreakerBoundaries(t *testing.T) {
	item := billing.ItemTTSCharacters
	nonBillable := map[string]billing.PriceSnapshot{
		item: {ItemCode: item, Billable: false, Reason: "byok"},
	}

	tests := []struct {
		name          string
		reservedMicro int64
		quantity      int64
		degraded      bool
		snapshot      map[string]billing.PriceSnapshot
		wantEstimate  int64
		wantNear      bool
		wantExhausted bool
	}{
		{
			name: "89 percent", reservedMicro: 1000, quantity: 890,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 890,
		},
		{
			name: "90 percent", reservedMicro: 1000, quantity: 900,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 900, wantNear: true,
		},
		{
			name: "100 percent", reservedMicro: 1000, quantity: 1000,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 1000, wantNear: true, wantExhausted: true,
		},
		{
			name: "over reserved", reservedMicro: 1000, quantity: 5000,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 5000, wantNear: true, wantExhausted: true,
		},
		{
			name: "degraded session never trips", reservedMicro: 1000, quantity: 5000, degraded: true,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 5000,
		},
		{
			name: "no reservation never trips", reservedMicro: 0, quantity: 5000,
			snapshot: priceSnapshot(item, 1, 1), wantEstimate: 5000,
		},
		{
			name: "non billable item is not estimated", reservedMicro: 1000, quantity: 5000,
			snapshot: nonBillable,
		},
		{
			name: "unknown item is not estimated", reservedMicro: 1000, quantity: 5000,
			snapshot: map[string]billing.PriceSnapshot{},
		},
		{
			name: "unit size rounds up", reservedMicro: 10, quantity: 1,
			snapshot: priceSnapshot(item, 300, 1000), wantEstimate: 1,
		},
		{
			name: "missing unit size counts per unit", reservedMicro: 1000, quantity: 100,
			snapshot: map[string]billing.PriceSnapshot{
				item: {ItemCode: item, Billable: true, UnitPriceMicro: 10},
			},
			wantEstimate: 1000, wantNear: true, wantExhausted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubControlPlane(t)
			sink := newTestSink(t, stub, SinkConfig{
				ReservedMicro: tt.reservedMicro,
				Snapshot:      tt.snapshot,
				Degraded:      tt.degraded,
			})

			sink.Record(item, tt.quantity, nil)

			budget := sink.Budget()
			if budget.EstimatedMicro != tt.wantEstimate {
				t.Errorf("estimated = %d, want %d", budget.EstimatedMicro, tt.wantEstimate)
			}
			if budget.ReservedMicro != tt.reservedMicro {
				t.Errorf("reserved = %d, want %d", budget.ReservedMicro, tt.reservedMicro)
			}
			if budget.Degraded != tt.degraded {
				t.Errorf("degraded = %v, want %v", budget.Degraded, tt.degraded)
			}
			if got := sink.NearLimit(); got != tt.wantNear {
				t.Errorf("near limit = %v, want %v", got, tt.wantNear)
			}
			if got := sink.Exhausted(); got != tt.wantExhausted {
				t.Errorf("exhausted = %v, want %v", got, tt.wantExhausted)
			}
		})
	}
}

// TestSinkBreakerSurvivesFlush 估算盯的是整个会话的累计量：上报成功后不能归零。
func TestSinkBreakerSurvivesFlush(t *testing.T) {
	stub := newStubControlPlane(t)
	item := billing.ItemTTSCharacters
	sink := newTestSink(t, stub, SinkConfig{ReservedMicro: 1000, Snapshot: priceSnapshot(item, 1, 1)})

	sink.Record(item, 500, nil)
	if sink.Exhausted() {
		t.Fatal("exhausted too early")
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := sink.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
	if got := sink.Budget().EstimatedMicro; got != 500 {
		t.Fatalf("estimated after flush = %d, want 500 (session cumulative)", got)
	}

	sink.Record(item, 500, nil)
	if !sink.Exhausted() {
		t.Fatal("exhausted = false, want true for a session that spent its reservation")
	}
	if got := sink.Budget().EstimatedMicro; got != 1000 {
		t.Fatalf("estimated = %d, want 1000", got)
	}
}

// TestSinkSettleFlushesFirst settle 之前必须先 flush；flush 失败也只是少收钱，不能挡住 settle。
func TestSinkSettleFlushesFirst(t *testing.T) {
	tests := []struct {
		name         string
		usageBody    func(call int, req billing.UsageBatchRequest) (int, string)
		wantUsage    int
		wantSequence []string
	}{
		{
			name:         "flush succeeds",
			usageBody:    statusScript(http.StatusOK),
			wantUsage:    1,
			wantSequence: []string{billing.PathUsageEvents, billing.PathSettle},
		},
		{
			name:         "flush fails",
			usageBody:    statusScript(http.StatusInternalServerError),
			wantUsage:    2, // 1 次尝试 + 1 次重试
			wantSequence: []string{billing.PathUsageEvents, billing.PathUsageEvents, billing.PathSettle},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubControlPlane(t)
			stub.onUsage(tt.usageBody)
			sink := newTestSink(t, stub, SinkConfig{})

			sink.Record(billing.ItemLLMInput, 42, nil)
			resp, err := sink.Settle(context.Background(), "client_close")
			if err != nil {
				t.Fatalf("settle: %v", err)
			}
			if !resp.Settled || resp.ChargedMicro != 12345 {
				t.Fatalf("settle response = %+v, want the control plane answer", resp)
			}
			if got := stub.usageCalls(); got != tt.wantUsage {
				t.Errorf("usage calls = %d, want %d", got, tt.wantUsage)
			}

			settles := stub.settleRequests()
			if len(settles) != 1 {
				t.Fatalf("settle calls = %d, want 1", len(settles))
			}
			if settles[0].SessionID != "sess-1" || settles[0].Reason != "client_close" {
				t.Errorf("settle request = %+v, want session sess-1 reason client_close", settles[0])
			}
			if settles[0].EndedAt.IsZero() {
				t.Errorf("ended_at is zero, want the settle time")
			}

			sequence := stub.requestSequence()
			if !reflect.DeepEqual(sequence, tt.wantSequence) {
				t.Fatalf("request order = %v, want %v", sequence, tt.wantSequence)
			}
		})
	}
}

// TestSinkTickerFlushesAndCloseStopsIt 兜底 ticker 会自己 flush，Close 之后安静下来。
func TestSinkTickerFlushesAndCloseStopsIt(t *testing.T) {
	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{FlushInterval: 10 * time.Millisecond})

	sink.Start(context.Background())
	sink.Record(billing.ItemLLMInput, 11, nil)
	stub.waitForUsageCalls(t, 1)
	// 上报请求回来之后缓冲才会被清空，所以这里等的是缓冲的状态。
	waitFor(t, "the background flush to drain the buffer", func() bool { return sink.Pending() == 0 })

	sink.Close()

	settled := stub.usageCalls()
	time.Sleep(50 * time.Millisecond)
	if got := stub.usageCalls(); got != settled {
		t.Fatalf("usage calls grew from %d to %d after Close", settled, got)
	}
}

// TestSinkBufferFullTriggersFlush 缓冲条目到上限要触发一次 flush，不用等 ticker。
func TestSinkBufferFullTriggersFlush(t *testing.T) {
	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{
		FlushInterval:    time.Hour, // 只有“满了”这条路能触发
		MaxBufferEntries: 2,
	})

	sink.Start(context.Background())
	for i := 0; i < 3; i++ {
		sink.Record(billing.ItemTTSCharacters, 1, map[string]any{"sentence": fmt.Sprintf("s%d", i)})
	}

	stub.waitForUsageCalls(t, 1)
	if got := stub.usageCalls(); got != 1 {
		t.Fatalf("usage calls = %d, want 1 (the buffer-full signal is deduped)", got)
	}
}

// TestSinkRecordDoesNotBlockWhileFlushInFlight Record 跑在热路径上，flush 正在等网络也不行。
func TestSinkRecordDoesNotBlockWhileFlushInFlight(t *testing.T) {
	stub := newStubControlPlane(t)
	sink := newTestSink(t, stub, SinkConfig{})

	sink.Record(billing.ItemLLMInput, 5, nil)
	stub.holdUsage()

	flushed := make(chan error, 1)
	go func() { flushed <- sink.Flush(context.Background()) }()
	stub.waitUsageEntered(t)

	recorded := make(chan struct{})
	go func() {
		sink.Record(billing.ItemTTSCharacters, 3, map[string]any{"source": "tts"})
		close(recorded)
	}()
	select {
	case <-recorded:
	case <-time.After(3 * time.Second):
		t.Fatal("Record blocked while a flush was in flight")
	}

	stub.release()
	if err := <-flushed; err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := sink.Pending(); got != 1 {
		t.Fatalf("pending = %d, want the usage recorded during the flush to survive", got)
	}
	if got := sink.Budget().EstimatedMicro; got != 0 {
		t.Fatalf("estimated = %d, want 0 without a price snapshot", got)
	}
}

// TestSinkWithoutClientIsSafe 配置漏了 client 时只报错，不 panic，也不挡住 settle。
func TestSinkWithoutClientIsSafe(t *testing.T) {
	sink := NewSink(SinkConfig{SessionID: "sess-1", DeviceID: "dev-1"})
	t.Cleanup(sink.Close)

	sink.Record(billing.ItemLLMInput, 5, nil)
	if got := sink.Pending(); got != 1 {
		t.Fatalf("pending = %d, want 1", got)
	}
	if err := sink.Flush(context.Background()); !errors.Is(err, errNoClient) {
		t.Fatalf("flush err = %v, want errNoClient", err)
	}
	if _, err := sink.Settle(context.Background(), "client_close"); !errors.Is(err, errNoClient) {
		t.Fatalf("settle err = %v, want errNoClient", err)
	}
}
