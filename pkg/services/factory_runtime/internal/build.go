package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"path/filepath"
	"strings"
)

const defaultSessionID = "~default"

// RuntimeFactory constructs hosted runtime bundles. It is stateless.
type runtimeWorkstationService = workers.WorkstationExecutionService

type RuntimeFactory struct {
	quorumPolicy              interfaces.QuorumPolicyService
	outputShaping             interfaces.InvocationOutputShapingService
	workPropagation           interfaces.WorkPropagationPolicyService
	workService               work.Service
	decisionEnvelopes         interfaces.DecisionEnvelopeService
	invocationInterpolation   interfaces.InvocationInterpolationService
	baseLogger                *zap.Logger
	loggerFactory             factory.RuntimeLoggerFactory
	runtimeLogs               factory.RuntimeLogOwner
	runtimeMetrics            factory.RuntimeMetricsOwner
	newID                     factory.IDGenerator
	workRequestIDs            work.RequestIDGenerator
	runtimeDirs               factory.RuntimeDirectoryFileSystem
	inputFiles                factory.InputFileSystem
	inputDirectoryWalker      factory.InputDirectoryWalker
	orchestrationCompilation  factory.OrchestrationCompilation
	providerSessions          providersessions.Service
	workerPoolBoundaryFactory factory.WorkstationPoolBoundaryFactory
}

func NewRuntimeFactory(
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	workService work.Service,
	decisionEnvelopes interfaces.DecisionEnvelopeService,
	invocationInterpolation interfaces.InvocationInterpolationService,
	baseLogger *zap.Logger,
	loggerFactory factory.RuntimeLoggerFactory,
	runtimeLogs factory.RuntimeLogOwner,
	runtimeMetrics factory.RuntimeMetricsOwner,
	newID factory.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles factory.InputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
	orchestrationCompilation factory.OrchestrationCompilation,
	providerSessions providersessions.Service,
	workerPoolBoundaryFactory factory.WorkstationPoolBoundaryFactory,
) *RuntimeFactory {
	return &RuntimeFactory{
		quorumPolicy:              quorumPolicy,
		outputShaping:             outputShaping,
		workPropagation:           workPropagation,
		workService:               workService,
		decisionEnvelopes:         decisionEnvelopes,
		invocationInterpolation:   invocationInterpolation,
		baseLogger:                baseLogger,
		loggerFactory:             loggerFactory,
		runtimeLogs:               runtimeLogs,
		runtimeMetrics:            runtimeMetrics,
		newID:                     newID,
		workRequestIDs:            workRequestIDs,
		runtimeDirs:               runtimeDirs,
		inputFiles:                inputFiles,
		inputDirectoryWalker:      inputDirectoryWalker,
		orchestrationCompilation:  orchestrationCompilation,
		providerSessions:          providerSessions,
		workerPoolBoundaryFactory: workerPoolBoundaryFactory,
	}
}

