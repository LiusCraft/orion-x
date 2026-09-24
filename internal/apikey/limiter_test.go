package apikey

import (
	"strconv"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestTokenBucketBurstAndRefill(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(1, 3, clock.now)

	for i := 0; i < 3; i++ {
		d := limiter.Allow("key-1")
		if !d.Allowed {
			t.Fatalf("call %d rejected, want allowed (burst = 3)", i+1)
		}
		if want := 3 - i - 1; d.Remaining != want {
			t.Fatalf("call %d Remaining = %d, want %d", i+1, d.Remaining, want)
		}
		if d.Limit != 3 {
			t.Fatalf("Limit = %d, want 3", d.Limit)
		}
	}

	denied := limiter.Allow("key-1")
	if denied.Allowed {
		t.Fatal("4th call allowed, want rejected")
	}
	if denied.Remaining != 0 {
		t.Fatalf("denied Remaining = %d, want 0", denied.Remaining)
	}
	if denied.RetryAfter <= 0 || denied.RetryAfter > time.Second {
		t.Fatalf("RetryAfter = %v, want (0, 1s]", denied.RetryAfter)
	}

	// 桶没回满就不放行：半秒只够半个令牌。
	clock.advance(500 * time.Millisecond)
	if limiter.Allow("key-1").Allowed {
		t.Fatal("token granted before a full token was refilled")
	}

	clock.advance(500 * time.Millisecond)
	if !limiter.Allow("key-1").Allowed {
		t.Fatal("token not granted after a full refill interval")
	}
}

func TestTokenBucketCapsAtBurst(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(10, 2, clock.now)

	if !limiter.Allow("key-1").Allowed {
		t.Fatal("first call rejected")
	}
	clock.advance(time.Hour) // 回填一小时也不该超过 burst

	d := limiter.Allow("key-1")
	if !d.Allowed || d.Remaining != 1 {
		t.Fatalf("after a long idle: %+v, want allowed with Remaining = 1", d)
	}
}

func TestTokenBucketIsPerKey(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(1, 1, clock.now)

	if !limiter.Allow("key-a").Allowed {
		t.Fatal("key-a rejected on its first call")
	}
	if limiter.Allow("key-a").Allowed {
		t.Fatal("key-a allowed twice with burst = 1")
	}
	// 一把失控的脚本不该拖垮同一账号的另一条集成。
	if !limiter.Allow("key-b").Allowed {
		t.Fatal("key-b rejected although its own bucket is full")
	}
}

func TestTokenBucketRetryAfterTracksRefillRate(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(0.5, 1, clock.now) // 2 秒一个令牌

	limiter.Allow("key-1")
	d := limiter.Allow("key-1")
	if d.Allowed {
		t.Fatal("second call allowed with burst = 1")
	}
	if d.RetryAfter < time.Second || d.RetryAfter > 2*time.Second {
		t.Fatalf("RetryAfter = %v, want ~2s", d.RetryAfter)
	}
}

func TestTokenBucketForgetResetsBucket(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(0.001, 1, clock.now)

	if !limiter.Allow("key-1").Allowed {
		t.Fatal("first call rejected")
	}
	if limiter.Allow("key-1").Allowed {
		t.Fatal("second call allowed with burst = 1")
	}
	limiter.Forget("key-1")
	if !limiter.Allow("key-1").Allowed {
		t.Fatal("bucket was not reset by Forget")
	}
}

func TestTokenBucketDisabled(t *testing.T) {
	limiter := NewTokenBucketLimiter(0, 0, nil)
	for i := 0; i < 100; i++ {
		d := limiter.Allow("key-1")
		if !d.Allowed {
			t.Fatalf("call %d rejected although limiting is disabled", i+1)
		}
		if d.Limit != 0 {
			t.Fatalf("Limit = %d, want 0 (响应头据此判断要不要发)", d.Limit)
		}
	}
}

// 桶的数量不能无限增长：撤销或早已停用的 key 不该永远占着内存。
func TestTokenBucketSweepsIdleBuckets(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)}
	limiter := NewTokenBucketLimiter(1, 1, clock.now).(*tokenBucketLimiter)

	for i := 0; i < sweepThreshold; i++ {
		limiter.Allow(strconv.Itoa(i))
	}
	if len(limiter.buckets) < sweepThreshold {
		t.Fatalf("expected at least %d buckets, got %d", sweepThreshold, len(limiter.buckets))
	}

	clock.advance(sweepIdleAfter + time.Minute)
	limiter.Allow("fresh-key") // 下一次判定时顺手清理

	if got := len(limiter.buckets); got > 2 {
		t.Fatalf("idle buckets were not swept: %d left", got)
	}
}
