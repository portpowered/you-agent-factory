package wire

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/google/uuid"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	costswire "github.com/portpowered/infinite-you/pkg/services/costs/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

// providerOverrideService keeps an optional edge replacement distinct from
// the process Providers root in Wire's type graph. Both expose the same
// Providers-owned operations; the distinction is composition metadata, not a
// second provider contract.
type providerOverrideService = factorysessionwire.ProviderOverrideService

// compositeProcessLifecycle aggregates every inert service that retains
// resources lazily while commands execute into the single ProcessLifecycle
// Process.Close reaches. Each closer runs even if an earlier one fails, and
// every failure is joined into one returned error, so one resource's
// teardown failure never masks or skips another's.
type compositeProcessLifecycle struct {
	closers []func(context.Context) error
	once    sync.Once
	err     error
}

func (c *compositeProcessLifecycle) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		for _, closeFn := range c.closers {
			if closeFn == nil {
				continue
			}
			if err := closeFn(ctx); err != nil {
				c.err = errors.Join(c.err, err)
			}
		}
	})
	return c.err
}

// provideApplicationProcessLifecycle composes the process-wide shutdown path
// Process.Close reaches: the singular Factory Sessions root, the Models host
// supervisor, Providers lifecycle (executable/session teardown), the singular
// Events root (pkg/wire/events_providers.go). Factory Sessions owns every
// activated ACP runtime through its canonical process root, so its Close
// drains those sessions on process shutdown.
// The process-scoped direct Worker Sessions pool closes first so an admitted
// local invocation can publish its terminal observation before shared roots
// are closed.
func provideApplicationProcessLifecycle(
	service providers.Service,
	modelsService models.Service,
	eventsService events.Service,
	factorySessions factorysessions.Service,
	localWorkerSessions *localWorkerSessionsBoundary,
	metricsOwner factoryruntime.RuntimeMetricsOwner,
) (initializerapplication.ProcessLifecycle, error) {
	lifecycle, ok := service.(interface {
		Close(context.Context) error
	})
	if !ok {
		return nil, fmt.Errorf("construct application process: Providers lifecycle is required")
	}
	if modelsService == nil {
		return nil, fmt.Errorf("construct application process: Models service is required")
	}
	modelsLifecycle, lifecycleOK := modelsService.(interface {
		Close(context.Context) error
	})
	if !lifecycleOK {
		return nil, fmt.Errorf("construct application process: Models lifecycle is required")
	}
	eventsLifecycleValue, ok := eventsService.(eventsLifecycle)
	if !ok {
		return nil, fmt.Errorf("construct application process: Events lifecycle is required")
	}
	factorySessionsLifecycle, ok := factorySessions.(interface {
		Close(context.Context) error
	})
	if !ok {
		return nil, fmt.Errorf("construct application process: Factory Sessions lifecycle is required")
	}
	var closeMetrics func(context.Context) error
	if metricsOwner != nil {
		metricsLifecycle, lifecycleOK := metricsOwner.(interface {
			Close(context.Context) error
		})
		if !lifecycleOK {
			return nil, fmt.Errorf("construct application process: Runtime Metrics lifecycle is required")
		}
		closeMetrics = metricsLifecycle.Close
	}
	return &compositeProcessLifecycle{closers: []func(context.Context) error{
		func(ctx context.Context) error {
			// Wire treats the process-scoped direct Worker Sessions boundary as
			// required: construction returns an error before this lifecycle is
			// assembled when that boundary cannot be built.
			return localWorkerSessions.Close(ctx)
		},
		factorySessionsLifecycle.Close,
		modelsLifecycle.Close,
		lifecycle.Close,
		eventsLifecycleValue.Close,
		closeMetrics,
	}}, nil
}

func provideProvidersService(edges serviceedges.Edges) (providers.Service, error) {
	return provideConfiguredProvidersService(edges, nil, nil)
}

