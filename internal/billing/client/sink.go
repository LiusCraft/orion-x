package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
)

const (
	defaultFlushInterval    = 30 * time.Second
	defaultMaxBufferEntries = 64
	defaultMaxRetries       = 5
	defaultRetryBase        = 100 * time.Millisecond
	defaultRetryMax         = 5 * time.Second
)

// errNoClient 表示 Sink 没有可用的 HTTP 客户端（配置错误，不是运行期故障）。
var errNoClient = errors.New("billing/client: sink has no http client")

// 编译期确认 Sink 实现了数据面的 port。
var _ billing.Sink = (*Sink)(nil)

// SinkConfig 是一次设备会话的计费参数。ReservedMicro / MaxSessionSeconds /
// Snapshot / Degraded 都来自 authorize 的响应；authorize 本身失败放行的会话
// Degraded=true，只上报不熔断（§14.3）。
type SinkConfig struct {
	Client            *Client
	SessionID         string
	DeviceID          string
	ReservedMicro     int64
	MaxSessionSeconds int
	// Snapshot 是 authorize 下发的单价快照，本地估算的唯一价格来源（§15.4）。
	Snapshot map[string]billing.PriceSnapshot
	// Degraded 为真时只有一个上报义务：没有预冻结也没有熔断参数。
	Degraded bool
	// Now 用于测试注入，默认 time.Now。
	Now func() time.Time
	// FlushInterval 是兜底 flush 周期，<=0 时取 30s（§15.3）。
	FlushInterval time.Duration
	// MaxBufferEntries 是缓冲条目上限，到了就触发一次 flush，<=0 时取 64。
	MaxBufferEntries int
	// MaxRetries 是单批上报的重试次数（不含首次），<=0 时取 5。
	MaxRetries int
	// RetryBase 是首次退避间隔，<=0 时取 100ms。
	RetryBase time.Duration
	// RetryMax 是退避上限，<=0 时取 5s。
	RetryMax time.Duration
}

// Budget 是本地估算状态。估算值一律向上取整，只用于熔断，不产生账单。
type Budget struct {
	EstimatedMicro int64
	ReservedMicro  int64
	Degraded       bool
}

// bufferKey 是缓冲的索引：同一个计费项、同一个 turn、同一份维度才合并。
type bufferKey struct {
	ItemCode  string
	TurnIndex int64
	dimsKey   string
}

// bufferEntry 是缓冲里累加出来的一条待上报事件。OccurredAt 取第一次累加的时间，
// 之后的累加不改写它——它代表“这个量是什么时候产生的”。
type bufferEntry struct {
	Quantity   int64
	OccurredAt time.Time
	Dimensions map[string]any
}

// snapshotEntry 是一次 flush 的只读快照。
type snapshotEntry struct {
	key        bufferKey
	quantity   int64
	occurredAt time.Time
	dimensions map[string]any
}

// Sink 是一个设备会话的计费 Sink：内存缓冲、批量上报、本地熔断（§15）。
//
// 两把锁的分工是刻意的：mu 只保护内存状态，临界区里不做任何 I/O，所以 Record 跑
// 在音频与 LLM 的热路径上也不会被网络拖住；flushMu 把 Flush 串行化并跨 I/O 持有，
// 与 mu 的嵌套顺序永远是 flushMu → mu，不会成环。
type Sink struct {
	client            *Client
	sessionID         string
	deviceID          string
	snapshot          map[string]billing.PriceSnapshot
	reservedMicro     int64
	maxSessionSeconds int
	degraded          bool

	now              func() time.Time
	flushInterval    time.Duration
	maxBufferEntries int
	maxRetries       int
	retryBase        time.Duration
	retryMax         time.Duration

	mu     sync.Mutex
	buffer map[bufferKey]*bufferEntry
	// quantities 是会话累计量：只增不减，估算的唯一依据。flush 成功也不减——预冻结
	// 保的是整个会话，熔断要盯的也是整个会话。
	quantities map[string]int64
	turnIndex  int64
	startedAt  time.Time
	closed     bool
	loopCancel context.CancelFunc

	flushMu sync.Mutex
	// wake 是缓冲到上限时的非阻塞提示（容量 1，只提示一次就够）。
	wake chan struct{}
	// startOnce 让 Start 幂等；wg 让 Close 等 goroutine 退出。
	startOnce sync.Once
	wg        sync.WaitGroup
}

