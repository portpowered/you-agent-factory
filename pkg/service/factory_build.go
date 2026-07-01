// backendsizecheck:ignore-file service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
// pkgmaintcheck:ignore-file-lines service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/ingest"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	cursorprovider "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
	"go.uber.org/zap"
)

type runtimeBundleBuildInput struct {
	dir                           string
	folderPath                    string
	sessionID                     string
	cfg                           *FactoryServiceConfig
	loadedFactoryCfg              *factoryconfig.LoadedFactoryConfig
	baseLogger                    *zap.Logger
	runtimeInstanceID             string
	clock                         factory.Clock
	recordPath                    string
	workflowID                    string
	providerOverride              workers.Provider
	providerCommandRunner         workers.CommandRunner
	commandRunnerOverride         workers.CommandRunner
	additionalFactoryOpts         []factory.FactoryOption
	prefetchedLocalModels         localModelDomain
	inferenceProgressPublisher    workerprovider.InferenceProgressPublisher
	inferenceProgressPublisherSet bool
	dispatchCompleted             func(string)
}

type liveSessionState struct {
	bundle                *factoryRuntimeBundle
	handle                *liveRuntimeHandle
	spec                  *runtimebuild.SessionBuildSpec
	javascriptCheckpoints *factorysessions.JavaScriptCheckpointStore
	responseStreamsOnce   sync.Once
	responseStreams       *factorysessions.SessionResponseStreamSet
}

// BuildFactoryService loads factory.json from the config directory, constructs
// the petri net, factory runtime, file watcher, and session metrics.
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	core, err := BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	service := NewFactoryServiceFromCore(core)
	shell := FactoryServiceShell{Service: service}
	service = AttachModelServiceCollaborator(shell, ProvideModelServiceCollaborator(shell, cfg))
	service = AttachFactorySaveCollaborator(
		FactoryServiceShell{Service: service},
		ProvideFactorySaveCollaborator(FactoryServiceShell{Service: service}, cfg),
	)
	return AttachSessionGatewayCollaborator(
		FactoryServiceShell{Service: service},
		ProvideSessionGatewayCollaborator(FactoryServiceShell{Service: service}, cfg),
	), nil
}

func wireModelAssetPuller(cfg *FactoryServiceConfig, production modelAssetPuller) modelAssetPuller {
	if cfg != nil && cfg.ModelAssets != nil {
		return cfg.ModelAssets
	}
	return production
}

type serviceCoordinatorPolicy struct {
	dir                           string
	executionBaseDir              string
	runtimeMode                   interfaces.RuntimeMode
	port                          int
	verbose                       bool
	runtimeInstanceID             string
	workFile                      string
	workflowID                    string
	mockWorkersConfig             *factoryconfig.MockWorkersConfig
	simpleDashboardRenderer       SimpleDashboardRenderer
	apiServerStarter              APIServerStarter
	apiServerReady                <-chan struct{}
	workstationLoader             factoryconfig.WorkstationLoader
	modelCacheDir                 string
	runnerID                      string
	providerOverride              workers.Provider
	providerCommandRunnerOverride workers.CommandRunner
	commandRunnerOverride         workers.CommandRunner
}

const (
	runtimeMetricLifecycleStarted     = "runtime.lifecycle.started"
	runtimeMetricLifecycleStopped     = "runtime.lifecycle.stopped"
	runtimeMetricStateActive          = "runtime.state.active"
	runtimeMetricStateIdle            = "runtime.state.idle"
	runtimeMetricStatePaused          = "runtime.state.paused"
	runtimeMetricStateFailed          = "runtime.state.failed"
	runtimeMetricQueueInFlight        = "runtime.queue.in_flight"
	runtimeMetricQueueSubmissionCount = "queue.submission_count"
	runtimeMetricDispatchStarted      = "dispatch.started"
	runtimeMetricDispatchComplete     = "dispatch.completed"
	runtimeMetricDispatchDuration     = "dispatch.duration"
	runtimeMetricDispatchRetries      = "dispatch.retry_count"
	runtimeMetricDispatchCost         = "dispatch.cost"
	runtimeMetricProviderRequest      = "provider.requested"
	runtimeMetricProviderComplete     = "provider.completed"
	runtimeMetricProviderFailed       = "provider.failed"
	runtimeMetricProviderDuration     = "provider.duration"
	runtimeMetricProviderInputTok     = "provider.input_tokens"
	runtimeMetricProviderOutputTok    = "provider.output_tokens"
	runtimeMetricProviderCost         = "provider.cost"
	runtimeMetricScriptStarted        = "script.started"
	runtimeMetricScriptComplete       = "script.completed"
	runtimeMetricScriptDuration       = "script.duration"
	runtimeMetricScriptTimedOut       = "script.timed_out"
	runtimeMetricScriptFailed         = "script.failed"
)

const runtimeMetricsObserverPollInterval = 5 * time.Millisecond

type runtimeMetricsObservation struct {
	runtimeStatus interfaces.RuntimeStatus
	factoryState  interfaces.FactoryState
	inFlightCount int
	initialized   bool
}

func (fs *FactoryService) coordinatorPolicy() serviceCoordinatorPolicy {
	if fs == nil {
		return serviceCoordinatorPolicy{}
	}
	if hasExplicitServiceCoordinatorPolicy(fs.policy) {
		return fs.policy
	}
	return serviceCoordinatorPolicyFromConfig(fs.cfg)
}

func hasExplicitServiceCoordinatorPolicy(policy serviceCoordinatorPolicy) bool {
	return hasExplicitServiceCoordinatorValuePolicy(policy) || hasExplicitServiceCoordinatorReferencePolicy(policy)
}

func hasExplicitServiceCoordinatorValuePolicy(policy serviceCoordinatorPolicy) bool {
	return policy.dir != "" ||
		policy.executionBaseDir != "" ||
		policy.runtimeMode != "" ||
		policy.port != 0 ||
		policy.verbose ||
		policy.runtimeInstanceID != "" ||
		policy.workFile != "" ||
		policy.workflowID != "" ||
		policy.modelCacheDir != "" ||
		policy.runnerID != ""
}

func hasExplicitServiceCoordinatorReferencePolicy(policy serviceCoordinatorPolicy) bool {
	return policy.mockWorkersConfig != nil ||
		policy.simpleDashboardRenderer != nil ||
		policy.apiServerStarter != nil ||
		policy.apiServerReady != nil ||
		policy.workstationLoader != nil ||
		policy.providerOverride != nil ||
		policy.providerCommandRunnerOverride != nil ||
		policy.commandRunnerOverride != nil
}

func serviceCoordinatorPolicyFromConfig(cfg *FactoryServiceConfig) serviceCoordinatorPolicy {
	if cfg == nil {
		return serviceCoordinatorPolicy{}
	}
	return serviceCoordinatorPolicy{
		dir:                           cfg.Dir,
		executionBaseDir:              cfg.ExecutionBaseDir,
		runtimeMode:                   cfg.RuntimeMode,
		port:                          cfg.Port,
		verbose:                       cfg.Verbose,
		runtimeInstanceID:             cfg.RuntimeInstanceID,
		workFile:                      cfg.WorkFile,
		workflowID:                    cfg.WorkflowID,
		mockWorkersConfig:             cfg.MockWorkersConfig,
		simpleDashboardRenderer:       cfg.SimpleDashboardRenderer,
		apiServerStarter:              cfg.APIServerStarter,
		apiServerReady:                cfg.APIServerReady,
		workstationLoader:             cfg.WorkstationLoader,
		modelCacheDir:                 cfg.ModelCacheDir,
		runnerID:                      cfg.RunnerID,
		providerOverride:              cfg.ProviderOverride,
		providerCommandRunnerOverride: cfg.ProviderCommandRunnerOverride,
		commandRunnerOverride:         cfg.CommandRunnerOverride,
	}
}

