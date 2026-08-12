package factorysessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"strings"
	"time"
)

// ModelInvocationTimeout is the Factory Sessions-owned lifecycle budget for a
// one-shot model invocation.
type ModelInvocationTimeout time.Duration

const DefaultModelInvocationTimeout ModelInvocationTimeout = ModelInvocationTimeout(10 * time.Second)

// InvocationInputSourceKind identifies the transport-independent source
// category supplied for one Factory Session invocation.
type InvocationInputSourceKind string

const InvocationInputSourceKindText InvocationInputSourceKind = "text"

// InvocationRequest carries one normalized transport request into the Factory
// Session invocation owner.
type InvocationRequest struct {
	Args            *map[string]any
	Content         []work.WorkContentPart
	ContentProvided bool
	// PreparedInvocationInput carries a Work-normalized CLI invocation. Public
	// API requests leave this nil and continue through service-owned normalization.
	PreparedInvocationInput *work.PreparedInvocationInput
	RequestID               *string
	SourceKind              *InvocationInputSourceKind
	TimeoutMillis           *int64
}

// ResolvedInvocationInput is the Factory Session-owned normalized invocation
// input shared by orchestrator-specific execution paths.
type ResolvedInvocationInput struct {
	Source              work.InputSourceLabel
	Content             []work.WorkContentPart
	NormalizedArguments *work.NormalizedArguments
}

// RuntimeBinding contains values selected while opening one Factory Session
// runtime. Process-scoped construction dependencies remain in the injected
// Service.
type RuntimeBinding struct {
	Clock factoryruntime.Clock
}

