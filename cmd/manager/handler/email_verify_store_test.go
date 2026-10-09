package handler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestVerifyStore 用 miniredis 起一个进程内 Redis，测试不依赖外部服务。
func newTestVerifyStore(t *testing.T) (*VerifyStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewVerifyStore(rdb), mr
}

func TestVerifyStoreLifecycle(t *testing.T) {
	s, _ := newTestVerifyStore(t)
	ctx := context.Background()

	token, err := s.IssueToken(ctx, "user-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64 hex chars", len(token))
	}

	userID, err := s.ConsumeToken(ctx, token)
	if err != nil {
		t.Fatalf("ConsumeToken() error = %v", err)
	}
	if userID != "user-1" {
		t.Errorf("ConsumeToken() userID = %q, want user-1", userID)
	}

	// 单次使用：重放一律 ErrTokenNotFound。
	if _, err := s.ConsumeToken(ctx, token); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("replayed token error = %v, want ErrTokenNotFound", err)
	}
}

func TestVerifyStoreReissueInvalidatesOld(t *testing.T) {
	s, _ := newTestVerifyStore(t)
	ctx := context.Background()

	oldToken, err := s.IssueToken(ctx, "user-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	newToken, err := s.IssueToken(ctx, "user-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() second error = %v", err)
	}

	if _, err := s.ConsumeToken(ctx, oldToken); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("reissued-away token error = %v, want ErrTokenNotFound", err)
	}
	if _, err := s.ConsumeToken(ctx, newToken); err != nil {
		t.Fatalf("ConsumeToken(new) error = %v", err)
	}
}

func TestVerifyStoreExpired(t *testing.T) {
	s, mr := newTestVerifyStore(t)
	ctx := context.Background()

	token, err := s.IssueToken(ctx, "user-1", time.Minute)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	mr.FastForward(2 * time.Minute)

	if _, err := s.ConsumeToken(ctx, token); !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expired token error = %v, want ErrTokenNotFound", err)
	}
}

func TestVerifyStoreConcurrentConsume(t *testing.T) {
	s, _ := newTestVerifyStore(t)
	ctx := context.Background()

	token, err := s.IssueToken(ctx, "user-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	ok := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.ConsumeToken(ctx, token); err == nil {
				ok <- struct{}{}
			}
		}()
	}
	wg.Wait()

	if got := len(ok); got != 1 {
		t.Errorf("successful concurrent consumes = %d, want 1", got)
	}
}

func TestVerifyStoreDoesNotStoreRawToken(t *testing.T) {
	s, mr := newTestVerifyStore(t)
	ctx := context.Background()

	token, err := s.IssueToken(ctx, "user-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	for _, key := range mr.Keys() {
		if strings.Contains(key, token) {
			t.Errorf("raw token found in redis key: %q", key)
		}
		if strings.HasPrefix(key, verifyKeyPrefix+"token:") && len(strings.TrimPrefix(key, verifyKeyPrefix+"token:")) != 64 {
			t.Errorf("token key = %q, want SHA-256 hex suffix", key)
		}
	}
}

func TestVerifyStoreResendCooldown(t *testing.T) {
	s, mr := newTestVerifyStore(t)
	ctx := context.Background()

	allowed, err := s.AllowResend(ctx, "user-1", time.Minute)
	if err != nil || !allowed {
		t.Fatalf("AllowResend() = (%v, %v), want (true, nil)", allowed, err)
	}
	allowed, err = s.AllowResend(ctx, "user-1", time.Minute)
	if err != nil || allowed {
		t.Fatalf("AllowResend() within cooldown = (%v, %v), want (false, nil)", allowed, err)
	}
	allowed, err = s.AllowResend(ctx, "user-2", time.Minute)
	if err != nil || !allowed {
		t.Fatalf("AllowResend(other user) = (%v, %v), want (true, nil)", allowed, err)
	}

	mr.FastForward(time.Minute)
	allowed, err = s.AllowResend(ctx, "user-1", time.Minute)
	if err != nil || !allowed {
		t.Fatalf("AllowResend() after cooldown = (%v, %v), want (true, nil)", allowed, err)
	}
}

func TestVerifyStorePing(t *testing.T) {
	s, mr := newTestVerifyStore(t)
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	mr.Close()
	if err := s.Ping(context.Background()); err == nil {
		t.Error("Ping() after server close = nil, want error")
	}
}