func resolveFactoryServiceRoot(cfg *FactoryServiceConfig) (string, *zap.Logger, error) {
	factoryRootDir, err := factorysessions.AbsolutizeFactoryDirectory(cfg.Dir)
	if err != nil {
		return "", nil, err
	}
	cfg.Dir = factoryRootDir
	baseLogger := cfg.Logger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	if cfg.RuntimeInstanceID == "" {
		cfg.RuntimeInstanceID = uuid.NewString()
	}
	cfg.Logger = baseLogger
	return factoryRootDir, baseLogger, nil
}

// RuntimeLogDiagnostics describes the active runtime log selected during
// service construction.
type RuntimeLogDiagnostics struct {
	Path                string
	RootDir             string
	StartTimeUTC        time.Time
	MetricsPath         string
	MetricsRootDir      string
	MetricsStartTimeUTC time.Time
}

// RuntimeLogDiagnostics returns the selected runtime log metadata for startup
// diagnostics without exposing the sink writer.
func (fs *FactoryService) RuntimeLogDiagnostics() RuntimeLogDiagnostics {
	bundle := fs.currentRuntimeBundle()
	if bundle == nil || bundle.logSink == nil {
		return RuntimeLogDiagnostics{}
	}
	return RuntimeLogDiagnostics{
		Path:                bundle.logSink.Path(),
		RootDir:             bundle.logSink.RootDir(),
		StartTimeUTC:        bundle.logSink.StartTimeUTC(),
		MetricsPath:         runtimeMetricsPath(bundle.metricsSink),
		MetricsRootDir:      runtimeMetricsRootDir(bundle.metricsSink),
		MetricsStartTimeUTC: runtimeMetricsStartTime(bundle.metricsSink),
	}
}

func runtimeMetricsPath(sink *logging.RuntimeMetricsSink) string {
	if sink == nil {
		return ""
	}
	return sink.Path()
}

func runtimeMetricsRootDir(sink *logging.RuntimeMetricsSink) string {
	if sink == nil {
		return ""
	}
	return sink.RootDir()
}

func runtimeMetricsStartTime(sink *logging.RuntimeMetricsSink) time.Time {
	if sink == nil {
		return time.Time{}
	}
	return sink.StartTimeUTC()
}

func runtimeLogStartTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func loadFactoryConfigForService(
	cfg *FactoryServiceConfig,
	logger *zap.Logger,
) (*factoryconfig.LoadedFactoryConfig, *interfaces.ReplayArtifact, error) {
	logger.Info("loading factory config", zap.String("dir", cfg.Dir))
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForMode(cfg)
	if err != nil {
		logger.Error("failed to load factory config", zap.Error(err))
		return nil, nil, fmt.Errorf("load factory config: %w", err)
	}
	runtimebuild.WarnPortableBundledReplacementReport(logger, "runtime config load replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	warnReplayMetadataMismatches(cfg, replayArtifact, logger)
	return loadedFactoryCfg, replayArtifact, nil
}

func serviceClockForMode(clock factory.Clock, replayArtifact *interfaces.ReplayArtifact) factory.Clock {
	if clock == nil && replayArtifact != nil {
		clock = replay.NewArtifactClock(replayArtifact)
	}
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	return factory.EnsureClock(clock)
}

func replayFactoryModeOptions(
	replayArtifact *interfaces.ReplayArtifact,
) (*replay.SideEffects, []factory.FactoryOption, error) {
	if replayArtifact == nil {
		return nil, nil, nil
	}
	replaySideEffects, err := replay.NewSideEffects(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	replaySubmissionHook, err := replay.NewSubmissionHook(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	replayWorkStateChangeHook, err := replay.NewWorkStateChangeHook(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay work state change hook: %w", err)
	}
	replayDeliveryPlan, err := replay.NewCompletionDeliveryPlan(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}
	return replaySideEffects, []factory.FactoryOption{
		factory.WithSubmissionHook(replaySubmissionHook),
		factory.WithSubmissionHook(replayWorkStateChangeHook),
		factory.WithCompletionDeliveryPlanner(replayDeliveryPlan),
	}, nil
}

func buildRuntimeBundle(
	ctx context.Context,
	input runtimeBundleBuildInput,
) (*factoryRuntimeBundle, error) {
	sessionID := strings.TrimSpace(input.sessionID)
	if sessionID == "" {
		sessionID = defaultFactorySessionID
	}
	logSink, runtimeInstanceID, err := buildRuntimeLogSink(input.cfg, input.baseLogger, input.runtimeInstanceID)
	if err != nil {
		return nil, err
	}
	logger := runtimebuild.NewSessionLogger(runtimeSessionBaseLogger(input.baseLogger, logSink), sessionID, input.folderPath, input.dir)
	metricsSink, err := buildRuntimeMetricsSink(input.cfg, sessionID, runtimeInstanceID, input.folderPath, input.dir)
	if err != nil {
		logger.Warn(
			"runtime metrics sink unavailable; continuing without metrics",
			zap.Error(err),
			zap.String("runtime_instance_id", runtimeInstanceID),
			zap.String("runtime_metrics_root_dir", strings.TrimSpace(input.cfg.RuntimeMetricsDir)),
		)
		metricsSink = nil
	}
	bundleBuilt := false
	defer func() {
		if !bundleBuilt {
			_ = closeRuntimeBundleSinks(logSink, metricsSink)
		}
	}()
	if input.cfg != nil && runtimeInstanceID != "" {
		input.cfg.RuntimeInstanceID = runtimeInstanceID
	}

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, input.loadedFactoryCfg.FactoryConfig())
	if err != nil {
		logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(input.cfg.RunnerID, input.loadedFactoryCfg.FactoryConfig())
	eventHistory := factoryevents.NewFactoryEventHistory(net, input.clock.Now, input.loadedFactoryCfg)
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	if editableFactory, err := editableEventFactorySnapshot(input); err != nil {
		logger.Warn("editable factory event snapshot unavailable; using runtime-thin factory event payload", zap.Error(err))
	} else {
		eventHistory.SetInitialStructureFactory(editableFactory)
	}
	localModels := input.prefetchedLocalModels
	if localModels.manager == nil {
		localModels = newRuntimeLocalModelDependencies(input.cfg)
	}
	workerOpts, err := loadRuntimeBundleWorkerOptions(input, logger, effectiveFactoryRunnerID, eventHistory, localModels)
	if err != nil {
		return nil, err
	}

	bundleBuilt = true
	return assembleRuntimeBundle(input, logger, logSink, metricsSink, net, eventHistory, localModels, workerOpts)
}

func loadRuntimeBundleWorkerOptions(
	input runtimeBundleBuildInput,
	logger *zap.Logger,
	effectiveFactoryRunnerID string,
	eventHistory *factoryevents.FactoryEventHistory,
	localModels localModelDomain,
) ([]factory.FactoryOption, error) {
	workerOpts, err := loadWorkersFromConfig(
		input.loadedFactoryCfg.FactoryDir(),
		input.loadedFactoryCfg.FactoryConfig(),
		effectiveFactoryRunnerID,
		input.loadedFactoryCfg,
		runtimeWorkflowContext(input.loadedFactoryCfg.FactoryConfig(), input.sessionID),
		logging.NewZapLogger(logger, input.cfg.Verbose),
		input.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		input.providerOverride,
		input.inferenceProgressPublisher,
		wrapProviderCommandRunnerForProgress(input, input.providerCommandRunner),
		input.commandRunnerOverride,
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		eventHistory.RecordModelEvent,
		input.clock.Now,
		localModels,
	)
	if err != nil {
		logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}
	return workerOpts, nil
}

func editableEventFactorySnapshot(input runtimeBundleBuildInput) (factoryapi.Factory, error) {
	if input.loadedFactoryCfg == nil || input.loadedFactoryCfg.FactoryConfig() == nil {
		return factoryapi.Factory{}, fmt.Errorf("loaded factory config is unavailable")
	}
	factoryCfg, err := factoryconfig.CloneFactoryConfig(input.loadedFactoryCfg.FactoryConfig())
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("clone factory config: %w", err)
	}
	if err := factoryconfig.ApplySupportedPortableBundledFiles(input.loadedFactoryCfg.FactoryDir(), factoryCfg, true, false); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("inline portable bundled files: %w", err)
	}
	if err := factoryconfig.ApplySharedFactoryStarterWork(input.loadedFactoryCfg.FactoryDir(), factoryCfg); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
	}
	return replay.GeneratedFactoryFromRuntimeConfig(
		input.loadedFactoryCfg.FactoryDir(),
		factoryCfg,
		input.loadedFactoryCfg,
		replay.WithGeneratedFactorySourceDirectory(input.loadedFactoryCfg.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(input.workflowID),
	)
}

func assembleRuntimeBundle(
	input runtimeBundleBuildInput,
	logger *zap.Logger,
	logSink *logging.RuntimeLogSink,
	metricsSink *logging.RuntimeMetricsSink,
	net *state.Net,
	eventHistory *factoryevents.FactoryEventHistory,
	localModels localModelDomain,
	workerOpts []factory.FactoryOption,
) (*factoryRuntimeBundle, error) {
	recording, err := buildRuntimeRecorder(
		input.cfg,
		input.loadedFactoryCfg.FactoryDir(),
		input.loadedFactoryCfg.FactoryConfig(),
		input.loadedFactoryCfg,
		input.clock,
		input.recordPath,
		input.workflowID,
	)
	if err != nil {
		return nil, err
	}

	bundle := &factoryRuntimeBundle{
		dir:               input.dir,
		folderPath:        input.folderPath,
		runtimeInstanceID: input.runtimeInstanceID,
		startedAtUTC:      input.clock.Now().UTC(),
		eventHistory:      eventHistory,
		net:               net,
		runtimeCfg:        input.loadedFactoryCfg,
		modelResources:    localModels.resources,
		modelAssets:       localModels.assets,
		localModels:       localModels.manager,
		localModelRuntime: localModels.runtime,
		modelHost:         localModels.host,
		leaseExecution:    localModels.leaseExecution,
		logger:            logger,
		logSink:           logSink,
		metricsSink:       metricsSink,
		recording:         recording,
		recordPath:        input.recordPath,
		dispatchCompleted: input.dispatchCompleted,
	}
	opts := []factory.FactoryOption{
		factory.WithNet(net),
		factory.WithRuntimeMode(input.cfg.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(logger, input.cfg.Verbose)),
		factory.WithRuntimeConfig(input.loadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(input.loadedFactoryCfg.FactoryConfig(), input.sessionID)),
		factory.WithClock(input.clock),
		factory.WithFactoryEventHistory(eventHistory),
		factory.WithSubmissionRecorder(bundle.recordSubmissionMetric),
		factory.WithDispatchRecorder(bundle.recordDispatchMetric),
		factory.WithCompletionRecorder(bundle.recordCompletionMetrics),
	}
	if input.recordPath != "" {
		opts = append(opts, factory.WithFactoryEventRecorder(func(event factoryapi.FactoryEvent) {
			if recording != nil {
				recording.RecordEvent(event)
			}
		}))
	}
	opts = append(opts, input.additionalFactoryOpts...)
	opts = append(opts, workerOpts...)
	opts = append(opts, input.cfg.ExtraOptions...)

	activeFactory, err := runtime.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	listener, err := buildRuntimeListener(input.dir, activeFactory, logger, net)
	if err != nil {
		return nil, err
	}

	bundle.factory = activeFactory
	bundle.listener = listener
	return bundle, nil
}

func buildRuntimeLogSink(
	cfg *FactoryServiceConfig,
	baseLogger *zap.Logger,
	runtimeInstanceID string,
) (*logging.RuntimeLogSink, string, error) {
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	if strings.TrimSpace(runtimeInstanceID) == "" {
		runtimeInstanceID = uuid.NewString()
	}
	if cfg == nil {
		return nil, runtimeInstanceID, fmt.Errorf("factory service config is required to build runtime log sink")
	}
	if !runtimeFileLoggingEnabled(cfg.RuntimeFileLoggingPolicy) {
		return nil, runtimeInstanceID, nil
	}
	logSink, err := logging.BuildRuntimeLogger(baseLogger, runtimeInstanceID, cfg.RuntimeLogDir, cfg.RuntimeLogConfig)
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

func runtimeSessionBaseLogger(baseLogger *zap.Logger, logSink *logging.RuntimeLogSink) *zap.Logger {
	if logSink != nil {
		return logSink.Logger()
	}
	if baseLogger != nil {
		return baseLogger
	}
	return zap.NewNop()
}

func buildRuntimeMetricsSink(
	cfg *FactoryServiceConfig,
	sessionID string,
	runtimeInstanceID string,
	folderPath string,
	factoryDir string,
) (*logging.RuntimeMetricsSink, error) {
	if cfg == nil {
		return nil, fmt.Errorf("factory service config is required to build runtime metrics sink")
	}
	metricsSink, err := logging.BuildRuntimeMetricsSink(
		sessionID,
		runtimeInstanceID,
		folderPath,
		factoryDir,
		cfg.RuntimeMetricsDir,
		cfg.RuntimeMetricsConfig,
	)
	if err != nil {
		return nil, fmt.Errorf("build runtime metrics sink: %w", err)
	}
	return metricsSink, nil
}

func closeRuntimeBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	var errs []error
	if logSink != nil {
		if err := logSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if metricsSink != nil {
		if err := metricsSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *factoryRuntimeBundle) recordSubmissionMetric(record interfaces.FactorySubmissionRecord) {
	fields := metrics.Fields{
		WorkID:  strings.TrimSpace(record.Request.WorkID),
		TraceID: strings.TrimSpace(record.Request.TraceID),
	}
	r.emitMetricCounter(runtimeMetricQueueSubmissionCount, 1, fields)
}

func (r *factoryRuntimeBundle) recordDispatchMetric(record interfaces.FactoryDispatchRecord) {
	fields := runtimeDispatchMetricFields(record.Dispatch)
	r.dispatchMetricFields.Store(record.DispatchID, fields)
	r.emitMetricCounter(runtimeMetricDispatchStarted, 1, fields)
	r.emitWorkerBoundaryStartMetrics(fields)
}

func (r *factoryRuntimeBundle) recordCompletionMetrics(record interfaces.FactoryCompletionRecord) {
	fields, _ := r.dispatchMetricFields.LoadAndDelete(record.DispatchID)
	metricFields, _ := fields.(metrics.Fields)
	if metricFields.DispatchID == "" {
		metricFields.DispatchID = record.DispatchID
	}
	metricFields.Outcome = string(record.Result.Outcome)
	r.emitMetricCounter(runtimeMetricDispatchComplete, 1, metricFields)
	r.emitMetricSample(runtimeMetricDispatchDuration, float64(record.Result.Metrics.Duration.Milliseconds()), "ms", metricFields)
	r.emitMetricSample(runtimeMetricDispatchRetries, float64(record.Result.Metrics.RetryCount), "", metricFields)
	if record.Result.Metrics.Cost > 0 {
		r.emitMetricSample(runtimeMetricDispatchCost, record.Result.Metrics.Cost, "usd", metricFields)
	}
	r.emitWorkerBoundaryCompletionMetrics(record.Result, metricFields)
	if r.dispatchCompleted != nil {
		r.dispatchCompleted(record.DispatchID)
	}
}

func runtimeDispatchMetricFields(dispatch interfaces.WorkDispatch) metrics.Fields {
	fields := metrics.Fields{
		DispatchID:  dispatch.DispatchID,
		TraceID:     strings.TrimSpace(dispatch.Execution.TraceID),
		Workstation: strings.TrimSpace(dispatch.WorkstationName),
		WorkerType:  strings.TrimSpace(dispatch.WorkerType),
	}
	if len(dispatch.Execution.WorkIDs) > 0 {
		fields.WorkID = strings.TrimSpace(dispatch.Execution.WorkIDs[0])
	}
	return fields
}

func (r *factoryRuntimeBundle) emitWorkerBoundaryStartMetrics(fields metrics.Fields) {
	workerDef, ok := r.runtimeWorkerDefinition(fields.WorkerType)
	if !ok || workerDef == nil {
		return
	}
	switch workerDef.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		providerFields := fields
		providerFields.Provider = normalizedRuntimeMetricProvider(workerDef.ModelProvider)
		r.emitMetricCounter(runtimeMetricProviderRequest, 1, providerFields)
	case interfaces.WorkerTypeScript:
		r.emitMetricCounter(runtimeMetricScriptStarted, 1, fields)
	}
}

func (r *factoryRuntimeBundle) emitWorkerBoundaryCompletionMetrics(result interfaces.WorkResult, fields metrics.Fields) {
	workerDef, ok := r.runtimeWorkerDefinition(fields.WorkerType)
	if !ok || workerDef == nil {
		return
	}
	switch workerDef.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		r.emitProviderCompletionMetrics(result, fields, workerDef)
	case interfaces.WorkerTypeScript:
		r.emitScriptCompletionMetrics(result, fields)
	}
}

func (r *factoryRuntimeBundle) emitProviderCompletionMetrics(
	result interfaces.WorkResult,
	fields metrics.Fields,
	workerDef *interfaces.WorkerConfig,
) {
	providerFields := fields
	providerFields.Provider = normalizedRuntimeMetricProvider(providerMetricProvider(result.Diagnostics, workerDef))
	r.emitMetricCounter(runtimeMetricProviderComplete, 1, providerFields)
	if result.Outcome == interfaces.OutcomeFailed {
		providerFields.Reason = providerMetricFailureReason(result)
		r.emitMetricCounter(runtimeMetricProviderFailed, 1, providerFields)
	}
	if durationMS, ok := providerMetricDurationMilliseconds(result.Diagnostics); ok {
		r.emitMetricSample(runtimeMetricProviderDuration, durationMS, "ms", providerFields)
	}
	if inputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, cursorprovider.ResponseMetadataInputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderInputTok, inputTokens, "tokens", providerFields)
	}
	if outputTokens, ok := providerMetricMetadataFloat(result.Diagnostics, cursorprovider.ResponseMetadataOutputTokens); ok {
		r.emitMetricSample(runtimeMetricProviderOutputTok, outputTokens, "tokens", providerFields)
	}
	if result.Metrics.Cost > 0 {
		r.emitMetricSample(runtimeMetricProviderCost, result.Metrics.Cost, "usd", providerFields)
	}
}