// Service is the singular Factory Sessions root contract and the only
// cross-service session authority. Identity, live control, durable execution,
// invocation, response stream, and opening operations already owned by
// Sessions remain reachable through this one named interface. The published
// identity slice uses plain IdentityNormalizeRequest,
// IdentityNormalizeProviderRequest, ResolvedIdentity, and the logical-target
// typed errors; peers must not import the private identity subservice.
// The published live-control slice uses OpenRequest/OpenResult,
// ReadProjection, SessionProjection, ControlRequest, LifecycleControlResult,
// ErrSessionNotFound, and *ControlError through LiveControlService; peers
// that only manage live sessions depend on that narrow capability rather than
// this broader aggregate and never import private live-runtime registry or
// host types.
// The published durable-execution slice uses DurableStartRequest,
// DurableAsyncStartResult, DurableResumeRequest, DurableControlRequest,
// DurableControlResult, DurableInspectResult, *DurableValidationError,
// ErrDurableSessionNotFound, *DurableResumeError, and *DurableControlError on
// Service durable execution methods; peers must not import nested
// durable-execution or internal/execution implementation packages as the
// peer-facing source of truth.
// The published invocation slice uses InvocationRequest,
// ResolvedInvocationInput, InvocationResult, InvocationTimeout,
// InvocationTerminalStatus, InvocationErrorCode, and *InvocationValidationError
// as plain root vocabulary shared by the singular Service aggregate and
// InvocationService; peers that need only one-shot invocation use the
// owner-published capability and must not import private invocation subservice
// types.
// The published response-stream slice uses ResponseStreamSubscriptionRequest,
// ResponseStreamCursor, ResponseStreamEvent, ResponseStreamGap,
// ResponseStreamKindGap, ResponseStreamCompletionKind,
// ResponseStreamCompletionPhase, ErrResponseStreamStaleCursor, and
// ErrResponseStreamSubscriptionClosed on SubscribeFactoryResponseEvents; peers
// must not import private response-stream store or manager types and must not
// depend on a nested stream interface for peer import.
// The published opening/binding slice uses OpeningBindingRequest,
// OpeningBindingResult, *OpeningBindingError, and ErrOpeningBindingInvalid on
// ForRuntime; peers supply already-constructed peer root capabilities through
// plain binding inputs without downcasting or bundling nested opening
// interfaces. Binding stays inert during construction characterization.
// The process-scoped root uses ForRuntime to create an isolated runtime view; a
// bound view serves the remaining application operations. Peers must depend on
// the smallest owner-published capability it uses: LiveControlService for
// live control, DurableExecutionService for durable execution,
// InvocationService for one-shot invocation, or TargetExecutionService for the
// established combined target behavior. Service remains the singular aggregate
// authority for callers that genuinely need a combined surface.
type Service interface {
	StartAsync(context.Context, StartRequest) (AsyncStartResult, error)
	StartSync(context.Context, StartRequest) (SyncStartResult, error)
	ResumeInterruptedSession(context.Context, string, ResumeSessionRequest) (AsyncStartResult, error)
	GetSession(context.Context, string) (SessionReadResult, error)
	Pause(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	Resume(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	Terminate(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	Approve(context.Context, string, ApproveRequest) (LifecycleControlResult, error)
	RetryDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error)
	InterruptDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error)
	GetResult(context.Context, string, ResultRequest) (ResultReadResult, error)
	ListDispatches(context.Context, string) (ListDispatchesResult, error)
	QueryDispatches(context.Context, DispatchQueryRequest) (ListDispatchesResult, error)
	GetDispatch(context.Context, string, string) (DispatchDetail, error)
	ListArtifacts(context.Context, string) (ListArtifactsResult, error)
	GetArtifact(context.Context, string, string) (ArtifactDetail, error)
	ReadEvents(context.Context, string, EventReconnectRequest) (EventReadResult, error)
	ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error)
	InvokeFactorySession(context.Context, string, InvocationRequest) (InvocationResult, error)
	ActivateNamedFactory(context.Context, string) error
	ForRuntime(OpeningBindingRequest) (Service, error)
	OpenFactorySession(context.Context, OpenRequest) (*OpenResult, error)
	OpenFactorySessionFromFolder(context.Context, string, *TargetRef, bool, bool) (*OpenResult, error)
	ListFactorySessions(context.Context) ([]ReadProjection, error)
	GetFactorySession(context.Context, string) (SessionProjection, error)
	GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (SyncPreflightResult, error)
	GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error)
	SubscribeFactoryResponseEvents(context.Context, ResponseEventSubscriptionRequest) (*ResponseEventCursor, error)
	SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error
	ReadDurableFactorySessionEventStream(context.Context, string, EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(context.Context, string, EventReconnectRequest) error
	ObserveForSession(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
	PauseLiveFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	ResumeLiveFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	CloseFactorySession(context.Context, string) error
	PauseDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	ResumeDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	CancelDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	TerminateDurableFactorySession(context.Context, string, ControlRequest) (LifecycleControlResult, error)
	ApproveDurableFactorySession(context.Context, string, ApproveRequest) (LifecycleControlResult, error)
	RetryDurableFactorySessionDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error)
	InterruptDurableFactorySessionDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error)
	ApplyLiveChange(context.Context, string, LiveChangeRequest) (LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string) (LiveChangeResult, error)
}

// --- merged from live_control_contract.go ---

// Live-control root slice freezes open, list, get/snapshot, pause, resume, and
// close vocabulary on the singular Service. Peers consume these plain root
// contracts without importing private live-runtime registry or host types:
//
//   - Open: OpenRequest → *OpenResult
//   - List: []ReadProjection
//   - Get/snapshot: SessionProjection
//   - Pause/Resume: ControlRequest → LifecycleControlResult
//   - Close: session identity → error
//
// Typed failures peers distinguish with errors.Is / errors.As:
//   - ErrSessionNotFound for missing live sessions
//   - *ControlError for rejected lifecycle transitions (Outcome InvalidState or
//     TerminalSession), without nested live-runtime imports
//
// Live-control operations remain methods on Service and are also exposed as
// the narrow LiveControlService capability for peers that need no other
// Factory Sessions behavior.

