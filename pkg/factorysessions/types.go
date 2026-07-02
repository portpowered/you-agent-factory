package factorysessions

import (
	"strings"

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
	Handle                  any
	IsDefault               bool
	Project                 string
	Target                  TargetRef
	RuntimeFactorySessionID string
}

// NewSessionID allocates a unique live session identifier.
func NewSessionID() string {
	return uuid.NewString()
}

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

// SessionResponseStream keeps ordered internal provider progress for one live
// Factory Session runtime. It is separate from canonical factory event history
// and from service-coordinator state.
type SessionResponseStream = responsestream.SessionResponseStream

// SessionResponseStreamSet keeps the dispatch-keyed response streams owned by
// one live Factory Session runtime.
type SessionResponseStreamSet = responsestream.StreamSet

// SessionResponseStreamEvent is the internal envelope for provider progress and
// response fragments within one Factory Session runtime.
type SessionResponseStreamEvent = responsestream.Event

// SessionResponseStreamEventKind identifies internal response-stream semantics.
type SessionResponseStreamEventKind = responsestream.EventKind

// SessionResponseStreamReadResult is the internal bounded catch-up view for
// one response-stream subscriber resume point.
type SessionResponseStreamReadResult = responsestream.ReadResult

// SessionResponseStreamCompactionSummary records bounded fidelity loss for
// stream subscribers that resume after truncation or coalescing.
type SessionResponseStreamCompactionSummary = responsestream.CompactionSummary

// SessionResponseStreamEventType identifies provider-neutral internal response
// stream event semantics.
type SessionResponseStreamEventType = responsestream.EventType

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

// NewSessionResponseStreamSetWithFactory allocates a dispatch-keyed stream set
// using the supplied stream constructor.
func NewSessionResponseStreamSetWithFactory(
	newStream func() *SessionResponseStream,
) *SessionResponseStreamSet {
	return responsestream.NewStreamSetWithFactory(newStream)
}

// SessionResponseStreamSubscription is an internal live-session response-stream
// cursor that can read retained and live dispatch progress.
type SessionResponseStreamSubscription = responsestream.Subscription
