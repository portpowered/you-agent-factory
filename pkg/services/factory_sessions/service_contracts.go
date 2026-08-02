package factorysessions

import (
	"context"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

// StartResult is the Sessions-owned envelope returned by Service.Start. Only
// one of Live, Async, or Sync is populated for a successful request; the
// stable SessionID and Status are copied to the top-level result when the
// selected start path produces them; validate-only live opens intentionally
// return empty identity/status values.
type StartResult struct {
	SessionID string
	Status    LifecycleStatus
	Mode      StartMode
	Live      *OpenResult
	Async     *AsyncStartResult
	Sync      *SyncStartResult
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
// ErrSessionNotFound, and *ControlError on these Service methods; peers must
// not import private live-runtime registry or host types and must not depend
// on a second peer-facing live-session interface.
// The published durable-execution slice uses DurableStartRequest,
// DurableAsyncStartResult, DurableResumeRequest, DurableControlRequest,
// DurableControlResult, DurableInspectResult, *DurableValidationError,
// ErrDurableSessionNotFound, *DurableResumeError, and *DurableControlError on
// ExecutionService methods embedded in Service; peers must not import nested
// durable-execution or internal/execution implementation packages as the
// peer-facing source of truth.
// The published invocation slice uses InvocationRequest,
// ResolvedInvocationInput, InvocationResult, InvocationTimeout,
// InvocationTerminalStatus, InvocationErrorCode, and *InvocationValidationError
// as plain root vocabulary on the singular Service aggregate; peers must not
// import private invocation subservice types and must not depend on a separately
// published peer-facing invoker interface.
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
// Service rather than introducing a second peer-facing session authority.
type Service interface {
	ExecutionService
	ForRuntime(OpeningBindingRequest) (Service, error)
	// ReadCurrentFactoryForSession reads detached editable definition facts for
	// one live session. Sessions owns runtime selection, version lookup, and
	// all current-Factory lifecycle policy.
	ReadCurrentFactoryForSession(context.Context, string) (factorydefinitions.EditableFactory, error)
	// SaveFactoryForSession owns optimistic concurrency, persistence rollback,
	// idle checks, and activation for one session-scoped definition save.
	SaveFactoryForSession(context.Context, string, factorydefinitions.SaveMode, factorydefinitions.EditableFactory) (factorydefinitions.EditableFactory, error)
	// ActivateFactory resolves a persisted named definition through the unary
	// Definitions root and swaps it only after the addressed runtime is idle.
	ActivateFactory(context.Context, string) error
	// Start is the root start adapter. It selects the existing live or durable
	// start path from the request mode while keeping the caller on the singular
	// Sessions authority.
	Start(context.Context, StartRequest) (StartResult, error)
	// InvokeFactorySession keeps session invocation and its result vocabulary on
	// the singular root. Transport mapping may translate the root result to a
	// generated representation, but it does not receive a separately injected
	// invoker.
	InvokeFactorySession(context.Context, string, InvocationRequest) (InvocationResult, error)
	// ActivateNamedFactory serializes named-factory activation through the
	// current Factory Session runtime.
	ActivateNamedFactory(context.Context, string) error
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
// Live-control operations remain methods on Service; this file does not publish
// a separate peer-facing live-session interface.

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
