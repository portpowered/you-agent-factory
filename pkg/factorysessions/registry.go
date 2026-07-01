package factorysessions

import (
	"path/filepath"
	"sort"
	"strings"
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
	if m == nil {
		return nil
	}
	if IsDefaultSessionSelector(id) {
		return m.DefaultSession()
	}
	id = strings.TrimSpace(id)
	if id == "" {
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
	if IsDefaultSessionSelector(id) {
		if session := m.DefaultSession(); session != nil {
			id = session.ID
		} else {
			return
		}
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

// IsDefaultSessionSelector reports whether sessionID is the compatibility default alias.
func IsDefaultSessionSelector(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	return id == "" || id == DefaultSessionID
}

// LogicalSessionKeyID returns the stable logical-session key for one live session target.
func LogicalSessionKeyID(session *LiveSession) string {
	if session == nil {
		return ""
	}
	folderPath := filepath.Clean(strings.TrimSpace(session.FolderPath))
	if folderPath == "." {
		folderPath = ""
	}
	targetKind := strings.TrimSpace(string(session.Target.Kind))
	targetName := strings.TrimSpace(session.Target.Name)
	if targetKind == "" {
		targetKind = string(TargetKindDefault)
	}
	return strings.Join([]string{folderPath, targetKind, targetName}, "::")
}

// DefaultSession returns the live session marked as the default compatibility session.
func (m *Registry) DefaultSession() *LiveSession {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultSessionLocked()
}

func (m *Registry) defaultSessionLocked() *LiveSession {
	for _, session := range m.sessions {
		if session != nil && session.IsDefault {
			return session
		}
	}
	return nil
}

// FindByLogicalSessionKeyID returns the live session registered for logicalSessionKeyID.
func (m *Registry) FindByLogicalSessionKeyID(logicalSessionKeyID string) *LiveSession {
	if m == nil {
		return nil
	}
	logicalSessionKeyID = strings.TrimSpace(logicalSessionKeyID)
	if logicalSessionKeyID == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session == nil {
			continue
		}
		if LogicalSessionKeyID(session) == logicalSessionKeyID {
			return session
		}
	}
	return nil
}