func (r *factoryRuntimeBundle) emitScriptCompletionMetrics(result interfaces.WorkResult, fields metrics.Fields) {
	scriptFields := fields
	if timedOut := scriptMetricTimedOut(result); timedOut {
		scriptFields.Reason = "timeout"
	}
	if result.Outcome == interfaces.OutcomeFailed && scriptFields.Reason == "" {
		scriptFields.Reason = scriptMetricFailureReason(result)
	}
	r.emitMetricCounter(runtimeMetricScriptComplete, 1, scriptFields)
	if durationMS, ok := scriptMetricDurationMilliseconds(result); ok {
		r.emitMetricSample(runtimeMetricScriptDuration, durationMS, "ms", scriptFields)
	}
	if scriptMetricTimedOut(result) {
		r.emitMetricCounter(runtimeMetricScriptTimedOut, 1, scriptFields)
		return
	}
	if result.Outcome == interfaces.OutcomeFailed {
		r.emitMetricCounter(runtimeMetricScriptFailed, 1, scriptFields)
	}
}

func (r *factoryRuntimeBundle) runtimeWorkerDefinition(workerName string) (*interfaces.WorkerConfig, bool) {
	if r == nil || r.runtimeCfg == nil || strings.TrimSpace(workerName) == "" {
		return nil, false
	}
	return r.runtimeCfg.Worker(strings.TrimSpace(workerName))
}

func normalizedRuntimeMetricProvider(provider string) string {
	return interfaces.CanonicalProviderSessionProvider(strings.TrimSpace(provider))
}

func providerMetricProvider(diagnostics *interfaces.WorkDiagnostics, workerDef *interfaces.WorkerConfig) string {
	if diagnostics != nil && diagnostics.Provider != nil && strings.TrimSpace(diagnostics.Provider.Provider) != "" {
		return diagnostics.Provider.Provider
	}
	if workerDef != nil {
		return workerDef.ModelProvider
	}
	return ""
}

func providerMetricFailureReason(result interfaces.WorkResult) string {
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		return string(result.FailureMetadata.Type)
	}
	return strings.TrimSpace(string(result.Outcome))
}

func providerMetricDurationMilliseconds(diagnostics *interfaces.WorkDiagnostics) (float64, bool) {
	if durationMS, ok := providerMetricMetadataFloat(diagnostics, cursorprovider.ResponseMetadataDurationAPIMS); ok {
		return durationMS, true
	}
	return providerMetricMetadataFloat(diagnostics, cursorprovider.ResponseMetadataDurationMS)
}

