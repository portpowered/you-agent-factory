package factorysessions

import (
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// CurrentFactoryName is the domain identifier for the current Factory selector.
const CurrentFactoryName = "UNDEFINED"

// OpenRequest is the transport-independent request to discover, validate, or
// open a Factory Session from a folder.
type OpenRequest struct {
	FolderPath     string
	Target         *TargetRef
	ValidateOnly   bool
	InitNewFactory bool
}

// NewLiveSession constructs a registry entry for a started session.
func NewLiveSession(
	sessionID string,
	factoryDir string,
	folderPath string,
	executionBaseDir string,
	target TargetRef,
	handle any,
	isDefault bool,
	project string,
	clock factory.Clock,
	generateSessionID SessionIDGenerator,
	eventIDs ResponseEventIDGenerator,
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
	if err := EnsureRuntimeFactorySessionID(session, generateSessionID); err != nil {
		return nil
	}
	session.ResponseEvents = NewSessionResponseEventStore(CanonicalFactorySessionID(session), clock, eventIDs)
	return session
}

func stringPointerOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
