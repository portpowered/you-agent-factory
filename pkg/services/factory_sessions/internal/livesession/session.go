// Package livesession owns construction of live Factory Session records.
package livesession

import (
	"errors"
	"strings"

	"github.com/google/uuid"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

// SessionState tracks session-owned runtime metadata that stays attached to a
// private live-session record rather than mutable service-global configuration.
type SessionState struct {
	FactoryDir       string
	FolderPath       string
	ExecutionBaseDir string
}

// LiveSession is the owner-private mutable record for one hosted Factory
// Session. Public operations project it into detached root contract values.
type LiveSession struct {
	ID string
	SessionState
	Handle                  any
	Runtime                 *factorysessions.LiveRuntime
	IsDefault               bool
	Project                 string
	Target                  factorysessions.TargetRef
	RuntimeFactorySessionID string
	ResponseEvents          *responseeventstore.SessionResponseEventStore
	JavaScriptCheckpoints   factoryruntime.JavaScriptCheckpointStore
}

// CompleteResponseEvents marks the session-owned response-event publication
// scope complete while retaining its immutable events for catch-up readers.
func (s *LiveSession) CompleteResponseEvents() {
	if s == nil || s.ResponseEvents == nil {
		return
	}
	s.ResponseEvents.Complete()
}

// CloseResponseEvents closes the response-event store owned by this live
// session and detaches its active subscribers.
func (s *LiveSession) CloseResponseEvents() {
	if s == nil || s.ResponseEvents == nil {
		return
	}
	s.ResponseEvents.Close()
}

// New constructs a registry entry for a started session.
func New(
	sessionID string,
	factoryDir string,
	folderPath string,
	executionBaseDir string,
	target factorysessions.TargetRef,
	handle any,
	isDefault bool,
	project string,
	clock factoryruntime.Clock,
	generateSessionID factorysessions.SessionIDGenerator,
	eventIDs factorysessions.ResponseEventIDGenerator,
) *LiveSession {
	if clock == nil || generateSessionID == nil || eventIDs == nil {
		return nil
	}
	session := &LiveSession{
		ID: sessionID,
		SessionState: SessionState{
			FactoryDir:       factoryDir,
			FolderPath:       folderPath,
			ExecutionBaseDir: executionBaseDir,
		},
		Handle:    handle,
		IsDefault: isDefault,
		Project:   project,
		Target:    target,
	}
	if err := EnsureRuntimeID(session, generateSessionID); err != nil {
		return nil
	}
	session.ResponseEvents = responseeventstore.NewSessionResponseEventStore(
		CanonicalID(session), clock, eventIDs,
	)
	return session
}

// CanonicalID returns the durable runtime identity for one live session.
// Default-route sessions keep the ~default registry alias but expose a UUID
// runtime identity to clients.
func CanonicalID(session *LiveSession) string {
	if session == nil {
		return ""
	}
	if runtimeID := strings.TrimSpace(session.RuntimeFactorySessionID); runtimeID != "" {
		return runtimeID
	}
	return strings.TrimSpace(session.ID)
}

// IsUUIDID reports whether sessionID is a UUID runtime identity.
func IsUUIDID(sessionID string) bool {
	_, err := uuid.Parse(strings.TrimSpace(sessionID))
	return err == nil
}

// EnsureRuntimeID assigns a UUID runtime identity to default sessions that
// still use the ~default registry alias.
func EnsureRuntimeID(session *LiveSession, generateID factorysessions.SessionIDGenerator) error {
	if session == nil {
		return nil
	}
	if strings.TrimSpace(session.RuntimeFactorySessionID) != "" {
		return nil
	}
	if session.ID != factorysessions.DefaultSessionID {
		return nil
	}
	if generateID == nil {
		return errors.New("Factory Session ID generator is required")
	}
	session.RuntimeFactorySessionID = strings.TrimSpace(generateID())
	if session.RuntimeFactorySessionID == "" {
		return errors.New("Factory Session ID generator returned an empty identity")
	}
	return nil
}