func providerMetricMetadataFloat(diagnostics *interfaces.WorkDiagnostics, key string) (float64, bool) {
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata == nil {
		return 0, false
	}
	value := strings.TrimSpace(diagnostics.Provider.ResponseMetadata[key])
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func scriptMetricTimedOut(result interfaces.WorkResult) bool {
	if result.FailureMetadata != nil && result.FailureMetadata.Type == interfaces.WorkFailureTypeTimeout {
		return true
	}
	if result.Diagnostics == nil || result.Diagnostics.Command == nil {
		return false
	}
	return result.Diagnostics.Command.TimedOut
}

func scriptMetricFailureReason(result interfaces.WorkResult) string {
	if result.FailureMetadata != nil && result.FailureMetadata.Type != "" {
		return string(result.FailureMetadata.Type)
	}
	if result.Diagnostics != nil && result.Diagnostics.Command != nil && result.Diagnostics.Command.ExitCode != 0 {
		return "exit_code"
	}
	return strings.TrimSpace(string(result.Outcome))
}

func scriptMetricDurationMilliseconds(result interfaces.WorkResult) (float64, bool) {
	if result.Diagnostics != nil && result.Diagnostics.Command != nil && result.Diagnostics.Command.Duration > 0 {
		return float64(result.Diagnostics.Command.Duration.Milliseconds()), true
	}
	if result.Metrics.Duration <= 0 {
		return 0, false
	}
	return float64(result.Metrics.Duration.Milliseconds()), true
}

// localModelDomain wires pkg/localmodels runtime dependencies constructed at
// service build time and copied onto each factoryRuntimeBundle.
type localModelDomain struct {
	resources      *localModelResourceLimiter
	assets         modelAssetPuller
	runtime        localModelRuntime
	manager        *managedLocalModelManager
	host           modelhost.Host
	leaseExecution *modelhost.LeaseExecution
}

func newRuntimeLocalModelDependencies(cfg *FactoryServiceConfig) localModelDomain {
	modelResources := newLocalModelResourceLimiter()
	modelAssets := newModelAssetPuller(strings.TrimSpace(cfg.ModelCacheDir))
	localModelRuntime := cfg.LocalModelRuntimeOverride
	if localModelRuntime == nil {
		localModelRuntime = newOmniVoiceLocalRuntime(nil)
	}
	host := modelhost.NewCatalogHost(modelhost.NewLocalAssetGateway(modelAssets), modelhost.Options{
		SourceResolver: modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
		Diagnostics:    modelHostDiagnostics(cfg, cfg.Logger),
	})
	domain := localModelDomain{
		resources: modelResources,
		assets:    modelAssets,
		runtime:   localModelRuntime,
		manager:   newManagedLocalModelManager(modelAssets, localModelRuntime),
		host:      host,
	}
	domain.leaseExecution = modelhost.NewLeaseExecution(host, modelAssets, localModelRuntime, localModelHooks())
	return domain
}

func localModelHooks() localmodels.Hooks {
	return localmodels.Hooks{
		MarkResourceWaitStarted:  markModelExecutionResourceWaitStarted,
		MarkResourceWaitFinished: markModelExecutionResourceWaitFinished,
		MarkLoadRequested:        markModelExecutionLoadRequested,
		MarkLoadFinished:         markModelExecutionLoadFinished,
		MarkLoadReused:           markModelExecutionLoadReused,
	}
}

func newLocalModelResourceLimiter() *localModelResourceLimiter {
	return localmodels.NewResourceLimiter(localModelHooks())
}

func newManagedLocalModelManager(assetPuller modelAssetPuller, runtime localModelRuntime) *managedLocalModelManager {
	return localmodels.NewManager(assetPuller, runtime, localModelHooks())
}

func newOmniVoiceLocalRuntime(runner workers.CommandRunner) localModelRuntime {
	return localmodels.NewOmniVoiceRuntime(runner)
}

func wrapLocalModelRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	modelDomain localModelDomain,
) workers.Runner {
	if modelDomain.leaseExecution != nil {
		return modelDomain.leaseExecution.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
	}
	if modelDomain.host != nil && modelDomain.runtime != nil && modelDomain.assets != nil {
		if leaseExec := modelhost.NewLeaseExecution(
			modelDomain.host,
			modelDomain.assets,
			modelDomain.runtime,
			localModelHooks(),
		); leaseExec != nil {
			return leaseExec.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
		}
	}
	if modelDomain.manager != nil {
		return modelDomain.manager.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
	}
	return inner
}

func buildHostedWorkersConfig(cfg *FactoryServiceConfig, logger *zap.Logger, clock factory.Clock) hostedworkers.Config {
	hostedCfg := hostedworkers.Config{Logger: logger}
	if supervisorClock, ok := clock.(clockwork.Clock); ok && supervisorClock != nil {
		hostedCfg.Clock = supervisorClock
	}
	if cfg != nil {
		hostedCfg.HTTPClient = cfg.HostedPollerHTTPClient
		hostedCfg.SecretResolver = cfg.HostedPollerSecretResolver
		hostedCfg.LinearEndpoint = strings.TrimSpace(cfg.HostedLinearEndpoint)
	}
	return hostedCfg
}

func buildRuntimeRecorder(
	cfg *FactoryServiceConfig,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	clock factory.Clock,
	recordPath string,
	workflowID string,
) (*replay.Recorder, error) {
	recordingArtifact, err := newRecordingArtifact(
		&FactoryServiceConfig{
			RecordPath: recordPath,
			WorkflowID: workflowID,
		},
		factoryDir,
		factoryCfg,
		runtimeCfg,
		clock,
	)
	if err != nil || recordingArtifact == nil {
		return nil, err
	}
	recording, err := replay.NewRecorder(
		recordPath,
		recordingArtifact,
		replay.WithFlushInterval(cfg.RecordFlushInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("create replay recorder: %w", err)
	}
	return recording, nil
}

func buildRuntimeListener(
	factoryDir string,
	activeFactory factory.Factory,
	logger *zap.Logger,
	net *state.Net,
) (*ingest.FileWatcher, error) {
	inputsDir := filepath.Join(factoryDir, interfaces.InputsDir)
	if !dirExists(inputsDir) {
		if err := os.MkdirAll(inputsDir, 0o755); err != nil {
			return nil, fmt.Errorf("create inputs dir: %w", err)
		}
	} else {
		logger.Info("using inputs/ directory", zap.String("dir", inputsDir))
	}
	return ingest.NewFileWatcher(
		inputsDir,
		activeFactory,
		logger,
		ingest.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)),
	), nil
}

func (fs *FactoryService) dashboardLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fs.renderDashboard(ctx)
		}
	}
}

func (fs *FactoryService) renderDashboard(ctx context.Context) {
	now := factory.EnsureClock(fs.clock).Now()
	input, err := fs.buildSimpleDashboardRenderInput(ctx, now)
	if err != nil {
		if fs.logger != nil {
			fs.logger.Error("simple dashboard render failed", zap.Error(err))
		}
		return
	}
	fs.cfg.SimpleDashboardRenderer(input)
}

func (fs *FactoryService) buildSimpleDashboardRenderInput(ctx context.Context, now time.Time) (SimpleDashboardRenderInput, error) {
	es, err := fs.GetEngineStateSnapshot(ctx)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	renderData, err := fs.simpleDashboardRenderData(ctx, es.TickCount, es.ActiveThrottlePauses)
	if err != nil {
		return SimpleDashboardRenderInput{}, err
	}
	return SimpleDashboardRenderInput{
		EngineState: *es,
		RenderData:  renderData,
		Now:         now,
	}, nil
}

func (fs *FactoryService) simpleDashboardRenderData(
	ctx context.Context,
	selectedTick int,
	activeThrottlePauses []interfaces.ActiveThrottlePause,
) (dashboardrender.SimpleDashboardRenderData, error) {
	events, err := fs.GetFactoryEvents(ctx)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	worldState, err := projections.ReconstructFactoryWorldState(events, selectedTick)
	if err != nil {
		return dashboardrender.SimpleDashboardRenderData{}, err
	}
	renderData := dashboardrender.SimpleDashboardRenderDataFromWorldState(worldState)
	renderData.ActiveThrottlePauses = projections.ProjectActiveThrottlePauses(worldState.Topology, activeThrottlePauses)
	return renderData, nil
}

// dirExists returns true if the path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func effectiveFactoryRunnerID(override string, factoryCfg *interfaces.FactoryConfig) string {
	if runner := interfaces.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if factoryCfg == nil {
		return ""
	}
	return interfaces.NormalizeRunnerID(factoryCfg.Runner)
}

