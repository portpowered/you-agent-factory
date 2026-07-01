package factorysessions

import (
	"strings"

	"github.com/google/uuid"
)

// CanonicalFactorySessionID returns the durable runtime identity for one live
// session. Default-route sessions keep the ~default registry alias but expose a
// UUID runtime identity to clients.
func CanonicalFactorySessionID(session *LiveSession) string {
	if session == nil {
		return ""
	}
	if runtimeID := strings.TrimSpace(session.RuntimeFactorySessionID); runtimeID != "" {
		return runtimeID
	}
	return strings.TrimSpace(session.ID)
}

// IsUUIDFactorySessionID reports whether sessionID is a UUID runtime identity.
func IsUUIDFactorySessionID(sessionID string) bool {
	_, err := uuid.Parse(strings.TrimSpace(sessionID))
	return err == nil
}

// EnsureRuntimeFactorySessionID assigns a UUID runtime identity to default
// sessions that still use the ~default registry alias.
func EnsureRuntimeFactorySessionID(session *LiveSession) {
	if session == nil {
		return
	}
	if strings.TrimSpace(session.RuntimeFactorySessionID) != "" {
		return
	}
	if session.IsDefault || session.ID == DefaultSessionID {
		session.RuntimeFactorySessionID = NewSessionID()
	}
}
