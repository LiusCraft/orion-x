package voicebot

import (
	"sync"
)

// eventBus 事件总线实现
type eventBus struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
}

func NewEventBus() EventBus {
	return &eventBus{
		subscribers: make(map[EventType][]EventHandler),
	}
}

// Publish 发布事件
func (eb *eventBus) Publish(event Event) {
	eb.mu.RLock()
	handlers, ok := eb.subscribers[event.Type()]
	eb.mu.RUnlock()

	if ok {
		for _, handler := range handlers {
			go handler(event)
		}
	}
}

// Subscribe 订阅事件
func (eb *eventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)
}

// Unsubscribe 取消订阅
// 注意：由于函数不能直接比较，此方法需要基于ID或其他方式实现
// 暂时不实现，使用闭包引用的方式管理订阅
func (eb *eventBus) Unsubscribe(eventType EventType, handler EventHandler) {
	// TODO: 需要使用订阅ID或其他机制来实现取消订阅
}
