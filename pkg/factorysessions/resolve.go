package factorysessions

import "strings"

// FindByLogicalSessionKeyID returns the live session registered for logicalSessionKeyID.
func (m *Registry) FindByLogicalSessionKeyID(logicalSessionKeyID string) *LiveSession {
	if m == nil {
		return nil
	}
	logicalSessionKeyID = strings.TrimSpace(logicalSessionKeyID)
	if logicalSessionKeyID == "" {
		return nil
	}
	for _, sessionID := range m.IDs() {
		session := m.Get(sessionID)
		if session == nil {
			continue
		}
		if LogicalSessionKeyID(session) == logicalSessionKeyID {
			return session
		}
	}
	return nil
}
