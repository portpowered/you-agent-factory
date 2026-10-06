// Package roles contains implementation-facing Factory Sessions collaborator
// contracts. These roles are private to the owning service; public consumers
// depend on the root Service or service-owned transport adapters.
package roles

import (
	"context"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

type InvocationMetricsRecorder = factorysessions.InvocationMetricsRecorder

// FactoryEventReader is the private presentation-bridge reader capability.
// It is an alias to an unnamed interface so the Factory Sessions root does not
// publish another named service interface.
type FactoryEventReader = interface {
	SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error)
}

// HostedInvocationOperation is the operation-valued result retained by the
// hosted CLI path after opening. It is intentionally private to implementation
// roles rather than part of the Factory Sessions root interface inventory.
type HostedInvocationOperation interface {
	factorysessions.InvocationService
	FactoryEventReader
}

type RuntimeResolver interface {
	Resolve(sessionID string) *livesession.LiveSession
}

type CurrentRuntimeResolver interface {
	CurrentRuntime() *factorysessions.LiveRuntime
}

type RuntimeReader interface {
	WithRuntimeRead(func(*factorysessions.LiveRuntime) error) error
}

type DirectoryInspection = factorysessions.DirectoryInspection

type CursorPersistenceFileSystem = factorysessions.CursorPersistenceFileSystem

type CursorPersistenceTemporaryFile = factorysessions.CursorPersistenceTemporaryFile

type CursorPersistenceCreateTemporaryFile = factorysessions.CursorPersistenceCreateTemporaryFile

type CursorStoreFactory func(string) (factorysessions.CursorStore, error)

type RequestPreparation interface {
	PrepareStart(factorysessions.StartRequest) (factorysessions.StartRequest, error)
	PrepareControl(factorysessions.ControlRequest) (factorysessions.ControlRequest, error)
	PrepareApprove(factorysessions.ApproveRequest) (factorysessions.ApproveRequest, error)
	PrepareRetryDispatch(factorysessions.RetryDispatchRequest) (factorysessions.RetryDispatchRequest, error)
	PrepareInterruptDispatch(factorysessions.InterruptDispatchRequest) (factorysessions.InterruptDispatchRequest, error)
	PrepareListSessions(factorysessions.ListSessionsRequest) (factorysessions.ListSessionsRequest, error)
	PrepareResult(factorysessions.ResultRequest) (factorysessions.ResultRequest, error)
	PrepareEventReconnect(factorysessions.EventReconnectRequest) (factorysessions.EventReconnectRequest, error)
}

type Registry interface {
	Upsert(*livesession.LiveSession, bool)
	Select(string) bool
	Current() *livesession.LiveSession
	Get(string) *livesession.LiveSession
	Remove(string)
	Count() int
	IDs() []string
	DefaultSession() *livesession.LiveSession
}

type RuntimePersistenceStore interface {
	Save(sessionID string, encoded []byte) error
	Load(sessionID string) ([]byte, error)
}

type RuntimePersistenceFileSystem = factorysessions.RuntimePersistenceFileSystem

type RuntimePersistenceStoreFactory func(string) (RuntimePersistenceStore, error)

type LifecycleRuntime interface {
	StartLifecycle(context.Context, context.Context) error
	StartWorkerLifecycle(context.Context) (factorysessions.RuntimeStop, error)
	CompleteStartup(context.Context) error
	WaitForRuntime(context.Context) error
	StopLifecycle(context.Context) error
	FailStartup(error) error
	CurrentRuntimeBundle() runtimeports.RuntimeInstance
}

type ProcessRuntime interface {
	RunTransport(context.Context, http.Handler) error
	Stop(context.Context) error
}

type ProcessActivation interface {
	Start(context.Context, context.Context) error
	StartWorkers(context.Context) (factorysessions.RuntimeStop, error)
	Stop(context.Context) error
}

type ProcessRuntimeFactory interface {
	Bind(LifecycleRuntime, factorysessions.RuntimeHostRequest, *zap.Logger) (ProcessRuntime, error)
}

type RuntimeHostOperation interface {
	Run(context.Context, http.Handler, LifecycleRuntime, *zap.Logger, factorysessions.RuntimeHostRequest, factorysessions.RuntimeHostObserver) error
}

type LifecyclePlanRequest struct {
	Runtime     ProcessActivation
	Components  factorysessions.BoundProcessComponents
	Close       func() error
	OrderlyStop lifecycle.OrderlyStopOperation
}

