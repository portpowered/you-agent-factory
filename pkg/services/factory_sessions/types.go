package factorysessions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	internalcontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
)

// DefaultSessionID is the stable alias for the primary live factory session.
const DefaultSessionID = "~default"

// ErrSessionNotFound reports that no live Factory Session matched a selector.
var ErrSessionNotFound = errors.New("factory session not found")

// ErrRuntimeNotAvailable reports that no live Factory Session runtime is selected.
var ErrRuntimeNotAvailable = errors.New("factory session runtime is not available")

// RuntimeInstanceIDGenerator supplies opaque identities for newly opened
// Factory Session runtime instances. Wire selects the process implementation.
type RuntimeInstanceIDGenerator func() string

// SessionIDGenerator supplies opaque identities for live sessions, durable
// sessions, and session-owned invocation requests. Wire selects the process
// implementation and tests replace it only at the external edge.
type SessionIDGenerator = internalcontracts.SessionIDGenerator

type ResponseEventIDGenerator = responseeventstore.ResponseEventIDGenerator

// LiveRuntime is the host-independent runtime capability attached to a live
// Factory Session. Process hosts retain their private lifecycle handles, while
// application services operate through this bounded domain view.
type LiveRuntime struct {
	Factory        factory.Service
	BackendScopeID string
	RuntimeConfig  interfaces.LoadedFactorySource
}

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
	Runtime                 *LiveRuntime
	IsDefault               bool
	Project                 string
	Target                  TargetRef
	RuntimeFactorySessionID string
	ResponseEvents          *SessionResponseEventStore
	JavaScriptCheckpoints   factory.JavaScriptCheckpointStore
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
func EnsureRuntimeFactorySessionID(session *LiveSession, generateID SessionIDGenerator) error {
	if session == nil {
		return nil
	}
	if strings.TrimSpace(session.RuntimeFactorySessionID) != "" {
		return nil
	}
	if session.ID == DefaultSessionID {
		if generateID == nil {
			return errors.New("Factory Session ID generator is required")
		}
		session.RuntimeFactorySessionID = strings.TrimSpace(generateID())
		if session.RuntimeFactorySessionID == "" {
			return errors.New("Factory Session ID generator returned an empty identity")
		}
	}
	return nil
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

// SessionResponseEventStore retains immutable FactoryResponseEvent records for
// one live Factory Session runtime. It is separate from canonical factory event
// history and from service-coordinator state.
type SessionResponseEventStore = responseeventstore.SessionResponseEventStore

// ResponseEventSubscription is the Factory Sessions-owned retained-then-live
// cursor used by application and transport consumers.
type ResponseEventSubscription = responseeventstore.Subscription

// ResponseEventCursor is the Factory Sessions-owned retained-then-live
// capability exposed to consumers. Callers depend on this role rather than the
// concrete session store so tests can script the public event boundary.
type ResponseEventCursor interface {
	Next(context.Context) ([]FactoryResponseEvent, error)
	Drain() ([]FactoryResponseEvent, error)
	Detach()
}

// ResponseEventSubscribeOption configures one Factory Session response-event
// subscription without exposing the concrete store package to consumers.
type ResponseEventSubscribeOption = responseeventstore.SubscribeOption

var (
	// ErrResponseEventStoreExpired reports that a completed response-event
	// stream is outside its late-subscription retention window.
	ErrResponseEventStoreExpired = responseeventstore.ErrStoreExpired
	// ErrResponseEventSubscriptionClosed reports that a response-event cursor
	// was detached or its owning store was closed.
	ErrResponseEventSubscriptionClosed = responseeventstore.ErrSubscriptionClosed
)

// WithResponseEventDispatchFilter limits a response-event subscription to one
// dispatch identity.
func WithResponseEventDispatchFilter(dispatchID string) ResponseEventSubscribeOption {
	return responseeventstore.WithDispatchFilter(dispatchID)
}

// SessionResponseStream keeps ordered internal provider progress for one live
// Factory Session runtime. It is separate from canonical factory event history
// and from service-coordinator state.
type SessionResponseStream = responsestream.SessionResponseStream

// SessionResponseStreamSet keeps the dispatch-keyed response streams owned by
// one live Factory Session runtime.
type SessionResponseStreamSet = responsestream.StreamSet

// ResponseStreamRegistry owns the response streams for all live Factory
// Sessions in one runtime scope.
type ResponseStreamRegistry = responsestream.Registry

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
	StopSummary            *StopSummary
	StreamIdentity         *RuntimeStreamIdentity
	Usage                  RuntimeUsage
}

// StopKind classifies why a Factory Session or Work item cannot currently
// continue through its normal automation path.
type StopKind string

const (
	StopKindPaused      StopKind = "PAUSED"
	StopKindInterrupted StopKind = "INTERRUPTED"
	StopKindBlocked     StopKind = "BLOCKED"
	StopKindNeedsHuman  StopKind = "NEEDS_HUMAN"
)

// StopDispatchStatus is the canonical lifecycle state of the dispatch that
// most directly explains a stop condition.
type StopDispatchStatus string

const (
	StopDispatchStatusRunning     StopDispatchStatus = "RUNNING"
	StopDispatchStatusCompleted   StopDispatchStatus = "COMPLETED"
	StopDispatchStatusFailed      StopDispatchStatus = "FAILED"
	StopDispatchStatusInterrupted StopDispatchStatus = "INTERRUPTED"
)

type StopDispatchKind string
type StopFailureType string

const (
	StopDispatchKindPetriTransition StopDispatchKind = "PETRI_TRANSITION"
	StopFailureTypeUnknown          StopFailureType  = "unknown"
)

// StopSummary is the Factory Sessions-owned stopped-state read model.
// Transports convert this detached result without re-deriving its policy.
type StopSummary struct {
	SessionID                string
	StopKind                 StopKind
	SessionLifecycleStatus   *string
	WorkID                   *string
	WorkName                 *string
	WorkTypeName             *string
	WorkState                *string
	LatestDispatch           *StopDispatchSummary
	LatestResultSummary      *string
	SuggestedRecoverySurface *string
	SuggestedRecoveryAction  *string
}

type StopDispatchSummary struct {
	DispatchID      string
	Status          StopDispatchStatus
	DispatchKind    StopDispatchKind
	WorkstationName *string
	FailureDetail   *StopFailureDetail
}

type StopFailureDetail struct {
	Reason  StopFailureType
	Message string
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
	History                  *RuntimeTokenHistory
	ID                       string
	Name                     *string
	PlaceID                  string
	PreviousChainingTraceIDs *[]string
	Tags                     *map[string]string
	TraceID                  string
	WorkID                   string
	WorkType                 string
}

type RuntimeTokenHistory struct {
	ConsecutiveFailures map[string]int
	LastError           string
	PlaceVisits         map[string]int
	TotalVisits         map[string]int
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
