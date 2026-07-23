package factorysessions

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
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

// Service is the complete Factory Sessions application boundary. The
// process-scoped root uses ForRuntime to create an isolated runtime view; a
// bound view serves the remaining application operations.
type Service interface {
	ExecutionService
	OpenFactorySession(context.Context, OpenRequest) (*OpenResult, error)
	OpenFactorySessionFromFolder(context.Context, string, *TargetRef, bool, bool) (*OpenResult, error)
	ListFactorySessions(context.Context) ([]ReadProjection, error)
	GetFactorySession(context.Context, string) (SessionProjection, error)
	ResolveFactorySession(string) *LiveSession
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
	SubscribeSessionResponseStream(string, string, int64) (*SessionResponseStreamSubscription, error)
	SessionResponseStreamDispatchIDs(string) ([]string, error)
	ResponseStreams(*LiveSession) *SessionResponseStreamSet
	CloseSessionResponseStreams(*LiveSession)
	JavaScriptCheckpointStore(*LiveSession) factoryruntime.JavaScriptCheckpointStore
	InferenceProgressPublisherFactory(*zap.Logger) func(string) ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
}