type LifecyclePlanOperation func(LifecyclePlanRequest) (lifecycle.Plan, error)

type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
}

// CanonicalSessionInvoker is the owner-private invocation seam used by the
// canonical Factory Sessions root. The compatibility-shaped SessionInvoker
// remains available for existing callers, but canonical code must not call it
// in reverse.
type CanonicalSessionInvoker interface {
	Invoke(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
}

type InvocationInputResolver interface {
	ResolveInvocationInput(*factorydefinitions.FactoryConfig, factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error)
}

type ModelInvocationOperation interface {
	InvokeModel(context.Context, InvocationTarget, string, models.Request) (models.Result, error)
	ResolveModelInvocationFactoryDir(string) (string, error)
	ExportModelInvocationArtifact(string, string) error
}

type InvocationOperation interface {
	ModelInvocationOperation
	InvokeFactory(context.Context, InvocationTarget, factorysessions.InvocationRequest) (FactoryInvocationOutcome, error)
}

type InvocationTarget = factorysessions.InvocationTarget

type FactoryInvocationOutcome = factorysessions.FactoryInvocationOutcome

type ApplicationRuntime interface {
	LifecycleRuntime
}

type RuntimeAssembly interface {
	FactoryConfigForSession(context.Context, string) (*factorydefinitions.FactoryConfig, error)
	CurrentRuntimeResolver
	RuntimeResolver
	RuntimeReader
	work.RuntimeResolver
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
	Complete(
		factoryRootDir string,
		clock factoryruntime.Clock,
		logger *zap.Logger,
		runtimeBuild runtimeports.RuntimeReplacementBuilder,
		startupRuntime runtimeports.RuntimeInstance,
		modelsScope models.RuntimeScopeRef,
		completion factoryruntime.RuntimeInitialCompletion,
		runtimeLifecycle runtimeports.RuntimeLifecycle,
		runtimeSidecars factorysessions.RuntimeSidecars,
		factorySessionID string,
		dir string,
		executionBaseDir string,
		runtimeMode factorydefinitions.RuntimeMode,
		backendScopeID string,
		workFile string,
		workflowID string,
	) (ApplicationRuntime, SessionInvoker, factorydefinitions.SessionHost, factorydefinitions.DefinitionActivationGateway, error)
}

// SessionGateway exposes bound session operations; live startup belongs to Root.
type SessionGateway interface {
	CloseFactorySession(context.Context, string) error
	StartDurable(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error)
	Invoke(context.Context, factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error)
	Get(context.Context, factorysessions.SessionGetRequest) (factorysessions.SessionGetResult, error)
	List(context.Context, factorysessions.SessionListRequest) (factorysessions.SessionListResult, error)
	Control(context.Context, factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error)
	ReadResult(context.Context, factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error)
	SubscribeResponses(context.Context, factorysessions.SessionResponseSubscriptionRequest) (factorysessions.SessionResponseSubscriptionResult, error)
	QueryDispatches(context.Context, factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error)
	StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error)
	StartSync(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error)
	ResumeInterruptedSession(context.Context, string, factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error)
	GetSession(context.Context, string) (factorysessions.SessionReadResult, error)
	Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Resume(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Cancel(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Terminate(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
	Approve(context.Context, string, factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error)
	RetryDispatch(context.Context, string, factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error)
	InterruptDispatch(context.Context, string, factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error)
	GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error)
	ListDispatches(context.Context, string) (factorysessions.ListDispatchesResult, error)
	GetDispatch(context.Context, string, string) (factorysessions.DispatchDetail, error)
	ListArtifacts(context.Context, string) (factorysessions.ListArtifactsResult, error)
	GetArtifact(context.Context, string, string) (factorysessions.ArtifactDetail, error)
	ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error)
	ListSessions(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.InvocationResult, error)
	ActivateNamedFactory(context.Context, string) error
	ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error)
	GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	GetFactorySessionSyncPreflight(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor, *factorydefinitions.FactorySessionLogicalResolveHint) (factorysessions.SyncPreflightResult, error)
	SubscribeFactoryResponseEvents(context.Context, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	ProbeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) error
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(context.Context, string, factorysessions.EventReconnectRequest) error
	ApplyLiveChange(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string) (factorysessions.LiveChangeResult, error)
}

// InvocationService is the completed invocation capability forwarded through
// the Factory Sessions assembly. Its engine lives in the invocation owner.
type InvocationService interface {
	SessionInvoker
	CanonicalSessionInvoker
	InvocationInputResolver
}
