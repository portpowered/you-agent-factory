package internal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"path/filepath"
	"strings"
)

const defaultSessionID = "~default"

// inputFileSystem is the Factory Runtime-owned input-tree effect. It stays
// below the service root; the wire package exposes only the construction seam
// needed by the process composition boundary.
type inputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// engineOpening opens scoped state using the already selected runtime behavior.
type engineOpening interface {
	Open(
		net *state.Net,
		runtimeScheduler scheduler.Scheduler,
		statelessService workers.Service,
		workerSessionsService workersessions.Service,
		workerAttempts factory.WorkerAttemptOpener,
		runtimeDefinitions interfaces.RuntimeDefinitionLookup,
		invocationFileReader interfaces.FileReader,
		workflowContext *factory_context.FactoryContext,
		publicSessionID string,
		runtimeMode interfaces.RuntimeMode,
		logger factory.Logger,
		clock factory.Clock,
		workerAttemptScheduler platformclock.TimerSource,
		inlineDispatch bool,
		eventHistory recordings.RuntimeLedger,
		recordingID string,
		runtimeID string,
		worldStateProjector factory.WorldStateProjector,
		restoredWorldState *interfaces.FactoryWorldState,
		skipRestoredDispatchReconciliation bool,
		submissionRecorder recordings.SubmissionRecorder,
		factoryEventRecorder factory.FactoryEventRecorder,
		submissionHooks []factory.SubmissionHook,
		dispatchRecorder recordings.DispatchRecorder,
		completionRecorder factory.CompletionRecorder,
		petriMutationRecorder factory.PetriMutationRecorder,
		completionDeliveryPlanner factory.CompletionDeliveryPlanner,
	) (factoryhost.Engine, error)
}

// RuntimeFactory constructs hosted runtime bundles. It is stateless.

type RuntimeFactory struct {
	loggerFactory            factory.RuntimeLoggerFactory
	runtimeLogs              factory.RuntimeLogOwner
	runtimeMetrics           factory.RuntimeMetricsOwner
	newID                    factory.IDGenerator
	workRequestIDs           work.RequestIDGenerator
	runtimeDirs              factory.RuntimeDirectoryFileSystem
	inputFiles               inputFileSystem
	inputDirectoryWalker     factory.InputDirectoryWalker
	orchestrationCompilation factory.OrchestrationCompilation
	engineOpening            engineOpening
	definitionMapper         definitionmapping.Mapping
	workerAttemptScheduler   platformclock.TimerSource
	submissionRecorder       recordings.SubmissionRecorder
	dispatchRecorder         recordings.DispatchRecorder
	worldStateProjector      factory.WorldStateProjector
	recordingsRuntime        recordings.RuntimeScopeService
}

func NewRuntimeFactory(
	loggerFactory factory.RuntimeLoggerFactory,
	runtimeLogs factory.RuntimeLogOwner,
	runtimeMetrics factory.RuntimeMetricsOwner,
	newID factory.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles inputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
	orchestrationCompilation factory.OrchestrationCompilation,
	workerAttemptScheduler platformclock.TimerSource,
	definitionMapper definitionmapping.Mapping,
	engineOpening engineOpening,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	worldStateProjector factory.WorldStateProjector,
	recordingsRuntime recordings.RuntimeScopeService,
) *RuntimeFactory {
	return &RuntimeFactory{
		loggerFactory:            loggerFactory,
		runtimeLogs:              runtimeLogs,
		runtimeMetrics:           runtimeMetrics,
		newID:                    newID,
		workRequestIDs:           workRequestIDs,
		runtimeDirs:              runtimeDirs,
		inputFiles:               inputFiles,
		inputDirectoryWalker:     inputDirectoryWalker,
		orchestrationCompilation: orchestrationCompilation,
		workerAttemptScheduler:   workerAttemptScheduler,
		definitionMapper:         definitionMapper,
		engineOpening:            engineOpening,
		submissionRecorder:       submissionRecorder,
		dispatchRecorder:         dispatchRecorder,
		worldStateProjector:      worldStateProjector,
		recordingsRuntime:        recordingsRuntime,
	}
}

