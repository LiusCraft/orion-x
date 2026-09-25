package apikey

import (
	"sync"
	"time"
)

// usageDelta 是一个刷库窗口内某个 key 的增量。
type usageDelta struct {
	Calls    int64
	LastUsed time.Time
}

// usageCounter 聚合 last_used_at / call_count（§1 D7）：热路径只在内存里加一，
// 刷库交给进程级单个 goroutine。代价是进程重启丢一个窗口（§3.2），且禁止用于对账。
type usageCounter struct {
	mu      sync.Mutex
	pending map[string]usageDelta
}

func newUsageCounter() *usageCounter {
	return &usageCounter{pending: make(map[string]usageDelta)}
}

// Touch 记一次成功走到业务层的调用；被限流拦下的不该经过这里（§5.2 路径 A）。
func (c *usageCounter) Touch(keyID string, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.pending[keyID]
	d.Calls++
	if at.After(d.LastUsed) {
		d.LastUsed = at
	}
	c.pending[keyID] = d
}

// Forget 丢掉某个 key 的待刷计数（撤销时调用）。
func (c *usageCounter) Forget(keyID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, keyID)
}

// Drain 取走并清空当前窗口的增量。返回的 map 归调用方所有。
func (c *usageCounter) Drain() map[string]usageDelta {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) == 0 {
		return nil
	}
	out := c.pending
	c.pending = make(map[string]usageDelta)
	return out
}
