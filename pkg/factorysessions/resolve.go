package factorysessions

import "strings"

// IsDefaultSessionSelector reports whether sessionID is the compatibility default alias.
func IsDefaultSessionSelector(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	return id == "" || id == DefaultSessionID
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
