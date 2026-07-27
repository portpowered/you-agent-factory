package wire

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorynamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedfactories"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workstationexecution"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorycheckpointstore "github.com/portpowered/infinite-you/pkg/services/factory_runtime/checkpointstore"
	factorycheckpointsummary "github.com/portpowered/infinite-you/pkg/services/factory_runtime/checkpointsummary"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/javascript"
	factoryruntimeservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsservice "github.com/portpowered/infinite-you/pkg/services/provider_sessions/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingartifacts "github.com/portpowered/infinite-you/pkg/services/recordings/artifacts"
	recordingsservice "github.com/portpowered/infinite-you/pkg/services/recordings/service"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workservice "github.com/portpowered/infinite-you/pkg/services/work/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/invocation"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	workersservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
	hostedworkers "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic/linear"
	workerworktree "github.com/portpowered/infinite-you/pkg/services/workers/worktree"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
	wirefactorydefinitions "github.com/portpowered/infinite-you/pkg/wire/factorydefinitions"
	"go.uber.org/zap"
)

func provideProviderRegistry(edges serviceedges.Edges) (*providerregistry.Registry, error) {
	commandRunner, err := provideWorkersProviderCommandRunner(edges)
	if err != nil {
		return nil, err
	}
	builtIns, err := providerregistry.BuiltInRegistrations(providerregistry.BuiltInDependencies{
		CommandRunner:   commandRunner,
		OperatingSystem: string(resolveWorkersOperatingSystem(edges)),
		TemporaryFiles:  provideWorkersProviderTemporaryFileSystem(edges),
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
		codexWalkDirectory = providersessions.CodexWalkDirectory(filepath.WalkDir)
	}
	codexResolveSymlinks := edges.ProviderSessionCodexResolveSymlinks
	if codexResolveSymlinks == nil {
		codexResolveSymlinks = providersessions.CodexResolveSymlinks(filepath.EvalSymlinks)
	}
	cursorWalkDirectory := edges.ProviderSessionCursorWalkDirectory
	if cursorWalkDirectory == nil {
		cursorWalkDirectory = providersessions.CursorWalkDirectory(filepath.WalkDir)
	}
	cursorResolveSymlinks := edges.ProviderSessionCursorResolveSymlinks
	if cursorResolveSymlinks == nil {
		cursorResolveSymlinks = providersessions.CursorResolveSymlinks(filepath.EvalSymlinks)
	}
	cursorOpenDatabase := edges.ProviderSessionCursorOpenDatabase
	if cursorOpenDatabase == nil {
		cursorOpenDatabase = providersessions.CursorOpenSQLDatabase(sql.Open)
	}
	operatingSystem := edges.ProviderSessionOperatingSystem
	if operatingSystem == "" {
		operatingSystem = providersessions.OperatingSystem(runtime.GOOS)
	}
	return providersessionsservice.New(
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
	return factoryruntimejavascript.New(files, resolveHome, resolveSymlinks)
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

func provideFactoryDefinitionValidationService(
	workflows factoryruntime.JavaScriptWorkflows,
	loader *factoryloading.Loader,
) *factoryvalidation.Service {
	return factoryvalidation.New(
		factoryruntimeservice.NewOrchestratorDefinitionValidator(workflows),
		loader.LoadSourceFromCanonicalJSON,
	)
}

func provideFactoryDefinitionValidator(
	service *factoryvalidation.Service,
) factorydefinitions.Validator {
	return service
}

func provideDefinitionValidationOperation(
	service *factoryvalidation.Service,
) factorydefinitions.DefinitionValidationOperation {
	return service
}

func provideSubmittedDefinitionValidationOperation(
	service *factoryvalidation.Service,
) factorydefinitions.SubmittedDefinitionValidationOperation {
	return service
}

func provideLoadedFactorySourceFactory() factorydefinitions.LoadedFactorySourceFactory {
	return wirefactorydefinitions.LoadedFactorySourceFactory()
}

func provideNamedFactoryCatalog(
	namedPaths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	return factorynamedfactories.New(namedPaths, fileSystem)
}

func provideFactoryDefinitionPersistence(
	validator factorydefinitions.Validator,
	loader *factoryloading.Loader,
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
	return wirefactorydefinitions.Persistence(
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

func provideFactoryDefinitionsFactory(
	persistence factorydefinitions.Persistence,
	loader *factoryloading.Loader,
	applySupportedFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
	namedPaths factorydefinitions.NamedPathResolver,
	namedFactoryCatalogFileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
	clock factorydefinitions.Clock,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) factorysessionwire.FactoryDefinitionsFactory {
	return func(
		sessionHost factorysessions.DefinitionHost,
		validator factorydefinitions.Validator,
	) factorydefinitions.Service {
		definitions := factorydefinitionsservice.New(
			sessionHost,
			clock,
			versionFileSystem,
			validator,
			func(
				factoryDir string,
				workstationLoader factorydefinitions.WorkstationLoader,
			) (factorydefinitions.MutableLoadedFactorySource, error) {
				return loader.LoadRuntimeSource(factoryDir, workstationLoader)
			},
			namedPaths.ReadCurrentPointer,
			func(
				ctx context.Context,
				segment string,
				payload []byte,
				_ factorydefinitions.Validator,
			) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
				return persistence.PrepareFactoryLayout(ctx, segment, payload)
			},
			persistence.CreateNamedFactory,
			namedPaths.WriteCurrentPointer,
			wirefactorydefinitions.PortableFactoryConfigPreparer(
				applySupportedFiles,
				applyStarterWork,
			),
			wirefactorydefinitions.FactorySnapshotCapturer(),
			persistence.ReplaceFactoryLayout,
			namedPaths,
			namedFactoryCatalogFileSystem,
		)
		if definitions == nil {
			return nil
		}
		attached, err := factorydefinitionsservice.AttachEffectiveCatalog(
			definitions,
			listEffective,
		)
		if err != nil {
			return nil
		}
		return attached
	}
}

func provideEditableFactoryValidator(
	validator factorydefinitions.DefinitionValidationOperation,
	loader *factoryloading.Loader,
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
		return factorydefinitionsservice.CaptureInitialSnapshot(
			loaded,
			wirefactorydefinitions.PortableFactoryConfigPreparer(
				applySupportedFiles,
				applyStarterWork,
			),
			wirefactorydefinitions.FactorySnapshotCapturer(),
		)
	}
}

func provideAutomationFactory() factorysessionwire.AutomationFactory {
	return func(
		logger *zap.Logger,
		clock factoryruntime.Clock,
		commandRunner workers.CommandRunner,
		workflowID string,
		defaultFactoryDir string,
		hostedPollers automations.HostedPollers,
	) automations.Service {
		return automationservice.NewService(
			logger,
			clock,
			commandRunner,
			workflowID,
			defaultFactoryDir,
			hostedPollers,
			workersservice.ResolveTemplateFields,
			factoryworkstationexecution.NewService(),
		)
	}
}

func provideFactorySessionsService(
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directories factorysessionwire.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles factorysessionwire.InvocationInputReader,
	initialWorkFiles factorysessionwire.InitialWorkReader,
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
) (factorysessions.Service, error) {
	return factorysessionwire.NewService(func() factoryruntime.JavaScriptCheckpointStore {
		return factorycheckpointstore.New()
	}, sessionResultProjection, interpolation, invocationWorkTypes, ttsObservability, eventIDs, sessionIDs, resolveHome, directories, namedPaths, invocationInputFiles, initialWorkFiles, resolveSymlinks)
}

func provideFactorySessionExecutionFactory(
	workflows factoryruntime.JavaScriptWorkflows,
	recordingWriter recordingartifacts.Writer,
	stores factorysessionwire.RuntimePersistenceStoreFactory,
	syncWaits factorysessionwire.SyncWaitScheduler,
	sessionIDs factorysessions.SessionIDGenerator,
	responseEventIDs factorysessions.ResponseEventIDGenerator,
	invocation factorysessionwire.WorkerInvocationFactory,
	invocationWithProgress factorysessionwire.WorkerInvocationWithProgressFactory,
	allocator agypty.PTYAllocator,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	edges serviceedges.Edges,
) factorysessionwire.FactorySessionExecutionFactory {
	return func(
		projectRoot string,
		persistencePolicy factorysessions.PersistencePolicy,
		provider workerprovider.Provider,
		clock factoryruntime.Clock,
		workerPresetIDs map[string]struct{},
		workerSettings factoryruntime.JavaScriptWorkerSettings,
		mockWorkersEnabled bool,
	) (factorysessions.ExecutionService, error) {
		executor := workerinvocation.NewExecutor(provider)
		var liveChildInvocation factorysessionwire.LiveChildInvocationFactory
		if !mockWorkersEnabled &&
			invocationWithProgress != nil &&
			adaptRunner != nil &&
			allocator != nil &&
			edges.ProviderCommandRunner != nil {
			runner := adaptRunner(edges.ProviderCommandRunner)
			liveChildInvocation = func(publisher workers.ProgressPublisher) (workers.InvocationExecutor, error) {
				return invocationWithProgress(runner, allocator, publisher)
			}
		}
		return factorysessionwire.NewDurableExecution(
			projectRoot,
			persistencePolicy,
			stores,
			executor,
			clock,
			syncWaits,
			factorycheckpointsummary.New(),
			workflows,
			workerPresetIDs,
			workerSettings,
			recordingWriter,
			sessionIDs,
			liveChildInvocation,
			responseEventIDs,
		)
	}
}

func provideStandaloneSessionExecutionFactory(
	workflows factoryruntime.JavaScriptWorkflows,
	recordingWriter recordingartifacts.Writer,
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
			factorycheckpointsummary.New(),
			workflows,
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
	return recordingsservice.NewProjectionService
}

func provideRuntimeLedgerFactory() factorysessionwire.RuntimeLedgerFactory {
	return func() factoryruntime.RuntimeLedgerFactory {
		return func(topology recordings.InitialStructureSource, now func() time.Time, definitions factorydefinitions.RuntimeDefinitionLookup) recordings.RuntimeEventLedger {
			return recordingsservice.NewRuntimeLedger(topology, now, uuid.NewString(), definitions)
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
	return recordingartifacts.NewAtomicWriter(makeDirectories, createTemporaryFile, removePath, renamePath)
}

func provideLoadedFactorySnapshotCapturer() factorydefinitions.LoadedFactorySnapshotCapturer {
	return wirefactorydefinitions.LoadedFactorySnapshotCapturer()
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
		return recordingsservice.NewLifecycleRuntimeRecorder(
			flushInterval,
			loaded,
			now,
			recordPath,
			captureLoadedFactorySnapshot,
		)
	}
}

func provideReplayClockFactory() factorysessionwire.ReplayClockFactory {
	return recordingsservice.NewReplayClock
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
		return recordingsservice.NewReplayExecution(
			artifact,
			wirefactorydefinitions.FactorySnapshotJSONDecoder(),
			wirefactorydefinitions.ReplayRuntimeConfigDecoder(),
		)
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func provideWorkersRuntimeFactory(
	interpolation factorydefinitions.InvocationInterpolationService,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	defaultAllocator agypty.PTYAllocator,
	edges serviceedges.Edges,
	providerRegistry *providerregistry.Registry,
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
		return workersservice.NewRuntimeWithSelection(
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
			factoryworkstationexecution.NewService(),
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
	return workersservice.BuildRuntimeExecutors
}

func provideWorkersMockCommandRunnerFactory() factoryruntime.WorkersMockCommandRunnerFactory {
	return workersservice.NewMockCommandRunner
}

func provideWorkerHostedPollersFactory(edges serviceedges.Edges) (factorysessionwire.WorkerHostedPollersFactory, error) {
	checkpointStore := edges.HostedLinearCheckpointStore
	if checkpointStore == nil {
		var err error
		checkpointStore, err = hostedlinear.NewCheckpointStore(platformfilesystem.Local{})
		if err != nil {
			return nil, err
		}
	}
	return func(
		logger *zap.Logger,
		clock workers.HostedPollerClock,
		httpClient workers.HostedPollerHTTPDoer,
		secretResolver workers.HostedPollerSecretResolver,
		linearEndpoint string,
	) automations.HostedPollers {
		if clock == nil {
			clock = clockwork.NewRealClock()
		}
		if httpClient == nil {
			httpClient = &http.Client{Timeout: hostedlinear.DefaultRequestTimeout}
		}
		if secretResolver == nil {
			secretResolver = hostedlinear.NewSecretResolver(os.Getenv, os.ReadFile)
		}
		return hostedworkers.New(logger, clock, httpClient, secretResolver, linearEndpoint, checkpointStore)
	}, nil
}

func provideWorkersLocalRuntimeHooksFactory() factorysessionwire.WorkersLocalRuntimeHooksFactory {
	return workersservice.LocalRuntimeHooks
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
		return workersservice.NewInvocationWithProgress(
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
		return workersservice.NewInvocation(
			runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
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
		return workersservice.NewProviderFromCommandRunner(
			runner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}

func provideSessionExecutionOpeningFactory(
	runtimes *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	build factorysessionwire.StandaloneSessionExecutionFactory,
	invocation factorysessionwire.WorkerInvocationFactory,
	resolveClock factoryruntime.ClockResolver,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	adaptRunner factorysessionwire.WorkerCommandRunnerAdapter,
	paths factorysessionwire.ExecutionOpeningFileSystem,
	allocator agypty.PTYAllocator,
	logger *zap.Logger,
) (*factorysessionwire.ExecutionOpeningFactory, error) {
	workerEdges, err := withStandaloneWorkerProductionEdges(edges)
	if err != nil {
		return nil, err
	}
	return factorysessionwire.NewExecutionOpeningFactory(
		runtimes, projectRuntimeOpeningExternalEffects(workerEdges), adaptRunner(workerEdges.ProviderCommandRunner), allocator,
		build, invocation, resolveClock, artifactRoots, adaptRunner, paths, logger,
	)
}

func provideInvocationOperation(
	openRuntime *factorysessionwire.RuntimeOpeningFactory,
	edges serviceedges.Edges,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessionwire.InvocationOperation, error) {
	return factorysessionwire.NewInvocationOperation(
		openRuntime,
		projectRuntimeOpeningExternalEffects(edges),
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

// projectRuntimeOpeningExternalEffects is the sole selection from the process
// edge aggregate into the effects consumed by Factory Session runtime opening.
func projectRuntimeOpeningExternalEffects(edges serviceedges.Edges) factorysessionwire.RuntimeOpeningExternalEffects {
	return factorysessionwire.RuntimeOpeningExternalEffects{
		Clock:                     edges.Clock,
		ProviderOverride:          edges.ProviderOverride,
		ModelPullMetricsRecorder:  edges.ModelPullMetricsRecorder,
		InvocationMetricsRecorder: edges.InvocationMetricsRecorder,
		ProviderCommandRunner:     edges.ProviderCommandRunner,
		ScriptCommandRunner:       edges.ScriptCommandRunner,
		SubmissionRecorder:        edges.SubmissionRecorder,
		DispatchRecorder:          edges.DispatchRecorder,
		RuntimeHostObserver:       edges.RuntimeHostObserver,
		HostedClock:               edges.HostedClock,
		HostedHTTPClient:          edges.HostedHTTPClient,
		HostedSecretResolver:      edges.HostedSecretResolver,
		HostedLinearEndpoint:      edges.HostedLinearEndpoint,
	}
}

func withStandaloneWorkerProductionEdges(overrides serviceedges.Edges) (serviceedges.Edges, error) {
	commandRunner, err := providePlatformProcessCommandRunner(overrides)
	if err != nil {
		return serviceedges.Edges{}, err
	}
	return serviceedges.Merge(serviceedges.Edges{
		ProviderCommandRunner: commandRunner,
		ScriptCommandRunner:   commandRunner,
	}, overrides), nil
}

func provideAgyPTYAllocator(edges serviceedges.Edges) (agypty.PTYAllocator, error) {
	clock := edges.AgyPTYClock
	if clock == nil {
		clock = platformclock.Real{}
	}
	host := edges.AgyPTYHost
	if host == nil {
		host = platformpty.NewHost()
	}
	return agypty.NewAllocator(host, clock)
}

func provideWorkerCommandRunnerAdapter() factorysessionwire.WorkerCommandRunnerAdapter {
	return workerprocess.AdaptCommandRunner
}

func provideWorkRequestIDGenerator(edges serviceedges.Edges) work.RequestIDGenerator {
	if edges.WorkRequestIDGenerator != nil {
		return edges.WorkRequestIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionRuntimeInstanceIDGenerator(edges serviceedges.Edges) factorysessions.RuntimeInstanceIDGenerator {
	if edges.FactorySessionRuntimeInstanceIDGenerator != nil {
		return edges.FactorySessionRuntimeInstanceIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionIDGenerator(edges serviceedges.Edges) factorysessions.SessionIDGenerator {
	if edges.FactorySessionIDGenerator != nil {
		return edges.FactorySessionIDGenerator
	}
	return uuid.NewString
}

func provideFactorySessionResponseEventIDGenerator(edges serviceedges.Edges) factorysessions.ResponseEventIDGenerator {
	if edges.FactorySessionResponseEventIDGenerator != nil {
		return edges.FactorySessionResponseEventIDGenerator
	}
	return uuid.NewString
}

func provideWorkSubmittedFileReader(edges serviceedges.Edges) work.SubmittedFileReader {
	if edges.WorkSubmittedFileReader != nil {
		return edges.WorkSubmittedFileReader
	}
	return os.ReadFile
}

func provideWorkFactory(
	readFile work.SubmittedFileReader,
	contentStaging work.ContentStagingService,
	contentMaterializer work.ContentMaterializer,
) factorysessionwire.WorkFactory {
	return func(runtimes work.RuntimeResolver) work.Service {
		return workservice.NewService(runtimes, readFile, contentStaging, contentMaterializer)
	}
}
