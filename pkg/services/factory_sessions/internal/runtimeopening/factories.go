package runtimeopening

import (
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// ExternalEffects is the exact invocation-local effect set consumed while
// opening Factory Session runtime state. Wire projects the process edge
// aggregate into this owner-defined contract before it reaches a runtime
// opening consumer.
type ExternalEffects struct {
	Clock                     factoryruntime.Clock
	ProviderOverride          workers.Provider
	ModelPullMetricsRecorder  models.PullMetricsRecorder
	InvocationMetricsRecorder roles.InvocationMetricsRecorder
	ProviderCommandRunner     platformprocess.CommandRunner
	ScriptCommandRunner       platformprocess.CommandRunner
	SubmissionRecorder        recordings.SubmissionRecorder
	DispatchRecorder          recordings.DispatchRecorder
	RuntimeHostObserver              factorysessions.RuntimeHostObserver
	FactoryVisualizationSink         factoryvisualization.Sink
	FactoryVisualizationRootObserver factoryvisualization.RootObserver
	HostedClock               workers.HostedPollerClock
	HostedHTTPClient          workers.HostedPollerHTTPDoer
	HostedSecretResolver      workers.HostedPollerSecretResolver
	HostedLinearEndpoint      string
}

// The factory roles below are consumed only while opening a Factory Session
// runtime. Keeping them here makes the dependency direction explicit: Wire
// supplies implementations, while this package owns the operation signature it
// needs. They are aliases to function signatures so the remaining legacy Wire
// providers can be cut over without an intermediate adapter graph.
type WorkFactory = func(work.RuntimeResolver) work.Service

type AutomationFactory = func(
	*zap.Logger,
	factoryruntime.Clock,
	workers.CommandRunner,
	string,
	string,
	automations.HostedPollers,
) automations.Service

type FactorySessionExecutionFactory = func(
	string,
	factorysessions.PersistencePolicy,
	workers.Provider,
	factoryruntime.Clock,
	map[string]struct{},
	factoryruntime.JavaScriptWorkerSettings,
	*workers.MockWorkersConfig,
) (factorysessions.ExecutionService, error)

type ConductorInvocationWithProgressFactory = func(
	workers.ProviderRegistry,
	workers.CommandRunner,
	workers.PTYAllocator,
	workers.ProgressPublisher,
) (workers.InvocationExecutor, error)

type RecordingsProjectionFactory = func() recordings.ProjectionService

type RecordingsFactory = func(recordings.Ledger, recordings.ProjectionService) recordings.Service

type RuntimeLedgerFactory = func() factoryruntime.RuntimeLedgerFactory

type ReplayClockFactory = func(*factorydefinitions.ReplayArtifact) recordings.Clock

type WorkersRuntimeFactory = func(
	roles.CurrentRuntimeResolver,
	models.Service,
	models.RuntimeScopeRef,
	workers.CommandRunner,
	workers.CommandRunner,
	workers.PTYAllocator,
	*zap.Logger,
	bool,
	string,
	*bool,
	workers.Provider,
	func() time.Time,
	work.ContentMaterializer,
	[]providers.Integration,
) (workers.RuntimeService, error)

type AutomationHostedSourcesFactory = automations.HostedSourcesFactory

type WorkersLocalRuntimeHooksFactory = func() models.LocalRuntimeHooks

type FactoryDefinitionsFactory = func(
	factorysessions.DefinitionHost,
	factorydefinitions.DefinitionActivationGateway,
	factorydefinitions.Validator,
) factorydefinitions.Service

type DurableExecutionFactory func(
	factorydefinitions.RuntimeOpeningRequest,
	factorysessions.SessionRuntimeOpeningRequest,
	RuntimeRoot,
	factoryruntime.Clock,
	workers.Provider,
	*workers.MockWorkersConfig,
	FactorySessionExecutionFactory,
	factorysessions.ProviderIdentityResolver,
) (factorysessions.ExecutionService, error)

type WorkerExecutionFactory func(
	factoryruntime.RuntimeOpeningRequest,
	workers.RuntimeOpeningRequest,
	factoryruntime.Clock,
	*zap.Logger,
	workers.CommandRunner,
	workers.CommandRunner,
	workers.PTYAllocator,
	workers.Provider,
	roles.CurrentRuntimeResolver,
	models.Service,
	models.RuntimeScopeRef,
	work.Service,
	WorkersRuntimeFactory,
) (workers.RuntimeService, error)