func provideConfiguredProvidersService(
	edges serviceedges.Edges,
	integrations []operatorsettings.ACPIntegration,
	workersRunner platformprocess.CommandRunner,
) (providers.Service, error) {
	agyPTYEffect, err := provideProvidersAgyPTYEffect(edges)
	if err != nil {
		return nil, err
	}
	configuration := providerswire.Configuration{
		ACPIntegrations:  projectACPIntegrations(integrations),
		CatalogOverrides: edges.ProviderCatalogCapabilityOverrides,
		Registrations:    edges.ProviderRegistrations,
	}
	catalogProbe := edges.ProviderCatalogProbe
	if catalogProbe == nil {
		catalogProbe = providerswire.IdentityCatalogProbe
	}
	if workersRunner != nil {
		contextualRunner := workerswire.NewContextualMockWorkerCommandRunner(
			workersRunner,
			provideWorkersAgentToolFileSystem(edges),
		)
		loggedRunner := providerCommandRunnerWithLogging(edges, contextualRunner)
		return newConfiguredProvidersService(configuration, catalogProbe, loggedRunner, agyPTYEffect, effectiveProviderCommandClock(edges), effectiveProviderScheduler(edges), logging.NoopLogger{},
			providePlatformProcessCommandFactory(edges), provideProvidersExecutableLocator(edges), provideProvidersStdioPipeFactory(edges), provideProvidersCodexPromptFiles(edges), provideProvidersCodexHomeResolver(edges))
	}
	if edges.ProviderCommandRunner != nil {
		contextualRunner := workerswire.NewContextualMockWorkerCommandRunner(
			edges.ProviderCommandRunner,
			provideWorkersAgentToolFileSystem(edges),
		)
		loggedRunner := providerCommandRunnerWithLogging(edges, contextualRunner)
		return newConfiguredProvidersService(configuration, catalogProbe, loggedRunner, agyPTYEffect, effectiveProviderCommandClock(edges), effectiveProviderScheduler(edges), logging.NoopLogger{},
			providePlatformProcessCommandFactory(edges), provideProvidersExecutableLocator(edges), provideProvidersStdioPipeFactory(edges), provideProvidersCodexPromptFiles(edges), provideProvidersCodexHomeResolver(edges))
	}
	commandRunner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	contextualRunner := workerswire.NewContextualMockWorkerCommandRunner(
		commandRunner,
		provideWorkersAgentToolFileSystem(edges),
	)
	loggedRunner := providerCommandRunnerWithLogging(edges, contextualRunner)
	return newConfiguredProvidersService(configuration, catalogProbe, loggedRunner, agyPTYEffect, effectiveProviderCommandClock(edges), effectiveProviderScheduler(edges), logging.NoopLogger{},
		providePlatformProcessCommandFactory(edges), provideProvidersExecutableLocator(edges), provideProvidersStdioPipeFactory(edges), provideProvidersCodexPromptFiles(edges), provideProvidersCodexHomeResolver(edges))
}

func provideProvidersExecutableLocator(edges serviceedges.Edges) platformprocess.ExecutableLocator {
	if edges.ProvidersExecutableLocator != nil {
		return edges.ProvidersExecutableLocator
	}
	return platformprocess.HostExecutableLocator{}
}

// provideProvidersStdioPipeFactory is the single canonical selection of the
// parent-owned ACP standard-stream channel implementation. An injected edge
// wins so functional tests can replace the exact channel effect; otherwise the
// policy-free host pipe channel is used. Nothing below this boundary is allowed
// to select a channel for itself.
func provideProvidersStdioPipeFactory(edges serviceedges.Edges) platformprocess.StdioPipeFactory {
	if edges.ProvidersStdioPipeFactory != nil {
		return edges.ProvidersStdioPipeFactory
	}
	return platformprocess.NewParentOwnedStdio
}

func providerCommandRunnerWithLogging(
	edges serviceedges.Edges,
	runner platformprocess.CommandRunner,
) platformprocess.CommandRunner {
	return workerswire.NewLoggingCommandRunner(
		runner,
		logging.NoopLogger{},
		effectiveProviderCommandClock(edges).Now,
	)
}

func effectiveProviderCommandClock(edges serviceedges.Edges) platformclock.Source {
	return edges.Clock
}

// Operational waits use the scheduler already selected at the process boundary.
func effectiveProviderScheduler(edges serviceedges.Edges) platformclock.TimerSource {
	return edges.ProcessScheduler
}

func projectACPIntegrations(integrations []operatorsettings.ACPIntegration) []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, 0, len(integrations))
	for _, integration := range integrations {
		result = append(result, providers.ACPIntegration{
			ID: integration.ID, Name: providers.ID(integration.Name),
			Transport: integration.Transport, Command: integration.Command,
		})
	}
	return result
}

