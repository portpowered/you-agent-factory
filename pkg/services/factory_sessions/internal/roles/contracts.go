// Package roles contains implementation-facing Factory Sessions collaborator
// contracts. These roles are private to the owning service; public consumers
// depend on the root Service or service-owned transport adapters.
package roles

import (
	"context"
	"io"
	"net/http"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

type RuntimeResources struct {
	Directory         string
	RuntimeInstanceID string
	BackendScopeID    string
	Clock             factoryruntime.Clock
	Diagnostics       factoryruntime.RuntimeLogDiagnostics
	Logger            *zap.Logger
	Close             func() error
}

type RuntimeVisualizationServices struct {
	Reader      RuntimeReader
	Projections recordings.ProjectionService
}

type OpenedApplicationRuntime struct {
	Process ProcessRuntime
	// Cancellation is the explicit invocation-local stop authority published
	// into the opened HTTP view by the application-opening operation.
	Cancellation initializer.InvocationCancellation
	// OrderlyStop is an already-bound process lifecycle operation. Factory
	// Sessions supplies the Recordings-owned implementation when a live
	// recording exists; Initializer only orders and awaits it.
	OrderlyStop        lifecycle.OrderlyStopOperation
	FactoryRuntime     factoryruntime.Service
	FactoryDefinitions factorydefinitions.Service
	WorkflowPreview    factoryruntime.WorkflowPreviewOperation
	FactorySessions    factorysessions.Service
	Recordings         recordings.Service
	LiveControl        factorysessions.LiveControlService
	Work               work.Service
	Models             models.Service
	ModelsScope        models.RuntimeScopeRef
	ModelInvoker       workers.ModelInvoker
	Workers            workers.Service
	ProviderSessions   providersessions.Service
	WorkerSessions     workersessions.ObservationService
	WorkerPrompts      workers.PromptTemplates
	// OperatorSettingsPath is the resolved document used by the opened
	// runtime. Transport adapters receive it as data and never resolve or read
	// configuration themselves.
	OperatorSettingsPath   string
	Logger                 *zap.Logger
	Visualization          RuntimeVisualizationServices
	Resources              RuntimeResources
	HistoricalReplay       *factorysessions.HistoricalReplayInspection
	ReplayMetadataWarnings []recordings.MetadataMismatchWarning
}

type OpenedProcessApplication struct {
	Plan                   lifecycle.Plan
	Diagnostics            factoryruntime.RuntimeLogDiagnostics
	Ready                  <-chan initializer.RuntimeHostBinding
	CleanInvocation        factoryruntime.Service
	ReplayMetadataWarnings []recordings.MetadataMismatchWarning
	// HostedInvocation is a narrow operation result for the hosted CLI path;
	// it is not the opened runtime's HTTP service table.
	HostedInvocation HostedInvocationOperation
	HistoricalReplay *factorysessions.HistoricalReplayInspection
}

type OpenedInvocationRuntime struct {
	Workers        workers.Service
	ModelInvoker   workers.ModelInvoker
	Sessions       factorysessions.Service
	LiveControl    factorysessions.LiveControlService
	Invoker        SessionInvoker
	InputResolver  InvocationInputResolver
	Execution      durableexecution.Service
	Lifecycle      LifecycleRuntime
	ModelsScope    models.RuntimeScopeRef
	RuntimeID      string
	GenerationID   string
	CloseArtifacts func() error
}

type OpenedExecutionRuntime struct {
	Execution       durableexecution.Service
	Recordings      recordings.Service
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
	durableexecution.Service
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
	durableexecution.Service,
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
	OpenStdio(
		context.Context,
		factorysessions.StdioOpeningRequest,
	) (StdioApplication, error)
}

type DirectJavaScriptRunOperation interface {
	Supports(string) bool
	Open(
		context.Context,
		factorysessions.DirectJavaScriptRunRequest,
		initializer.InvocationCancellation,
	) (factorysessions.DirectJavaScriptApplication, error)
}

type DirectJavaScriptHostAdapter func(
	OwnedExecutionService,
	factorysessions.RuntimeHostRequest,
	initializer.InvocationCancellation,
	factorysessions.RuntimeHostObserver,
) (lifecycle.Component, error)

type DirectJavaScriptSyncRunner func(
	context.Context,
	durableexecution.Service,
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
	CurrentRuntimeBundle() runtimeports.RuntimeInstance
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
	Runtime     ProcessRuntime
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
		runtimeBuild runtimeports.RuntimeReplacementBuilder,
		startupRuntime runtimeports.RuntimeInstance,
		modelsScope models.RuntimeScopeRef,
		startupSpec factoryruntime.SessionBuildSpec,
		runtimeLifecycle runtimeports.RuntimeLifecycle,
		runtimeSidecars factorysessions.RuntimeSidecars,
		durableExecution durableexecution.Service,
		factoryDefinitions factorydefinitions.Service,
		factorySessionID string,
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
	) (ApplicationRuntime, factorysessions.Service, SessionInvoker, factorydefinitions.SessionHost, factorydefinitions.DefinitionActivationGateway, error)
}
