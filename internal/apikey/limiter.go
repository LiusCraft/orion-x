package apikey

import (
	"sync"
	"time"
)

// LimitDecision 是一次限流判定的全部事实：放不放行、配额多少、还剩多少、还要等多久
// （Retry-After 与 X-RateLimit-Reset 都从它算）。
type LimitDecision struct {
	Allowed   bool
	Limit     int
	Remaining int
	// RetryAfter 是拒绝时建议的退避时长（未被拒绝时为零值）。
	RetryAfter time.Duration
}

// Limiter 是每 key 限流的抽象。现在是进程内令牌桶（§1 D8）；多副本到来时换成
// 网关层或 Redis 实现，语义与响应头不变（§6 的多副本演进路径）。接口留在这里，
// 就是为了让那次替换只动构造函数一处。
type Limiter interface {
	Allow(keyID string) LimitDecision
	// Forget 丢掉某个 key 的桶（撤销时调用：撤销后的桶留着没有任何意义）。
	Forget(keyID string)
}

// NewTokenBucketLimiter 构造进程内令牌桶：每 key 一个桶，按 rps 匀速回填，
// 容量为 burst。rps <= 0 或 burst <= 0 表示不限流（开关关掉配额）。
//
// 桶是软保护：进程重启即清零，这只影响"重启瞬间多放过去几个请求"，
// 不影响正确性，也不需要持久化。
func NewTokenBucketLimiter(rps float64, burst int, now func() time.Time) Limiter {
	if now == nil {
		now = time.Now
	}
	return &tokenBucketLimiter{
		rps:     rps,
		burst:   burst,
		now:     now,
		buckets: make(map[string]*bucket),
	}
}

type bucket struct {
	tokens float64
	last   time.Time
}

type tokenBucketLimiter struct {
	rps   float64
	burst int
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

// sweepThreshold 是触发惰性清理的桶数量门槛：key 用得久、数量多起来之后，
// 已撤销或早已停用的 key 的桶不该永远占着内存。
const (
	sweepThreshold = 4096
	sweepIdleAfter = 10 * time.Minute
)

func (l *tokenBucketLimiter) Allow(keyID string) LimitDecision {
	if l.rps <= 0 || l.burst <= 0 {
		return LimitDecision{Allowed: true}
	}

	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) >= sweepThreshold {
		l.sweepLocked(now)
	}

	b, ok := l.buckets[keyID]
	if !ok {
		// 新桶从满的开始：第一次调用不该因为"桶是空的"被拒。
		b = &bucket{tokens: float64(l.burst), last: now}
		l.buckets[keyID] = b
	}

	if elapsed := now.Sub(b.last); elapsed > 0 {
		b.tokens = min(float64(l.burst), b.tokens+elapsed.Seconds()*l.rps)
		b.last = now
	}

	decision := LimitDecision{Limit: l.burst}
	if b.tokens >= 1 {
		b.tokens--
		decision.Allowed = true
		decision.Remaining = int(b.tokens)
		return decision
	}

	decision.Remaining = 0
	// 攒出下一个令牌还要多久。退避时长按秒向上取整由上层做，
	// 这里给的是精确时长，测试可以直接断言它。
	decision.RetryAfter = time.Duration((1 - b.tokens) / l.rps * float64(time.Second))
	return decision
}

func (l *tokenBucketLimiter) Forget(keyID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, keyID)
}

// sweepLocked 丢掉"闲置够久、按回填速率早该回满"的桶。删掉它等价于新建一个满桶：
// 下一次判定拿到的令牌数与留着这个桶再回填的结果完全相同，所以清理不影响任何判定。
func (l *tokenBucketLimiter) sweepLocked(now time.Time) {
	for id, b := range l.buckets {
		idle := now.Sub(b.last)
		if idle <= sweepIdleAfter {
			continue
		}
		if b.tokens+idle.Seconds()*l.rps >= float64(l.burst) {
			delete(l.buckets, id)
		}
	}
}