func providePlatformProcessCommandFactory(edges serviceedges.Edges) platformprocess.CommandFactory {
	if edges.PlatformProcessCommandFactory != nil {
		return edges.PlatformProcessCommandFactory
	}
	return exec.Command
}

func provideProviderRegistry(
	_ serviceedges.Edges,
	providersService providers.Service,
) (initializerapplication.ProviderRegistry, error) {
	if providersService == nil {
		return nil, fmt.Errorf("construct provider identity projection: Providers service is required")
	}
	return providerIdentityProjection{service: providersService}, nil
}

type providerIdentityProjection struct {
	service providers.Service
}

func (projection providerIdentityProjection) CanonicalIdentity(identity string) (string, error) {
	resolved, err := projection.service.ResolveIdentity(
		context.Background(),
		providers.ResolveIdentityRequest{Identity: identity},
	)
	if err != nil {
		return "", err
	}
	return resolved.ID.String(), nil
}

// provideWorkersProviderCommandRunner resolves the shared provider CLI runner
// used by native executors and by migrated catalog Integrations on the
// conductor path. Injected edges win; otherwise the platform process runner is
// adapted once for both ownership boundaries.
func provideWorkersProviderCommandRunner(edges serviceedges.Edges) (platformprocess.CommandRunner, error) {
	if edges.ProviderCommandRunner != nil {
		return edges.ProviderCommandRunner, nil
	}
	defaultCommandRunner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return defaultCommandRunner, nil
}

func provideFactorySessionProviderIdentityResolver(
	providersService providers.Service,
) factorysessions.ProviderIdentityResolver {
	return func(identity string) (string, error) {
		if providersService == nil {
			return "", fmt.Errorf("Providers service is required")
		}
		resolved, err := providersService.ResolveIdentity(
			context.Background(),
			providers.ResolveIdentityRequest{Identity: identity},
		)
		if err != nil {
			return "", err
		}
		return resolved.ID.String(), nil
	}
}

func provideProviderSessions(writer recordings.WorkerRecordingWriter) (providersessions.Service, error) {
	captured, ok := writer.(recordings.WorkerCapturedActivityReader)
	if !ok || captured == nil {
		return nil, fmt.Errorf("provider-session captured activity reader is required")
	}
	return providersessionswire.NewService(captured)
}

func provideFactoryRuntimeIDGenerator(edges serviceedges.Edges) factoryruntime.IDGenerator {
	if edges.FactoryRuntimeIDGenerator != nil {
		return edges.FactoryRuntimeIDGenerator
	}
	return uuid.NewString
}

func provideFactoryRuntimeDirectories(edges serviceedges.Edges) factoryruntime.RuntimeDirectoryFileSystem {
	if edges.FactoryRuntimeDirectories != nil {
		return edges.FactoryRuntimeDirectories
	}
	return platformfilesystem.Local{}
}

func provideFactoryRuntimeInputs(edges serviceedges.Edges) factoryruntimewire.InputFileSystem {
	if edges.FactoryRuntimeInputs != nil {
		return edges.FactoryRuntimeInputs
	}
	return platformfilesystem.Local{}
}

func provideFactoryRuntimeInputDirectoryWalker(edges serviceedges.Edges) factoryruntime.InputDirectoryWalker {
	if edges.FactoryRuntimeInputDirectoryWalker != nil {
		return edges.FactoryRuntimeInputDirectoryWalker
	}
	return filepath.WalkDir
}

func provideFactoryRuntimeWorkflowSources(edges serviceedges.Edges) factoryruntime.WorkflowSourceFileSystem {
	if edges.FactoryRuntimeWorkflowSources != nil {
		return edges.FactoryRuntimeWorkflowSources
	}
	return platformfilesystem.Local{}
}

func provideFactoryRuntimeWorkflowSourceResolveSymlinks(edges serviceedges.Edges) factoryruntime.WorkflowSourceResolveSymlinks {
	if edges.FactoryRuntimeWorkflowSourceResolveSymlinks != nil {
		return edges.FactoryRuntimeWorkflowSourceResolveSymlinks
	}
	return filepath.EvalSymlinks
}

func provideFactoryRuntimeWorkflowHome(edges serviceedges.Edges) factoryruntime.WorkflowHomeResolver {
	if edges.FactoryRuntimeWorkflowHome != nil {
		return edges.FactoryRuntimeWorkflowHome
	}
	return os.UserHomeDir
}

