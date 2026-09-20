package client

import (
	"context"
	"sync"
	"time"

	"github.com/liuscraft/orion-x/internal/billing"
	"github.com/liuscraft/orion-x/internal/logging"
)

// 这个文件是通道层真正要用的那层薄封装：把 authorize → Sink → flush → settle 串成
// 一个会话句柄，并把 §14.3 的 fail open 语义收在一处。
//
// 通道层只知道“这个会话能不能开、记了多少、什么时候交账”，不需要自己拼 SinkConfig。

// defaultSuspensionTTL 是“已被停服”标记的本地 TTL。它不是一个熔断，只是用已知的
// 最后状态：控制面不可达时，上一次明确拒绝过 account_suspended 的设备仍然拒。
const defaultSuspensionTTL = 5 * time.Minute

// SuspensionCache 记住最近被停服的设备（进程级，按 device_id）。
type SuspensionCache struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[string]time.Time
}

// NewSuspensionCache 创建停服标记缓存，ttl <= 0 时取 5 分钟。
func NewSuspensionCache(ttl time.Duration) *SuspensionCache {
	if ttl <= 0 {
		ttl = defaultSuspensionTTL
	}
	return &SuspensionCache{ttl: ttl, now: time.Now, entries: make(map[string]time.Time)}
}

// Mark 记下某个设备已被停服。
func (c *SuspensionCache) Mark(deviceID string) {
	if c == nil || deviceID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[deviceID] = c.now().Add(c.ttl)
}

// Suspended 判断设备是否还在停服 TTL 内。
func (c *SuspensionCache) Suspended(deviceID string) bool {
	if c == nil || deviceID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	expiresAt, ok := c.entries[deviceID]
	if !ok {
		return false
	}
	if c.now().After(expiresAt) {
		delete(c.entries, deviceID)
		return false
	}
	return true
}

// SessionConfig 是一次设备会话的计费参数。
type SessionConfig struct {
	Client      *Client
	DeviceID    string
	SessionID   string
	Channel     string
	Suspensions *SuspensionCache
	Now         func() time.Time
	// FlushInterval / MaxBufferEntries / MaxRetries 透传给 Sink，<=0 时取默认值。
	FlushInterval    time.Duration
	MaxBufferEntries int
	MaxRetries       int
}

// Session 是一个设备会话的计费句柄。
type Session struct {
	client      *Client
	suspensions *SuspensionCache
	deviceID    string
	sessionID   string
	channel     string
	now         func() time.Time
	flushEvery  time.Duration
	maxEntries  int
	maxRetries  int

	mu     sync.Mutex
	sink   *Sink
	resp   billing.AuthorizeResponse
	closed bool
}

// NewSession 创建一个会话句柄。Authorize 之前 Record / Flush 都是 no-op。
func NewSession(cfg SessionConfig) *Session {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Session{
		client:      cfg.Client,
		suspensions: cfg.Suspensions,
		deviceID:    cfg.DeviceID,
		sessionID:   cfg.SessionID,
		channel:     cfg.Channel,
		now:         now,
		flushEvery:  cfg.FlushInterval,
		maxEntries:  cfg.MaxBufferEntries,
		maxRetries:  cfg.MaxRetries,
	}
}

