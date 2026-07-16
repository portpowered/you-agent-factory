package factorysessions

import (
	"strings"
	"time"

	"github.com/google/uuid"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
)

// DefaultSessionID is the stable alias for the primary live factory session.
const DefaultSessionID = "~default"

type compatibilityError string

func (err compatibilityError) Error() string { return string(err) }

func (err compatibilityError) Is(target error) bool {
	return target != nil && target.Error() == string(err)
}

// ErrNotFound reports that no live Factory Session matched a requested identity.
var ErrNotFound error = compatibilityError("factory session not found")

// ErrResultUnavailable reports that a Factory Session cannot expose JavaScript results.
var ErrResultUnavailable error = compatibilityError("factory session result unavailable")

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
	ResponseEvents          *SessionResponseEventStore
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
	if session.ID == DefaultSessionID {
		session.RuntimeFactorySessionID = NewSessionID()
	}
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

// BindResponseEventCompletion completes the session-owned response-event store
// when its canonical FactoryEvent history observes terminal session completion.
func BindResponseEventCompletion(
	session *LiveSession,
	addRecorder func(func(interfaces.FactoryEventType)),
) {
	if session == nil || addRecorder == nil {
		return
	}
	addRecorder(func(eventType interfaces.FactoryEventType) {
		if eventType == interfaces.FactoryEventTypeSessionCompleted {
			session.CompleteResponseEvents()
		}
	})
}

// SessionResponseEventStore retains immutable FactoryResponseEvent records for
// one live Factory Session runtime. It is separate from canonical factory event
// history and from service-coordinator state.
type SessionResponseEventStore = responseeventstore.SessionResponseEventStore

// NewSessionResponseEventStore allocates an empty response-event store owned
// by one live Factory Session runtime.
func NewSessionResponseEventStore(factorySessionID string) *SessionResponseEventStore {
	return responseeventstore.NewSessionResponseEventStore(factorySessionID)
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

// RuntimeProjection is the Factory Session-owned live runtime read model.
// Transport packages map this detached value to their public contract.
type RuntimeProjection struct {
	Artifacts              *[]interfaces.FactoryArtifact
	Budgets                *RuntimeBudgets
	Dialect                *string
	JavaScript             *JavaScriptRuntimeProjection
	Lifecycle              RuntimeLifecycle
	LifecycleControlStatus *string
	OrchestratorKind       string
	Petri                  *PetriRuntimeProjection
	PolicyHash             *string
	Progress               RuntimeProgress
	SourceHash             *string
	SourceRef              *string
	Status                 string
	StreamIdentity         *RuntimeStreamIdentity
	Usage                  RuntimeUsage
}

type RuntimeBudgets struct{ MaxAgents *int }

type RuntimeLifecycle struct {
	FinishedAt *time.Time
	StartedAt  time.Time
	UpdatedAt  time.Time
}

type RuntimeStreamIdentity struct {
	BackendScopeID      string
	FactorySessionID    string
	LogicalSessionKeyID string
	StreamGenerationID  string
}

type RuntimeLogicalTarget struct {
	FolderPath       string
	Kind             string
	NamedTarget      *string
	ProviderBoundary *RuntimeLogicalProviderBoundary
}

type RuntimeLogicalProviderBoundary struct {
	Boundary string
	Kind     string
	Provider string
}

type RuntimeProgress struct {
	Categories    RuntimeStatusCategories
	FactoryState  string
	InFlightCount int
	TotalTokens   int
}

type RuntimeStatusCategories struct {
	Failed     int
	Initial    int
	Processing int
	Terminal   int
}

type RuntimeUsage struct{ Resources []RuntimeResourceUsage }

type RuntimeResourceUsage struct {
	Available int
	Name      string
	Total     int
}

type PetriRuntimeProjection struct {
	EnabledTransitions []PetriEnabledTransition
	Marking            []RuntimeToken
}

type PetriEnabledTransition struct {
	TransitionID string
	WorkerType   string
}

type RuntimeToken struct {
	ChainingTraceDepth       *int
	CreatedAt                time.Time
	CurrentChainingTraceID   *string
	EnteredAt                time.Time
	ID                       string
	Name                     *string
	PlaceID                  string
	PreviousChainingTraceIDs *[]string
	Tags                     *map[string]string
	TraceID                  string
	WorkID                   string
	WorkType                 string
}

type JavaScriptRuntimeProjection struct {
	ArgsDigest          *string
	Checkpoints         *[]interfaces.FactorySessionJavaScriptCheckpointEventRef
	ChildDispatchCounts interfaces.FactorySessionChildDispatchCounts
	Phase               *string
	Phases              []string
	ScriptStatus        interfaces.FactorySessionJavaScriptScriptStatus
}

// SyncPreflightReason identifies the reconnect recovery decision for a live
// Factory Session. Transports map these stable values to their public enums.
type SyncPreflightReason string

const (
	SyncPreflightReasonOK                       SyncPreflightReason = "ok"
	SyncPreflightReasonCursorStale              SyncPreflightReason = "cursor_stale"
	SyncPreflightReasonSessionNotFound          SyncPreflightReason = "session_not_found"
	SyncPreflightReasonLogicalSessionRemap      SyncPreflightReason = "logical_session_remap"
	SyncPreflightReasonLogicalSessionUnresolved SyncPreflightReason = "logical_session_unresolved"
)

// SyncPreflightReconnectCursor retains the acknowledged cursor and whether it
// belongs to the resolved stream generation.
type SyncPreflightReconnectCursor struct {
	AfterEventID             *string
	AfterSequence            *int64
	Provided                 bool
	ValidForStreamGeneration bool
}

// SyncPreflightResult is the Factory Session-owned reconnect validation result.
type SyncPreflightResult struct {
	BackendScopeID      *string
	CheckpointReusable  bool
	FactorySessionID    *string
	LogicalSessionKeyID *string
	Reason              SyncPreflightReason
	ReconnectCursor     SyncPreflightReconnectCursor
	RequestedSessionID  string
	StreamGenerationID  *string
}
