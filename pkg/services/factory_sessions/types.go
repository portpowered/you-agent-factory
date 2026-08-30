package factorysessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
	"strings"
	"time"
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
// Alias is declared in contracts.go next to the other internal/contracts ports.

// ResponseEventIDGenerator supplies opaque identities for response events.
type ResponseEventIDGenerator func() string

// LiveRuntime is the host-independent runtime capability attached to a live
// Factory Session. Process hosts retain their private lifecycle handles, while
// application services operate through this bounded domain view.
type LiveRuntime struct {
	Factory factory.Service
	// Binding is the opaque activation capability published by Factory
	// Runtime. Factory remains populated as a compatibility fallback while
	// callers migrate off hosted runtime products.
	Binding factory.RuntimeBinding
	// WorkAndEventIngress is the migration-only Work-submission and event-
	// subscription boundary published alongside the opaque runtime capability.
	// Factory Sessions resolves it once when it binds Factory so peers read a
	// declared capability instead of recovering one from the runtime value.
	// It retires together with factory.APIFactory once Work admission owns
	// submission and Recordings owns canonical event reads.
	WorkAndEventIngress   factory.APIFactory
	Clock                 factory.Clock
	BackendScopeID        string
	RuntimeConfig         interfaces.LoadedFactorySource
	LiveChangeEvents      LiveChangeEventLog
	LiveChangeApplication LiveChangeApplication
	LiveChangeAdmission   LiveChangeAdmission
	LiveChangeLogger      *zap.Logger
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

// OpenResult is the plain live-control open outcome of opening or validating a
// session folder. Peers read SessionID and Session for stable live identity.
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
	PendingHumanApprovals  []interfaces.FactoryWorldHumanApproval
	Petri                  *PetriRuntimeProjection
	PolicyHash             *string
	Progress               RuntimeProgress
	SourceHash             *string
	SourceRef              *string
	Status                 string
	StopSummary            *StopSummary
	StreamIdentity         *RuntimeStreamIdentity
	// RetainedMetricsSessionIDs is an internal Factory Sessions projection of
	// the exact canonical lineage selected for this live session. Transport
	// mappers intentionally omit it from the public runtime shape.
	RetainedMetricsSessionIDs []string
	Usage                     RuntimeUsage
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

// --- merged from execution_owned_helpers.go ---

// Root-owned lifecycle helpers are published as function values so the Sessions
// service root keeps exactly the Service InterfaceType surface required by
// pkg-structure (no package-level FuncDecl exports).
var (
	IsTerminalLifecycleStatus              = isTerminalLifecycleStatus
	AllowsRetryDispatchOnTerminal          = allowsRetryDispatchOnTerminal
	AllowsInterruptDispatchOnSession       = allowsInterruptDispatchOnSession
	InspectionLinksForSession              = inspectionLinksForSession
	EmptySessionUsage                      = emptySessionUsage
	EvaluateLifecycleControl               = evaluateLifecycleControl
	LifecycleControlLinksForSession        = lifecycleControlLinksForSession
	LifecycleControlOutcomeClass           = lifecycleControlOutcomeClass
	LifecycleStatusFromFactoryRuntimeState = lifecycleStatusFromFactoryRuntimeState
	LiveLifecycleControlLinksForSession    = liveLifecycleControlLinksForSession
	LiveLifecycleControlLogFields          = liveLifecycleControlLogFields
	MaterializeEventReadStream             = materializeEventReadStream
	NewValidationError                     = newValidationError
)

// IsTerminalLifecycleStatus reports whether status is terminal and therefore
// immutable except for explicitly allowed inspection or retry behaviors.
func isTerminalLifecycleStatus(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusSucceeded,
		LifecycleStatusFailed,
		LifecycleStatusCanceled,
		LifecycleStatusTimedOut,
		LifecycleStatusInterrupted,
		LifecycleStatusTerminated:
		return true
	default:
		return false
	}
}

// AllowsRetryDispatchOnTerminal reports whether retry-dispatch remains permitted
// after the session reaches a terminal status. Failed sessions may still accept
// retry-dispatch for failed child dispatches.
func allowsRetryDispatchOnTerminal(status LifecycleStatus) bool {
	return status == LifecycleStatusFailed
}