func provideJavaScriptWorkflows(
	files factoryruntime.WorkflowSourceFileSystem,
	resolveHome factoryruntime.WorkflowHomeResolver,
	resolveSymlinks factoryruntime.WorkflowSourceResolveSymlinks,
) factoryruntime.JavaScriptWorkflows {
	return factoryruntimewire.NewJavaScriptWorkflows(files, resolveHome, resolveSymlinks)
}

func provideJavaScriptWorkflowDefinitions(
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.JavaScriptWorkflowDefinitions {
	return workflows
}

func provideWorkflowPreviewOperation(
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.WorkflowPreviewOperation {
	return workflows
}

func provideOrchestratorDefinitionValidator(
	workflows factoryruntime.JavaScriptWorkflows,
) factorydefinitions.OrchestratorDefinitionValidator {
	return factoryruntimewire.NewOrchestratorDefinitionValidator(workflows)
}

func provideFactoryDefinitionValidationService(
	workflows factoryruntime.JavaScriptWorkflows,
	compilation factorydefinitionswire.Compilation,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) factorydefinitions.ValidationOperations {
	_ = workflows
	return factorydefinitionswire.NewValidationOperations(
		orchestratorValidator,
		compilation.LoadCanonicalFactorySource,
	)
}

func provideFactoryDefinitionValidator(
	service factorydefinitions.ValidationOperations,
) factorydefinitions.Validator {
	return service
}

func provideDefinitionValidationOperation(
	service factorydefinitions.ValidationOperations,
) factorydefinitions.DefinitionValidationOperation {
	return service
}

func provideSubmittedDefinitionValidationOperation(
	service factorydefinitions.ValidationOperations,
) factorydefinitions.SubmittedDefinitionValidationOperation {
	return service
}

func provideLoadedFactorySourceFactory() factorydefinitions.LoadedFactorySourceFactory {
	return factorydefinitionswire.LoadedFactorySourceFactory()
}

func provideNamedFactoryCatalog(
	namedPaths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	return factorydefinitionswire.NewNamedFactoryCatalog(namedPaths, fileSystem)
}

func provideFactoryDefinitionPersistence(
	validator factorydefinitions.Validator,
	loader *factorydefinitionswire.Loader,
	pruneRemovedDocs factorydefinitions.PortableBundledDocsPruner,
	materializeFiles factorydefinitions.PortableBundledFilesMaterializer,
	validateWrites factorydefinitions.PortableBundledFileWritesValidator,
	copySupportedFiles factorydefinitions.PortableBundledFilesCopier,
	writer *definitionsPersistenceWriter,
	persistenceFileSystem factorydefinitions.PersistenceFileSystem,
	namedPaths factorydefinitions.NamedPathResolver,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
	decode factorydefinitions.FactoryConfigJSONDecoder,
	encode factorydefinitions.FactoryConfigJSONEncoder,
) (factorydefinitions.PackagedFactoryPersistence, error) {
	return factorydefinitionswire.Persistence(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(payload)
		},
		loader,
		pruneRemovedDocs,
		materializeFiles,
		validateWrites,
		copySupportedFiles,
		(*factorydefinitionswire.AuthoredLayoutWriter)(writer),
		persistenceFileSystem,
		namedPaths,
		directoryReplacementStore,
		decode,
		encode,
	)
}

func provideNamedFactoryPersistenceOperation(
	persistence factorydefinitions.Persistence,
) factorydefinitions.NamedFactoryPersistenceOperation {
	return persistence.PersistNamedFactory
}

func provideFactoryScaffoldInitializer(
	initialize factorydefinitions.ScaffoldInitializer,
) factorysessions.FactoryScaffoldInitializer {
	return func(factoryDir string) error {
		return initialize(factorydefinitions.ScaffoldConfig{Dir: factoryDir})
	}
}
func provideEditableFactoryValidator(
	validator factorydefinitions.DefinitionValidationOperation,
) factorysessions.EditableFactoryValidator {
	return func(
		ctx context.Context,
		snapshot *factorydefinitions.FactorySnapshot,
		workstationLoader factorydefinitions.WorkstationLoader,
	) error {
		return factorydefinitionswire.ValidateEditableSnapshot(
			ctx,
			snapshot,
			workstationLoader,
			func(
				snapshot *factorydefinitions.FactorySnapshot,
				workstationLoader factorydefinitions.WorkstationLoader,
			) (factorydefinitions.DefinitionValidationRequest, error) {
				return validationentry.MapEditableFactorySnapshot(snapshot, workstationLoader)
			},
			validator,
		)
	}
}

func provideInitialFactorySnapshotFactory(
	prepare factorydefinitions.PortableFactoryConfigPreparer,
	capture factorydefinitions.LoadedFactorySnapshotCapturer,
) factorydefinitions.InitialFactorySnapshotFactory {
	return func(loaded factorydefinitions.LoadedFactorySource) (*factorydefinitions.FactorySnapshot, error) {
		return factorydefinitionswire.CaptureInitialSnapshot(loaded, prepare, capture)
	}
}

func provideAutomationsRoot(service *automationswire.Owner) automations.Root {
	return automationswire.NewRoot(service)
}

func provideFactorySessionResponseEventRetentionLimits(
	edges serviceedges.Edges,
) *factorysessions.ResponseEventRetentionLimits {
	return edges.FactorySessionResponseEventRetentionLimits
}

// provideFactorySessionReconnectValidator preserves Recordings cursor validation.
func provideFactorySessionReconnectValidator(projections recordings.ProjectionService) factorysessions.ReconnectCursorValidator {
	return projections.ValidateReconnectReplay
}

// provideInvocationWorldStateProjector binds the already-injected Recordings projection.
func provideInvocationWorldStateProjector(projections recordings.ProjectionService) factoryruntime.WorldStateProjector {
	return projections.ReconstructFactoryWorldState
}

// provideSessionCheckpointStoreFactory supplies session-owned checkpoint state.
func provideSessionCheckpointStoreFactory() factoryruntime.JavaScriptCheckpointStoreFactory {
	return func() factoryruntime.JavaScriptCheckpointStore {
		return factoryruntimewire.NewJavaScriptCheckpointStore()
	}
}

// provideFactorySessionsCapability publishes the already-composed Sessions root
// through the neutral process capability. The initializer retains the opaque
// value without importing the Sessions service; pkg/root reifies it at the
// caller-facing boundary.
func provideFactorySessionsCapability(
	service factorysessions.Service,
) (processcontract.FactorySessionsCapability, error) {
	if service == nil {
		return nil, fmt.Errorf("construct factory sessions capability: service is required")
	}
	return factorySessionsCapability{service: service}, nil
}

type factorySessionsCapability struct {
	service factorysessions.Service
}

func (capability factorySessionsCapability) FactorySessions() any {
	return capability.service
}

// provideFactoryVisualizationMetricsQuery composes the one process-scoped,
// stateless query from the narrow Platform Metrics reader. The artifact root
// remains a per-call request value, so this construction does not retain
// runtime/session state.
func provideFactoryVisualizationMetricsQuery(
	logger logging.Logger,
) (factoryvisualization.RuntimeMetricsQuery, error) {
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		return nil, err
	}
	return factoryvisualizationwire.NewRuntimeMetricsQuery(
		reader,
		logger,
	)
}

