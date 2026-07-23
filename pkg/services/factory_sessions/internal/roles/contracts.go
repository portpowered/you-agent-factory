// Package roles contains implementation-facing Factory Sessions collaborator
// contracts. These roles are private to the owning service; public consumers
// depend on the root Service or service-owned transport adapters.
package roles

import (
	"context"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

type InvocationMetricsRecorder = factorysessions.InvocationMetricsRecorder

type ApplicationOpeningPorts = factorysessions.ApplicationOpeningPorts

type ApplicationOpeningRequest = factorysessions.ApplicationOpeningRequest

type RuntimeResources struct {
	Directory         string
	RuntimeInstanceID string
	BackendScopeID    string
	Diagnostics       factoryruntime.RuntimeLogDiagnostics
	Logger            *zap.Logger
	Close             func() error
}

type RuntimeHTTPServices = factorysessions.RuntimeHTTPServices

type RuntimeVisualizationServices struct {
	Reader      RuntimeReader
	Projections recordings.ProjectionService
}

type OpenedApplicationRuntime struct {
	Process       ProcessRuntime
	HTTP          RuntimeHTTPServices
	Visualization RuntimeVisualizationServices
	Resources     RuntimeResources
}

type OpenedProcessApplication struct {
	Plan        lifecycle.Plan
	Diagnostics factoryruntime.RuntimeLogDiagnostics
}

type OpenedInvocationRuntime struct {
	Workers        workers.Service
	Sessions       factorysessions.Service
	Invoker        SessionInvoker
	InputResolver  InvocationInputResolver
	Execution      factorysessions.ExecutionService
	Lifecycle      LifecycleRuntime
	CloseArtifacts func() error
}

type OpenedExecutionRuntime struct {
	Execution       factorysessions.ExecutionService
	WorkflowPreview factoryruntime.WorkflowPreviewOperation
	Resources       RuntimeResources
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

type ExecutionOpeningFileSystem = factorysessions.ExecutionOpeningFileSystem

type OwnedExecutionService interface {
	factorysessions.ExecutionService
	Close() error
}

type ExecutionServiceBuilder func(
	context.Context,
	string,
	string,
	string,
	string,
) (OwnedExecutionService, error)

type StdioApplication interface {
	Run(context.Context) error
}

type FixtureStdioApplicationBuilder func(
	context.Context,
	factorysessions.ExecutionService,
	io.Reader,
	io.Writer,
) (StdioApplication, error)

type RuntimeStdioApplicationBuilder func(
	context.Context,
	OpenedExecutionRuntime,
	io.Reader,
	io.Writer,
) (StdioApplication, error)

type StdioExecutionOpening interface {
	ResolveProjectRoot(string) (string, error)
	OpenExecutionRuntime(context.Context, factorysessions.ExecutionRuntimeOpeningRequest) (OpenedExecutionRuntime, error)
	Build(context.Context, string, string, string, string) (OwnedExecutionService, error)
}

type StdioOpeningOperation interface {
	OpenStdio(context.Context, factorysessions.StdioOpeningRequest) (StdioApplication, error)
}

type DirectJavaScriptRunOperation interface {
	Supports(string) bool
	Run(context.Context, factorysessions.DirectJavaScriptRunRequest) error
}

type DirectJavaScriptSyncRunner func(
	context.Context,
	factorysessions.ExecutionService,
	factorysessions.StartRequest,
	bool,
	io.Writer,
) error

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
	FindByLogicalSessionKeyID(string) *livesession.LiveSession
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
	CurrentRuntimeBundle() factoryruntime.HostedInstance
}

type ProcessRuntime interface {
	Start(context.Context, context.Context) error
	StartWorkers(context.Context) (factorysessions.RuntimeStop, error)
	RunTransport(context.Context, http.Handler) error
	Stop(context.Context) error
}

type ProcessRuntimeFactory interface {
	Bind(LifecycleRuntime, factorysessions.RuntimeHostRequest, factorysessions.RuntimeHostObserver, *zap.Logger) (ProcessRuntime, error)
}

type RuntimeHostOperation interface {
	Run(context.Context, http.Handler, LifecycleRuntime, *zap.Logger, factorysessions.RuntimeHostRequest, factorysessions.RuntimeHostObserver) error
}

type LifecyclePlanRequest struct {
	Runtime    ProcessRuntime
	Components factorysessions.BoundProcessComponents
	Close      func() error
}

type LifecyclePlanOperation func(LifecyclePlanRequest) (lifecycle.Plan, error)

type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
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
	InvokeFactory(context.Context, InvocationTarget, factorysessions.InvocationRequest, factorysessions.FactoryEventConsumer) (FactoryInvocationOutcome, error)
}

type InvocationTarget = factorysessions.InvocationTarget

type FactoryInvocationOutcome = factorysessions.FactoryInvocationOutcome

type ApplicationRuntime interface {
	LifecycleRuntime
	factoryruntime.Service
}

type RuntimeAssembly interface {
	CurrentRuntimeResolver
	RuntimeResolver
	RuntimeReader
	work.RuntimeResolver
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
	Complete(
		factoryRootDir string,
		clock factoryruntime.Clock,
		baseLogger *zap.Logger,
		logger *zap.Logger,
		runtimeBuild factoryruntime.ReplacementBuilder,
		startupRuntime factoryruntime.HostedInstance,
		startupSpec factoryruntime.SessionBuildSpec,
		runtimeLifecycle factoryruntime.Lifecycle,
		runtimeSidecars factorysessions.RuntimeSidecars,
		durableExecution factorysessions.ExecutionService,
		dir string,
		executionBaseDir string,
		runtimeMode factorydefinitions.RuntimeMode,
		backendScopeID string,
		workFile string,
		workflowID string,
		workstationLoader factorydefinitions.WorkstationLoader,
		loadFactory factorydefinitions.LoadedFactoryLoader,
		factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
		editableFactoryValidator factorysessions.EditableFactoryValidator,
		reconnectCursorValidator factorysessions.ReconnectCursorValidator,
		worldStateProjector factoryruntime.WorldStateProjector,
		invocationMetricsRecorder InvocationMetricsRecorder,
	) (ApplicationRuntime, factorysessions.Service, SessionInvoker, factorydefinitions.SessionHost, error)
}
