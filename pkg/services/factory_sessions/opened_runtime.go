package factorysessions

import (
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// SessionRuntimeOpeningRequest contains Factory Session identity,
// persistence, startup, and hosting values for one runtime.
type SessionRuntimeOpeningRequest struct {
	PersistencePolicy PersistencePolicy
	BackendScopeID    string
	SystemConfigHome  string
	SystemConfigPath  string
	WorkFile          string
	Host              RuntimeHostRequest
}

// RuntimeOpeningRequest is the Factory Sessions operation input assembled from
// bounded owner requests. The complete service graph remains injected; this
// value carries only per-runtime customer and process selections.
type RuntimeOpeningRequest struct {
	FactoryDefinition factorydefinitions.RuntimeOpeningRequest
	FactoryRuntime    factoryruntime.RuntimeOpeningRequest
	FactorySession    SessionRuntimeOpeningRequest
	Workers           workers.RuntimeOpeningRequest
	Recordings        recordings.RuntimeOpeningRequest
	Models            models.RuntimeOpeningRequest
	OperatorDefaults  operatorsettings.ResolvedDefaults
}

// ApplicationOpeningPorts are the invocation-local external effects that may
// differ for one process application. The process edge aggregate is resolved
// by Wire and remains inside the Factory Sessions application-opening
// operation; Initializer and transports carry only these owner-defined ports.
type ApplicationOpeningPorts struct {
	InvocationMetricsRecorder InvocationMetricsRecorder
	RuntimeHostObserver       RuntimeHostObserver
}

// ApplicationOpeningRequest is the complete owner-bounded input for opening
// one lifecycle-ready process application.
type ApplicationOpeningRequest struct {
	Runtime *RuntimeOpeningRequest
	Ports   ApplicationOpeningPorts
}

// RuntimeResources are the immutable diagnostics and cleanup edge for one
// opened Factory Session runtime.
type RuntimeResources struct {
	Directory         string
	RuntimeInstanceID string
	BackendScopeID    string
	Diagnostics       factoryruntime.RuntimeLogDiagnostics
	Logger            *zap.Logger
	Close             func() error
}

// RuntimeHTTPServices are the exact service-root roles used to bind the HTTP
// and mapping surface for one opened runtime.
type RuntimeHTTPServices struct {
	FactoryRuntime     factoryruntime.Service
	FactoryDefinitions factorydefinitions.Service
	WorkflowPreview    factoryruntime.WorkflowPreviewOperation
	FactorySessions    Service
	SessionInvocation  SessionInvoker
	SessionExecution   ExecutionService
	Work               work.Service
	Models             models.Service
	Workers            workers.Service
	ProviderSessions   providersessions.Service
	WorkerPrompts      workers.PromptTemplates
	Logger             *zap.Logger
}

// RuntimeVisualizationServices are the exact read roles used by Factory
// Visualization.
type RuntimeVisualizationServices struct {
	Reader      RuntimeReader
	Projections recordings.ProjectionService
}

// OpenedApplicationRuntime is the lifecycle application view consumed while
// constructing an Initializer plan.
type OpenedApplicationRuntime struct {
	Process       ProcessRuntime
	HTTP          RuntimeHTTPServices
	Visualization RuntimeVisualizationServices
	Resources     RuntimeResources
}

// OpenedProcessApplication contains only the lifecycle-ready values produced
// by the Factory Sessions application-opening operation. It intentionally
// carries no product services, external-edge aggregate, adapter, or deferred
// constructor.
type OpenedProcessApplication struct {
	Plan        lifecycle.Plan
	Diagnostics factoryruntime.RuntimeLogDiagnostics
}

// OpenedInvocationRuntime is the exact view consumed by the invocation
// operation. It contains no unrelated HTTP, visualization, or model catalog
// roles.
type OpenedInvocationRuntime struct {
	Workers        workers.Service
	Sessions       Service
	Invoker        SessionInvoker
	InputResolver  InvocationInputResolver
	Execution      ExecutionService
	Lifecycle      LifecycleRuntime
	CloseArtifacts func() error
}

// OpenedExecutionRuntime is the runtime-backed Factory Session execution/MCP
// view.
type OpenedExecutionRuntime struct {
	Execution       ExecutionService
	WorkflowPreview factoryruntime.WorkflowPreviewOperation
	Resources       RuntimeResources
}
