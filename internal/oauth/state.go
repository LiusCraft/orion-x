package oauth

import (
	"sync"
	"time"
)

const (
	stateTTL        = 10 * time.Minute
	cleanupInterval = 5 * time.Minute
)

type stateEntry struct {
	redirectTo string
	expiresAt  time.Time
}

// StateStore 服务端授权 state 存储，用于 OAuth CSRF 校验。
// 使用服务端存储而非 cookie，保证 login（本地代理）与 callback（公网回调）
// 前后 hostname/IP 不一致时仍可靠。
type StateStore struct {
	mu    sync.Mutex
	items map[string]stateEntry
}

func NewStateStore() *StateStore {
	s := &StateStore{items: make(map[string]stateEntry)}
	go s.cleanupLoop()
	return s
}

func (s *StateStore) Put(key, redirectTo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = stateEntry{redirectTo: redirectTo, expiresAt: time.Now().Add(stateTTL)}
}

// Take 原子读取并删除 entry。成功返回 (redirectTo, true)。
func (s *StateStore) Take(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.items, key)
		return "", false
	}
	delete(s.items, key) // one-time use
	return entry.redirectTo, true
}

func (s *StateStore) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.items {
			if now.After(v.expiresAt) {
				delete(s.items, k)
			}
		}
		s.mu.Unlock()
	}
}