func provideRuntimeMetricsQueryCapability(
	query factoryvisualization.RuntimeMetricsQuery,
) (processcontract.RuntimeMetricsQueryCapability, error) {
	if query == nil {
		return nil, errors.New("construct runtime metrics query capability: query is required")
	}
	return runtimeMetricsQueryCapability{query: query}, nil
}

// provideProviderPriceTableReader exposes the immutable pricing facts owned by
// Providers. It is separate from the configurable Operator Settings document.
func provideProviderPriceTableReader() (costs.PriceTableReader, error) {
	return providerswire.NewPriceTableReader()
}

// provideCostsQuery composes valuation over canonical metrics, immutable
// Providers pricing facts, and the singular Operator Settings root. Costs
// receives the explicit settings path per request and does not read files.
func provideCostsQuery(
	pricing costs.PriceTableReader,
	settings operatorsettings.Service,
	metrics factoryvisualization.RuntimeMetricsQuery,
	logger logging.Logger,
) (costs.CostsQuery, error) {
	return costswire.NewCostsQuery(pricing, settings, metrics, logger)
}

func provideCostsQueryCapability(
	query costs.CostsQuery,
) (processcontract.RuntimeCostsQueryCapability, error) {
	return runtimeCostsQueryCapability{query: query}, nil
}

type runtimeCostsQueryCapability struct {
	query costs.CostsQuery
}

