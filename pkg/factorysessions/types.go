package factorysessions

import (
	"github.com/google/uuid"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
)

// DefaultSessionID is the stable alias for the primary live factory session.
const DefaultSessionID = "~default"

// TargetKind identifies whether a session target is the default factory or a named layout.
type TargetKind string

const (
	TargetKindDefault TargetKind = "default"
	TargetKindNamed   TargetKind = "named"
)

// TargetRef selects a discovered factory session target.
type TargetRef struct {
	Kind TargetKind
	Name string
}

// Target describes a runnable factory directory discovered under a session folder.
type Target struct {
	Ref        TargetRef
	Label      string
	FolderPath string
	FactoryDir string
	Project    string
}

// OpenResult is the internal outcome of opening or validating a session folder.
type OpenResult struct {
	SessionID       string
	Targets         []Target
	InitsNewFactory bool
	FolderPath      string
}

// SessionState tracks session-owned runtime metadata that should stay attached
// to the live session rather than mutable service-global configuration.
type SessionState struct {
	FactoryDir       string
	FolderPath       string
	ExecutionBaseDir string
}

// LiveSession tracks one live factory session and its runtime handle.
// Handle is typed by the composition root (for example *service.liveRuntimeHandle).
type LiveSession struct {
	ID string
	SessionState
	Handle    any
	IsDefault bool
	Project   string
	Target    TargetRef
}

// NewSessionID allocates a unique live session identifier.
func NewSessionID() string {
	return uuid.NewString()
}

// SessionResponseStream keeps ordered internal provider progress for one live
// Factory Session runtime. It is separate from canonical factory event history
// and from service-coordinator state.
type SessionResponseStream = responsestream.SessionResponseStream

// SessionResponseStreamEvent is the internal envelope for provider progress and
// response fragments within one Factory Session runtime.
type SessionResponseStreamEvent = responsestream.Event

// SessionResponseStreamEventKind identifies internal response-stream semantics.
type SessionResponseStreamEventKind = responsestream.EventKind

// SessionResponseStreamRetentionLimits documents bounded-retention controls for
// one internal session response stream.
type SessionResponseStreamRetentionLimits = responsestream.RetentionLimits

// SessionResponseStreamRetentionAccounting summarizes retained stream bytes,
// event count, and oldest event timestamp for retention decisions.
type SessionResponseStreamRetentionAccounting = responsestream.RetentionAccounting

// NewSessionResponseStream allocates an empty internal response stream owned by
// one live Factory Session runtime.
func NewSessionResponseStream() *SessionResponseStream {
	return responsestream.NewSessionResponseStream()
}