// NewSink 构造一个会话级 Sink。cfg.Client 为 nil 时只告警：上报会失败，但不会 panic，
// 计费是旁路，不能把连接拖下水。
func NewSink(cfg SinkConfig) *Sink {
	if cfg.Client == nil {
		logging.Warnf("billing/client: sink created without http client, usage will not be reported session_id=%s", cfg.SessionID)
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	flushInterval := cfg.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	maxBufferEntries := cfg.MaxBufferEntries
	if maxBufferEntries <= 0 {
		maxBufferEntries = defaultMaxBufferEntries
	}
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}
	retryBase := cfg.RetryBase
	if retryBase <= 0 {
		retryBase = defaultRetryBase
	}
	retryMax := cfg.RetryMax
	if retryMax <= 0 {
		retryMax = defaultRetryMax
	}
	if retryMax < retryBase {
		retryMax = retryBase
	}
	return &Sink{
		client:            cfg.Client,
		sessionID:         cfg.SessionID,
		deviceID:          cfg.DeviceID,
		snapshot:          cfg.Snapshot,
		reservedMicro:     cfg.ReservedMicro,
		maxSessionSeconds: cfg.MaxSessionSeconds,
		degraded:          cfg.Degraded,
		now:               now,
		flushInterval:     flushInterval,
		maxBufferEntries:  maxBufferEntries,
		maxRetries:        maxRetries,
		retryBase:         retryBase,
		retryMax:          retryMax,
		buffer:            make(map[bufferKey]*bufferEntry),
		quantities:        make(map[string]int64),
		wake:              make(chan struct{}, 1),
	}
}

// Record 实现 billing.Sink。非阻塞：只往内存缓冲里按 (item_code, turn_index, 维度)
// 累加，不碰网络、不写阻塞 channel、不打日志（§15.3 最后一条纪律）。quantity <= 0
// 直接忽略，dimensions 会被拷贝。
func (s *Sink) Record(itemCode string, quantity int64, dims map[string]any) {
	if itemCode == "" || quantity <= 0 {
		return
	}
	key := bufferKey{ItemCode: itemCode, dimsKey: dimensionsKey(dims)}
	recordedAt := s.now()

	s.mu.Lock()
	key.TurnIndex = s.turnIndex
	entry, ok := s.buffer[key]
	if ok {
		entry.Quantity += quantity
	} else {
		s.buffer[key] = &bufferEntry{
			Quantity:   quantity,
			OccurredAt: recordedAt,
			Dimensions: cloneDims(dims),
		}
	}
	s.quantities[itemCode] += quantity
	overflow := len(s.buffer) >= s.maxBufferEntries
	s.mu.Unlock()

	if overflow {
		s.notifyFlush()
	}
}

// Flush 实现 billing.Sink。把缓冲里的量按 billing.MaxUsageEventsPerBatch 分批上报，
// 失败按指数退避重试（RetryBase 起、RetryMax 封顶、MaxRetries 次）；最终失败时事件
// 留在缓冲里并返回错误——宁可少收这笔钱，也不能丢事实（§15.3）。
//
// 实现上不去动缓冲里的条目：只有上报成功的量才从缓冲里减掉，所以失败路径天然是
// “原样留着”，也不会有把飞行期新增用量一起抹掉的窗口。
func (s *Sink) Flush(ctx context.Context) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	pending := s.snapshotPending()
	if len(pending) == 0 {
		return nil
	}
	if s.client == nil {
		return errNoClient
	}

	for start := 0; start < len(pending); start += billing.MaxUsageEventsPerBatch {
		end := min(start+billing.MaxUsageEventsPerBatch, len(pending))
		batch := pending[start:end]

		resp, err := s.reportBatch(ctx, batch)
		if err != nil {
			// 传输层/控制面已经不可用，剩下的批次留着下一轮再试，不必各自再等一遍退避。
			return err
		}
		s.commit(batch)
		if len(resp.Rejected) > 0 {
			logging.Warnf("billing: usage batch partly rejected %s rejected=%d reasons=%s",
				s.ident(), len(resp.Rejected), summarizeRejections(resp.Rejected))
		}
	}
	return nil
}