// Build constructs one hosted runtime bundle from explicit runtime values and
// collaborators. Dependencies are deliberately flat so Wire and callers expose
// the real construction graph.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (f *RuntimeFactory) Build(
	ctx context.Context,
	dir string,
	folderPath string,
	sessionID string,
	runnerID string,
	runtimeMode interfaces.RuntimeMode,
	verbose bool,
	runtimeScheduler scheduler.Scheduler,
	workerExecutorOverrides map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	runtimeFileLoggingPolicy RuntimeFileLoggingPolicy,
	runtimeMetricsPolicy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	loadedFactoryCfg factory.LoadedConfig,
	runtimeInstanceID string,
	backendScopeID string,
	clock factory.Clock,
	recordPath string,
	initialFactory *interfaces.FactorySnapshot,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeOpening,
	loadWorkerExecutors func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error),
	workerService runtimeWorkstationService,
	providerInvocation workers.WorkstationRequestExecutor,
	workerSessionsFactory factory.WorkerSessionsFactory,
	dispatchCompleted func(string),
	mockWorkersConfigs ...*workers.MockWorkersConfig,
) (*factoryhost.Bundle, error) {
	if f == nil || f.newID == nil {
		return nil, fmt.Errorf("Factory Runtime ID generator is required")
	}
	if f.workRequestIDs == nil {
		return nil, fmt.Errorf("Work Request ID generator is required")
	}
	if f.runtimeDirs == nil || f.inputFiles == nil || f.inputDirectoryWalker == nil {
		return nil, fmt.Errorf("Factory Runtime runtime directory filesystem, input filesystem, and input directory walker are required")
	}
	if f.orchestrationCompilation == nil {
		return nil, fmt.Errorf("Factory Runtime orchestration compilation is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("Factory Runtime clock is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = defaultSessionID
	}
	if f == nil || f.loggerFactory == nil {
		return nil, fmt.Errorf("runtime logger factory is required")
	}
	logSink, runtimeInstanceID, err := openRuntimeLogScope(
		f.runtimeLogs,
		runtimeFileLoggingPolicy,
		runtimeLogDir,
		runtimeLogConfig,
		sessionID,
		folderPath,
		dir,
		runtimeInstanceID,
	)
	if err != nil {
		return nil, err
	}
	logger := newSessionLogger(
		runtimeSessionBaseLogger(f.baseLogger, logSink),
		sessionID,
		folderPath,
		dir,
	)
	structuredLogger := f.loggerFactory(logger, verbose)
	if structuredLogger == nil {
		_ = factoryhost.CloseBundleSinks(logSink, nil)
		return nil, fmt.Errorf("runtime logger factory returned nil")
	}
	if workerSessionsFactory == nil {
		_ = factoryhost.CloseBundleSinks(logSink, nil)
		return nil, fmt.Errorf("Worker Sessions factory is required")
	}
	if f.workerPoolBoundaryFactory == nil {
		_ = logSink.Close()
		return nil, fmt.Errorf("Workstation pool boundary factory is required")
	}
	metricsSink, err := openRuntimeMetricsScope(
		f.runtimeMetrics,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		sessionID,
		runtimeInstanceID,
		folderPath,
		dir,
	)
	if err != nil {
		_ = factoryhost.CloseBundleSinks(logSink, nil)
		return nil, err
	}
	bundleBuilt := false
	runtimeScopeOwned := false
	var runtimeScopeRecorder recordings.RuntimeRecorder
	defer func() {
		if !bundleBuilt {
			if runtimeScopeOwned && runtimeScopeRecorder != nil {
				_ = runtimeScopeRecorder.Finalize(clock.Now().UTC())
			}
			_ = factoryhost.CloseBundleSinks(logSink, metricsSink)
		}
	}()
	net, err := f.compileOrchestrationNet(ctx, dir, loadedFactoryCfg.FactoryConfig(), logger)
	if err != nil {
		return nil, err
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(runnerID, loadedFactoryCfg.FactoryConfig())
	if recordingsRuntime == nil {
		return nil, fmt.Errorf("Recordings runtime opening is required")
	}
	loaded, ok := loadedFactoryCfg.(interfaces.LoadedFactorySource)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("loaded Factory source is required for Recordings runtime scope")
	}
	if err := validateConfiguredRuntimeWorkers(loadedFactoryCfg); err != nil {
		return nil, err
	}
	opened, openErr := recordingsRuntime.OpenRuntime(ctx, recordings.RuntimeScopeRequest{
		Topology:         net,
		Definitions:      loadedFactoryCfg,
		LoadedFactory:    loaded,
		Now:              clock.Now,
		RecordingID:      runtimeInstanceID,
		RecordPath:       recordPath,
		FactorySessionID: sessionID,
	})
	if openErr != nil {
		return nil, openErr
	}
	eventHistory := opened.Ledger
	recording := opened.Recorder
	runtimeScopeRecorder = opened.Recorder
	runtimeScopeOwned = opened.Recorder != nil
	if eventHistory == nil {
		return nil, fmt.Errorf("Recordings runtime ledger is required")
	}
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	if initialFactory != nil {
		eventHistory.SetInitialStructureFactory(initialFactory)
	}
	var workerExecutors map[string]workers.WorkerExecutor
	if loadWorkerExecutors != nil {
		workerExecutors, err = loadWorkerExecutors(eventHistory, logger)
		if err != nil {
			return nil, err
		}
	}
	var mockWorkersConfig *workers.MockWorkersConfig
	if len(mockWorkersConfigs) > 0 {
		mockWorkersConfig = mockWorkersConfigs[0]
	}
	var sharedWorkersService workers.Service
	if service, ok := workerService.(workers.Service); ok {
		sharedWorkersService = service
	}
	var promptRenderer runtime.PromptRenderer
	if renderer, ok := workerService.(runtime.PromptRenderer); ok {
		promptRenderer = renderer
	}
	var templateFieldResolver runtime.TemplateFieldResolver
	if resolver, ok := workerService.(runtime.TemplateFieldResolver); ok {
		templateFieldResolver = resolver
	}
	var progressPublisher workers.ProgressPublisher
	if publisherProvider, ok := workerService.(interface {
		RuntimeProgressPublisher() workers.ProgressPublisher
	}); ok {
		progressPublisher = publisherProvider.RuntimeProgressPublisher()
	}
	directWorkstationExecutor := runtime.NewWorkstationRequestExecutor(
		runtime.WorkstationRequestExecutorConfig{
			Service:                    sharedWorkersService,
			RuntimeDefinitions:         loadedFactoryCfg,
			InvocationInterpolation:    f.invocationInterpolation,
			InvocationFileReader:       invocationFileReader(f.inputFiles),
			WorkflowContext:            RuntimeWorkflowContext(loadedFactoryCfg.FactoryConfig(), sessionID),
			FactorySessionID:           sessionID,
			RuntimeID:                  runtimeInstanceID,
			RecordingID:                workerRecordingIdentity(runtimeInstanceID, recordPath),
			EventHistory:               eventHistory,
			NewID:                      f.newID,
			PromptRenderer:             promptRenderer,
			TemplateFieldResolver:      templateFieldResolver,
			PromptSourceReader:         invocationFileReader(f.inputFiles),
			MockWorkers:                mockWorkersConfig,
			ProgressPublisher:          progressPublisher,
			Net:                        net,
			ExpectedArtifactFileSystem: f.inputFiles,
		},
	)

	bundle, err := assembleRuntimeBundle(
		dir, folderPath, sessionID, runtimeMode, verbose, runtimeScheduler,
		workerExecutorOverrides, workerExecutorDecorator, inlineDispatch, submissionRecorder,
		dispatchRecorder,
		loadedFactoryCfg, runtimeInstanceID,
		backendScopeID, clock, recordPath, recording, submissionHooks,
		completionPlanner, petriMutationRecorder, worldStateProjector,
		dispatchCompleted, logger, structuredLogger, logSink, metricsSink, net, eventHistory,
		workerExecutors,
		workerService,
		directWorkstationExecutor,
		mockWorkersConfig,
		providerInvocation,
		workerSessionsFactory,
		f.workerPoolBoundaryFactory,
		f.providerSessions,
		f.workService,
		f.quorumPolicy,
		f.outputShaping,
		f.workPropagation,
		f.workRequestIDs,
		f.newID,
		f.runtimeDirs,
		f.inputFiles,
		f.inputDirectoryWalker,
		f.decisionEnvelopes,
		f.invocationInterpolation,
	)
	if err != nil {
		return nil, err
	}
	bundleBuilt = true
	return bundle, nil
}

func validateConfiguredRuntimeWorkers(loaded factory.LoadedConfig) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return fmt.Errorf("factory config is required")
	}
	for _, configured := range loaded.FactoryConfig().Workers {
		definition, ok := loaded.Worker(configured.Name)
		if !ok || definition == nil {
			continue
		}
		if interfaces.IsScriptWorkerType(definition.Type) && strings.TrimSpace(definition.Command) == "" {
			return fmt.Errorf(
				"construct script worker %q: misconfigured: script command is required",
				configured.Name,
			)
		}
	}
	return nil
}