// LiveControlService is the owner-published Factory Sessions capability for
// opening, listing, reading, pausing, resuming, and closing live Factory
// Sessions. It uses the existing public request, projection, result, and
// typed-error vocabulary, so the authoritative Service satisfies it
// structurally without an adapter, duplicate registry, or second construction
// path.
//
// A peer that receives only this capability cannot access durable execution,
// invocation, response-event streaming, inspection, or runtime-opening
// operations through its dependency.
type LiveControlService interface {
	OpenFactorySession(context.Context, LiveControlOpenRequest) (*LiveControlOpenResult, error)
	ListFactorySessions(context.Context) ([]LiveControlListItem, error)
	GetFactorySession(context.Context, string) (LiveControlSnapshot, error)
	PauseLiveFactorySession(context.Context, string, LiveControlRequest) (LiveControlResult, error)
	ResumeLiveFactorySession(context.Context, string, LiveControlRequest) (LiveControlResult, error)
	CloseFactorySession(context.Context, string) error
}

// Service satisfies LiveControlService structurally. This assertion keeps the
// narrow public capability synchronized with its authoritative implementation.
var _ LiveControlService = (Service)(nil)

// LiveChangeService is the owner-published Factory Sessions capability for
// normalized, revisioned live changes and idempotent recovery.
//
// The aggregate Service is asserted below so callers that need the complete
// Factory Sessions surface and callers that only need live changes share one
// implementation and one admission path.
var _ LiveChangeService = (Service)(nil)

// LiveControlOpenRequest is the plain root open request for live session control.
// It is the published name for OpenRequest on the live-control slice.
type LiveControlOpenRequest = OpenRequest

// LiveControlOpenResult is the plain root open result carrying stable session
// identity and discovered targets for the live-control slice.
type LiveControlOpenResult = OpenResult

// LiveControlListItem is one live session row returned by list through the
// live-control root vocabulary.
type LiveControlListItem = ReadProjection

// LiveControlSnapshot is the plain root get/snapshot result for one live
// session, including stable identity and live projection shape.
type LiveControlSnapshot = SessionProjection

// LiveControlRequest is the plain pause/resume request metadata published on
// the live-control root slice.
type LiveControlRequest = ControlRequest

// LiveControlResult is the plain pause/resume success shape published on the
// live-control root slice.
type LiveControlResult = LifecycleControlResult

// LiveControlError is the typed rejected-lifecycle-transition failure published
// on the live-control root slice. Peers match it with errors.As and inspect
// Outcome without importing nested live-runtime packages.
type LiveControlError = ControlError

// LiveChangeOutcome identifies the terminal result returned by the
// session-scoped live-change operation.
type LiveChangeOutcome string

const (
	LiveChangeOutcomeApplied  LiveChangeOutcome = "APPLIED"
	LiveChangeOutcomeNoOp     LiveChangeOutcome = "NO_OP"
	LiveChangeOutcomeReplayed LiveChangeOutcome = "REPLAYED"
	LiveChangeOutcomeFailed   LiveChangeOutcome = "FAILED"
)

// LiveChangeLifecycle is the small lifecycle vocabulary needed for admission.
// A Factory Session may remain IDLE after its current Work finishes, so IDLE
// is intentionally represented as eligible rather than terminal.
type LiveChangeLifecycle string

const (
	LiveChangeLifecycleRunning   LiveChangeLifecycle = "RUNNING"
	LiveChangeLifecycleIdle      LiveChangeLifecycle = "IDLE"
	LiveChangeLifecyclePaused    LiveChangeLifecycle = "PAUSED"
	LiveChangeLifecycleCompleted LiveChangeLifecycle = "COMPLETED"
	LiveChangeLifecycleFailed    LiveChangeLifecycle = "FAILED"
)

