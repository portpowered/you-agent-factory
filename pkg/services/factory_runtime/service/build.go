package service

import (
	"context"
	"fmt"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/definitionmapping"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/scheduler"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"path/filepath"
	"strings"
)

const defaultSessionID = "~default"

type runtimeWorkstationService interface {
	StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error)
	StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error)
	DispatchWorkstation(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}

// RuntimeFactory constructs hosted runtime bundles. It is stateless.
type RuntimeFactory struct {
	quorumPolicy         interfaces.QuorumPolicyService
	outputShaping        interfaces.InvocationOutputShapingService
	workPropagation      interfaces.WorkPropagationPolicyService
	decisionEnvelopes    interfaces.DecisionEnvelopeService
	loggerFactory        factory.RuntimeLoggerFactory
	runtimeLogs          factory.RuntimeLogSinkFactory
	runtimeMetrics       factory.RuntimeMetricsSinkFactory
	newID                factory.IDGenerator
	workRequestIDs       work.RequestIDGenerator
	runtimeDirs          factory.RuntimeDirectoryFileSystem
	inputFiles           factory.InputFileSystem
	inputDirectoryWalker factory.InputDirectoryWalker
}

func NewRuntimeFactory(
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	decisionEnvelopes interfaces.DecisionEnvelopeService,
	loggerFactory factory.RuntimeLoggerFactory,
	runtimeLogs factory.RuntimeLogSinkFactory,
	runtimeMetrics factory.RuntimeMetricsSinkFactory,
	newID factory.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles factory.InputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
) *RuntimeFactory {
	return &RuntimeFactory{
		quorumPolicy:         quorumPolicy,
		outputShaping:        outputShaping,
		workPropagation:      workPropagation,
		decisionEnvelopes:    decisionEnvelopes,
		loggerFactory:        loggerFactory,
		runtimeLogs:          runtimeLogs,
		runtimeMetrics:       runtimeMetrics,
		newID:                newID,
		workRequestIDs:       workRequestIDs,
		runtimeDirs:          runtimeDirs,
		inputFiles:           inputFiles,
		inputDirectoryWalker: inputDirectoryWalker,
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
	baseLogger *zap.Logger,
	runtimeInstanceID string,
	backendScopeID string,
	clock factory.Clock,
	recordPath string,
	recording recordings.RuntimeRecorder,
	initialFactory *interfaces.FactorySnapshot,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	petriMutationRecorder factory.PetriMutationRecorder,
	worldStateProjector factory.WorldStateProjector,
	newRuntimeLedger factory.RuntimeLedgerFactory,
	loadWorkerExecutors func(recordings.WorkerEventRecorder, *zap.Logger) (map[string]workers.WorkerExecutor, error),
	workerService runtimeWorkstationService,
	dispatchCompleted func(string),
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
	logSink, runtimeInstanceID, err := buildRuntimeLogSink(
		f.runtimeLogs,
		runtimeFileLoggingPolicy,
		runtimeLogDir,
		runtimeLogConfig,
		baseLogger,
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
		_ = logSink.Close()
		return nil, fmt.Errorf("runtime logger factory returned nil")
	}
	metricsSink, err := buildRuntimeMetricsSink(
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
		if logSink != nil {
			_ = logSink.Close()
		}
		return nil, err
	}
	bundleBuilt := false
	defer func() {
		if !bundleBuilt {
			_ = factoryhost.CloseBundleSinks(logSink, metricsSink)
		}
	}()
	mapper, err := definitionmapping.New(f.newID)
	if err != nil {
		return nil, err
	}
	net, err := mapper.Map(ctx, loadedFactoryCfg.FactoryConfig())
	if err != nil {
		logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(runnerID, loadedFactoryCfg.FactoryConfig())
	if newRuntimeLedger == nil {
		return nil, fmt.Errorf("Recordings runtime ledger factory is required")
	}
	eventHistory := newRuntimeLedger(net, clock.Now, loadedFactoryCfg)
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

	bundleBuilt = true
	return assembleRuntimeBundle(
		dir, folderPath, sessionID, runtimeMode, verbose, runtimeScheduler,
		workerExecutorOverrides, workerExecutorDecorator, inlineDispatch, submissionRecorder,
		dispatchRecorder,
		loadedFactoryCfg, runtimeInstanceID,
		backendScopeID, clock, recordPath, recording, submissionHooks,
		completionPlanner, petriMutationRecorder, worldStateProjector,
		dispatchCompleted, logger, structuredLogger, logSink, metricsSink, net, eventHistory,
		workerExecutors,
		workerService,
		f.quorumPolicy,
		f.outputShaping,
		f.workPropagation,
		f.workRequestIDs,
		f.newID,
		f.runtimeDirs,
		f.inputFiles,
		f.inputDirectoryWalker,
		f.decisionEnvelopes,
	)
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
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
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	workRequestIDs work.RequestIDGenerator,
	newID factory.IDGenerator,
	runtimeDirs factory.RuntimeDirectoryFileSystem,
	inputFiles factory.InputFileSystem,
	inputDirectoryWalker factory.InputDirectoryWalker,
	decisionEnvelopes interfaces.DecisionEnvelopeService,
) (*factoryhost.Bundle, error) {
	bundle := factoryhost.NewBundle(
		dir, folderPath, runtimeInstanceID, strings.TrimSpace(backendScopeID),
		clock.Now().UTC(), eventHistory, net, loadedFactoryCfg,
		logger, logSink, metricsSink, recording, recordPath, dispatchCompleted,
	)
	factoryEventRecorder := factory.FactoryEventRecorder(nil)
	if recordPath != "" {
		factoryEventRecorder = func(event interfaces.FactoryEvent) {
			if recording == nil {
				return
			}
			recording.RecordEvent(event)
		}
	}
	for workerType, executor := range workerExecutorOverrides {
		workerExecutors[workerType] = executor
	}
	if workerExecutorDecorator != nil {
		for workerType, executor := range workerExecutors {
			workerExecutors[workerType] = workerExecutorDecorator(workerType, executor)
		}
	}
	effectiveSubmissionRecorder := recordings.SubmissionRecorder(bundle.RecordSubmissionMetric)
	if submissionRecorder != nil {
		effectiveSubmissionRecorder = submissionRecorder
	}
	activeFactory, err := runtime.New(
		net,
		runtimeScheduler,
		workerExecutors,
		workerService,
		loadedFactoryCfg,
		RuntimeWorkflowContext(loadedFactoryCfg.FactoryConfig(), sessionID),
		runtimeMode,
		structuredLogger,
		clock,
		inlineDispatch,
		eventHistory,
		worldStateProjector,
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
		workRequestIDs,
		newID,
		decisionEnvelopes,
	)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	if err := ensureRuntimeInputsDir(dir, logger, runtimeDirs); err != nil {
		return nil, err
	}

	bundle.Factory = activeFactory
	bundle.InputFiles = inputFiles
	bundle.InputDirectoryWalker = inputDirectoryWalker
	bundle.WorkRequestIDs = workRequestIDs
	return bundle, nil
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

func effectiveFactoryRunnerID(override string, factoryCfg *interfaces.FactoryConfig) string {
	if runner := workerrunner.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if factoryCfg == nil {
		return ""
	}
	return workerrunner.NormalizeRunnerID(factoryCfg.Runner)
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

func buildRuntimeLogSink(
	build factory.RuntimeLogSinkFactory,
	policy RuntimeFileLoggingPolicy,
	runtimeLogDir string,
	runtimeLogConfig factory.RuntimeLogStorageConfig,
	baseLogger *zap.Logger,
	runtimeInstanceID string,
) (factory.RuntimeLogSink, string, error) {
	if baseLogger == nil {
		return nil, runtimeInstanceID, fmt.Errorf("base logger is required")
	}
	if strings.TrimSpace(runtimeInstanceID) == "" {
		return nil, runtimeInstanceID, fmt.Errorf("runtime instance ID is required")
	}
	if !runtimeFileLoggingEnabled(policy) {
		return nil, runtimeInstanceID, nil
	}
	if build == nil {
		return nil, runtimeInstanceID, fmt.Errorf("runtime log sink factory is required")
	}
	logSink, err := build(baseLogger, runtimeInstanceID, runtimeLogDir, runtimeLogConfig)
	if err != nil {
		return nil, runtimeInstanceID, fmt.Errorf("build runtime logger: %w", err)
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
	if baseLogger != nil {
		return baseLogger
	}
	return zap.NewNop()
}

func buildRuntimeMetricsSink(
	build factory.RuntimeMetricsSinkFactory,
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
	if build == nil {
		return nil, fmt.Errorf("runtime metrics sink factory is required")
	}
	metricsSink, err := build(
		factory.RuntimeMetricsScope{
			SessionID: sessionID, RuntimeInstanceID: runtimeInstanceID,
			FolderPath: folderPath, FactoryDir: factoryDir,
		},
		runtimeMetricsDir,
		runtimeMetricsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("build runtime metrics sink: %w", err)
	}
	return metricsSink, nil
}