func assembleRuntimeBundle(
	dir string,
	folderPath string,
	sessionID string,
	runtimeMode interfaces.RuntimeMode,
	verbose bool,
	runtimeScheduler scheduler.Scheduler,
	workerExecutorOverrides map[string]workers.WorkerExecutor,
	workerExecutorDecorator func(string, workers.WorkerExecutor) workers.WorkerExecutor,
	inlineDispatch bool,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	loadedFactoryCfg factory.LoadedConfig,
	runtimeInstanceID string,
	backendScopeID string,
	clock factory.Clock,
	recordPath string,
	recording recordings.RuntimeRecorder,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	dispatchCompleted func(string),
	logger *zap.Logger,
	structuredLogger factory.Logger,
	logSink factory.RuntimeLogSink,
	metricsSink factory.RuntimeMetricsSink,
	net *state.Net,
	eventHistory recordings.RuntimeLedger,
	workerExecutors map[string]workers.WorkerExecutor,
	workerService runtimeWorkstationService,
	directWorkstationExecutor workers.WorkstationRequestExecutor,
	mockWorkersConfig *workers.MockWorkersConfig,
	providerInvocation workers.WorkstationRequestExecutor,
	workerSessionsFactory factory.WorkerSessionsFactory,
	workerPoolBoundaryFactory factory.WorkstationPoolBoundaryFactory,
	providerSessions providersessions.Service,
	workService work.Service,
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	workRequestIDs work.RequestIDGenerator,
	newID factory.IDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles factory.InputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
	decisionEnvelopes interfaces.DecisionEnvelopeService,
	invocationInterpolation interfaces.InvocationInterpolationService,
) (*factoryhost.Bundle, error) {
	workerExecutors = prepareRuntimeWorkerExecutors(workerExecutors, workerExecutorOverrides, workerExecutorDecorator)
	bindRuntimeLogger(directWorkstationExecutor, structuredLogger)
	bundle := factoryhost.NewBundle(
		dir, folderPath, runtimeInstanceID, strings.TrimSpace(backendScopeID),
		clock.Now().UTC(), eventHistory, net, loadedFactoryCfg,
		logger, logSink, metricsSink, recording, recordPath, dispatchCompleted,
	)
	factoryEventRecorder := runtimeFactoryEventRecorder(recordPath, recording)
	// Runtime dispatch uses the detached Workers Execute capability below. The
	// workstation boundary remains only for the Worker Sessions direct/child
	// compatibility path until P5C-4/P6-C retires that graph.
	workstationBoundary := buildRuntimeWorkstationBoundary(
		workerPoolBoundaryFactory,
		workerService,
		workerExecutors,
		net,
		directWorkstationExecutor,
		providerInvocation,
	)
	workerSessions, err := buildRuntimeWorkerSessions(workerSessionsFactory, workstationBoundary, clock)
	if err != nil {
		return nil, err
	}
	// TODO(P6-C): delete the legacy workstation pool/request-executor graph
	// after the remaining Worker Sessions caller family migrates to Execute.
	statelessService, err := requireRuntimeExecuteService(workerService)
	if err != nil {
		return nil, err
	}
	effectiveSubmissionRecorder := runtimeSubmissionRecorder(bundle, submissionRecorder)
	activeFactory, err := runtime.New(
		net,
		runtimeScheduler,
		statelessService,
		workerSessions,
		loadedFactoryCfg,
		invocationInterpolation,
		invocationFileReader(inputFiles),
		RuntimeWorkflowContext(loadedFactoryCfg.FactoryConfig(), sessionID),
		runtimeMode,
		structuredLogger,
		clock,
		inlineDispatch,
		eventHistory,
		workerRecordingIdentity(runtimeInstanceID, recordPath),
		runtimeInstanceID,
		worldStateProjector,
		providerSessions,
		effectiveSubmissionRecorder,
		factoryEventRecorder,
		submissionHooks,
		effectiveDispatchRecorder(dispatchRecorder, bundle.RecordDispatchMetric),
		bundle.RecordCompletionMetrics,
		petriMutationRecorder,
		completionPlanner,
		quorumPolicy,
		outputShaping,
		workPropagation,
		workService,
		workRequestIDs,
		newID,
		runtimeDirs,
		decisionEnvelopes,
	)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	if err := configureRuntimeFactory(activeFactory, mockWorkersConfig, inputFiles, dir, logger, runtimeDirs); err != nil {
		return nil, err
	}

	bundle.Factory = activeFactory
	bundle.InputFiles = inputFiles
	bundle.InputDirectoryWalker = inputDirectoryWalker
	bundle.WorkRequestIDs = workRequestIDs
	return bundle, nil
}

