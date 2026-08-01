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
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	"go.uber.org/zap"
)

func provideApplicationProcessLifecycle(service providers.Service) (initializerapplication.ProcessLifecycle, error) {
	lifecycle, ok := service.(providers.Lifecycle)
	if !ok {
		return nil, fmt.Errorf("construct application process: Providers lifecycle is required")
	}
	return lifecycle, nil
}

func provideProvidersService(edges serviceedges.Edges) (providers.Service, error) {
	return provideConfiguredProvidersService(edges, nil, nil)
}

func provideConfiguredProvidersService(
	edges serviceedges.Edges,
	integrations []operatorsettings.ACPIntegration,
	workersRunner workers.CommandRunner,
) (providers.Service, error) {
	agyPTYPlatform, err := provideProvidersAgyPTYPlatform(edges)
	if err != nil {
		return nil, err
	}
	options := []providerswire.Option{
		providerswire.WithAgyPTY(agyPTYPlatform),
		providerswire.WithCommandFactory(providePlatformProcessCommandFactory(edges)),
		providerswire.WithExecutableLocator(edges.ProvidersExecutableLocator),
		providerswire.WithACPIntegrations(projectACPIntegrations(integrations)...),
		providerswire.WithRegistrations(edges.ProviderRegistrations...),
	}
	if workersRunner != nil {
		options = append(options, providerswire.WithCommandEffectRunner(
			workersProviderCommandRunner{runner: workersRunner},
		))
		return providerswire.NewService(options...)
	}
	if edges.ProviderCommandRunner != nil {
		options = append(options, providerswire.WithCommandRunner(edges.ProviderCommandRunner))
		return providerswire.NewService(options...)
	}
	commandRunner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	options = append(options, providerswire.WithCommandRunner(commandRunner))
	return providerswire.NewService(options...)
}

// workersProviderCommandRunner is the composition-root projection from the
// Workers command edge into the Providers-owned private effect contract. The
// projection keeps only the correlation fields needed by Workers mock
// interception; Providers adapters never import the Workers contract.
type workersProviderCommandRunner struct {
	runner workers.CommandRunner
}

