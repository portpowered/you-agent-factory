// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package wire

import (
	"context"
	"database/sql"
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
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
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
// Events root (pkg/wire/events_providers.go), and the on-demand Factory
// Sessions activation the production ACP prompt-delegation consumer lazily
// opens runtimes through (see provideACPServerFactoryTarget) -- so every
// runtime retained by the application has a deterministic close on process
// shutdown.
// The process-scoped direct Worker Sessions pool closes first so an admitted
// local invocation can publish its terminal observation before shared roots
// are closed.
func provideApplicationProcessLifecycle(
	service providers.Service,
	modelsService models.Service,
	eventsService events.Service,
	factorySessions factorysessions.Service,
	factoryTarget *factorysessionwire.OnDemandFactoryTargetService,
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
		func(context.Context) error {
			if factoryTarget == nil {
				return nil
			}
			return factoryTarget.Close()
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
	agyPTYPlatform, err := provideProvidersAgyPTYPlatform(edges)
	if err != nil {
		return nil, err
	}
	options := []providerswire.Option{
		providerswire.WithAgyPTY(agyPTYPlatform),
		providerswire.WithAgyCommandClock(effectiveProviderCommandClock(edges)),
		providerswire.WithCommandFactory(providePlatformProcessCommandFactory(edges)),
		providerswire.WithExecutableLocator(edges.ProvidersExecutableLocator),
		providerswire.WithACPIntegrations(projectACPIntegrations(integrations)...),
		providerswire.WithCatalogCapabilityOverrides(edges.ProviderCatalogCapabilityOverrides...),
		providerswire.WithRegistrations(edges.ProviderRegistrations...),
	}
	if workersRunner != nil {
		contextualRunner := workerswire.NewContextualMockWorkerCommandRunner(
			workersRunner,
			provideWorkersAgentToolFileSystem(edges),
		)
		loggedRunner := providerCommandRunnerWithLogging(edges, contextualRunner)
		options = append(options, providerswire.WithWorkersCommandRunner(
			workerswire.NewProviderCommandRunner(loggedRunner),
		))
		return newConfiguredProvidersService(options, loggedRunner)
	}
	if edges.ProviderCommandRunner != nil {
		contextualRunner := workerswire.NewContextualMockWorkerCommandRunner(
			edges.ProviderCommandRunner,
			provideWorkersAgentToolFileSystem(edges),
		)
		loggedRunner := providerCommandRunnerWithLogging(edges, contextualRunner)
		options = append(options, providerswire.WithCommandRunner(edges.ProviderCommandRunner))
		options = append(options, providerswire.WithWorkersCommandRunner(
			workerswire.NewProviderCommandRunner(loggedRunner),
		))
		return newConfiguredProvidersService(options, loggedRunner)
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
	options = append(options, providerswire.WithCommandRunner(commandRunner))
	options = append(options, providerswire.WithWorkersCommandRunner(
		workerswire.NewProviderCommandRunner(loggedRunner),
	))
	return newConfiguredProvidersService(options, loggedRunner)
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
	if edges.Clock != nil {
		return edges.Clock
	}
	return platformclock.Real{}
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

func resolveWorkersOperatingSystem(edges serviceedges.Edges) workers.OperatingSystem {
	if edges.WorkersOperatingSystem != "" {
		return edges.WorkersOperatingSystem
	}
	return workers.OperatingSystem(runtime.GOOS)
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

func provideProviderSessions(edges serviceedges.Edges) (providersessions.Service, error) {
	files := edges.ProviderSessionFileSystem
	if files == nil {
		files = platformfilesystem.Local{}
	}
	resolveHome := edges.ProviderSessionResolveHomeDirectory
	if resolveHome == nil {
		resolveHome = os.UserHomeDir
	}
	codexWalkDirectory := edges.ProviderSessionCodexWalkDirectory
	if codexWalkDirectory == nil {
		codexWalkDirectory = providersessionswire.CodexWalkDirectory(filepath.WalkDir)
	}
	codexResolveSymlinks := edges.ProviderSessionCodexResolveSymlinks
	if codexResolveSymlinks == nil {
		codexResolveSymlinks = providersessionswire.CodexResolveSymlinks(filepath.EvalSymlinks)
	}
	cursorWalkDirectory := edges.ProviderSessionCursorWalkDirectory
	if cursorWalkDirectory == nil {
		cursorWalkDirectory = providersessionswire.CursorWalkDirectory(filepath.WalkDir)
	}
	cursorResolveSymlinks := edges.ProviderSessionCursorResolveSymlinks
	if cursorResolveSymlinks == nil {
		cursorResolveSymlinks = providersessionswire.CursorResolveSymlinks(filepath.EvalSymlinks)
	}
	cursorOpenDatabase := edges.ProviderSessionCursorOpenDatabase
	if cursorOpenDatabase == nil {
		cursorOpenDatabase = providersessionswire.CursorOpenSQLDatabase(sql.Open)
	}
	operatingSystem := edges.ProviderSessionOperatingSystem
	if operatingSystem == "" {
		operatingSystem = runtime.GOOS
	}
	return providersessionswire.NewService(
		files,
		resolveHome,
		codexWalkDirectory,
		codexResolveSymlinks,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		providersessionswire.OperatingSystem(operatingSystem),
	)
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
	loader *factorydefinitionswire.Loader,
	orchestratorValidator factorydefinitions.OrchestratorDefinitionValidator,
) factorydefinitions.ValidationOperations {
	_ = workflows
	return factorydefinitionswire.NewValidationOperations(
		orchestratorValidator,
		loader.LoadSourceFromCanonicalJSON,
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
	fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem,
	ensureInbox factorydefinitions.InputInboxSentinelEnsurer,
	persistenceFileSystem factorydefinitions.PersistenceFileSystem,
	namedPaths factorydefinitions.NamedPathResolver,
	directoryReplacementStore factorydefinitions.DirectoryReplacementStore,
) (factorydefinitions.PackagedFactoryPersistence, error) {
	return factorydefinitionswire.Persistence(
		validator,
		func(payload []byte) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapFactoryJSONForPersistence(
				payload,
				loader.LoadSourceFromCanonicalJSON,
			)
		},
		loader,
		pruneRemovedDocs,
		materializeFiles,
		validateWrites,
		copySupportedFiles,
		fileSystem,
		ensureInbox,
		persistenceFileSystem,
		namedPaths,
		directoryReplacementStore,
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
	loader *factorydefinitionswire.Loader,
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
				return validationentry.MapEditableFactorySnapshot(
					snapshot,
					workstationLoader,
					loader.LoadSourceFromCanonicalJSON,
				)
			},
			validator,
		)
	}
}

func provideInitialFactorySnapshotFactory(
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
) factorydefinitions.InitialFactorySnapshotFactory {
	return func(
		loaded factorydefinitions.LoadedFactorySource,
	) (*factorydefinitions.FactorySnapshot, error) {
		return factorydefinitionswire.CaptureInitialSnapshot(
			loaded,
			factorydefinitionswire.PortableFactoryConfigPreparer(
				applySupportedFiles,
				applyStarterWork,
			),
			factorydefinitionswire.LoadedFactorySnapshotCapturer(),
		)
	}
}

func provideAutomationsRoot(
	hostedSourceInputs automationswire.HostedSourceInputs,
	logger *zap.Logger,
	clock factoryruntime.Clock,
	commandRunner platformprocess.CommandRunner,
	workstationExecution factorydefinitions.WorkstationExecutionPolicyService,
) (automations.Root, error) {
	return automationswire.NewRoot(
		logger,
		clock,
		commandRunner,
		"",
		"",
		hostedSourceInputs,
		workerswire.ResolveTemplateFields,
		workstationExecution,
	)
}

func provideFactorySessionResponseEventRetentionLimits(
	edges serviceedges.Edges,
) *factorysessions.ResponseEventRetentionLimits {
	return edges.FactorySessionResponseEventRetentionLimits
}

func provideFactorySessionsAssembly(
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directories factorysessionwire.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles factorysessionwire.InvocationInputReader,
	initialWorkFiles factorysessionwire.InitialWorkReader,
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	eventsService events.Service,
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessionwire.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) (factorysessionwire.RuntimeAssembly, error) {
	return factorysessionwire.NewRuntimeAssembly(func() factoryruntime.JavaScriptCheckpointStore {
		return factoryruntimewire.NewJavaScriptCheckpointStore()
	}, sessionResultProjection, interpolation, invocationWorkTypes, ttsObservability, eventIDs, responseEventRetentionLimits, sessionIDs, resolveHome, directories, namedPaths, invocationInputFiles, initialWorkFiles, resolveSymlinks, eventsService, clock, liveChangeCoordinator, recordedSessionInventory)
}

func provideFactorySessionsService(
	assembly factorysessionwire.RuntimeAssembly,
	opening *factorysessionwire.RuntimeOpening,
	liveChangeCoordinator factorysessionwire.LiveChangeCoordinator,
) (factorysessions.Service, error) {
	return factorysessionwire.NewServiceFromAssembly(assembly, opening, liveChangeCoordinator)
}

// provideFactorySessionRuntimeOpeningAdapter binds the temporary opening
// seams to the already-published Factory Sessions root. The adapter is a
// value-only compatibility view and does not own another graph or lifecycle.
func provideFactorySessionRuntimeOpeningAdapter(
	service factorysessions.Service,
) (*factorysessionwire.RuntimeOpeningAdapter, error) {
	return factorysessionwire.NewRuntimeOpeningAdapter(service)
}

// provideFactorySessionDetachedOperations publishes the one detached value
// capability from the already-composed Sessions root. The neutral process
// wrapper keeps initializer/application independent of product services while
// preserving the concrete Sessions view for pkg/root callers.
func provideFactorySessionDetachedOperations(
	service factorysessions.Service,
) (processcontract.DetachedOperationsCapability, error) {
	operations, err := factorysessionwire.NewDetachedOperations(service)
	if err != nil {
		return nil, err
	}
	return detachedOperationsCapability{operations: operations}, nil
}

type detachedOperationsCapability struct {
	operations factorysessions.DetachedService
}

func (capability detachedOperationsCapability) DetachedOperations() any {
	return capability.operations
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
	if query == nil {
		return nil, errors.New("construct runtime costs query capability: query is required")
	}
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

func provideFactorySessionExecutionRuntimeOpening(
	opening *factorysessionwire.RuntimeOpeningAdapter,
) (processcontract.ExecutionRuntimeOpeningCapability, error) {
	if opening == nil {
		return nil, errors.New("construct execution runtime opening capability: opening is required")
	}
	return executionRuntimeOpeningCapability{
		opening: func(
			ctx context.Context,
			request factorysessions.ExecutionRuntimeOpeningRequest,
		) (factorysessions.OpenedExecutionRuntime, error) {
			projectRoot := request.ProjectRoot
			opened, err := opening.OpenExecutionRuntime(ctx, &factorysessions.RuntimeOpeningRequest{
				FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
					Directory:        projectRoot,
					ExecutionBaseDir: projectRoot,
				},
				FactoryRuntime: factoryruntime.RuntimeOpeningRequest{
					LogDirectory:      filepath.Join(projectRoot, ".you-agent-factory", "runtime-logs"),
					FileLoggingPolicy: factoryruntime.RuntimeFileLoggingPolicyDisabled,
					MetricsDirectory:  filepath.Join(projectRoot, ".you-agent-factory", "runtime-metrics"),
					MetricsPolicy:     factoryruntime.RuntimeMetricsPolicyDisabled,
				},
				FactorySession: factorysessions.SessionRuntimeOpeningRequest{
					FactorySessionID:  request.FactorySessionID,
					PersistencePolicy: request.PersistencePolicy,
					SystemConfigHome:  request.SystemConfigHome,
				},
				Recordings: recordings.RuntimeOpeningRequest{ReplayPath: request.ReplayPath},
			})
			if err != nil {
				return factorysessions.OpenedExecutionRuntime{}, err
			}
			return factorysessions.OpenedExecutionRuntime{
				Execution: opened.Execution,
				Close:     opened.Resources.Close,
			}, nil
		},
	}, nil
}

type executionRuntimeOpeningCapability struct {
	opening factorysessions.ExecutionRuntimeOpeningFunc
}

func (capability executionRuntimeOpeningCapability) ExecutionRuntimeOpening() any {
	return capability.opening
}

func provideOrchestrationJavaScriptExecution(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.OrchestrationJavaScriptExecution {
	return factoryruntimewire.NewOrchestrationJavaScriptExecution(newID, workflows)
}

func provideOrchestrationCompilation(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.OrchestrationCompilation {
	return factoryruntimewire.NewOrchestrationCompilation(newID, workflows)
}

func provideFactorySessionExecutionFactory(
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	recordingWriter recordings.PortableRecordingWriter,
	stores factorysessionwire.RuntimePersistenceStoreFactory,
	syncWaits factorysessionwire.SyncWaitScheduler,
	sessionIDs factorysessions.SessionIDGenerator,
	responseEventIDs factorysessions.ResponseEventIDGenerator,
	responseEventRetentionLimits *factorysessions.ResponseEventRetentionLimits,
	allocator providerswire.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	providerOverride providerOverrideService,
	eventsService events.Service,
	liveChangeCoordinator factorysessionwire.LiveChangeCoordinator,
) factorysessionwire.FactorySessionExecutionFactory {
	// The allocator, runner adapter, and fixed provider override are read only
	// to decide whether this process can reach a provider at all. No invocation executor is built
	// here any more: a runtime-backed session's children are Workers, and the
	// provider edge they use is the one Workers already composes for that
	// session, reached through workers.ProviderInvocationRoute. Rebuilding a
	// second edge here is exactly the bypass this convergence removed.
	return func(
		projectRoot string,
		persistencePolicy factorysessions.PersistencePolicy,
		provider providers.Service,
		clock factoryruntime.Clock,
		workerPresetIDs map[string]struct{},
		workerSettings factoryruntime.JavaScriptWorkerSettings,
		mockWorkers *workers.MockWorkersConfig,
		_ []operatorsettings.ACPIntegration,
	) (factorysessionwire.DurableExecutionService, error) {
		// Whether this session runs children live is the same question the
		// deleted live-child block answered, asked the same way: an explicit
		// provider, or a reachable provider command edge that mock workers are
		// not standing in for. What changed is only that the answer no longer
		// carries an executor -- the route does.
		mockAllowsLive := mockWorkers == nil || mockWorkers.UnmatchedDispatchPolicy.PassthroughUnmatched()
		childExecutorMode := factorysessions.ChildExecutorModeFake
		if provider != nil ||
			(mockAllowsLive && providerOverride == nil && adaptRunner != nil && allocator != nil) {
			childExecutorMode = factorysessions.ChildExecutorModeLive
		}
		return factorysessionwire.NewDurableExecution(
			projectRoot,
			persistencePolicy,
			stores,
			childExecutorMode,
			clock,
			syncWaits,
			factoryruntimewire.NewJavaScriptCheckpointSummaries(),
			workflows,
			orchestration,
			workerPresetIDs,
			workerSettings,
			recordingWriter,
			sessionIDs,
			responseEventIDs,
			responseEventRetentionLimits,
			eventsService,
			liveChangeCoordinator,
		)
	}
}

func provideStandaloneSessionExecutionFactory(
	workflows factoryruntime.JavaScriptWorkflows,
	orchestration factoryruntime.OrchestrationJavaScriptExecution,
	recordingWriter recordings.PortableRecordingWriter,
	stores factorysessionwire.RuntimePersistenceStoreFactory,
	syncWaits factorysessionwire.SyncWaitScheduler,
	sessionIDs factorysessions.SessionIDGenerator,
	fixtureFiles factorysessionwire.ContractFixtureReader,
	liveChangeCoordinator factorysessionwire.LiveChangeCoordinator,
	execution factorysessionwire.WorkerExecution,
) factorysessionwire.StandaloneSessionExecutionFactory {
	return func(
		provider factorysessions.ExecutionProvider,
		projectRoot string,
		fixtureCatalogPath string,
		childExecutorMode string,
		workerExecution factorysessionwire.WorkerExecution,
		clock factoryruntime.Clock,
	) (factorysessionwire.DurableExecutionService, error) {
		return factorysessionwire.NewStandaloneExecution(
			provider,
			projectRoot,
			stores,
			fixtureCatalogPath,
			childExecutorMode,
			workerExecution,
			clock,
			syncWaits,
			factoryruntimewire.NewJavaScriptCheckpointSummaries(),
			workflows,
			orchestration,
			recordingWriter,
			sessionIDs,
			fixtureFiles,
			liveChangeCoordinator,
		)
	}
}

func provideFactorySessionSyncWaitScheduler() factorysessionwire.SyncWaitScheduler {
	return platformclock.Real{}
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

func provideWorkersWorktree(
	edges serviceedges.Edges,
) (workers.FactoryWorktreePreparer, error) {
	worktreeFileSystem := edges.WorkersWorktreeFileSystem
	if worktreeFileSystem == nil {
		worktreeFileSystem = platformfilesystem.Local{}
	}
	worktreeGit := edges.WorkersWorktreeGit
	if worktreeGit == nil {
		processRunner, err := providePlatformProcessCommandRunner(edges)
		if err != nil {
			return nil, err
		}
		adapter, err := workerswire.NewPlatformGitCommander(processRunner)
		if err != nil {
			return nil, err
		}
		worktreeGit = adapter
	}
	worktreePreparer, err := workerswire.NewWorktree(worktreeFileSystem, worktreeGit)
	if err != nil {
		return nil, err
	}
	return worktreePreparer, nil
}

type factoryWorktreeReleaser interface {
	Release(context.Context, workers.FactoryWorktreePreparation) error
}

func provideWorkersWorktreeRelease(
	worktreePreparer workers.FactoryWorktreePreparer,
) func(context.Context, workers.FactoryWorktreePreparation) error {
	releaser, ok := worktreePreparer.(factoryWorktreeReleaser)
	if !ok {
		return nil
	}
	return releaser.Release
}

// provideStatelessWorkersService composes the process-scoped Execute owner.
// It is deliberately independent of Factory Runtime and Factory Session
// opening: a caller can execute one detached target before either lifecycle is
// opened, while the legacy runtime root receives this same owner below.
func provideStatelessWorkersService(
	providersService providers.Service,
	modelsService models.Service,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreePreparer workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	providerOverride providerOverrideService,
	agentToolFileSystem workers.AgentToolFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
) (workers.Service, error) {
	return provideStatelessWorkersServiceWithMock(
		providersService,
		modelsService,
		scriptCommandRunner,
		factoryDocsFileSystem,
		clock,
		logger,
		worktreePreparer,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentToolFileSystem,
		decisionEnvelopes,
		nil,
	)
}

// provideMockStatelessWorkersService is the explicit mock-feature composition
// used only by the public root's opt-in detached Workers builder. Normal
// process composition continues through provideStatelessWorkersService.
func provideMockStatelessWorkersService(
	providersService providers.Service,
	modelsService models.Service,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreePreparer workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	providerOverride providerOverrideService,
	agentToolFileSystem workers.AgentToolFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	mockWorkers *workers.MockWorkersConfig,
) (workers.Service, error) {
	return provideStatelessWorkersServiceWithMock(
		providersService,
		modelsService,
		scriptCommandRunner,
		factoryDocsFileSystem,
		clock,
		logger,
		worktreePreparer,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentToolFileSystem,
		decisionEnvelopes,
		mockWorkers,
	)
}

func provideStatelessWorkersServiceWithMock(
	providersService providers.Service,
	modelsService models.Service,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreePreparer workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	providerOverride providerOverrideService,
	agentToolFileSystem workers.AgentToolFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	mockWorkers *workers.MockWorkersConfig,
) (workers.Service, error) {
	if clock == nil {
		return nil, fmt.Errorf("construct stateless Workers: clock is required")
	}
	factoryDocs, err := workerswire.NewFactoryDocsLoader(factoryDocsFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct stateless Workers: %w", err)
	}
	scriptRunner := scriptCommandRunner
	agentDependencies := workerswire.AgentDependencies{
		Providers: providersService,
		Publish:   func(workers.ProgressFragment) {},
		// Decision-envelope interpretation belongs to Factory Definitions.
		// The detached Execute path routes envelope output through this
		// injected owner instead of re-implementing the contract.
		DecisionEnvelopes: decisionEnvelopes,
	}
	scriptConfig := workerswire.ScriptConfig{RequestSelected: true}
	scriptDependencies := workerswire.ScriptDependencies{
		CommandRunner: scriptRunner,
		FactoryDocs:   factoryDocs,
		Now:           clock.Now,
		Publish:       func(workers.ProgressFragment) {},
		Record:        func(workers.ScriptEvent) {},
	}
	inferenceConfig := workerswire.InferenceConfig{
		Worker: models.LocalWorker{
			Name: "request-selected-inference",
			Type: factorydefinitions.WorkerTypeInference,
		},
	}
	inferenceDependencies := workerswire.InferenceDependencies{Models: modelsService}
	loggerValue := logging.NewZapLogger(logger, false)
	if mockWorkers != nil {
		return workerswire.NewMockService(
			agentDependencies,
			scriptConfig,
			scriptDependencies,
			inferenceConfig,
			inferenceDependencies,
			mockWorkers,
			workerswire.MockDependencies{},
			nil,
			loggerValue,
			clock.Now,
			worktreePreparer,
			worktreeRelease,
			temporaryFiles,
			agentToolFileSystem,
			providerOverride,
		)
	}
	return workerswire.NewService(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		nil,
		loggerValue,
		clock.Now,
		worktreePreparer,
		worktreeRelease,
		temporaryFiles,
		agentToolFileSystem,
		providerOverride,
	)
}

func provideWorkersRetryRandomSource(edges serviceedges.Edges) platformrandom.Source {
	if edges.WorkersRetryRandomSource != nil {
		return edges.WorkersRetryRandomSource
	}
	return platformrandom.CryptoSource{}
}

func provideWorkersWorkstationFileSystem(edges serviceedges.Edges) platformfilesystem.ReadFileInspector {
	if edges.WorkersWorkstationFileSystem != nil {
		return edges.WorkersWorkstationFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkersProviderTemporaryFileSystem(edges serviceedges.Edges) platformfilesystem.TemporaryFileSystem {
	if edges.WorkersProviderTemporaryFileSystem != nil {
		return edges.WorkersProviderTemporaryFileSystem
	}
	return platformfilesystem.Local{}
}

// provideProvidersAgyPTYPlatform projects the Providers-owned PTY effect into
// the Workers-private invocation seam at the canonical composition boundary.
func provideProvidersAgyPTYPlatform(edges serviceedges.Edges) (providerswire.AgyPTYPlatformDependencies, error) {
	allocator, err := provideProvidersAgyPTYAllocator(edges)
	if err != nil {
		return providerswire.AgyPTYPlatformDependencies{}, err
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	return providerswire.AgyPTYPlatformDependencies{
		Allocator: allocator,
		Locator:   executableLocator,
		Inspector: executableInspector,
	}, nil
}

func provideWorkersFactoryDocsFileSystem(edges serviceedges.Edges) platformfilesystem.ReadFileTree {
	if edges.WorkersFactoryDocsFileSystem != nil {
		return edges.WorkersFactoryDocsFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkerProcessEnvironment() func() []string {
	return os.Environ
}

func provideWorkersAgentToolFileSystem(edges serviceedges.Edges) workers.AgentToolFileSystem {
	if edges.WorkersAgentToolFileSystem != nil {
		return edges.WorkersAgentToolFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkerCurrentWorkingDirectory() func() (string, error) {
	return os.Getwd
}

func provideWorkersMockCommandRunnerFactory() factoryruntime.WorkersMockCommandRunnerFactory {
	return func(
		config *workers.MockWorkersConfig,
		runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
		next platformprocess.CommandRunner,
	) platformprocess.CommandRunner {
		return workerswire.NewMockCommandRunner(
			config,
			runtimeConfig,
			next,
			platformfilesystem.Local{},
		)
	}
}

func provideConductorInvocationWithProgressFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
	allocator providerswire.PTYAllocator,
) factorysessionwire.ConductorInvocationWithProgressFactory {
	commandClock := edges.Clock
	if commandClock == nil {
		commandClock = platformclock.Real{}
	}
	resolveSymlinks := edges.WorkersResolveSymlinks
	if resolveSymlinks == nil {
		resolveSymlinks = filepath.EvalSymlinks
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	executableFiles := edges.WorkersExecutableFileReader
	if executableFiles == nil {
		executableFiles = platformfilesystem.Local{}
	}
	operatingSystem := resolveWorkersOperatingSystem(edges)
	temporaryFiles := provideWorkersProviderTemporaryFileSystem(edges)
	return func(
		selectedProviders providers.Service,
		runner platformprocess.CommandRunner,
		publisher workers.ProgressPublisher,
	) (workers.InvocationExecutor, error) {
		if selectedProviders == nil {
			selectedProviders = providersService
		}
		return workerswire.NewConductorInvocationWithProgress(
			selectedProviders,
			runner,
			commandClock,
			allocator,
			resolveSymlinks,
			executableLocator,
			executableInspector,
			executableFiles,
			operatingSystem,
			publisher,
			temporaryFiles,
		)
	}
}

func provideProviderFromCommandRunnerFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
) factorysessionwire.ProviderFromCommandRunnerFactory {
	commandClock := edges.Clock
	if commandClock == nil {
		commandClock = platformclock.Real{}
	}
	resolveSymlinks := edges.WorkersResolveSymlinks
	if resolveSymlinks == nil {
		resolveSymlinks = filepath.EvalSymlinks
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	executableFiles := edges.WorkersExecutableFileReader
	if executableFiles == nil {
		executableFiles = platformfilesystem.Local{}
	}
	operatingSystem := resolveWorkersOperatingSystem(edges)
	temporaryFiles := provideWorkersProviderTemporaryFileSystem(edges)
	return func(runner platformprocess.CommandRunner) (providers.Service, error) {
		return workerswire.NewProviderFromCommandRunner(
			providersService, runner, commandClock, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}