func prepareRuntimeWorkerExecutors(
	workerExecutors map[string]workers.WorkerExecutor,
	overrides map[string]workers.WorkerExecutor,
	decorate func(string, workers.WorkerExecutor) workers.WorkerExecutor,
) map[string]workers.WorkerExecutor {
	if workerExecutors == nil {
		workerExecutors = make(map[string]workers.WorkerExecutor)
	}
	for workerType, executor := range overrides {
		workerExecutors[workerType] = executor
	}
	if decorate != nil {
		for workerType, executor := range workerExecutors {
			workerExecutors[workerType] = decorate(workerType, executor)
		}
	}
	return workerExecutors
}

func runtimeFactoryEventRecorder(
	recordPath string,
	recording recordings.RuntimeRecorder,
) factory.FactoryEventRecorder {
	if recordPath == "" {
		return nil
	}
	return func(event interfaces.FactoryEvent) {
		if recording != nil {
			recording.RecordEvent(event)
		}
	}
}

func buildRuntimeWorkerSessions(
	workerSessionsFactory factory.WorkerSessionsFactory,
	boundary workers.WorkstationPoolBoundary,
	clock platformclock.Source,
) (workersessions.Service, error) {
	workerSessions, err := workerSessionsFactory(boundary, clock)
	if err != nil {
		return nil, fmt.Errorf("construct Worker Sessions service: %w", err)
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("construct Worker Sessions service: factory returned nil")
	}
	return workerSessions, nil
}