// Build opens session-owned resources using fixed process behavior.
// Worker boundaries and observations belong to the admitted session.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (f *RuntimeFactory) Build(
	ctx context.Context,
	baseLogger *zap.Logger,
	dir string,
	folderPath string,
	sessionID string,
	metricsSessionID string,
	runnerID string,
	runtimeMode interfaces.RuntimeMode,
	verbose bool,
	runtimeScheduler scheduler.Scheduler,
	inlineDispatch bool,
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
	restoredWorldState *interfaces.FactoryWorldState,
	skipRestoredDispatchReconciliation bool,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	recordFlushInterval time.Duration,
	resumeCanonicalEvents []interfaces.FactoryEvent,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
	dispatchCompleted func(string),
	mockWorkersConfigs ...*workers.MockWorkersConfig,
) (result *factoryhost.Bundle, buildErr error) {
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
	// Retain ownership before the first open. Finalize the recording before
	// closing its sinks, and preserve cleanup failures alongside the build error.
	var logSink factory.RuntimeLogSink
	var metricsSink factory.RuntimeMetricsSink
	var runtimeScopeRecorder recordings.RuntimeRecorder
	bundleBuilt := false
	defer func() {
		if !bundleBuilt {
			var recordingErr error
			if runtimeScopeRecorder != nil {
				recordingErr = runtimeScopeRecorder.Finalize(clock.Now().UTC())
				if recordingErr == nil {
					runtimeScopeRecorder = nil
				}
			}
			sinksErr := factoryhost.CloseBundleSinks(logSink, metricsSink)
			buildErr = errors.Join(buildErr, recordingErr, sinksErr)
			if recordingErr != nil || sinksErr != nil {
				// A failed build has no runnable service, but its unreleased
				// resources must reach the opening owner's retryable cleanup.
				// Sink wrappers suppress releases that already succeeded.
				result = &factoryhost.Bundle{
					Dir: dir, FolderPath: folderPath, FactorySessionID: sessionID,
					RuntimeInstanceID: runtimeInstanceID, BackendScopeID: backendScopeID,
					Recording: runtimeScopeRecorder, LogSink: logSink, MetricsSink: metricsSink,
				}
			}
		}
	}()
	logSink, runtimeInstanceID, err := openRuntimeLogScope(
		f.runtimeLogs,
		baseLogger,
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
		runtimeSessionBaseLogger(baseLogger, logSink),
		sessionID,
		folderPath,
		dir,
	)
	structuredLogger := f.loggerFactory(logger, verbose)
	if structuredLogger == nil {
		return nil, fmt.Errorf("runtime logger factory returned nil")
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("worker sessions service is required")
	}
	metricsSink, err = openRuntimeMetricsScope(
		f.runtimeMetrics,
		runtimeMetricsPolicy,
		runtimeMetricsDir,
		runtimeMetricsConfig,
		metricsSessionID,
		runtimeInstanceID,
		folderPath,
		dir,
	)
	if err != nil {
		return nil, err
	}
	net, err := f.compileOrchestrationNet(ctx, dir, loadedFactoryCfg.FactoryConfig(), logger)
	if err != nil {
		return nil, err
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(runnerID, loadedFactoryCfg.FactoryConfig())
	if f.recordingsRuntime == nil {
		return nil, fmt.Errorf("Recordings runtime opening is required")
	}
	loaded, ok := loadedFactoryCfg.(interfaces.LoadedFactorySource)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("loaded Factory source is required for Recordings runtime scope")
	}
	if err := validateConfiguredRuntimeWorkers(loadedFactoryCfg); err != nil {
		return nil, err
	}
	// Prepare the input destination before binding a recording identity.
	// A failed directory open must leave that identity available for retry;
	// finalizing a bound recording would permanently reject its later writes.
	if err := ensureRuntimeInputsDir(dir, logger, f.runtimeDirs); err != nil {
		return nil, err
	}
	// Filesystem effects may complete after cancellation. Do not bind an
	// unpublished recording or engine when opening admission has been canceled.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opened, openErr := f.recordingsRuntime.OpenRuntime(ctx, recordings.RuntimeScopeRequest{
		Topology:           net,
		FlushInterval:      recordFlushInterval,
		ReplayEvents:       cloneFactoryEvents(resumeCanonicalEvents),
		Definitions:        loadedFactoryCfg,
		LoadedFactory:      loaded,
		Now:                clock.Now,
		RecordingID:        runtimeInstanceID,
		RecordPath:         recordPath,
		FactorySessionID:   sessionID,
		CanonicalSessionID: metricsSessionID,
	})
	runtimeScopeRecorder = opened.Recorder
	if openErr != nil {
		return nil, openErr
	}
	eventHistory := opened.Ledger
	recording := opened.Recorder
	if eventHistory == nil {
		return nil, fmt.Errorf("Recordings runtime ledger is required")
	}
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	if scoped, ok := eventHistory.(interface{ SetPublicSessionID(string) }); ok {
		scoped.SetPublicSessionID(sessionID)
	}
	if initialFactory != nil {
		eventHistory.SetInitialStructureFactory(initialFactory)
	}
	if source, ok := loadedFactoryCfg.(interface {
		InvocationSensitiveJSONPointers() []string
	}); ok {
		if setter, ok := eventHistory.(interface {
			SetInvocationSensitiveJSONPointers([]string)
		}); ok {
			setter.SetInvocationSensitiveJSONPointers(source.InvocationSensitiveJSONPointers())
		}
	}
	var mockWorkersConfig *workers.MockWorkersConfig
	if len(mockWorkersConfigs) > 0 {
		mockWorkersConfig = mockWorkersConfigs[0]
	}
	bundle, err := f.assembleRuntimeBundle(
		ctx,
		dir, folderPath, sessionID, metricsSessionID, runtimeMode, verbose, runtimeScheduler,
		inlineDispatch,
		loadedFactoryCfg, runtimeInstanceID,
		backendScopeID, clock, recordPath, recording, submissionHooks,
		completionPlanner, petriMutationRecorder, restoredWorldState,
		skipRestoredDispatchReconciliation,
		dispatchCompleted, logger, structuredLogger, logSink, metricsSink, net, eventHistory,
		workerService,
		mockWorkersConfig,
		workerSessions,
		workerAttempts,
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

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (f *RuntimeFactory) assembleRuntimeBundle(
	ctx context.Context,
	dir string,
	folderPath string,
	sessionID string,
	canonicalSessionID string,
	runtimeMode interfaces.RuntimeMode,
	verbose bool,
	runtimeScheduler scheduler.Scheduler,
	inlineDispatch bool,
	loadedFactoryCfg factory.LoadedConfig,
	runtimeInstanceID string,
	backendScopeID string,
	clock factory.Clock,
	recordPath string,
	recording recordings.RuntimeRecorder,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	restoredWorldState *interfaces.FactoryWorldState,
	skipRestoredDispatchReconciliation bool,
	dispatchCompleted func(string),
	logger *zap.Logger,
	structuredLogger factory.Logger,
	logSink factory.RuntimeLogSink,
	metricsSink factory.RuntimeMetricsSink,
	net *state.Net,
	eventHistory recordings.RuntimeLedger,
	workerService workers.Service,
	mockWorkersConfig *workers.MockWorkersConfig,
	workerSessions workersessions.Service,
	workerAttempts factory.WorkerAttemptOpener,
) (*factoryhost.Bundle, error) {
	bundle := factoryhost.NewBundle(
		dir, folderPath, runtimeInstanceID, sessionID, strings.TrimSpace(backendScopeID),
		clock.Now().UTC(), eventHistory, net, loadedFactoryCfg,
		logger, logSink, metricsSink, recording, recordPath, dispatchCompleted,
	)
	factoryEventRecorder := f.factoryEventRecorder(recordPath, recording, eventHistory)
	if workerAttempts == nil {
		return nil, fmt.Errorf("worker sessions runtime attempt capability is required")
	}
	effectiveSubmissionRecorder := recordings.SubmissionRecorder(bundle.RecordSubmissionMetric)
	if f.submissionRecorder != nil {
		effectiveSubmissionRecorder = f.submissionRecorder
	}
	activeFactory, err := f.engineOpening.Open(
		net,
		runtimeScheduler,
		workerService,
		workerSessions,
		workerAttempts,
		loadedFactoryCfg,
		invocationFileReader(f.inputFiles),
		RuntimeWorkflowContext(loadedFactoryCfg.FactoryConfig(), canonicalSessionID),
		sessionID,
		runtimeMode,
		structuredLogger,
		clock,
		f.workerAttemptScheduler,
		inlineDispatch,
		eventHistory,
		workerRecordingIdentity(runtimeInstanceID),
		runtimeInstanceID,
		f.worldStateProjector,
		restoredWorldState,
		skipRestoredDispatchReconciliation,
		effectiveSubmissionRecorder,
		factoryEventRecorder,
		submissionHooks,
		effectiveDispatchRecorder(f.dispatchRecorder, bundle.RecordDispatchMetric),
		bundle.RecordCompletionMetrics,
		petriMutationRecorder,
		completionPlanner,
	)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	if configurable, ok := activeFactory.(interface {
		SetMockWorkersConfig(*workers.MockWorkersConfig)
	}); ok {
		configurable.SetMockWorkersConfig(mockWorkersConfig)
	}
	if configurable, ok := activeFactory.(interface {
		SetPromptSourceReader(func(string) ([]byte, error))
	}); ok && f.inputFiles != nil {
		configurable.SetPromptSourceReader(f.inputFiles.ReadFile)
	}
	// Filesystem effects may finish after admission was canceled. Keep the
	// unpublished engine inside this opening owner so unwind releases only its
	// resources, without sealing worker admission for a same-identity retry.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	bundle.Factory = activeFactory
	bundle.InputFiles = f.inputFiles
	bundle.InputDirectoryWalker = f.inputDirectoryWalker
	bundle.WorkRequestIDs = f.workRequestIDs
	return bundle, nil
}

func (*RuntimeFactory) factoryEventRecorder(recordPath string, recording recordings.RuntimeRecorder,
	eventHistory recordings.RuntimeLedger,
) factory.FactoryEventRecorder {
	factoryEventRecorder := factory.FactoryEventRecorder(nil)
	if recordPath != "" {
		factoryEventRecorder = func(event interfaces.FactoryEvent) {
			if recording == nil {
				return
			}
			var provenance []recordings.RecordingSecret
			if lookup, ok := eventHistory.(interface {
				SecretProvenanceDuringAppend(interfaces.FactoryEvent) []recordings.RecordingSecret
			}); ok {
				provenance = lookup.SecretProvenanceDuringAppend(event)
			}
			if withProvenance, ok := recording.(recordings.RuntimeRecorderWithProvenance); ok {
				withProvenance.RecordEventWithProvenance(event, provenance)
				return
			}
			recording.RecordEvent(event)
		}
	}
	return factoryEventRecorder
}

func invocationFileReader(inputFiles inputFileSystem) interfaces.FileReader {
	if inputFiles == nil {
		return nil
	}
	return inputFiles.ReadFile
}

// workerRecordingIdentity derives the Worker source-native recording identity
// from the concrete runtime lifecycle, independently of whether a Factory
// recording artifact was requested. All Worker Sessions in one runtime share
// one durable snapshot without collisions across later runs at the same path.
func workerRecordingIdentity(recordingID string) string {
	recordingID = strings.TrimSpace(recordingID)
	if recordingID == "" {
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
	files inputFileSystem
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
		binding, err := f.definitionMapper.Bind(ctx, cfg)
		if err != nil {
			logger.Error("failed to map JavaScript factory runtime net", zap.Error(err))
			return nil, fmt.Errorf("compile factory orchestration: %w", err)
		}
		return orchestration.PetriNet(binding), nil
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
	baseLogger *zap.Logger,
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
	logSink, err := owner.Open(baseLogger, factory.RuntimeLogScopeRequest{
		SessionID: sessionID, RuntimeInstanceID: runtimeInstanceID,
		FolderPath: folderPath, FactoryDirectory: factoryDir,
		RootDirectory: runtimeLogDir, Policy: policy, Config: runtimeLogConfig,
	})
	if logSink != nil {
		logSink = &closeOnceRuntimeLogSink{RuntimeLogSink: logSink}
	}
	if err != nil {
		return logSink, runtimeInstanceID, fmt.Errorf("open runtime log scope: %w", err)
	}
	if logSink == nil {
		return nil, runtimeInstanceID, fmt.Errorf("runtime log owner returned nil scope")
	}
	return logSink, runtimeInstanceID, nil
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
	return baseLogger
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
	if metricsSink != nil {
		metricsSink = &closeOnceRuntimeMetricsSink{RuntimeMetricsSink: metricsSink}
	}
	if err != nil {
		return metricsSink, fmt.Errorf("open runtime metrics scope: %w", err)
	}
	if metricsSink == nil {
		return nil, fmt.Errorf("runtime metrics owner returned nil scope")
	}
	return metricsSink, nil
}

type closeOnceRuntimeLogSink struct {
	factory.RuntimeLogSink
	mu     sync.Mutex
	closed bool
}

func (sink *closeOnceRuntimeLogSink) Close() error {
	if sink == nil || sink.RuntimeLogSink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.closed {
		return nil
	}
	err := sink.RuntimeLogSink.Close()
	sink.closed = err == nil
	return err
}

type closeOnceRuntimeMetricsSink struct {
	factory.RuntimeMetricsSink
	mu     sync.Mutex
	closed bool
}

func (sink *closeOnceRuntimeMetricsSink) Close() error {
	if sink == nil || sink.RuntimeMetricsSink == nil {
		return nil
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.closed {
		return nil
	}
	err := sink.RuntimeMetricsSink.Close()
	sink.closed = err == nil
	return err
}