func (runner workersProviderCommandRunner) Run(
	ctx context.Context,
	request providerswire.CommandRequest,
) (providerswire.CommandResult, error) {
	if runner.runner == nil {
		return providerswire.CommandResult{}, fmt.Errorf("Workers provider command runner is required")
	}
	result, err := runner.runner.Run(ctx, workers.CommandRequest{
		Command:         request.Command,
		Args:            request.Args,
		Stdin:           request.Stdin,
		Env:             request.Env,
		WorkDir:         request.WorkDir,
		DispatchID:      request.AttemptID,
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationName,
	})
	return providerswire.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func (runner workersProviderCommandRunner) RunStreaming(
	ctx context.Context,
	request providerswire.CommandRequest,
	observer providerswire.OutputChunkObserver,
) (providerswire.CommandResult, error) {
	if runner.runner == nil {
		return providerswire.CommandResult{}, fmt.Errorf("Workers provider command runner is required")
	}
	workersRequest := workers.CommandRequest{
		Command:         request.Command,
		Args:            request.Args,
		Stdin:           request.Stdin,
		Env:             request.Env,
		WorkDir:         request.WorkDir,
		DispatchID:      request.AttemptID,
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationName,
	}
	if streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	}); ok {
		var observerMu sync.Mutex
		var observerErr error
		result, err := streaming.RunStreaming(ctx, workersRequest, func(stream string, chunk []byte) {
			if observer == nil {
				return
			}
			observerMu.Lock()
			defer observerMu.Unlock()
			if observerErr == nil {
				observerErr = observer(stream, chunk)
			}
		})
		observerMu.Lock()
		err = errors.Join(err, observerErr)
		observerMu.Unlock()
		return providerswire.CommandResult{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}, err
	}
	result, err := runner.runner.Run(ctx, workersRequest)
	var observerErr error
	if observer != nil {
		if len(result.Stdout) > 0 {
			observerErr = observer(providerswire.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if observerErr == nil && len(result.Stderr) > 0 {
			observerErr = observer(providerswire.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	err = errors.Join(err, observerErr)
	return providerswire.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
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
) (workers.ProviderRegistry, error) {
	return workerswire.NewProviderRegistry(context.Background(), providersService)
}

func buildProviderRegistry(
	_ serviceedges.Edges,
	providersService providers.Service,
) (workers.ProviderRegistry, error) {
	return workerswire.NewProviderRegistry(context.Background(), providersService)
}

func provideProviderRegistryRebinder(
	providersService providers.Service,
	edges serviceedges.Edges,
) (workerswire.ProviderRegistryRebinder, error) {
	return func(providerRunner workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		if providerRunner == nil {
			return nil, nil, fmt.Errorf("provider registry rebind requires command runner")
		}
		rebound, err := provideConfiguredProvidersService(edges, nil, providerRunner)
		if err != nil {
			return nil, nil, err
		}
		registry, err := workerswire.NewProviderRegistry(context.Background(), rebound)
		return registry, rebound, err
	}, nil
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
func provideWorkersProviderCommandRunner(edges serviceedges.Edges) (workers.CommandRunner, error) {
	if edges.ProviderCommandRunner != nil {
		return workers.AdaptCommandRunner(edges.ProviderCommandRunner), nil
	}
	defaultCommandRunner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return workers.AdaptCommandRunner(defaultCommandRunner), nil
}

func provideFactorySessionProviderIdentityResolver(
	providers workers.ProviderRegistry,
) factorysessions.ProviderIdentityResolver {
	return providers.CanonicalIdentity
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

func provideFactoryRuntimeInputs(edges serviceedges.Edges) factoryruntime.InputFileSystem {
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
) (factorydefinitions.Persistence, error) {
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
			factorydefinitionswire.FactorySnapshotCapturer(),
		)
	}
}

func provideAutomationFactory(
	edges serviceedges.Edges,
	workstationExecution factorydefinitions.WorkstationExecutionPolicyService,
) factorysessionwire.AutomationFactory {
	return func(
		logger *zap.Logger,
		clock factoryruntime.Clock,
		commandRunner workers.CommandRunner,
		workflowID string,
		defaultFactoryDir string,
		hostedPollers automations.HostedPollers,
	) automations.Service {
		hostedSources := func(
			*zap.Logger,
			automations.HostedLinearClock,
			automations.HostedLinearHTTPDoer,
			automations.HostedLinearSecretResolver,
			string,
		) automations.HostedPollers {
			return hostedPollers
		}
		hostedClock := edges.HostedClock
		if hostedClock == nil {
			hostedClock = clockwork.NewRealClock()
		}
		service, err := automationswire.NewService(
			logger,
			clock,
			commandRunner,
			workflowID,
			defaultFactoryDir,
			hostedSources,
			nil,
			hostedClock,
			nil,
			nil,
			"",
			workerswire.ResolveTemplateFields,
			workstationExecution,
		)
		if err != nil {
			return nil
		}
		return service
	}
}

func provideFactorySessionResponseEventRetentionLimits(
	edges serviceedges.Edges,
) *factorysessions.ResponseEventRetentionLimits {
	return edges.FactorySessionResponseEventRetentionLimits
}

func provideFactorySessionsService(
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
) (factorysessions.Service, error) {
	return factorysessionwire.NewService(func() factoryruntime.JavaScriptCheckpointStore {
		return factoryruntimewire.NewJavaScriptCheckpointStore()
	}, sessionResultProjection, interpolation, invocationWorkTypes, ttsObservability, eventIDs, responseEventRetentionLimits, sessionIDs, resolveHome, directories, namedPaths, invocationInputFiles, initialWorkFiles, resolveSymlinks)
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
	invocationWithProgress factorysessionwire.WorkerInvocationWithProgressFactory,
	allocator workers.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	registry workers.ProviderRegistry,
	registryRebinder workerswire.ProviderRegistryRebinder,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	conductorInvocationWithProgress factorysessionwire.ConductorInvocationWithProgressFactory,
	edges serviceedges.Edges,
) factorysessionwire.FactorySessionExecutionFactory {
	return func(
		projectRoot string,
		persistencePolicy factorysessions.PersistencePolicy,
		provider workers.Runner,
		clock factoryruntime.Clock,
		workerPresetIDs map[string]struct{},
		workerSettings factoryruntime.JavaScriptWorkerSettings,
		mockWorkers *workers.MockWorkersConfig,
		acpIntegrations []operatorsettings.ACPIntegration,
	) (factorysessions.ExecutionService, error) {
		executor := workerswire.NewExecutor(provider)
		var liveChildInvocation factorysessionwire.LiveChildInvocationFactory
		// An explicit process provider is already the complete invocation edge.
		// Do not construct a second registered-provider path that would bypass it.
		if edges.ProviderOverride == nil && adaptRunner != nil && allocator != nil {
			commandRunner, err := provideWorkersProviderCommandRunner(edges)
			if err != nil {
				return nil, fmt.Errorf("resolve provider runner for live child invocation: %w", err)
			}
			runner := commandRunner
			runtimeRegistry := registry
			if mockWorkers != nil &&
				mockWorkers.UnmatchedDispatchPolicy.PassthroughUnmatched() &&
				runtimeRegistry != nil &&
				registryRebinder != nil &&
				workersMockCommandRunnerFactory != nil &&
				conductorInvocationWithProgress != nil {
				runner = workersMockCommandRunnerFactory(mockWorkers, nil, runner)
				var reboundProviders providers.Service
				_, reboundProviders, err = registryRebinder(runner)
				if err != nil {
					return nil, fmt.Errorf("rebind provider registry for live child invocation: %w", err)
				}
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return conductorInvocationWithProgress(reboundProviders, runner, allocator, publisher)
				}
			} else if mockWorkers == nil &&
				runtimeRegistry != nil &&
				conductorInvocationWithProgress != nil {
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return conductorInvocationWithProgress(nil, runner, allocator, publisher)
				}
			} else if mockWorkers == nil && invocationWithProgress != nil {
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return invocationWithProgress(runner, allocator, publisher)
				}
			}
		}
		_ = acpIntegrations
		return factorysessionwire.NewDurableExecution(
			projectRoot,
			persistencePolicy,
			stores,
			executor,
			clock,
			syncWaits,
			factoryruntimewire.NewJavaScriptCheckpointSummaries(),
			workflows,
			orchestration,
			workerPresetIDs,
			workerSettings,
			recordingWriter,
			sessionIDs,
			liveChildInvocation,
			responseEventIDs,
			responseEventRetentionLimits,
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
) factorysessionwire.StandaloneSessionExecutionFactory {
	return func(
		provider factorysessions.ExecutionProvider,
		projectRoot string,
		fixtureCatalogPath string,
		childExecutorMode string,
		executor workers.InvocationExecutor,
		clock factoryruntime.Clock,
	) (factorysessions.ExecutionService, error) {
		return factorysessionwire.NewStandaloneExecution(
			provider,
			projectRoot,
			stores,
			fixtureCatalogPath,
			childExecutorMode,
			executor,
			clock,
			syncWaits,
			factoryruntimewire.NewJavaScriptCheckpointSummaries(),
			workflows,
			orchestration,
			recordingWriter,
			sessionIDs,
			fixtureFiles,
		)
	}
}

func provideFactorySessionSyncWaitScheduler() factorysessionwire.SyncWaitScheduler {
	return platformclock.Real{}
}

func provideRecordingsProjectionFactory() factorysessionwire.RecordingsProjectionFactory {
	return recordingswire.NewProjectionService
}

func provideRuntimeLedgerFactory() factorysessionwire.RuntimeLedgerFactory {
	return func() factoryruntime.RuntimeLedgerFactory {
		return func(topology recordings.InitialStructureSource, now func() time.Time, definitions factorydefinitions.RuntimeDefinitionLookup) recordings.RuntimeEventLedger {
			return recordingswire.NewRuntimeLedger(topology, now, uuid.NewString(), definitions)
		}
	}
}

func providePortableRecordingWriter(edges serviceedges.Edges) (recordings.PortableRecordingWriter, error) {
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
		renamePath = os.Rename
	}
	return recordingswire.NewPortableRecordingWriter(makeDirectories, createTemporaryFile, removePath, renamePath)
}

func provideLoadedFactorySnapshotCapturer() factorydefinitions.LoadedFactorySnapshotCapturer {
	return factorydefinitionswire.LoadedFactorySnapshotCapturer()
}

func provideRuntimeRecorderFactory(
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
) recordings.RuntimeRecorderFactory {
	return func(
		flushInterval time.Duration,
		loaded factorydefinitions.LoadedFactorySource,
		now func() time.Time,
		recordPath string,
	) (recordings.RuntimeRecorder, error) {
		return recordingswire.NewLifecycleRuntimeRecorder(
			flushInterval,
			loaded,
			now,
			recordPath,
			captureLoadedFactorySnapshot,
		)
	}
}

func provideReplayClockFactory() factorysessionwire.ReplayClockFactory {
	return recordingswire.NewReplayClock
}

func provideReplayExecutionFactory() recordings.ReplayExecutionFactory {
	return func(
		artifact *factorydefinitions.ReplayArtifact,
	) (
		workers.Runner,
		workers.CommandRunner,
		[]recordings.ReplayHook,
		recordings.CompletionDeliveryPlanner,
		error,
	) {
		return recordingswire.NewReplayExecution(
			artifact,
			factorydefinitionswire.FactorySnapshotJSONDecoder(),
			factorydefinitionswire.ReplayRuntimeConfigDecoder(),
		)
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func provideWorkersRuntimeFactory(
	providersService providers.Service,
	interpolation factorydefinitions.InvocationInterpolationService,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	workstationExecution factorydefinitions.WorkstationExecutionPolicyService,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	defaultAllocator workers.PTYAllocator,
	edges serviceedges.Edges,
	providerRegistry workers.ProviderRegistry,
	providerRegistryRebinder workerswire.ProviderRegistryRebinder,
) (factorysessionwire.WorkersRuntimeFactory, error) {
	if defaultAllocator == nil {
		return nil, workers.ErrPTYHostRequired
	}
	factoryDocsFileSystem := provideWorkersFactoryDocsFileSystem(edges)
	factoryDocs, err := workerswire.NewFactoryDocsLoader(factoryDocsFileSystem)
	if err != nil {
		return nil, err
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
	agentToolFileSystem := provideWorkersAgentToolFileSystem(edges)
	agentRunHarness := workerswire.NewLibraryHarnessAdapter(agentToolFileSystem)
	return func(
		sessions factorysessionwire.CurrentRuntimeResolver,
		modelService models.Service,
		modelsScope models.RuntimeScopeRef,
		providerCommandRunner workers.CommandRunner,
		scriptCommandRunner workers.CommandRunner,
		allocator workers.PTYAllocator,
		logger *zap.Logger,
		verbose bool,
		factoryRunnerID string,
		runWorktree string,
		invocationSkipPermissionsOverride *bool,
		providerOverride workers.Runner,
		now func() time.Time,
		contentMaterializer work.ContentMaterializer,
		acpIntegrations []operatorsettings.ACPIntegration,
	) (workers.RuntimeService, error) {
		providerInjected := providerCommandRunner != nil
		scriptInjected := scriptCommandRunner != nil
		defaultCommandRunner, err := providePlatformProcessCommandRunner(edges)
		if err != nil {
			return nil, err
		}
		if providerCommandRunner == nil {
			providerCommandRunner = workers.AdaptCommandRunner(defaultCommandRunner)
		}
		if scriptCommandRunner == nil {
			scriptCommandRunner = workers.AdaptCommandRunner(defaultCommandRunner)
		}
		if allocator == nil {
			allocator = defaultAllocator
		}
		if configurator, ok := providersService.(providers.ACPConfiguration); ok {
			if configuredErr := configurator.ConfigureACPIntegrations(context.Background(), projectACPIntegrations(acpIntegrations)); configuredErr != nil {
				return nil, fmt.Errorf("configure ACP integrations for Workers runtime: %w", configuredErr)
			}
		}
		runtimeProviders := providersService
		runtimeRegistry, err := workerswire.NewProviderRegistry(context.Background(), runtimeProviders)
		if err != nil {
			return nil, fmt.Errorf("construct runtime provider registry: %w", err)
		}
		runtimeRebinder := providerRegistryRebinder
		providersLifecycleOwned := providerInjected || len(acpIntegrations) > 0
		if providersLifecycleOwned {
			runtimeRegistry, runtimeProviders, runtimeRebinder, err = provideRuntimeProviderBindings(
				edges,
				acpIntegrations,
				providerCommandRunner,
			)
			if err != nil {
				return nil, err
			}
		}
		return workerswire.NewRuntimeWithSelection(
			sessions,
			modelService,
			runtimeProviders,
			modelsScope,
			providerCommandRunner,
			scriptCommandRunner,
			allocator,
			logger,
			verbose,
			factoryRunnerID,
			runWorktree,
			invocationSkipPermissionsOverride,
			providerOverride,
			now,
			processEnvironment,
			currentWorkingDirectory,
			contentMaterializer,
			interpolation,
			workstationExecution,
			factoryDocs,
			resolveSymlinks,
			executableLocator,
			executableInspector,
			executableFiles,
			operatingSystem,
			worktreePreparer,
			agentRunHarness,
			retryRandom,
			workstationFiles,
			temporaryFiles,
			decisionEnvelopes,
			providerInjected,
			scriptInjected,
			providersLifecycleOwned,
			runtimeRegistry,
			runtimeRebinder,
		)
	}, nil
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
	clock := edges.Clock
	if clock == nil {
		clock = platformclock.Real{}
	}
	return providerswire.AgyPTYPlatformDependencies{
		Allocator: allocator,
		Clock:     clock,
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

func provideWorkersRuntimeExecutorsFactory() factoryruntime.WorkersRuntimeExecutorsFactory {
	return workerswire.BuildRuntimeExecutors
}

func provideWorkersMockCommandRunnerFactory() factoryruntime.WorkersMockCommandRunnerFactory {
	return workerswire.NewMockCommandRunner
}

func provideWorkersLocalRuntimeHooksFactory() factorysessionwire.WorkersLocalRuntimeHooksFactory {
	return workerswire.LocalRuntimeHooks
}

func provideWorkerInvocationWithProgressFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
) factorysessionwire.WorkerInvocationWithProgressFactory {
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
	return func(runner workers.CommandRunner, allocator workers.PTYAllocator, publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
		return workerswire.NewInvocationWithProgress(
			providersService, runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, publisher, temporaryFiles,
		)
	}
}

func provideWorkerInvocationFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
) factorysessionwire.WorkerInvocationFactory {
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
	return func(runner workers.CommandRunner, allocator workers.PTYAllocator) (workers.InvocationExecutor, error) {
		return workerswire.NewInvocation(
			providersService, runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}

func provideConductorInvocationWithProgressFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
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
		runner workers.CommandRunner,
		allocator workers.PTYAllocator,
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
	allocator workers.PTYAllocator,
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
	return func(runner workers.CommandRunner) (workers.Runner, error) {
		return workerswire.NewProviderFromCommandRunner(
			providersService, runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}