// LiveChangeRequest is the transport-neutral operator intent. RequestedValue
// is canonical JSON so transports do not need to agree on a Go value shape.
type LiveChangeRequest struct {
	RequestID        string          `json:"requestId"`
	ChangeID         string          `json:"changeId,omitempty"`
	ExpectedRevision int             `json:"expectedRevision"`
	Operation        string          `json:"operation"`
	TargetID         string          `json:"targetId"`
	RequestedValue   json.RawMessage `json:"requestedValue"`
	Actor            string          `json:"actor,omitempty"`
	Source           string          `json:"source,omitempty"`
	Reason           string          `json:"reason,omitempty"`
}

// LiveChangeResult is the detached terminal outcome. Factory is present only
// for an applied or replayed success and is always cloned at the boundary.
type LiveChangeResult struct {
	SessionID         string
	RequestID         string
	ChangeID          string
	Outcome           LiveChangeOutcome
	PreviousRevision  int
	NewRevision       int
	EffectiveSequence int
	Factory           *factorydefinitions.FactorySnapshot
	ResourceCapacity  *factoryruntime.ResourceCapacityResult
	FailureCode       string
	FailureMessage    string
}

// LiveChangeSessionState is the admission read model supplied by the owning
// Factory Session. The application does not infer revision from mutable
// runtime implementation state.
type LiveChangeSessionState struct {
	SessionID         string
	Lifecycle         LiveChangeLifecycle
	EffectiveRevision int
	EffectiveSequence int
	Factory           *factorydefinitions.FactorySnapshot
}

// LiveChangeStateProvider supplies the current lifecycle and revision
// projection for one Factory Session. It is evaluated only after request
// identity and replay checks permit a new admission attempt.
type LiveChangeStateProvider func(context.Context, string) (LiveChangeSessionState, error)

// LiveChangeApplicationRequest is the explicit application port input. The
// application must mutate the running runtime atomically with its own resource
// or orchestration policy and return the complete effective Factory snapshot.
type LiveChangeApplicationRequest struct {
	SessionID        string
	Request          LiveChangeRequest
	PreviousRevision int
	CurrentFactory   *factorydefinitions.FactorySnapshot
}

// LiveChangeApplicationResult is the successful application result.
type LiveChangeApplicationResult struct {
	Factory          *factorydefinitions.FactorySnapshot
	ResourceCapacity *factoryruntime.ResourceCapacityResult
}

// LiveChangePreflightResult lets an application reject a no-op or other
// target-specific condition before the request enters canonical history.
type LiveChangePreflightResult struct {
	Admissible       bool
	NoOp             bool
	Factory          *factorydefinitions.FactorySnapshot
	ResourceCapacity *factoryruntime.ResourceCapacityResult
}

// LiveChangeEventLog is the explicit canonical event boundary used by
// Factory Sessions. Implementations assign sequence and preserve stream
// behavior; the admission owner never reaches into a concrete ledger.
type LiveChangeEventLog interface {
	AppendLiveChangeEvent(factorydefinitions.FactoryEvent) (factorydefinitions.FactoryEvent, error)
	LiveChangeEvents() []factorydefinitions.FactoryEvent
}

// LiveChangeApplication applies an already-admitted live change to a running
// Factory Session. Resource-specific policy belongs behind this port.
type LiveChangeApplication interface {
	ApplyLiveChange(context.Context, LiveChangeApplicationRequest) (LiveChangeApplicationResult, error)
}

// LiveChangePreflight is an optional application capability for target lookup,
// exact no-op detection, and application-specific validation before admission.
type LiveChangePreflight interface {
	PreflightLiveChange(context.Context, LiveChangeApplicationRequest) (LiveChangePreflightResult, error)
}

// LiveChangeAdmission serializes change application with dispatch admission.
// The release function is always owned by the caller after a successful
// acquisition; a nil implementation falls back to the session-local guard.
type LiveChangeAdmission interface {
	AcquireLiveChange(context.Context, string) (release func(), err error)
}