func requireRuntimeExecuteService(
	workerService runtimeWorkstationService,
) (interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}, error) {
	statelessService, ok := workerService.(interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	})
	if !ok || statelessService == nil {
		return nil, fmt.Errorf("Workers Execute service is required for Factory Runtime dispatch")
	}
	return statelessService, nil
}

func runtimeSubmissionRecorder(
	bundle *factoryhost.Bundle,
	submissionRecorder recordings.SubmissionRecorder,
) recordings.SubmissionRecorder {
	if submissionRecorder != nil {
		return submissionRecorder
	}
	return recordings.SubmissionRecorder(bundle.RecordSubmissionMetric)
}

func configureRuntimeFactory(
	activeFactory factoryhost.Engine,
	mockWorkersConfig *workers.MockWorkersConfig,
	inputFiles factory.InputFileSystem,
	dir string,
	logger *zap.Logger,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
) error {
	if configurable, ok := activeFactory.(interface {
		SetMockWorkersConfig(*workers.MockWorkersConfig)
	}); ok {
		configurable.SetMockWorkersConfig(mockWorkersConfig)
	}
	if configurable, ok := activeFactory.(interface {
		SetPromptSourceReader(func(string) ([]byte, error))
	}); ok && inputFiles != nil {
		configurable.SetPromptSourceReader(inputFiles.ReadFile)
	}
	return ensureRuntimeInputsDir(dir, logger, runtimeDirs)
}

func bindRuntimeLogger(executor workers.WorkstationRequestExecutor, logger factory.Logger) {
	if loggerBinder, ok := executor.(interface {
		SetRuntimeLogger(factory.Logger)
	}); ok {
		loggerBinder.SetRuntimeLogger(logger)
	}
}

func invocationFileReader(inputFiles factory.InputFileSystem) interfaces.FileReader {
	if inputFiles == nil {
		return nil
	}
	return inputFiles.ReadFile
}

// workerRecordingIdentity keeps the Worker source-native recording identity
// distinct from the user-facing artifact path. Artifact paths are allowed to
// be absolute and may contain platform-specific separators or spaces, while
// Events source identities must remain portable opaque tokens. The identity is
// derived from the concrete runtime/Recording lifecycle identity, so all
// Worker Sessions captured by one Factory recording share one durable snapshot
// identity without allowing later recordings at the same path to collide.
func workerRecordingIdentity(recordingID, recordPath string) string {
	recordPath = strings.TrimSpace(recordPath)
	recordingID = strings.TrimSpace(recordingID)
	if recordPath == "" || recordingID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(recordingID))
	return "worker-recording-" + hex.EncodeToString(digest[:])
}