func (capability runtimeCostsQueryCapability) RuntimeCostsQuery() any {
	return capability.query
}

type runtimeMetricsQueryCapability struct {
	query factoryvisualization.RuntimeMetricsQuery
}

func (capability runtimeMetricsQueryCapability) RuntimeMetricsQuery() any {
	return capability.query
}

func provideOrchestrationJavaScriptExecution(service factoryruntimewire.Orchestration) factoryruntime.OrchestrationJavaScriptExecution {
	return factoryruntimewire.NewOrchestrationJavaScriptExecution(service)
}

func provideOrchestrationCompilation(service factoryruntimewire.Orchestration) factoryruntime.OrchestrationCompilation {
	return factoryruntimewire.NewOrchestrationCompilation(service)
}

// provideProcessDurableExecution selects the established process child mode
// from reachable provider effects, then forwards complete owner collaborators.
func provideProcessDurableExecution(
	resolveHome factorysessions.HomeDirectoryResolver,
	stores factorysessionwire.RuntimePersistenceStoreFactory,
	clock factoryruntime.Clock,
	syncWaits factorysessionwire.SyncWaitScheduler,
	checkpointSummaries factoryruntime.JavaScriptCheckpointSummaries,
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	writer recordings.PortableRecordingWriter,
	sessionIDs factorysessions.SessionIDGenerator,
	responseIDs factorysessions.ResponseEventIDGenerator,
	responses factorysessionwire.ResponseStreams,
	liveChange factorysessionwire.LiveChangeCoordinator,
	scope factorysessionwire.ProcessDurableScope,
	workerService workers.Service,
	providerOverride providerOverrideService,
	allocator providerswire.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	logger *zap.Logger,
) (factorysessionwire.DurableExecutionService, error) {
	mode := factorysessions.ChildExecutorModeFake
	if providerOverride != nil || (adaptRunner != nil && allocator != nil) {
		mode = factorysessions.ChildExecutorModeLive
	}
	return factorysessionwire.NewProcessDurableExecution(
		resolveHome, mode, stores, clock, syncWaits,
		checkpointSummaries, workflows, orchestration,
		writer, sessionIDs, responseIDs, responses, liveChange, scope, workerService,
		providerOverride, logger,
	)
}

func provideFactorySessionSyncWaitScheduler(edges serviceedges.Edges) factorysessionwire.SyncWaitScheduler {
	return edges.ProcessScheduler
}

func providePortableRecordingWriter(edges serviceedges.Edges) (recordings.PortableRecordingWriter, error) {
	makeDirectories, createTemporaryFile, removePath, renamePath, _ := provideRecordingFilesystemEffects(edges)
	return recordingswire.NewPortableRecordingWriter(makeDirectories, createTemporaryFile, removePath, renamePath)
}

func provideRecordingFilesystemEffects(
	edges serviceedges.Edges,
) (
	recordings.RecordingMakeDirectories,
	recordings.RecordingCreateTemporaryFile,
	recordings.RecordingRemovePath,
	recordings.RecordingRenamePath,
	recordings.RecordingReadFile,
) {
	makeDirectories := edges.RecordingMakeDirectories
	if makeDirectories == nil {
		makeDirectories = os.MkdirAll
	}
	createTemporaryFile := edges.RecordingCreateTempFile
	if createTemporaryFile == nil {
		createTemporaryFile = func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		}
	}
	removePath := edges.RecordingRemovePath
	if removePath == nil {
		removePath = os.Remove
	}
	renamePath := edges.RecordingRenamePath
	if renamePath == nil {
		renamePath = (platformfilesystem.Local{
			AllowRenameReplacement: runtime.GOOS == "windows",
		}).RenameReplacing
	}
	readFile := edges.RecordingReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	return makeDirectories, createTemporaryFile, removePath, renamePath, readFile
}

func provideLoadedFactorySnapshotCapturer() factorydefinitions.LoadedFactorySnapshotCapturer {
	return factorydefinitionswire.LoadedFactorySnapshotCapturer()
}

// Narrow the already-composed peers without allocating another service.
func provideRuntimeModelFactoryConfigReader(assembly factorysessionwire.RuntimeAssembly) factorysessionwire.RuntimeModelFactoryConfigReader {
	return assembly
}

func provideRuntimeModelWorkerExecution(workerService workers.Service) factorysessionwire.RuntimeModelWorkerExecution {
	return workerService
}
