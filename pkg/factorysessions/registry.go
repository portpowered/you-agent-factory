package factorysessions

import (
	"sort"
	"sync"
)

// Registry tracks live factory sessions and the selected session id.
type Registry struct {
	mu         sync.RWMutex
	selectedID string
	sessions   map[string]*LiveSession
}

// NewRegistry constructs an empty live session registry.
func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*LiveSession),
	}
}

// Upsert registers or replaces a live session. When selectSession is true, or no
// session is currently selected, the session becomes selected.
func (m *Registry) Upsert(session *LiveSession, selectSession bool) {
	if m == nil || session == nil || session.ID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	if selectSession || m.selectedID == "" {
		m.selectedID = session.ID
	}
}

// Select marks an existing session as selected.
func (m *Registry) Select(id string) bool {
	if m == nil || id == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[id]; !ok {
		return false
	}
	m.selectedID = id
	return true
}

// Current returns the selected live session when present.
func (m *Registry) Current() *LiveSession {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if session, ok := m.sessions[m.selectedID]; ok {
		return session
	}
	return nil
}

// Get returns the live session for id when registered.
func (m *Registry) Get(id string) *LiveSession {
	if m == nil || id == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Remove deletes a live session and promotes another session when needed.
func (m *Registry) Remove(id string) {
	if m == nil || id == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	if m.selectedID != id {
		return
	}
	m.selectedID = ""
	if len(m.sessions) == 0 {
		return
	}
	ids := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		ids = append(ids, sessionID)
	}
	sort.Strings(ids)
	m.selectedID = ids[0]
}

// Count returns the number of registered live sessions.
func (m *Registry) Count() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// IDs returns registered session ids in sorted order.
func (m *Registry) IDs() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