// LiveChangeOperation contains the runtime-scoped capabilities used by the
// process-scoped live-change coordinator. The coordinator is constructed once
// by Factory Sessions wire; state, event history, application behavior, clock,
// and logging remain explicit for each live or durable session operation.
type LiveChangeOperation struct {
	StateProvider LiveChangeStateProvider
	Events        LiveChangeEventLog
	Application   LiveChangeApplication
	Now           func() time.Time
	Logger        *zap.Logger
}

// LiveChangeCoordinator is the owner-published admission capability shared by
// live Factory Sessions and durable JavaScript execution. Implementations own
// request normalization, idempotency, recovery, and canonical event closure;
// callers supply only the runtime-specific operation ports.
type LiveChangeCoordinator interface {
	ApplyLiveChange(context.Context, string, LiveChangeRequest, LiveChangeOperation) (LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string, LiveChangeOperation) (LiveChangeResult, error)
}

// LiveChangeErrorCode identifies a safe, stable failure category.
type LiveChangeErrorCode string

const (
	LiveChangeErrorInvalidRequest         LiveChangeErrorCode = "INVALID_REQUEST"
	LiveChangeErrorSessionNotFound        LiveChangeErrorCode = "SESSION_NOT_FOUND"
	LiveChangeErrorLifecycleConflict      LiveChangeErrorCode = "LIFECYCLE_CONFLICT"
	LiveChangeErrorRevisionConflict       LiveChangeErrorCode = "REVISION_CONFLICT"
	LiveChangeErrorRequestConflict        LiveChangeErrorCode = "REQUEST_CONFLICT"
	LiveChangeErrorTargetNotFound         LiveChangeErrorCode = "TARGET_NOT_FOUND"
	LiveChangeErrorNoOp                   LiveChangeErrorCode = "NO_OP"
	LiveChangeErrorCapacityInUse          LiveChangeErrorCode = "RESOURCE_CAPACITY_IN_USE"
	LiveChangeErrorApplicationFailed      LiveChangeErrorCode = "APPLICATION_FAILED"
	LiveChangeErrorApplicationUnavailable LiveChangeErrorCode = "APPLICATION_UNAVAILABLE"
	LiveChangeErrorRecoveryUnavailable    LiveChangeErrorCode = "RECOVERY_UNAVAILABLE"
	LiveChangeErrorEventAppendFailed      LiveChangeErrorCode = "EVENT_APPEND_FAILED"
)

var (
	ErrLiveChangeInvalidRequest         = errors.New("live change request is invalid")
	ErrLiveChangeSessionNotFound        = errors.New("live change session not found")
	ErrLiveChangeLifecycleConflict      = errors.New("live change session lifecycle is not eligible")
	ErrLiveChangeRevisionConflict       = errors.New("live change expected revision is stale")
	ErrLiveChangeRequestConflict        = errors.New("live change request identity conflicts with a prior request")
	ErrLiveChangeTargetNotFound         = errors.New("live change target was not found")
	ErrLiveChangeNoOp                   = errors.New("live change is an exact no-op")
	ErrLiveChangeCapacityInUse          = errors.New("live change resource capacity is in use")
	ErrLiveChangeApplicationFailed      = errors.New("live change application failed")
	ErrLiveChangeApplicationUnavailable = errors.New("live change application is unavailable")
	ErrLiveChangeRecoveryUnavailable    = errors.New("live change recovery is unavailable")
	ErrLiveChangeEventAppendFailed      = errors.New("live change event append failed")
)

