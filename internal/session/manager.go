package session

import (
	"sync"
)

// Manager owns the lifecycle of active sessions. Channels decide retention;
// the manager only provides concurrency-safe storage and pipeline cleanup.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: make(map[string]*Session)}
}

func (m *Manager) Add(sess *Session) {
	if m == nil || sess == nil || sess.ID == "" {
		return
	}
	m.mu.Lock()
	m.sessions[sess.ID] = sess
	m.mu.Unlock()
}

func (m *Manager) Get(id string) (*Session, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()
	return sess, ok
}

func (m *Manager) Remove(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

// CloseSession removes a session before stopping its pipeline, so concurrent
// lookups cannot acquire a session that is being torn down.
func (m *Manager) CloseSession(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok && sess.Pipeline != nil {
		_ = sess.Pipeline.Stop()
	}
}
