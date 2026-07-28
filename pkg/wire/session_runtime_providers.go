package wire

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationswire "github.com/portpowered/infinite-you/pkg/services/automations/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionswire "github.com/portpowered/infinite-you/pkg/services/provider_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/invocation"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	workerworktree "github.com/portpowered/infinite-you/pkg/services/workers/worktree"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	"go.uber.org/zap"
)

func provideProvidersService(edges serviceedges.Edges) (providers.Service, error) {
	cursorPlatform := providerswire.CursorPlatformDependencies{
		OperatingSystem: string(resolveWorkersOperatingSystem(edges)),
		TemporaryFiles:  provideWorkersProviderTemporaryFileSystem(edges),
	}
	agyPTYPlatform, err := provideProvidersAgyPTYPlatform(edges)
	if err != nil {
		return nil, err
	}
	options := []providerswire.Option{
		providerswire.WithCursorPlatform(cursorPlatform),
		providerswire.WithAgyPTY(agyPTYPlatform),
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

func provideProviderRegistry(
	edges serviceedges.Edges,
	providersService providers.Service,
) (*providerregistry.Registry, error) {
	return buildProviderRegistry(edges, providersService)
}

func buildProviderRegistry(
	edges serviceedges.Edges,
	providersService providers.Service,
) (*providerregistry.Registry, error) {
	commandRunner, err := provideWorkersProviderCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	builtIns, err := providerregistry.BuiltInRegistrations(providerregistry.BuiltInDependencies{
		CommandRunner:    commandRunner,
		OperatingSystem:  string(resolveWorkersOperatingSystem(edges)),
		TemporaryFiles:   provideWorkersProviderTemporaryFileSystem(edges),
		ProvidersService: providersService,
	})
	if err != nil {
		return nil, err
	}
	registrations := append([]providerregistry.Registration(nil), builtIns...)
	for _, addition := range edges.ProviderRegistrations {
		registrations = append(
			registrations,
			providerregistry.ExternalRegistration(addition.Manifest, addition.Integration),
		)
	}
	return providerregistry.New(registrations...)
}

func provideProviderRegistryRebinder(
	edges serviceedges.Edges,
) (workerswire.ProviderRegistryRebinder, error) {
	operatingSystem := string(resolveWorkersOperatingSystem(edges))
	temporaryFiles := provideWorkersProviderTemporaryFileSystem(edges)
	agyPTYPlatform, err := provideProvidersAgyPTYPlatform(edges)
	if err != nil {
		return nil, err
	}
	externalRegistrations := append(
		[]providerregistry.Registration(nil),
		externalProviderRegistrations(edges)...,
	)
	return func(providerRunner workers.CommandRunner) (*providerregistry.Registry, error) {
		if providerRunner == nil {
			return nil, fmt.Errorf("provider registry rebind requires command runner")
		}
		platformRunner := workerprocess.ProjectPlatformCommandRunner(providerRunner)
		if platformRunner == nil {
			return nil, fmt.Errorf("provider registry rebind requires platform command runner")
		}
		providersService, err := providerswire.NewService(
			providerswire.WithWorkersCommandRunner(providerRunner),
			providerswire.WithCursorPlatform(providerswire.CursorPlatformDependencies{
				OperatingSystem: operatingSystem,
				TemporaryFiles:  temporaryFiles,
			}),
			providerswire.WithAgyPTY(agyPTYPlatform),
		)
		if err != nil {
			return nil, err
		}
		builtIns, err := providerregistry.BuiltInRegistrations(providerregistry.BuiltInDependencies{
			CommandRunner:    providerRunner,
			OperatingSystem:  operatingSystem,
			TemporaryFiles:   temporaryFiles,
			ProvidersService: providersService,
		})
		if err != nil {
			return nil, err
		}
		registrations := append(builtIns, externalRegistrations...)
		return providerregistry.New(registrations...)
	}, nil
}

func externalProviderRegistrations(edges serviceedges.Edges) []providerregistry.Registration {
	registrations := make([]providerregistry.Registration, 0, len(edges.ProviderRegistrations))
	for _, addition := range edges.ProviderRegistrations {
		registrations = append(
			registrations,
			providerregistry.ExternalRegistration(addition.Manifest, addition.Integration),
		)
	}
	return registrations
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
		return workerprocess.AdaptCommandRunner(edges.ProviderCommandRunner), nil
	}
	defaultCommandRunner, err := providePlatformProcessCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	return workerprocess.AdaptCommandRunner(defaultCommandRunner), nil
}

func provideFactorySessionProviderIdentityResolver(
	providers *providerregistry.Registry,
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
		operatingSystem = providersessionswire.OperatingSystem(runtime.GOOS)
	}
	return providersessionswire.NewService(
		files,
		resolveHome,
		codexWalkDirectory,
		codexResolveSymlinks,
		cursorWalkDirectory,
		cursorResolveSymlinks,
		cursorOpenDatabase,
		operatingSystem,
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
		return factoryeditable.ValidateSnapshot(
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
			workers.HostedPollerClock,
			workers.HostedPollerHTTPDoer,
			workers.HostedPollerSecretResolver,
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
	allocator agypty.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	registry *providerregistry.Registry,
	registryRebinder workerswire.ProviderRegistryRebinder,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	conductorInvocationWithProgress factorysessionwire.ConductorInvocationWithProgressFactory,
	edges serviceedges.Edges,
) factorysessionwire.FactorySessionExecutionFactory {
	return func(
		projectRoot string,
		persistencePolicy factorysessions.PersistencePolicy,
		provider workerprovider.Provider,
		clock factoryruntime.Clock,
		workerPresetIDs map[string]struct{},
		workerSettings factoryruntime.JavaScriptWorkerSettings,
		mockWorkers *workers.MockWorkersConfig,
	) (factorysessions.ExecutionService, error) {
		executor := workerinvocation.NewExecutor(provider)
		var liveChildInvocation factorysessionwire.LiveChildInvocationFactory
		if adaptRunner != nil &&
			allocator != nil &&
			edges.ProviderCommandRunner != nil {
			runner := adaptRunner(edges.ProviderCommandRunner)
			if mockWorkers != nil &&
				mockWorkers.UnmatchedDispatchPolicy.PassthroughUnmatched() &&
				registry != nil &&
				registryRebinder != nil &&
				workersMockCommandRunnerFactory != nil &&
				conductorInvocationWithProgress != nil {
				runner = workersMockCommandRunnerFactory(mockWorkers, nil, runner)
				reboundRegistry, err := registryRebinder(runner)
				if err != nil {
					return nil, fmt.Errorf("rebind provider registry for live child invocation: %w", err)
				}
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return conductorInvocationWithProgress(reboundRegistry, runner, allocator, publisher)
				}
			} else if mockWorkers == nil &&
				registry != nil &&
				conductorInvocationWithProgress != nil {
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return conductorInvocationWithProgress(registry, runner, allocator, publisher)
				}
			} else if mockWorkers == nil && invocationWithProgress != nil {
				liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
					return invocationWithProgress(runner, allocator, publisher)
				}
			}
		}
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
		workerprovider.Provider,
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
	interpolation factorydefinitions.InvocationInterpolationService,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	workstationExecution factorydefinitions.WorkstationExecutionPolicyService,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	defaultAllocator agypty.PTYAllocator,
	edges serviceedges.Edges,
	providerRegistry *providerregistry.Registry,
	providerRegistryRebinder workerswire.ProviderRegistryRebinder,
) (factorysessionwire.WorkersRuntimeFactory, error) {
	if defaultAllocator == nil {
		return nil, agypty.ErrHostRequired
	}
	factoryDocsFileSystem := provideWorkersFactoryDocsFileSystem(edges)
	factoryDocs, err := workerprompting.NewFactoryDocsLoader(factoryDocsFileSystem)
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
		adapter, err := workerworktree.NewPlatformGitCommander(processRunner)
		if err != nil {
			return nil, err
		}
		worktreeGit = adapter
	}
	worktreePreparer, err := workerworktree.New(worktreeFileSystem, worktreeGit)
	if err != nil {
		return nil, err
	}
	agentToolFileSystem := provideWorkersAgentToolFileSystem(edges)
	agentRunHarness := workeragentrun.NewLibraryHarnessAdapter(agentToolFileSystem)
	return func(
		sessions factorysessionwire.CurrentRuntimeResolver,
		modelService models.Service,
		modelsScope models.RuntimeScopeRef,
		providerCommandRunner workers.CommandRunner,
		scriptCommandRunner workers.CommandRunner,
		allocator agypty.PTYAllocator,
		logger *zap.Logger,
		verbose bool,
		factoryRunnerID string,
		invocationSkipPermissionsOverride *bool,
		providerOverride workerprovider.Provider,
		now func() time.Time,
		contentMaterializer work.ContentMaterializer,
	) (workers.RuntimeService, error) {
		providerInjected := providerCommandRunner != nil
		scriptInjected := scriptCommandRunner != nil
		defaultCommandRunner, err := providePlatformProcessCommandRunner(edges)
		if err != nil {
			return nil, err
		}
		if providerCommandRunner == nil {
			providerCommandRunner = workerprocess.AdaptCommandRunner(defaultCommandRunner)
		}
		if scriptCommandRunner == nil {
			scriptCommandRunner = workerprocess.AdaptCommandRunner(defaultCommandRunner)
		}
		if allocator == nil {
			allocator = defaultAllocator
		}
		return workerswire.NewRuntimeWithSelection(
			sessions,
			modelService,
			modelsScope,
			providerCommandRunner,
			scriptCommandRunner,
			allocator,
			logger,
			verbose,
			factoryRunnerID,
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
			providerRegistry,
			providerRegistryRebinder,
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
	allocator, err := provideAgyPTYAllocator(edges)
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

func provideWorkersRuntimeExecutorsFactory() factoryruntime.WorkersRuntimeExecutorsFactory {
	return workerswire.BuildRuntimeExecutors
}

func provideWorkersMockCommandRunnerFactory() factoryruntime.WorkersMockCommandRunnerFactory {
	return workerswire.NewMockCommandRunner
}

func provideWorkersLocalRuntimeHooksFactory() factorysessionwire.WorkersLocalRuntimeHooksFactory {
	return workerswire.LocalRuntimeHooks
}

func provideWorkerInvocationWithProgressFactory(edges serviceedges.Edges) factorysessionwire.WorkerInvocationWithProgressFactory {
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
	return func(runner workers.CommandRunner, allocator agypty.PTYAllocator, publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
		return workerswire.NewInvocationWithProgress(
			runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, publisher, temporaryFiles,
		)
	}
}

func provideWorkerInvocationFactory(edges serviceedges.Edges) factorysessionwire.WorkerInvocationFactory {
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
	return func(runner workers.CommandRunner, allocator agypty.PTYAllocator) (workers.InvocationExecutor, error) {
		return workerswire.NewInvocation(
			runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}

func provideConductorInvocationWithProgressFactory(edges serviceedges.Edges) factorysessionwire.ConductorInvocationWithProgressFactory {
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
		registry workers.ProviderRegistry,
		runner workers.CommandRunner,
		allocator workers.PTYAllocator,
		publisher workers.ProgressPublisher,
	) (workers.InvocationExecutor, error) {
		var concreteRegistry *providerregistry.Registry
		if registry != nil {
			typed, ok := registry.(*providerregistry.Registry)
			if !ok {
				return nil, fmt.Errorf("conductor invocation requires concrete provider registry")
			}
			concreteRegistry = typed
		}
		return workerswire.NewConductorInvocationWithProgress(
			concreteRegistry,
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
	edges serviceedges.Edges,
	allocator agypty.PTYAllocator,
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
	return func(runner workers.CommandRunner) (workerprovider.Provider, error) {
		return workerswire.NewProviderFromCommandRunner(
			runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}