// AllowsInterruptDispatchOnSession reports whether interrupt-dispatch remains
// permitted while the session is actively running goal work.
func allowsInterruptDispatchOnSession(status LifecycleStatus) bool {
	switch status {
	case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
		return true
	default:
		return false
	}
}

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func inspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session:    base,
		Status:     base,
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}

// EmptySessionUsage returns the stable zero usage projection for sessions without
// runtime consumption data.
func emptySessionUsage() SessionUsage {
	return SessionUsage{Resources: []ResourceUsage{}}
}

// EvaluateLifecycleControl classifies one lifecycle control request against the
// current durable session status without runtime-specific dispatch context.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this transition classifier keeps durable lifecycle control outcomes explicit across terminal and active states.
func evaluateLifecycleControl(operation LifecycleControlKind, status LifecycleStatus) LifecycleControlOutcome {
	if status == "" {
		return LifecycleControlOutcomeInvalidState
	}
	if status == LifecycleStatusInterrupted && operation == LifecycleControlResume {
		return LifecycleControlOutcomeAccepted
	}
	if isTerminalLifecycleStatus(status) {
		switch operation {
		case LifecycleControlRetryDispatch:
			if status == LifecycleStatusFailed {
				return LifecycleControlOutcomeAccepted
			}
			return LifecycleControlOutcomeTerminalSession
		case LifecycleControlCancel, LifecycleControlTerminate:
			if status == LifecycleStatusCanceled && operation == LifecycleControlCancel {
				return LifecycleControlOutcomeNoOp
			}
			if status == LifecycleStatusTerminated && operation == LifecycleControlTerminate {
				return LifecycleControlOutcomeNoOp
			}
			return LifecycleControlOutcomeTerminalSession
		default:
			return LifecycleControlOutcomeTerminalSession
		}
	}

	switch operation {
	case LifecycleControlPause:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusPaused:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlResume:
		switch status {
		case LifecycleStatusPaused, LifecycleStatusInterrupted:
			return LifecycleControlOutcomeAccepted
		case LifecycleStatusResuming, LifecycleStatusRunning:
			return LifecycleControlOutcomeNoOp
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlCancel:
		switch status {
		case LifecycleStatusCanceling:
			return LifecycleControlOutcomeNoOp
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlTerminate:
		switch status {
		case LifecycleStatusQueued,
			LifecycleStatusAwaitingApproval,
			LifecycleStatusRunning,
			LifecycleStatusPaused,
			LifecycleStatusResuming,
			LifecycleStatusCanceling:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	case LifecycleControlApprove:
		if status == LifecycleStatusAwaitingApproval {
			return LifecycleControlOutcomeAccepted
		}
		return LifecycleControlOutcomeInvalidState
	case LifecycleControlRetryDispatch:
		switch status {
		case LifecycleStatusRunning, LifecycleStatusPaused, LifecycleStatusResuming:
			return LifecycleControlOutcomeAccepted
		default:
			return LifecycleControlOutcomeInvalidState
		}
	default:
		return LifecycleControlOutcomeInvalidState
	}
}

// LifecycleControlLinksForSession builds post-control inspection links for one durable session.
func lifecycleControlLinksForSession(sessionID string, includeEvents bool) LifecycleControlLinks {
	inspection := inspectionLinksForSession(sessionID, includeEvents)
	return LifecycleControlLinks{
		Session:    inspection.Session,
		Status:     inspection.Status,
		Results:    inspection.Results,
		Dispatches: inspection.Dispatches,
		Artifacts:  inspection.Artifacts,
		Events:     inspection.Events,
	}
}

// LifecycleControlOutcomeClass normalizes lifecycle outcomes and errors into
// the stable low-cardinality class used by logs and metrics.
func lifecycleControlOutcomeClass(outcome LifecycleControlOutcome, err error) string {
	if err != nil {
		if errors.Is(err, ErrDurableSessionNotFound) {
			return LifecycleControlOutcomeClassNotFound
		}
		var controlErr *ControlError
		if errors.As(err, &controlErr) {
			return string(controlErr.Outcome)
		}
		return "ERROR"
	}
	if outcome == "" {
		return "ERROR"
	}
	return string(outcome)
}

// LifecycleStatusFromFactoryRuntimeState maps one live Petri factory runtime state
// into the shared Factory Session lifecycle vocabulary used by control surfaces.
func lifecycleStatusFromFactoryRuntimeState(factoryState string) LifecycleStatus {
	switch strings.ToUpper(strings.TrimSpace(factoryState)) {
	case "RUNNING", "IDLE":
		return LifecycleStatusRunning
	case "PAUSED":
		return LifecycleStatusPaused
	case "COMPLETED":
		return LifecycleStatusSucceeded
	case "FAILED":
		return LifecycleStatusFailed
	default:
		return ""
	}
}

// LiveLifecycleControlLinksForSession builds post-control inspection links for
// one live workspace Factory Session.
func liveLifecycleControlLinksForSession(sessionID string) LifecycleControlLinks {
	base := fmt.Sprintf("/factory-sessions/%s", strings.TrimSpace(sessionID))
	return LifecycleControlLinks{
		Session: base,
		Status:  base,
		Results: base + "/result",
		Events:  base + "/events",
	}
}

// LiveLifecycleControlLogFields returns the canonical structured fields for a
// live-session lifecycle control observation.
func liveLifecycleControlLogFields(sessionID string, operation LifecycleControlKind, outcomeClass string, status LifecycleStatus, control ControlRequest) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("operation", string(operation)),
		zap.String("outcome", outcomeClass),
	}
	if status != "" {
		fields = append(fields, zap.String("lifecycle_control_status", string(status)))
	}
	if control.RequestID != "" {
		fields = append(fields, zap.String("request_id", control.RequestID))
	}
	return fields
}

// MaterializeEventReadStream owns the finite stream lifecycle for one durable
// event read. Transports receive an already-closed live channel plus detached
// canonical history and do not manufacture channel-backed streams.
func materializeEventReadStream(result EventReadResult) *interfaces.FactoryEventStream {
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	stream := &interfaces.FactoryEventStream{Events: closed}
	for _, raw := range result.Events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		stream.History = append(stream.History, event)
	}
	return stream
}

