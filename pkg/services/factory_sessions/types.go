package factorysessions

import (
	"context"
	"errors"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	internalcontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
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

// ResponseEventIDGenerator supplies opaque identities for response events.
type ResponseEventIDGenerator func() string

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
	Session         *ScopedLiveSessionSummary
	Targets         []Target
	InitsNewFactory bool
	FolderPath      string
}

// ResponseEventCursor is the detached retained-then-live response-event value
// returned by Service. Function fields keep cursor behavior explicit without
// publishing an additional service-root interface.
type ResponseEventCursor struct {
	NextEvents   func(context.Context) ([]FactoryResponseEvent, error)
	DrainEvents  func() ([]FactoryResponseEvent, error)
	DetachCursor func()
}

func (c *ResponseEventCursor) Next(ctx context.Context) ([]FactoryResponseEvent, error) {
	return c.NextEvents(ctx)
}

func (c *ResponseEventCursor) Drain() ([]FactoryResponseEvent, error) { return c.DrainEvents() }

func (c *ResponseEventCursor) Detach() { c.DetachCursor() }

var (
	// ErrResponseEventStoreExpired reports that a completed response-event
	// stream is outside its late-subscription retention window.
	ErrResponseEventStoreExpired = errors.New("session response event store retention window has expired")
	// ErrResponseEventSubscriptionClosed reports that a response-event cursor
	// was detached or its owning store was closed.
	ErrResponseEventSubscriptionClosed = errors.New("session response event store subscription is closed")
)

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