// loadWorkersFromConfig instantiates worker executors from the loaded runtime config.
// Workers missing AGENTS.md keep the existing noop behavior so topology-only tests continue to work.
func loadWorkersFromConfig(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) ([]factory.FactoryOption, error) {
	var opts []factory.FactoryOption
	logger.Info("loading workers from runtime config", "working-directory", factoryDir)
	if factoryCfg == nil {
		return nil, fmt.Errorf("factory config is required")
	}
	preflight := runnerSelectionPreflight{
		skipCommandAvailability: providerOverride != nil || providerCommandRunner != nil || skipBuiltInRunnerPrerequisiteValidation,
	}
	if err := validateConfiguredWorkstationRunners(factoryCfg, factoryRunnerID, runtimeCfg, preflight); err != nil {
		return nil, err
	}
	for _, workerCfg := range factoryCfg.Workers {
		logger.Debug("loading worker", "worker", workerCfg.Name)
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || def == nil || def.Type == "" {
			logger.Debug("no AGENTS.md for worker; using noop executor", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, &workerexecutor.NoopExecutor{}))
			continue
		}
		executor := buildWorkerExecutor(runtimeCfg, factoryCfg, workerCfg.Name, factoryRunnerID, workflowContext, logger, providerOverride, inferenceProgressPublisher, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, modelRecorder, now, modelDomain)
		if executor != nil {
			logger.Info("loaded worker", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, executor))
		} else {
			logger.Error("failed to load worker", "worker", workerCfg.Name)
			return nil, fmt.Errorf("unsupported worker type for worker %q: %s", workerCfg.Name, def.Type)
		}
	}
	for _, workstationCfg := range factoryCfg.Workstations {
		def, ok := runtimeCfg.Workstation(workstationCfg.Name)
		if !ok || def == nil {
			continue
		}
		if def.Type != interfaces.WorkstationTypeLogical || def.WorkerTypeName != "" {
			continue
		}
		logger.Info("loading workerless logical workstation", "workstation", workstationCfg.Name)
		opts = append(opts, factory.WithWorkerExecutor(workstationCfg.Name, &workerexecutor.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			WorkflowContext: workflowContext,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}))
	}
	return opts, nil
}

func configuredWorkstationExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	inner workers.WorkstationRequestExecutor,
	logger logging.Logger,
) *workerexecutor.WorkstationExecutor {
	return &workerexecutor.WorkstationExecutor{
		RuntimeConfig:   runtimeCfg,
		DefaultRunnerID: factoryRunnerID,
		WorkflowContext: workflowContext,
		Executor:        inner,
		Renderer:        &workerprompting.DefaultPromptRenderer{},
		Logger:          logger,
	}
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) workers.WorkerExecutor {
	def, ok := runtimeCfg.Worker(workerName)
	if !ok {
		return nil
	}

	switch def.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		return buildProviderBackedWorkerExecutor(
			runtimeCfg,
			factoryCfg,
			def,
			factoryRunnerID,
			workflowContext,
			logger,
			providerOverride,
			inferenceProgressPublisher,
			providerCommandRunner,
			inferenceRecorder,
			modelRecorder,
			now,
			modelDomain,
		)
	case interfaces.WorkstationTypeLogical:
		return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, nil, logger)
	case interfaces.WorkerTypeScript:
		return buildScriptWorkerExecutor(
			runtimeCfg,
			def,
			factoryRunnerID,
			workflowContext,
			logger,
			cmdRunner,
			scriptRecorder,
		)
	default:
		return nil
	}
}

func buildProviderBackedWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	def *interfaces.WorkerConfig,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) workers.WorkerExecutor {
	runner := providerBackedRunner(
		def,
		logger,
		providerOverride,
		inferenceProgressPublisher,
		providerCommandRunner,
		inferenceRecorder,
		now,
	)
	runner = wrapLocalModelRunner(runner, runtimeCfg, factoryCfg, def, modelDomain)
	runner = modelDomain.resources.WrapRunner(runner, factoryCfg, def)
	runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
	agentExec := workerexecutor.NewAgentExecutorWithRunner(
		runtimeCfg,
		runner,
		workerexecutor.WithLogger(logger),
	)
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, agentExec, logger)
}

func providerBackedRunner(
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	now func() time.Time,
) workers.Runner {
	runner := newProviderRunner(def, logger, providerOverride, inferenceProgressPublisher, providerCommandRunner)
	if inferenceRecorder == nil {
		return runner
	}
	return wrapRecordingProviderRunner(runner, providerOverride, inferenceRecorder, now)
}

func newProviderRunner(
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
) workers.Runner {
	if providerOverride != nil {
		return workers.RunnerFromProvider(providerOverride)
	}
	return workerprovider.NewScriptWrapProvider(providerRunnerOptions(
		def,
		logger,
		inferenceProgressPublisher,
		providerCommandRunner,
	)...)
}

func providerRunnerOptions(
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
) []workerprovider.ScriptWrapProviderOption {
	opts := []workerprovider.ScriptWrapProviderOption{
		workerprovider.WithSkipPermissions(def.SkipPermissions),
		workerprovider.WithProviderLogger(logger),
	}
	if inferenceProgressPublisher != nil {
		opts = append(opts, workerprovider.WithInferenceProgressPublisher(inferenceProgressPublisher))
	}
	if providerCommandRunner != nil {
		opts = append(opts, workerprovider.WithProviderCommandRunner(providerCommandRunner))
	}
	return opts
}

func wrapRecordingProviderRunner(
	runner workers.Runner,
	providerOverride workerprovider.Provider,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	now func() time.Time,
) workers.Runner {
	recordingClock := workerprovider.WithRecordingProviderClock(now)
	if providerOverride != nil {
		return workers.RunnerFromProvider(
			workerprovider.NewRecordingProvider(providerOverride, inferenceRecorder, recordingClock),
		)
	}
	providerRunner, ok := runner.(*workerprovider.ScriptWrapProvider)
	if !ok {
		return runner
	}
	return workers.RunnerFromProvider(
		workerprovider.NewRecordingProvider(providerRunner, inferenceRecorder, recordingClock),
	)
}

func buildScriptWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	def *interfaces.WorkerConfig,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
) workers.WorkerExecutor {
	scriptOpts := scriptExecutorOptions(runtimeCfg, scriptRecorder)
	var scriptExec workers.WorkstationRequestExecutor
	if cmdRunner != nil {
		scriptExec = workerexecutor.NewScriptExecutorWithRunner(def, cmdRunner, logger, scriptOpts...)
	} else {
		scriptExec = workerexecutor.NewScriptExecutor(def, logger, scriptOpts...)
	}
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, scriptExec, logger)
}

func scriptExecutorOptions(
	runtimeCfg interfaces.RuntimeConfigLookup,
	scriptRecorder workers.ScriptEventRecorder,
) []workerexecutor.ScriptExecutorOption {
	var scriptOpts []workerexecutor.ScriptExecutorOption
	if runtimeCfg != nil && runtimeCfg.FactoryDir() != "" {
		scriptOpts = append(scriptOpts, workerexecutor.WithScriptFactoryDir(runtimeCfg.FactoryDir()))
	}
	if scriptRecorder != nil {
		scriptOpts = append(scriptOpts, workerexecutor.WithScriptEventRecorder(scriptRecorder))
	}
	return scriptOpts
}

func validateConfiguredWorkstationRunners(factoryCfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, preflight runnerSelectionPreflight) error {
	if factoryCfg == nil {
		return nil
	}
	for i, workstation := range factoryCfg.Workstations {
		runtimeWorkstation, ok := runtimeCfg.Workstation(workstation.Name)
		if ok && runtimeWorkstation != nil {
			workstation = *runtimeWorkstation
		}

		worker, _ := runtimeCfg.Worker(workstation.WorkerTypeName)
		workerModelProvider := ""
		if worker != nil {
			workerModelProvider = worker.ModelProvider
		}

		selection := interfaces.ResolveRunnerSelection(workstation.Runner, factoryRunnerID, workerModelProvider)
		workerOpenCodeAgent := ""
		if worker != nil {
			workerOpenCodeAgent = worker.OpenCodeAgent
		}
		if err := interfaces.ValidateOpenCodeAgentForRunnerSelection(workstation.OpenCodeAgent, workerOpenCodeAgent, selection); err != nil {
			return fmt.Errorf("workstations[%d](%s).openCodeAgent: %w", i, workstation.Name, err)
		}
		if !runnerSelectionRequiresValidation(selection) {
			continue
		}
		if err := validateResolvedRunnerSelection(selection, preflight); err != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", i, workstation.Name, err)
		}
	}
	return nil
}

type runnerSelectionPreflight struct {
	skipCommandAvailability bool
}