// reportBatch 上报一批事件，带指数退避重试。
func (s *Sink) reportBatch(ctx context.Context, batch []snapshotEntry) (billing.UsageBatchResponse, error) {
	// event_id 在 flush 时生成：一次 flush 的所有重试用同一份请求体，撞上就是
	// duplicated（正常现象），但重试不会换一批新 ID 把同一份量报两次（§14.4）。
	req := billing.UsageBatchRequest{
		SessionID: s.sessionID,
		// device_id 只在控制面查不到 reservation 时用得上：它用来补一条 degraded
		// 行，让 authorize 超时放行的会话也能落账（§14.2）。
		DeviceID: s.deviceID,
		Events:   make([]billing.UsageEventReport, 0, len(batch)),
	}
	for _, entry := range batch {
		req.Events = append(req.Events, billing.UsageEventReport{
			EventID:    uuid.NewString(),
			ItemCode:   entry.key.ItemCode,
			Quantity:   entry.quantity,
			TurnIndex:  entry.key.TurnIndex,
			OccurredAt: entry.occurredAt,
			Dimensions: entry.dimensions,
		})
	}

	delay := s.retryBase
	var lastErr error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, delay); err != nil {
				return billing.UsageBatchResponse{}, err
			}
			delay = min(delay*2, s.retryMax)
		}
		resp, err := s.client.ReportUsage(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			// 会话已经结束、连接已经断开，别在这儿把退避耗完。
			return billing.UsageBatchResponse{}, lastErr
		}
	}
	logging.Warnf("billing: usage report gave up after %d attempts %s events=%d: %v",
		s.maxRetries+1, s.ident(), len(batch), lastErr)
	return billing.UsageBatchResponse{}, lastErr
}

// commit 把已经成功上报的量从缓冲里减掉。只减这一批报出去的数、不整条删除，避免把
// 上报期间新累加进来的量一起抹掉。
func (s *Sink) commit(batch []snapshotEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range batch {
		current, ok := s.buffer[entry.key]
		if !ok {
			continue
		}
		current.Quantity -= entry.quantity
		if current.Quantity <= 0 {
			delete(s.buffer, entry.key)
		}
	}
}

// snapshotPending 复制一份待上报的条目，顺序稳定，方便测试与排查。
func (s *Sink) snapshotPending() []snapshotEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buffer) == 0 {
		return nil
	}
	out := make([]snapshotEntry, 0, len(s.buffer))
	for key, entry := range s.buffer {
		out = append(out, snapshotEntry{
			key:        key,
			quantity:   entry.Quantity,
			occurredAt: entry.OccurredAt,
			dimensions: entry.Dimensions,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key.less(out[j].key) })
	return out
}

// Start 启动兜底 flush 的 ticker（每 FlushInterval 一次），并记下会话的时间基准。
// 重复调用是幂等的，Close 之后调用不再起 goroutine。
func (s *Sink) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		loopCtx, cancel := context.WithCancel(ctx)

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			cancel()
			return
		}
		s.startedAt = s.now()
		s.loopCancel = cancel
		s.wg.Add(1)
		s.mu.Unlock()

		go func() {
			defer s.wg.Done()
			s.loop(loopCtx)
		}()
	})
}

// SetTurn 设置后续 Record 归属的 turn 序号。turn 边界由数据面的接线层通知。
func (s *Sink) SetTurn(index int64) {
	s.mu.Lock()
	s.turnIndex = index
	s.mu.Unlock()
}

// Settle 先 Flush 再结算：顺序反了的话，这个会话的用量会在结算之后才到，用户挂断
// 发现余额没变、过一会儿又变了（§14.5）。flush 最终失败只记日志与指标，照样调
// settle——宁可少收这笔钱，也不能把连接释放卡住（§15.3）。
func (s *Sink) Settle(ctx context.Context, reason string) (billing.SettleResponse, error) {
	if err := s.Flush(ctx); err != nil {
		logging.Errorf("billing_flush_failed_total %s reason=%s pending=%d estimated_micro=%d reserved_micro=%d elapsed=%s max_session_seconds=%d: %v",
			s.ident(), reason, s.Pending(), s.Budget().EstimatedMicro, s.reservedMicro, s.elapsed(), s.maxSessionSeconds, err)
	}
	if s.client == nil {
		return billing.SettleResponse{}, errNoClient
	}
	return s.client.Settle(ctx, billing.SettleRequest{
		SessionID: s.sessionID,
		Reason:    reason,
		EndedAt:   s.now(),
	})
}

// Close 停止 ticker 并等它退出。没有 Start 过也可以安全调用。
func (s *Sink) Close() {
	s.mu.Lock()
	s.closed = true
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

// Budget 返回本地估算状态。估算用 billing.Estimate 按会话累计量重算、向上取整；
// 重算而不是增量累加，所以 flush 失败之后再问它，答案还是对的。
func (s *Sink) Budget() Budget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Budget{
		EstimatedMicro: s.estimateLocked(),
		ReservedMicro:  s.reservedMicro,
		Degraded:       s.degraded,
	}
}

