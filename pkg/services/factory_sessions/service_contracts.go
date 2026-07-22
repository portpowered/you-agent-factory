package factorysessions

import (
	"context"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

// SessionInvoker is the canonical Factory Session invocation boundary.
type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
}

// ResolvedInvocationInput is the Factory Session-owned normalized invocation
// input shared by orchestrator-specific execution paths.
type ResolvedInvocationInput struct {
	Source              work.InputSourceLabel
	Content             []work.WorkContentPart
	NormalizedArguments *work.NormalizedArguments
}

// InvocationInputResolver applies the canonical Factory invocation signature
// and compatibility-input policy without selecting an orchestrator runtime.
type InvocationInputResolver interface {
	ResolveInvocationInput(*factorydefinitions.FactoryConfig, InvocationRequest) (ResolvedInvocationInput, error)
}

// InvocationTarget contains the bounded runtime selection values needed for a
// one-shot invocation. Process-wide configuration and edge bags do not cross
// this service boundary.
type InvocationTarget struct {
	FactoryDir                       string
	RunnerID                         string
	OperatorDefaults                 operatorconfig.ResolvedDefaults
	ExecutionBaseDir                 string
	HomeDir                          string
	Logger                           *zap.Logger
	Verbose                          bool
	RecordPath                       string
	ReplayPath                       string
	RuntimeLogDir                    string
	RuntimeLogConfig                 factoryruntime.RuntimeLogStorageConfig
	RuntimeMetricsDir                string
	RuntimeMetricsConfig             factoryruntime.RuntimeMetricsStorageConfig
	ModelCacheDir                    string
	WorkflowID                       string
	MockWorkersConfig                *workers.MockWorkersConfig
	SkipPermissionsOverride          *bool
	SkipRunnerPrerequisiteValidation bool
	MetricsRecorder                  InvocationMetricsRecorder
}

// FactoryInvocationOutcome contains the canonical Factory Event history and
// terminal result retained during the invocation.
type FactoryInvocationOutcome struct {
	Result factorydefinitions.FactoryInvocationResult
}

// FactoryEventConsumer receives ordered canonical events during one invocation.
// Transports may adapt this callback to presentation without moving transport
// encoding into the Factory Sessions owner.
type FactoryEventConsumer func([]factorydefinitions.FactoryEvent)

// InvocationOperation owns one-shot model and Factory invocation lifecycle.
// Callers supply invocation data; opening, readiness, session release and
// runtime shutdown remain behind this operation.
type ModelInvocationOperation interface {
	InvokeModel(context.Context, InvocationTarget, string, models.Request) (models.Result, error)
	ResolveModelInvocationFactoryDir(string) (string, error)
	ExportModelInvocationArtifact(string, string) error
}

type InvocationOperation interface {
	ModelInvocationOperation
	InvokeFactory(context.Context, InvocationTarget, InvocationRequest, FactoryEventConsumer) (FactoryInvocationOutcome, error)
}

// Gateway is the complete Factory Sessions application boundary.
type Gateway interface {
	OpenFactorySession(context.Context, OpenRequest) (*OpenResult, error)
	OpenFactorySessionFromFolder(context.Context, string, *TargetRef, bool, bool) (*OpenResult, error)
	ListFactorySessions(context.Context) ([]ReadProjection, error)
	GetFactorySession(context.Context, string) (SessionProjection, error)
	ResolveFactorySession(string) *LiveSession
	GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (SyncPreflightResult, error)
	GetFactorySessionResult(context.Context, string) (factoryruntime.LiveSessionResult, error)
	GetFactorySessionPartialResult(context.Context, string) (factoryruntime.PartialSessionResult, error)
	SubscribeFactoryResponseEvents(context.Context, ResponseEventSubscriptionRequest) (ResponseEventCursor, error)
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