func runnerSelectionRequiresValidation(selection interfaces.ResolvedRunnerSelection) bool {
	return selection.Source != interfaces.RunnerSelectionSourceDefault
}

func validateResolvedRunnerSelection(selection interfaces.ResolvedRunnerSelection, preflight runnerSelectionPreflight) error {
	if _, ok := interfaces.BuiltInRunnerMetadata(selection.RunnerID); !ok {
		return fmt.Errorf("unknown runner %q", selection.RunnerID)
	}
	if status, ok := workers.BuiltInRunnerStatus(selection.RunnerID); ok && !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}
	if !preflight.skipCommandAvailability {
		if err := workers.ValidateBuiltInRunnerPrerequisites(selection.RunnerID); err != nil {
			return err
		}
	}
	return nil
}

func runtimeBuildConfigFromService(cfg *FactoryServiceConfig) runtimebuild.Config {
	if cfg == nil {
		return runtimebuild.Config{}
	}
	applyOperatorDefaults := cfg != nil && cfg.ReplayPath == ""
	operatorDefaults := operatorconfig.ResolvedDefaults{}
	if cfg != nil {
		operatorDefaults = cfg.OperatorDefaults
	}
	return runtimebuild.Config{
		ExecutionBaseDir:                        cfg.ExecutionBaseDir,
		RunnerID:                                cfg.RunnerID,
		OperatorDefaults:                        operatorDefaults,
		ApplyOperatorDefaults:                   applyOperatorDefaults,
		RuntimeMode:                             cfg.RuntimeMode,
		Verbose:                                 cfg.Verbose,
		RuntimeInstanceID:                       cfg.RuntimeInstanceID,
		RuntimeLogDir:                           cfg.RuntimeLogDir,
		RuntimeLogConfig:                        cfg.RuntimeLogConfig,
		RuntimeMetricsDir:                       cfg.RuntimeMetricsDir,
		RuntimeMetricsConfig:                    cfg.RuntimeMetricsConfig,
		RecordPath:                              cfg.RecordPath,
		WorkflowID:                              cfg.WorkflowID,
		MockWorkersConfig:                       cfg.MockWorkersConfig,
		RecordFlushInterval:                     cfg.RecordFlushInterval,
		ModelCacheDir:                           cfg.ModelCacheDir,
		SkipBuiltInRunnerPrerequisiteValidation: cfg.SkipBuiltInRunnerPrerequisiteValidation,
		WorkstationLoader:                       cfg.WorkstationLoader,
		ProviderOverride:                        cfg.ProviderOverride,
		ProviderCommandRunnerOverride:           cfg.ProviderCommandRunnerOverride,
		CommandRunnerOverride:                   cfg.CommandRunnerOverride,
		LocalModelRuntimeOverride:               cfg.LocalModelRuntimeOverride,
		ExtraOptions:                            cfg.ExtraOptions,
	}
}

func newRuntimeBuildService(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	startupLocalModels *localModelDomain,
	progressPublisherFactory inferenceProgressPublisherFactory,
	dispatchCompletionFactory dispatchCompletionObserverFactory,
) *runtimebuild.Service {
	buildCfg := runtimeBuildConfigFromService(cfg)
	return runtimebuild.New(
		buildCfg,
		clock,
		baseLogger,
		func(ctx context.Context, input runtimebuild.SessionBuildSpec) (any, error) {
			bundleInput := runtimeBundleBuildInput{
				dir:                   input.Dir,
				folderPath:            input.FolderPath,
				sessionID:             input.SessionID,
				cfg:                   cfg,
				loadedFactoryCfg:      input.LoadedFactoryCfg,
				baseLogger:            input.BaseLogger,
				runtimeInstanceID:     input.RuntimeInstanceID,
				clock:                 input.Clock,
				recordPath:            input.RecordPath,
				workflowID:            input.WorkflowID,
				providerOverride:      input.ProviderOverride,
				providerCommandRunner: input.ProviderCommandRunner,
				commandRunnerOverride: input.CommandRunnerOverride,
				additionalFactoryOpts: input.AdditionalFactoryOpts,
			}
			if progressPublisherFactory != nil {
				bundleInput.inferenceProgressPublisher = progressPublisherFactory(bundleInput.sessionID)
				bundleInput.inferenceProgressPublisherSet = true
			}
			if dispatchCompletionFactory != nil {
				bundleInput.dispatchCompleted = dispatchCompletionFactory(bundleInput.sessionID)
			}
			if startupLocalModels != nil && startupLocalModels.manager != nil {
				bundleInput.prefetchedLocalModels = *startupLocalModels
				*startupLocalModels = localModelDomain{}
			}
			return buildRuntimeBundle(ctx, bundleInput)
		},
	)
}

func wrapProviderCommandRunnerForProgress(
	input runtimeBundleBuildInput,
	runner workers.CommandRunner,
) workers.CommandRunner {
	if !input.inferenceProgressPublisherSet || input.inferenceProgressPublisher == nil {
		return runner
	}
	if runner != nil {
		return runner
	}
	var logger logging.Logger = logging.NoopLogger{}
	if input.baseLogger != nil {
		logger = logging.NewZapLogger(input.baseLogger, input.cfg != nil && input.cfg.Verbose)
	}
	return workerprovider.NewInferenceProgressPublishingCommandRunner(
		input.inferenceProgressPublisher,
		logger,
	)
}

func asRuntimeBundle(bundle any) *factoryRuntimeBundle {
	if bundle == nil {
		return nil
	}
	return bundle.(*factoryRuntimeBundle)
}

func (r *factoryRuntimeBundle) metricsEmitter() metrics.MetricsEmitter {
	if r == nil {
		return metrics.NoopEmitter{}
	}
	return metrics.EnsureEmitter(r.metricsSink)
}