// NearLimit 在估算到 reserved 的 90% 时为真：调用方该停止接受新的 listen 窗口，
// 但让当前这个 turn 走完（§15.4）。
func (s *Sink) NearLimit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded || s.reservedMicro <= 0 {
		return false
	}
	// reserved - reserved/10 == ceil(0.9 × reserved)，写成减法是为了不溢出。
	return s.estimateLocked() >= s.reservedMicro-s.reservedMicro/10
}

// Exhausted 在估算到 reserved 的 100% 时为真：调用方该中断会话，然后
// flush → settle → close。
func (s *Sink) Exhausted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.degraded || s.reservedMicro <= 0 {
		return false
	}
	return s.estimateLocked() >= s.reservedMicro
}

// Pending 返回当前缓冲里的事件条数，供测试与日志用。
func (s *Sink) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.buffer)
}

// loop 是兜底 flush 的 ticker：定时兜一次底，顺手处理“缓冲满了”的提示。
func (s *Sink) loop(ctx context.Context) {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flushQuiet(ctx, "interval")
		case <-s.wake:
			s.flushQuiet(ctx, "buffer_full")
		}
	}
}

// flushQuiet 是后台 flush：失败只告警，不往上抛——这里没有能处理错误的人。
func (s *Sink) flushQuiet(ctx context.Context, trigger string) {
	if err := s.Flush(ctx); err != nil {
		if ctx.Err() != nil {
			return // 连接已经断了，剩下的交给会话边界的 Settle 补报
		}
		logging.Warnf("billing: background flush failed %s trigger=%s: %v", s.ident(), trigger, err)
	}
}

// notifyFlush 非阻塞地提示后台 goroutine 该 flush 了。Start 没调过时它什么也不做，
// 只是占掉那唯一的槽位。
func (s *Sink) notifyFlush() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// estimateLocked 用会话累计量与 authorize 快照重算估算金额。
func (s *Sink) estimateLocked() int64 {
	return billing.Estimate(s.quantities, s.snapshot)
}

// elapsed 返回会话从 Start 到现在的时长；Start 没调过时返回 0。
func (s *Sink) elapsed() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startedAt.IsZero() {
		return 0
	}
	return s.now().Sub(s.startedAt)
}

// ident 是日志里带的会话标识，只在告警路径上用。
func (s *Sink) ident() string {
	return fmt.Sprintf("session_id=%s device_id=%s", s.sessionID, s.deviceID)
}

// less 给 flush 提供一个稳定顺序：turn → item_code → 维度。
func (k bufferKey) less(other bufferKey) bool {
	if k.TurnIndex != other.TurnIndex {
		return k.TurnIndex < other.TurnIndex
	}
	if k.ItemCode != other.ItemCode {
		return k.ItemCode < other.ItemCode
	}
	return k.dimsKey < other.dimsKey
}

// dimensionsKey 把维度压成一个稳定的字符串。json 编码对 map 的键排序，所以同一个
// 维度集合每次都得到同一个 key。
func dimensionsKey(dims map[string]any) string {
	if len(dims) == 0 {
		return ""
	}
	buf, err := json.Marshal(dims)
	if err != nil {
		// 维度里塞了不可 JSON 编码的值：退回 fmt，至少还有一份尽量稳定的输出。
		return fmt.Sprintf("%v", dims)
	}
	return string(buf)
}

// cloneDims 拷贝维度：调用方之后改自己的 map 不能影响缓冲里的事实。
func cloneDims(dims map[string]any) map[string]any {
	if len(dims) == 0 {
		return nil
	}
	out := make(map[string]any, len(dims))
	for k, v := range dims {
		out[k] = v
	}
	return out
}

// summarizeRejections 把逐条拒绝的原因归并成一行日志，避免一批 500 条拒绝刷屏。
func summarizeRejections(rejected []billing.RejectedEvent) string {
	counts := make(map[string]int, len(rejected))
	reasons := make([]string, 0, len(rejected))
	for _, item := range rejected {
		reason := item.Reason
		if reason == "" {
			reason = "unspecified"
		}
		if _, ok := counts[reason]; !ok {
			reasons = append(reasons, reason)
		}
		counts[reason]++
	}
	sort.Strings(reasons)
	var b strings.Builder
	for i, reason := range reasons {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%d", reason, counts[reason])
	}
	return b.String()
}

// sleepContext 是可被取消的退避等待：ctx 结束就立刻返回，不把会话释放卡住。
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