// Authorize 做准入校验并准备好本地缓冲。
//
// 三种结果：
//   - 允许：返回 resp（Allowed=true），后续 Record / Flush 生效；
//   - 控制面明确拒绝：返回 resp（Allowed=false）且 err == nil——这是业务判断不是
//     传输错误，调用方据此拒绝连接；
//   - 请求失败或超时：按 §14.3 fail open，返回 Allowed=true 的降级响应
//     （Degraded=true：没有预冻结也没有熔断参数，只上报不熔断）。唯一例外是本地
//     记得这个设备已被停服。
func (s *Session) Authorize(ctx context.Context) (billing.AuthorizeResponse, error) {
	if s.client == nil {
		// 计费关闭或配置缺失：本地不放行也不拒绝，交给调用方按“无计费”处理。
		return billing.AuthorizeResponse{Allowed: true}, nil
	}

	resp, err := s.client.Authorize(ctx, billing.AuthorizeRequest{
		DeviceID:  s.deviceID,
		SessionID: s.sessionID,
		Channel:   s.channel,
	})
	if err != nil {
		if s.suspensions.Suspended(s.deviceID) {
			logging.Warnf("billing: device %s is locally marked suspended, rejecting session %s while control plane is unreachable: %v",
				s.deviceID, s.sessionID, err)
			return billing.AuthorizeResponse{Allowed: false, RejectReason: billing.RejectAccountSuspended}, nil
		}
		logging.Warnf("billing: authorize failed for session %s, continuing degraded (fail open): %v", s.sessionID, err)
		resp = billing.AuthorizeResponse{Allowed: true}
	}

	if !resp.Allowed && resp.RejectReason == billing.RejectAccountSuspended {
		s.suspensions.Mark(s.deviceID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.resp = resp
	if resp.Allowed {
		s.sink = NewSink(SinkConfig{
			Client:            s.client,
			SessionID:         s.sessionID,
			DeviceID:          s.deviceID,
			ReservedMicro:     resp.ReservedMicro,
			MaxSessionSeconds: resp.MaxSessionSeconds,
			Snapshot:          resp.SnapshotMap(),
			Degraded:          resp.ReservationID == "", // 没有 reservation = 降级放行
			Now:               s.now,
			FlushInterval:     s.flushEvery,
			MaxBufferEntries:  s.maxEntries,
			MaxRetries:        s.maxRetries,
		})
	}
	return resp, nil
}

// Start 启动本地缓冲的兜底 flush（每 30 秒一次，防止超长会话一直在内存里堆）。
func (s *Session) Start(ctx context.Context) {
	if sink := s.sinkLocked(); sink != nil {
		sink.Start(ctx)
	}
}

// Authorized 返回这次会话是否拿到了控制面的许可。
func (s *Session) Authorized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resp.Allowed
}

// Response 返回 authorize 的原始结果。
func (s *Session) Response() billing.AuthorizeResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resp
}

// Record 累加一笔用量。未授权或计费关闭时是 no-op。
func (s *Session) Record(itemCode string, quantity int64, dims map[string]any) {
	if sink := s.sinkLocked(); sink != nil {
		sink.Record(itemCode, quantity, dims)
	}
}

// Flush 在 turn / 会话边界上报。
func (s *Session) Flush(ctx context.Context) error {
	sink := s.sinkLocked()
	if sink == nil {
		return nil
	}
	return sink.Flush(ctx)
}

// SetTurn 设置后续 Record 归属的 turn 序号。
func (s *Session) SetTurn(index int64) {
	if sink := s.sinkLocked(); sink != nil {
		sink.SetTurn(index)
	}
}

// NearLimit 估算到了预冻结的 90%（不再接受新的 listen 窗口）。
func (s *Session) NearLimit() bool {
	if sink := s.sinkLocked(); sink != nil {
		return sink.NearLimit()
	}
	return false
}

// Exhausted 估算到了预冻结的 100%（中断会话）。
func (s *Session) Exhausted() bool {
	if sink := s.sinkLocked(); sink != nil {
		return sink.Exhausted()
	}
	return false
}

// Budget 返回本地估算状态，供日志与排查。
func (s *Session) Budget() Budget {
	if sink := s.sinkLocked(); sink != nil {
		return sink.Budget()
	}
	return Budget{}
}

// Settle 交账：先 flush（失败只记日志，不阻断），再调控制面的 settle。
func (s *Session) Settle(ctx context.Context, reason string) (billing.SettleResponse, error) {
	s.mu.Lock()
	sink := s.sink
	s.sink = nil
	authorized := s.resp.Allowed
	s.mu.Unlock()

	if sink == nil || !authorized {
		return billing.SettleResponse{Settled: false}, nil
	}
	defer sink.Close()
	return sink.Settle(ctx, reason)
}

// Close 释放本地资源（ticker）。不交账——交账请显式调 Settle。
func (s *Session) Close() {
	s.mu.Lock()
	sink := s.sink
	s.sink = nil
	s.closed = true
	s.mu.Unlock()
	if sink != nil {
		sink.Close()
	}
}

func (s *Session) sinkLocked() *Sink {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.sink
}