func (r *factoryRuntimeBundle) emitMetricCounter(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Counter(context.Background(), name, value, fields); err != nil {
		r.runtimeLogger().Warn("runtime metrics counter emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

func (r *factoryRuntimeBundle) emitMetricGauge(name string, value float64, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Gauge(context.Background(), name, value, fields); err != nil {
		r.runtimeLogger().Warn("runtime metrics gauge emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

func (r *factoryRuntimeBundle) emitMetricSample(name string, value float64, unit string, fields metrics.Fields) {
	if r == nil {
		return
	}
	if err := r.metricsEmitter().Sample(context.Background(), name, value, unit, fields); err != nil {
		r.runtimeLogger().Warn("runtime metrics sample emission failed", zap.String("metric_name", name), zap.Error(err))
	}
}

func (r *factoryRuntimeBundle) emitRuntimeLifecycleStart() {
	if r == nil {
		return
	}
	r.emitMetricCounter(runtimeMetricLifecycleStarted, 1, metrics.Fields{})
}

func (r *factoryRuntimeBundle) emitRuntimeLifecycleStop(outcome string, reason string) {
	if r == nil {
		return
	}
	r.emitMetricCounter(runtimeMetricLifecycleStopped, 1, metrics.Fields{
		Outcome: outcome,
		Reason:  strings.TrimSpace(reason),
	})
}

func (r *factoryRuntimeBundle) emitRuntimeStateMetrics(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
	if r == nil || snapshot == nil {
		return
	}
	r.emitMetricGauge(runtimeMetricStateActive, boolMetricValue(snapshot.RuntimeStatus == interfaces.RuntimeStatusActive), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStateIdle, boolMetricValue(snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStatePaused, boolMetricValue(snapshot.FactoryState == string(interfaces.FactoryStatePaused)), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricStateFailed, boolMetricValue(snapshot.FactoryState == string(interfaces.FactoryStateFailed)), metrics.Fields{})
	r.emitMetricGauge(runtimeMetricQueueInFlight, float64(snapshot.InFlightCount), metrics.Fields{})
}

func boolMetricValue(active bool) float64 {
	if active {
		return 1
	}
	return 0
}

func (fs *FactoryService) defaultSessionClosedDuringStartup() bool {
	if fs == nil || runtimeModeOrDefault(fs.cfg.RuntimeMode) != interfaces.RuntimeModeService {
		return false
	}
	return fs.sessionByID(defaultFactorySessionID) == nil
}

func (fs *FactoryService) handleDefaultRuntimeStartFailure(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	startErr error,
) error {
	if fs.defaultSessionClosedDuringStartup() {
		fs.clearRunState()
		_ = fs.stopLiveRuntime(currentRuntime)
		return nil
	}
	fs.clearRunState()
	fs.unregisterLiveSession(defaultFactorySessionID)
	stopErr := fs.stopLiveRuntime(currentRuntime)
	if isCanceledServiceStartup(ctx, startErr) {
		if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
			return stopErr
		}
		return nil
	}
	if stopErr != nil && !errors.Is(stopErr, context.Canceled) {
		return errors.Join(fmt.Errorf("start runtime: %w", startErr), stopErr)
	}
	return fmt.Errorf("start runtime: %w", startErr)
}

func runtimeStopOutcome(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], err error, forcedCancel bool) (string, string) {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return "canceled", ""
		}
		return "failed", err.Error()
	}
	if snapshot != nil && snapshot.FactoryState == string(interfaces.FactoryStateFailed) {
		return "failed", ""
	}
	if snapshot != nil && snapshot.RuntimeStatus == interfaces.RuntimeStatusFinished {
		return "completed", ""
	}
	if forcedCancel {
		return "canceled", ""
	}
	return "completed", ""
}

func metricsObservationFromSnapshot(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) runtimeMetricsObservation {
	observation := runtimeMetricsObservation{initialized: snapshot != nil}
	if snapshot == nil {
		return observation
	}
	observation.runtimeStatus = snapshot.RuntimeStatus
	observation.factoryState = interfaces.FactoryState(snapshot.FactoryState)
	observation.inFlightCount = snapshot.InFlightCount
	return observation
}

func (o runtimeMetricsObservation) changedFrom(previous runtimeMetricsObservation) bool {
	if !previous.initialized {
		return o.initialized
	}
	if !o.initialized {
		return false
	}
	return o.runtimeStatus != previous.runtimeStatus ||
		o.factoryState != previous.factoryState ||
		o.inFlightCount != previous.inFlightCount
}

func (fs *FactoryService) finalizeRuntimeLifecycleMetrics(handle *liveRuntimeHandle, last runtimeMetricsObservation) {
	if handle == nil || handle.runtime == nil || handle.runtime.factory == nil {
		return
	}
	handle.lifecycleMetricsOnce.Do(func() {
		snapshot, err := handle.runtime.factory.GetEngineStateSnapshot(context.Background())
		if err == nil {
			current := metricsObservationFromSnapshot(snapshot)
			if current.changedFrom(last) {
				handle.runtime.emitRuntimeStateMetrics(snapshot)
			}
			outcome, reason := runtimeStopOutcome(snapshot, handle.result(), false)
			handle.runtime.emitRuntimeLifecycleStop(outcome, reason)
			return
		}
		outcome, reason := runtimeStopOutcome(nil, handle.result(), false)
		handle.runtime.emitRuntimeLifecycleStop(outcome, reason)
	})
}

func (fs *FactoryService) observeRuntimeMetrics(ctx context.Context, handle *liveRuntimeHandle) {
	if handle == nil || handle.runtime == nil || handle.runtime.factory == nil {
		return
	}
	ticker := time.NewTicker(runtimeMetricsObserverPollInterval)
	defer ticker.Stop()
	var last runtimeMetricsObservation
	for {
		snapshot, err := handle.runtime.factory.GetEngineStateSnapshot(context.Background())
		if err == nil {
			current := metricsObservationFromSnapshot(snapshot)
			if current.changedFrom(last) {
				handle.runtime.emitRuntimeStateMetrics(snapshot)
				last = current
			}
		}
		select {
		case <-handle.runDone:
			fs.finalizeRuntimeLifecycleMetrics(handle, last)
			return
		case <-ctx.Done():
			// Temporary sidecar shutdown (for example during session runtime replacement)
			// must not block on runDone or emit lifecycle stop metrics; stopLiveRuntime
			// finalizes lifecycle telemetry after the runtime actually exits.
			return
		case <-ticker.C:
		}
	}
}

const (
	modelPullMetricAttempts      = "managed_runtime.pull.attempts"
	modelPullMetricSuccess       = "managed_runtime.pull.success"
	modelPullMetricFailure       = "managed_runtime.pull.failure"
	modelPullMetricSourceFailure = "managed_runtime.pull.source_failure"
)

func (fs *FactoryService) modelPullMetricsRecorder() ModelPullMetricsRecorder {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.ModelPullMetricsRecorder
}

func modelEventDiagnostics(success *interfaces.WorkDiagnostics, err error) *factoryapi.SafeWorkDiagnostics {
	if success != nil {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(success)
	}
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) {
		return interfaces.GeneratedSafeWorkDiagnosticsFromWorkDiagnostics(providerErr.Diagnostics)
	}
	return nil
}

func modelEventErrorClass(err error) string {
	var readinessErr *apisurface.ManagedRuntimeInvocationError
	if errors.As(err, &readinessErr) && readinessErr.ReadinessState != "" {
		return "MANAGED_RUNTIME_" + string(readinessErr.ReadinessState)
	}
	var providerErr *workerprovider.ProviderError
	if errors.As(err, &providerErr) && providerErr.Type != "" {
		return string(providerErr.Type)
	}
	if err == nil {
		return ""
	}
	return "MODEL_EXECUTION_FAILED"
}

type zapModelHostLogger struct {
	logger *zap.Logger
}

func newZapModelHostLogger(logger *zap.Logger) modelhost.Logger {
	if logger == nil {
		return nil
	}
	return zapModelHostLogger{logger: logger}
}

func (l zapModelHostLogger) Info(msg string, fields map[string]string) {
	l.logger.Info(msg, modelHostZapFields(fields)...)
}

func (l zapModelHostLogger) Warn(msg string, fields map[string]string) {
	l.logger.Warn(msg, modelHostZapFields(fields)...)
}

func modelHostZapFields(fields map[string]string) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	out := make([]zap.Field, 0, len(fields))
	for key, value := range fields {
		out = append(out, zap.String(key, value))
	}
	return out
}

type invocationMetricsRecorderAdapter struct {
	recorder InvocationMetricsRecorder
}

func newModelHostMetricsRecorder(recorder InvocationMetricsRecorder) modelhost.MetricsRecorder {
	if recorder == nil {
		return nil
	}
	return invocationMetricsRecorderAdapter{recorder: recorder}
}

func (a invocationMetricsRecorderAdapter) RecordMetric(name string, labels map[string]string) {
	if a.recorder == nil || strings.TrimSpace(name) == "" {
		return
	}
	a.recorder.RecordInvocationMetric(InvocationMetric{
		Name:   name,
		Labels: cloneMetricLabels(labels),
	})
}

func modelHostDiagnostics(cfg *FactoryServiceConfig, logger *zap.Logger) modelhost.Diagnostics {
	diagnostics := modelhost.Diagnostics{}
	if logger != nil {
		diagnostics.Logger = newZapModelHostLogger(logger.Named("modelhost"))
	}
	if cfg != nil {
		diagnostics.Metrics = newModelHostMetricsRecorder(cfg.InvocationMetricsRecorder)
	}
	return diagnostics
}