// NewValidationError constructs one field-scoped validation error.
func newValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
	}
}

func cloneDetachedStartRequest(request SessionStartRequest) SessionStartRequest {
	request.Definition = cloneDefinitionSelection(request.Definition)
	request.Source = cloneSource(request.Source)
	request.Input = clonePreparedInput(request.Input)
	request.Args = cloneAnyMap(request.Args)
	request.Policy = cloneAnyMap(request.Policy)
	request.Orchestrator = cloneOrchestratorOverride(request.Orchestrator)
	request.RuntimeOptions = cloneRuntimeOptions(request.RuntimeOptions)
	request.Target = cloneTargetRef(request.Target)
	return request
}

func cloneDefinitionSelection(selection SessionDefinitionSelection) SessionDefinitionSelection {
	selection.SourceRef = strings.TrimSpace(selection.SourceRef)
	selection.SourceHash = strings.TrimSpace(selection.SourceHash)
	if selection.DefinitionVersion != nil {
		version := *selection.DefinitionVersion
		selection.DefinitionVersion = &version
	}
	return selection
}

func cloneSource(source Source) Source {
	cloned := source
	cloned.FactoryInline = append([]byte(nil), source.FactoryInline...)
	if source.InlineWorkflow != nil {
		inline := *source.InlineWorkflow
		inline.ArgsSchema = append([]byte(nil), source.InlineWorkflow.ArgsSchema...)
		inline.DefaultPolicy = append([]byte(nil), source.InlineWorkflow.DefaultPolicy...)
		inline.Metadata = cloneStringMap(source.InlineWorkflow.Metadata)
		if len(source.InlineWorkflow.Agents) > 0 {
			inline.Agents = make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(source.InlineWorkflow.Agents))
			for name, agent := range source.InlineWorkflow.Agents {
				inline.Agents[name] = agent
			}
		}
		cloned.InlineWorkflow = &inline
	}
	return cloned
}

func cloneTargetRef(target *TargetRef) *TargetRef {
	if target == nil {
		return nil
	}
	cloned := *target
	return &cloned
}

func cloneOrchestratorOverride(override *OrchestratorOverride) *OrchestratorOverride {
	if override == nil {
		return nil
	}
	cloned := *override
	cloned.Raw = append([]byte(nil), override.Raw...)
	return &cloned
}

func cloneRuntimeOptions(options *RuntimeOptions) *RuntimeOptions {
	if options == nil {
		return nil
	}
	cloned := *options
	return &cloned
}