var liveChangeErrorSentinels = map[LiveChangeErrorCode]error{
	LiveChangeErrorInvalidRequest:         ErrLiveChangeInvalidRequest,
	LiveChangeErrorLifecycleConflict:      ErrLiveChangeLifecycleConflict,
	LiveChangeErrorRevisionConflict:       ErrLiveChangeRevisionConflict,
	LiveChangeErrorRequestConflict:        ErrLiveChangeRequestConflict,
	LiveChangeErrorTargetNotFound:         ErrLiveChangeTargetNotFound,
	LiveChangeErrorNoOp:                   ErrLiveChangeNoOp,
	LiveChangeErrorCapacityInUse:          ErrLiveChangeCapacityInUse,
	LiveChangeErrorApplicationFailed:      ErrLiveChangeApplicationFailed,
	LiveChangeErrorApplicationUnavailable: ErrLiveChangeApplicationUnavailable,
	LiveChangeErrorRecoveryUnavailable:    ErrLiveChangeRecoveryUnavailable,
	LiveChangeErrorEventAppendFailed:      ErrLiveChangeEventAppendFailed,
}

// LiveChangeError is the typed, safe error returned by admission. Cause is
// retained only for local errors.Is matching and is never serialized.
type LiveChangeError struct {
	Code             LiveChangeErrorCode
	Field            string
	Message          string
	RequestID        string
	ChangeID         string
	ResourceCapacity *factoryruntime.ResourceCapacityResult
	Cause            error
}

func (e *LiveChangeError) Error() string {
	if e == nil {
		return "live change error"
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return e.Message
}

func (e *LiveChangeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *LiveChangeError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	if e.Code == LiveChangeErrorSessionNotFound {
		return target == ErrLiveChangeSessionNotFound || target == ErrSessionNotFound
	}
	return liveChangeErrorSentinels[e.Code] == target
}

// NewLiveChangeError constructs a stable typed error without exposing an
// implementation error's unsafe message at the public boundary.
func NewLiveChangeError(code LiveChangeErrorCode, message string) *LiveChangeError {
	return &LiveChangeError{Code: code, Message: strings.TrimSpace(message)}
}

// NormalizeLiveChangeRequest validates and canonicalizes request identity and
// JSON value bytes before any session state or event-log mutation occurs.
func NormalizeLiveChangeRequest(request LiveChangeRequest) (LiveChangeRequest, error) {
	request.RequestID = strings.TrimSpace(request.RequestID)
	request.ChangeID = strings.TrimSpace(request.ChangeID)
	request.Operation = strings.ToLower(strings.TrimSpace(request.Operation))
	request.TargetID = strings.TrimSpace(request.TargetID)
	request.Actor = strings.TrimSpace(request.Actor)
	request.Source = strings.TrimSpace(request.Source)
	request.Reason = strings.Join(strings.Fields(request.Reason), " ")
	if request.RequestID == "" {
		return LiveChangeRequest{}, &LiveChangeError{Code: LiveChangeErrorInvalidRequest, Field: "requestId", Message: "request id is required"}
	}
	if request.ChangeID == "" {
		request.ChangeID = "live-change/" + request.RequestID
	}
	if request.ExpectedRevision < 0 {
		return LiveChangeRequest{}, &LiveChangeError{Code: LiveChangeErrorInvalidRequest, Field: "expectedRevision", Message: "expected revision must not be negative"}
	}
	if request.Operation == "" {
		return LiveChangeRequest{}, &LiveChangeError{Code: LiveChangeErrorInvalidRequest, Field: "operation", Message: "operation is required"}
	}
	if request.TargetID == "" {
		return LiveChangeRequest{}, &LiveChangeError{Code: LiveChangeErrorInvalidRequest, Field: "targetId", Message: "target id is required"}
	}
	canonical, err := canonicalJSON(request.RequestedValue)
	if err != nil {
		return LiveChangeRequest{}, &LiveChangeError{Code: LiveChangeErrorInvalidRequest, Field: "requestedValue", Message: "requested value must be valid JSON", Cause: err}
	}
	request.RequestedValue = canonical
	return request, nil
}

func canonicalJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("requested value is empty")
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// LiveChangeService is the narrow Factory Sessions capability for admitted
// live changes and crash recovery.
type LiveChangeService interface {
	ApplyLiveChange(context.Context, string, LiveChangeRequest) (LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string) (LiveChangeResult, error)
}