func ensureRuntimeInputsDir(
	factoryDir string,
	logger *zap.Logger,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
) error {
	inputsDir := filepath.Join(factoryDir, interfaces.InputsDir)
	if !dirExists(inputsDir, runtimeDirs) {
		if err := runtimeDirs.MkdirAll(inputsDir, 0o755); err != nil {
			return fmt.Errorf("create inputs dir: %w", err)
		}
		return nil
	}
	logger.Info("using inputs/ directory", zap.String("dir", inputsDir))
	return nil
}

func newSessionLogger(base *zap.Logger, sessionID string, folderPath string, factoryDir string) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("session_id", sessionID),
		zap.String("folder_path", folderPath),
		zap.String("factory_dir", factoryDir),
	)
}

// RuntimeWorkflowContext derives the canonical project/session execution context.
func RuntimeWorkflowContext(cfg *interfaces.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	projectID := factory_context.DefaultProjectID
	if cfg != nil && cfg.Project != "" {
		projectID = factory_context.ResolveProjectID(cfg.Project, nil, nil)
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = defaultSessionID
	}
	return &factory_context.FactoryContext{
		ProjectID: projectID,
		EnvVars:   make(map[string]string),
		SessionID: sessionID,
	}
}

type inputWorkflowSourceFiles struct {
	files factory.InputFileSystem
}

func (f inputWorkflowSourceFiles) ReadDir(path string) ([]fs.DirEntry, error) {
	return f.files.ReadDir(path)
}

func (f inputWorkflowSourceFiles) ReadFile(path string) ([]byte, error) {
	return f.files.ReadFile(path)
}

func (f inputWorkflowSourceFiles) Stat(path string) (fs.FileInfo, error) {
	return f.files.Stat(path)
}

func (f *RuntimeFactory) compileOrchestrationNet(
	ctx context.Context,
	dir string,
	cfg *interfaces.FactoryConfig,
	logger *zap.Logger,
) (*state.Net, error) {
	compileReq := factory.OrchestrationCompileRequest{
		Config:       cfg,
		FactoryDir:   dir,
		SourceReader: factory.NewWorkflowSourceReader(dir, inputWorkflowSourceFiles{files: f.inputFiles}),
	}
	compiled, err := f.orchestrationCompilation.Compile(ctx, compileReq)
	if err != nil {
		logger.Error("failed to compile factory orchestration", zap.Error(err))
		return nil, fmt.Errorf("compile factory orchestration: %w", err)
	}
	switch compiled.Kind {
	case factory.OrchestrationKindPetri:
		net, err := f.orchestrationCompilation.CompilePetriNet(ctx, compileReq)
		if err != nil {
			logger.Error("failed to compile factory orchestration", zap.Error(err))
			return nil, fmt.Errorf("compile factory orchestration: %w", err)
		}
		return net, nil
	case factory.OrchestrationKindJavaScript:
		mapper, err := definitionmapping.New(f.newID)
		if err != nil {
			return nil, err
		}
		net, err := mapper.Map(ctx, cfg)
		if err != nil {
			logger.Error("failed to map JavaScript factory runtime net", zap.Error(err))
			return nil, fmt.Errorf("compile factory orchestration: %w", err)
		}
		return net, nil
	default:
		return nil, fmt.Errorf("compile factory orchestration: unsupported orchestration kind %q", compiled.Kind)
	}
}

func effectiveFactoryRunnerID(override string, factoryCfg *interfaces.FactoryConfig) string {
	if runner := workers.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if factoryCfg == nil {
		return ""
	}
	return workers.NormalizeRunnerID(factoryCfg.Runner)
}

func effectiveDispatchRecorder(
	override recordings.DispatchRecorder,
	defaultRecorder recordings.DispatchRecorder,
) recordings.DispatchRecorder {
	if override != nil {
		return override
	}
	return defaultRecorder
}

func dirExists(path string, files factory.RuntimeDirectoryFileSystem) bool {
	info, err := files.Stat(path)
	return err == nil && info.IsDir()
}

