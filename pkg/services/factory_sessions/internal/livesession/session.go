// Package livesession owns construction of live Factory Session records.
package livesession

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

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
) *factorysessions.LiveSession {
	if clock == nil || generateSessionID == nil || eventIDs == nil {
		return nil
	}
	session := &factorysessions.LiveSession{
		ID: sessionID,
		SessionState: factorysessions.SessionState{
			FactoryDir:       factoryDir,
			FolderPath:       folderPath,
			ExecutionBaseDir: executionBaseDir,
		},
		Handle:    handle,
		IsDefault: isDefault,
		Project:   project,
		Target:    target,
	}
	if err := factorysessions.EnsureRuntimeFactorySessionID(session, generateSessionID); err != nil {
		return nil
	}
	session.ResponseEvents = responseeventstore.NewSessionResponseEventStore(
		factorysessions.CanonicalFactorySessionID(session), clock, eventIDs,
	)
	return session
}
