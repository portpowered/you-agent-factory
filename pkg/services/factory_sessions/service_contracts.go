package factorysessions

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	RequestID       *string
	SourceKind      *InvocationInputSourceKind
	TimeoutMillis   *int64
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
	GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.StateSnapshot, error)
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