func openRuntimeLogScope(
	owner factory.RuntimeLogOwner,
	policy RuntimeFileLoggingPolicy,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	sessionID string,
	folderPath string,
	factoryDir string,
	runtimeInstanceID string,
) (factory.RuntimeLogSink, string, error) {
	if strings.TrimSpace(runtimeInstanceID) == "" {
		return nil, runtimeInstanceID, fmt.Errorf("runtime instance ID is required")
	}
	if !runtimeFileLoggingEnabled(policy) {
		return nil, runtimeInstanceID, nil
	}
	if owner == nil {
		return nil, runtimeInstanceID, fmt.Errorf("runtime log owner is required")
	}
	logSink, err := owner.Open(factory.RuntimeLogScopeRequest{
		SessionID: sessionID, RuntimeInstanceID: runtimeInstanceID,
		FolderPath: folderPath, FactoryDirectory: factoryDir,
		RootDirectory: runtimeLogDir, Policy: policy, Config: runtimeLogConfig,
	})
	if err != nil {
		return nil, runtimeInstanceID, fmt.Errorf("open runtime log scope: %w", err)
	}
	if logSink == nil {
		return nil, runtimeInstanceID, fmt.Errorf("runtime log owner returned nil scope")
	}
	return &closeOnceRuntimeLogSink{RuntimeLogSink: logSink}, runtimeInstanceID, nil
}

func runtimeFileLoggingEnabled(policy RuntimeFileLoggingPolicy) bool {
	switch policy {
	case "", RuntimeFileLoggingPolicyEnabled:
		return true
	case RuntimeFileLoggingPolicyDisabled:
		return false
	default:
		return true
	}
}

func runtimeMetricsEnabled(policy RuntimeMetricsPolicy) bool {
	switch policy {
	case "", RuntimeMetricsPolicyEnabled:
		return true
	case RuntimeMetricsPolicyDisabled:
		return false
	default:
		return true
	}
}

func runtimeSessionBaseLogger(baseLogger *zap.Logger, logSink factory.RuntimeLogSink) *zap.Logger {
	if logSink != nil {
		return logSink.Logger()
	}
	if baseLogger != nil {
		return baseLogger
	}
	return zap.NewNop()
}

func openRuntimeMetricsScope(
	owner factory.RuntimeMetricsOwner,
	policy RuntimeMetricsPolicy,
	runtimeMetricsDir string,
	runtimeMetricsConfig factory.RuntimeMetricsStorageConfig,
	sessionID string,
	runtimeInstanceID string,
	folderPath string,
	factoryDir string,
) (factory.RuntimeMetricsSink, error) {
	if !runtimeMetricsEnabled(policy) {
		return nil, nil
	}
	if owner == nil {
		return nil, fmt.Errorf("runtime metrics owner is required")
	}
	metricsSink, err := owner.Open(factory.RuntimeMetricsScopeRequest{
		Scope: factory.RuntimeMetricsScope{
			SessionID: sessionID, RuntimeInstanceID: runtimeInstanceID,
			FolderPath: folderPath, FactoryDir: factoryDir,
		},
		RootDirectory: runtimeMetricsDir,
		Policy:        policy,
		Config:        runtimeMetricsConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("open runtime metrics scope: %w", err)
	}
	if metricsSink == nil {
		return nil, fmt.Errorf("runtime metrics owner returned nil scope")
	}
	return &closeOnceRuntimeMetricsSink{RuntimeMetricsSink: metricsSink}, nil
}

type closeOnceRuntimeLogSink struct {
	factory.RuntimeLogSink
	once sync.Once
	err  error
}

func (sink *closeOnceRuntimeLogSink) Close() error {
	if sink == nil || sink.RuntimeLogSink == nil {
		return nil
	}
	sink.once.Do(func() { sink.err = sink.RuntimeLogSink.Close() })
	return sink.err
}

type closeOnceRuntimeMetricsSink struct {
	factory.RuntimeMetricsSink
	once sync.Once
	err  error
}

func (sink *closeOnceRuntimeMetricsSink) Close() error {
	if sink == nil || sink.RuntimeMetricsSink == nil {
		return nil
	}
	sink.once.Do(func() { sink.err = sink.RuntimeMetricsSink.Close() })
	return sink.err
}
