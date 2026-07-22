package factorysessions

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// RuntimeSidecars owns runtime-scoped background services without exposing
// their concrete host implementation to Factory Sessions consumers.
type RuntimeSidecars = factoryruntime.Sidecars

// ApplicationRuntime combines Factory Runtime operations with Factory
// Sessions lifecycle ownership without exposing the concrete session host.
type ApplicationRuntime interface {
	LifecycleRuntime
	factoryruntime.Service
}

type DefinitionHost = factorydefinitions.SessionHost

// RuntimeAssembly owns the staged construction of one live Factory Sessions
// service. The directory roles are usable while Models, Workers, and Factory
// Runtime are built; Complete attaches the built default runtime and publishes
// only root service roles.
type RuntimeAssembly interface {
	CurrentRuntimeResolver
	RuntimeResolver
	RuntimeReader
	work.RuntimeResolver
	InferenceProgressPublisherFactory(*zap.Logger) func(string) ProgressPublisher
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
		runtimeSidecars RuntimeSidecars,
		durableExecution ExecutionService,
		dir string,
		executionBaseDir string,
		runtimeMode factorydefinitions.RuntimeMode,
		backendScopeID string,
		workFile string,
		workflowID string,
		workstationLoader factorydefinitions.WorkstationLoader,
		loadFactory factorydefinitions.LoadedFactoryLoader,
		factoryScaffoldInitializer FactoryScaffoldInitializer,
		editableFactoryValidator EditableFactoryValidator,
		reconnectCursorValidator ReconnectCursorValidator,
		worldStateProjector factoryruntime.WorldStateProjector,
		invocationMetricsRecorder InvocationMetricsRecorder,
	) (ApplicationRuntime, Service, SessionInvoker, DefinitionHost, error)
}
