// backendsizecheck:ignore-file service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
// pkgmaintcheck:ignore-file-lines service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/recording"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboardrender"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/workers/executor/agentrun"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	providerstructured "github.com/portpowered/infinite-you/pkg/workers/provider/structured"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"github.com/portpowered/infinite-you/pkg/workers/skippermissions"
	"go.uber.org/zap"
)

// FactoryConfigLoadResult carries factory config load outputs needed before runtime construction.
type FactoryConfigLoadResult struct {
	LoadedFactoryCfg *factoryconfig.LoadedFactoryConfig
	ReplayArtifact   *interfaces.ReplayArtifact
	RecordingReplay  *recordingreplay.RecordingReplayProjection
	SessionLogger    *zap.Logger
}

// LoadFactoryConfigForCompose loads factory.json or a portable recording for wire composition.
func LoadFactoryConfigForCompose(cfg *FactoryServiceConfig, root FactoryServiceRoot) (FactoryConfigLoadResult, error) {
	logger := runtimebuild.NewSessionLogger(root.BaseLogger, defaultFactorySessionID, root.FactoryRootDir, cfg.Dir)
	if result, portable, err := portableReplayLoadResult(cfg, FactoryConfigLoadResult{SessionLogger: logger}); portable {
		return result, err
	}
	loaded, artifact, err := loadFactoryConfigForService(cfg, logger)
	if err != nil {
		return FactoryConfigLoadResult{}, err
	}
	return FactoryConfigLoadResult{LoadedFactoryCfg: loaded, ReplayArtifact: artifact, SessionLogger: logger}, nil
}

func portableReplayLoadResult(cfg *FactoryServiceConfig, result FactoryConfigLoadResult) (FactoryConfigLoadResult, bool, error) {
	if cfg.ReplayPath == "" {
		return result, false, nil
	}
	projection, portable, err := loadPortableRecordingReplay(cfg.ReplayPath)
	if err != nil {
		return FactoryConfigLoadResult{}, true, fmt.Errorf("load portable replay: %w", err)
	}
	result.RecordingReplay = projection
	return result, portable, nil
}

func loadPortableRecordingReplay(path string) (*recordingreplay.RecordingReplayProjection, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read replay recording: %w", err)
	}
	var header struct {
		RecordingKind string `json:"recordingKind"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.RecordingKind != recording.KindJavaScriptFactorySession {
		return nil, false, nil
	}
	value, err := recording.DecodeAndValidate(bytes.NewReader(data))
	if err != nil {
		return nil, true, err
	}
	projection, err := recordingreplay.ReplayRecording(value)
	if err != nil {
		return nil, true, err
	}
	return &projection, true, nil
}

func composePortableReplayCore(cfg *FactoryServiceConfig, root FactoryServiceRoot, collaborators FactoryServiceCollaborators, load FactoryConfigLoadResult, clock factory.Clock, hosted hostedworkers.Config) (*FactoryCore, bool) {
	if load.RecordingReplay == nil {
		return nil, false
	}
	return &FactoryCore{
		cfg: cfg, root: root, collaborators: collaborators, hostedWorkers: hosted,
		clock: clock, logger: load.SessionLogger, modelAssets: collaborators.LocalModels.Assets,
		durableExecution: recordingreplay.NewService(*load.RecordingReplay),
	}, true
}

func workersSchedulerServiceConfig(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) workersservice.Config {
	if logger == nil {
		logger = zap.NewNop()
	}
	runner := workers.CommandRunner(workers.ExecCommandRunner{})
	workflowID := ""
	defaultFactoryDir := ""
	if cfg != nil {
		if cfg.WorkerApplication.ScriptCommandRunner != nil {
			runner = cfg.WorkerApplication.ScriptCommandRunner
		}
		workflowID = cfg.WorkflowID
		defaultFactoryDir = cfg.Dir
	}
	return workersservice.Config{
		Logger:            logger,
		Clock:             clock,
		CommandRunner:     runner,
		WorkflowID:        workflowID,
		DefaultFactoryDir: defaultFactoryDir,
		HostedWorkers:     hostedWorkers,
	}
}

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
	workerApplication             workerapplication.Components
	additionalFactoryOpts         []factory.FactoryOption
	prefetchedLocalModels         LocalModelDomain
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
	configured, err := ConfigWithWorkerApplication(cfg)
	if err != nil {
		return nil, err
	}
	cfg = configured
	core, err := BuildFactoryCore(ctx, cfg)
	if err != nil {
		return nil, err
	}
	service := NewFactoryServiceFromCore(core)
	shell := FactoryServiceShell{Service: service}
	if cfg != nil && cfg.ModelAPI != nil {
		service = AttachModelServiceCollaborator(shell, cfg.ModelAPI)
	} else {
		// Portable durable-session recordings intentionally contain no Factory
		// runtime configuration, and direct compatibility construction no longer
		// owns model-service assembly. Attach an explicit unavailable boundary so
		// every service facade remains a consumer of an already-built ModelAPI.
		service = AttachModelServiceCollaborator(shell, unavailableModelService{})
	}
	service = AttachFactorySaveCollaborator(
		FactoryServiceShell{Service: service},
		ProvideFactorySaveCollaborator(FactoryServiceShell{Service: service}, cfg),
	)
	return AttachSessionGatewayCollaborator(
		FactoryServiceShell{Service: service},
		ProvideSessionGatewayCollaborator(FactoryServiceShell{Service: service}, cfg),
	), nil
}

// ConfigWithWorkerApplication adapts direct service command-runner overrides
// into the worker application consumed by FactoryCore.
func ConfigWithWorkerApplication(cfg *FactoryServiceConfig) (*FactoryServiceConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("factory service config is required")
	}
	if cfg.WorkerApplication.Valid() && cfg.ProviderCommandRunnerOverride == nil && cfg.CommandRunnerOverride == nil {
		return cfg, nil
	}
	components := cfg.WorkerApplication
	var err error
	if components.Valid() {
		components, err = components.WithCommandRunners(
			cfg.ProviderCommandRunnerOverride,
			cfg.CommandRunnerOverride,
		)
	} else {
		components, err = workerapplication.New(cfg.Logger, workerapplication.Edges{
			ProviderCommandRunner: cfg.ProviderCommandRunnerOverride,
			ScriptCommandRunner:   cfg.CommandRunnerOverride,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("construct factory service worker application: %w", err)
	}
	configured := *cfg
	configured.WorkerApplication = components
	return &configured, nil
}

func wireModelAssetPuller(cfg *FactoryServiceConfig, production modelAssetPuller) modelAssetPuller {
	if cfg != nil && cfg.ModelAssets != nil {
		return cfg.ModelAssets
	}
	return production
}

type serviceCoordinatorPolicy struct {
	dir                     string
	executionBaseDir        string
	runtimeMode             interfaces.RuntimeMode
	port                    int
	verbose                 bool
	runtimeInstanceID       string
	workFile                string
	workflowID              string
	mockWorkersConfig       *factoryconfig.MockWorkersConfig
	simpleDashboardRenderer SimpleDashboardRenderer
	apiServerStarter        APIServerStarter
	apiServerReady          <-chan struct{}
	workstationLoader       factoryconfig.WorkstationLoader
	modelCacheDir           string
	runnerID                string
	providerOverride        workers.Provider
}

const (
	runtimeMetricLifecycleStarted     = "runtime.lifecycle.started"
	runtimeMetricLifecycleStopped     = "runtime.lifecycle.stopped"
	runtimeMetricStateActive          = "runtime.state.active"
	runtimeMetricStateIdle            = "runtime.state.idle"
	runtimeMetricStatePaused          = "runtime.state.paused"
	runtimeMetricStateFailed          = "runtime.state.failed"
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
		policy.providerOverride != nil
}

func serviceCoordinatorPolicyFromConfig(cfg *FactoryServiceConfig) serviceCoordinatorPolicy {
	if cfg == nil {
		return serviceCoordinatorPolicy{}
	}
	return serviceCoordinatorPolicy{
		dir:                     cfg.Dir,
		executionBaseDir:        cfg.ExecutionBaseDir,
		runtimeMode:             cfg.RuntimeMode,
		port:                    cfg.Port,
		verbose:                 cfg.Verbose,
		runtimeInstanceID:       cfg.RuntimeInstanceID,
		workFile:                cfg.WorkFile,
		workflowID:              cfg.WorkflowID,
		mockWorkersConfig:       cfg.MockWorkersConfig,
		simpleDashboardRenderer: cfg.SimpleDashboardRenderer,
		apiServerStarter:        cfg.APIServerStarter,
		apiServerReady:          cfg.APIServerReady,
		workstationLoader:       cfg.WorkstationLoader,
		modelCacheDir:           cfg.ModelCacheDir,
		runnerID:                cfg.RunnerID,
		providerOverride:        cfg.ProviderOverride,
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
	if bundle == nil || bundle.LogSink == nil {
		return RuntimeLogDiagnostics{}
	}
	return RuntimeLogDiagnostics{
		Path:                bundle.LogSink.Path(),
		RootDir:             bundle.LogSink.RootDir(),
		StartTimeUTC:        bundle.LogSink.StartTimeUTC(),
		MetricsPath:         runtimeMetricsPath(bundle.MetricsSink),
		MetricsRootDir:      runtimeMetricsRootDir(bundle.MetricsSink),
		MetricsStartTimeUTC: runtimeMetricsStartTime(bundle.MetricsSink),
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
	localModels := input.prefetchedLocalModels
	if localModels.Manager == nil {
		var err error
		localModels, err = modelhost.NewLocalDomain(LocalModelDomainDependencies(input.cfg))
		if err != nil {
			return nil, err
		}
	}
	hostInput := factoryservice.BuildInput{
		Dir:                   input.dir,
		FolderPath:            input.folderPath,
		SessionID:             sessionID,
		Config:                hostConfigFromService(input.cfg),
		LoadedFactoryCfg:      input.loadedFactoryCfg,
		BaseLogger:            input.baseLogger,
		RuntimeInstanceID:     input.runtimeInstanceID,
		BackendScopeID:        serviceBackendScopeID(input.cfg),
		Clock:                 input.clock,
		RecordPath:            input.recordPath,
		WorkflowID:            input.workflowID,
		ProviderOverride:      input.providerOverride,
		ProviderCommandRunner: input.providerCommandRunner,
		CommandRunnerOverride: input.commandRunnerOverride,
		AdditionalFactoryOpts: input.additionalFactoryOpts,
		PrefetchedLocalModels: localModels,
		DispatchCompleted:     input.dispatchCompleted,
		LoadWorkerOpts: func(eventHistory *factoryevents.FactoryEventHistory, logger *zap.Logger) ([]factory.FactoryOption, error) {
			effectiveRunnerID := effectiveFactoryRunnerID(input.cfg.RunnerID, input.loadedFactoryCfg.FactoryConfig())
			return loadRuntimeBundleWorkerOptions(input, logger, effectiveRunnerID, eventHistory, localModels)
		},
	}
	if input.inferenceProgressPublisherSet {
		hostInput.InferenceProgressPublisher = input.inferenceProgressPublisher
		hostInput.InferenceProgressPublisherSet = true
	}
	bundle, err := factoryservice.Build(ctx, hostInput)
	if err != nil {
		return nil, err
	}
	if input.cfg != nil && bundle != nil && bundle.RuntimeInstanceID != "" {
		input.cfg.RuntimeInstanceID = bundle.RuntimeInstanceID
	}
	return bundle, nil
}

func hostConfigFromService(cfg *FactoryServiceConfig) factoryservice.Config {
	if cfg == nil {
		return factoryservice.Config{}
	}
	return factoryservice.ConfigFromHostInput(factoryservice.HostConfigInput{
		RunnerID:                                cfg.RunnerID,
		RuntimeMode:                             cfg.RuntimeMode,
		Verbose:                                 cfg.Verbose,
		RuntimeInstanceID:                       cfg.RuntimeInstanceID,
		RuntimeLogDir:                           cfg.RuntimeLogDir,
		RuntimeLogConfig:                        cfg.RuntimeLogConfig,
		RuntimeFileLoggingPolicy:                factoryservice.RuntimeFileLoggingPolicy(cfg.RuntimeFileLoggingPolicy),
		RuntimeMetricsPolicy:                    factoryservice.RuntimeMetricsPolicy(cfg.RuntimeMetricsPolicy),
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
		LocalModelRuntimeOverride:               cfg.LocalModelRuntimeOverride,
		ModelAssetsOverride:                     cfg.ModelAssets,
		ModelHostOverride:                       cfg.ModelHostOverride,
		LocalModelHooks:                         localModelHooks(),
		ExtraOptions:                            cfg.ExtraOptions,
		InvocationMetricsRecorder:               invocationMetricsAdapter{recorder: cfg.InvocationMetricsRecorder},
		Logger:                                  cfg.Logger,
	})
}

type invocationMetricsAdapter struct {
	recorder InvocationMetricsRecorder
}

func (a invocationMetricsAdapter) RecordInvocationMetric(metric factoryservice.InvocationMetric) {
	if a.recorder == nil {
		return
	}
	a.recorder.RecordInvocationMetric(InvocationMetric{
		Name:   metric.Name,
		Labels: metric.Labels,
	})
}

func loadRuntimeBundleWorkerOptions(
	input runtimeBundleBuildInput,
	logger *zap.Logger,
	effectiveFactoryRunnerID string,
	eventHistory *factoryevents.FactoryEventHistory,
	localModels LocalModelDomain,
) ([]factory.FactoryOption, error) {
	workerOpts, err := loadWorkersFromApplication(
		input.loadedFactoryCfg.FactoryDir(),
		input.loadedFactoryCfg.FactoryConfig(),
		effectiveFactoryRunnerID,
		input.loadedFactoryCfg,
		runtimeWorkflowContext(input.loadedFactoryCfg.FactoryConfig(), input.sessionID),
		logging.NewZapLogger(logger, input.cfg.Verbose),
		input.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		input.cfg.InvocationSkipPermissionsOverride,
		input.providerOverride,
		input.inferenceProgressPublisher,
		input.workerApplication,
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		eventHistory.RecordModelEvent,
		eventHistory.RecordAgentRunEvent,
		input.clock.Now,
		localModels,
	)
	if err != nil {
		logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}
	return workerOpts, nil
}

// localModelDomain is the compatibility alias for extracted local-model wiring.
type localModelDomain = LocalModelDomain

func localModelHooks() localmodels.Hooks {
	return localmodels.Hooks{
		MarkResourceWaitStarted:  markModelExecutionResourceWaitStarted,
		MarkResourceWaitFinished: markModelExecutionResourceWaitFinished,
		MarkLoadRequested:        markModelExecutionLoadRequested,
		MarkLoadFinished:         markModelExecutionLoadFinished,
		MarkLoadReused:           markModelExecutionLoadReused,
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

func wrapLocalModelRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	modelDomain LocalModelDomain,
) workers.Runner {
	return factoryservice.WrapLocalModelRunner(inner, runtimeCfg, factoryCfg, workerDef, modelDomain)
}

func closeRuntimeBundleSinks(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	return factoryservice.CloseBundleSinks(logSink, metricsSink)
}

func buildHostedWorkersConfig(cfg *FactoryServiceConfig, logger *zap.Logger, clock factory.Clock) hostedworkers.Config {
	if cfg != nil && cfg.WorkerApplication.Valid() {
		return cfg.WorkerApplication.Hosted
	}
	hostedCfg := hostedworkers.Config{Logger: logger}
	if supervisorClock, ok := clock.(clockwork.Clock); ok && supervisorClock != nil {
		hostedCfg.Clock = supervisorClock
	}
	return hostedworkers.NewConfig(hostedCfg)
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
func loadWorkersFromApplication(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	workerApplication workerapplication.Components,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) ([]factory.FactoryOption, error) {
	var opts []factory.FactoryOption
	logger.Info("loading workers from runtime config", "working-directory", factoryDir)
	preflight := runnerSelectionPreflight{
		skipCommandAvailability: providerOverride != nil || workerApplication.ProviderCommandInjected || skipBuiltInRunnerPrerequisiteValidation,
	}
	if err := validateWorkerConstructionInputs(
		factoryCfg, factoryRunnerID, runtimeCfg, preflight, invocationSkipPermissionsOverride, workerApplication,
	); err != nil {
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
		executor, err := buildWorkerExecutor(runtimeCfg, factoryCfg, workerCfg.Name, factoryRunnerID, workflowContext, logger, invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher, workerApplication, scriptRecorder, inferenceRecorder, modelRecorder, agentRunRecorder, now, modelDomain)
		if err != nil {
			return nil, fmt.Errorf("construct worker %q: %w", workerCfg.Name, err)
		}
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

func validateWorkerConstructionInputs(
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	preflight runnerSelectionPreflight,
	invocationSkipPermissionsOverride *bool,
	workerApplication workerapplication.Components,
) error {
	if factoryCfg == nil {
		return fmt.Errorf("factory config is required")
	}
	if err := validateWorkerLoadPreflight(factoryCfg, factoryRunnerID, runtimeCfg, preflight, invocationSkipPermissionsOverride); err != nil {
		return err
	}
	if !workerApplication.Valid() {
		return fmt.Errorf("worker application components are required")
	}
	return nil
}

func validateWorkerLoadPreflight(
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	preflight runnerSelectionPreflight,
	invocationSkipPermissionsOverride *bool,
) error {
	if err := validateConfiguredWorkstationRunners(factoryCfg, factoryRunnerID, runtimeCfg, preflight); err != nil {
		return err
	}
	return skippermissions.ValidateInvocationSkipPermissionsWorkers(factoryCfg, runtimeCfg, invocationSkipPermissionsOverride)
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
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	workerApplication workerapplication.Components,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) (workers.WorkerExecutor, error) {
	def, ok := runtimeCfg.Worker(workerName)
	if !ok {
		return nil, nil
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
			invocationSkipPermissionsOverride,
			providerOverride,
			inferenceProgressPublisher,
			workerApplication.Provider,
			inferenceRecorder,
			modelRecorder,
			agentRunRecorder,
			now,
			modelDomain,
		)
	case interfaces.WorkstationTypeLogical:
		return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, nil, logger), nil
	case interfaces.WorkerTypeScript:
		return buildScriptWorkerExecutor(
			runtimeCfg,
			def,
			factoryRunnerID,
			workflowContext,
			logger,
			workerApplication.Script,
			scriptRecorder,
		)
	default:
		return nil, nil
	}
}

func buildProviderBackedWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	def *interfaces.WorkerConfig,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerFactory *workerprovider.Factory,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) (workers.WorkerExecutor, error) {
	runner, err := providerBackedRunner(
		runtimeCfg,
		def,
		logger,
		invocationSkipPermissionsOverride,
		providerOverride,
		inferenceProgressPublisher,
		providerFactory,
		inferenceRecorder,
		now,
	)
	if err != nil {
		return nil, err
	}
	runner = wrapLocalModelRunner(runner, runtimeCfg, factoryCfg, def, modelDomain)
	runner = modelDomain.Resources.WrapRunner(runner, factoryCfg, def)
	runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
	inferenceExecutor := workerexecutor.NewAgentExecutorWithRunner(
		runtimeCfg,
		runner,
		workerexecutor.WithLogger(logger),
	)
	agentRunExecutor := workeragentrun.NewAgentRunExecutor(
		runtimeCfg,
		runner,
		workeragentrun.WithAgentRunLogger(logger),
		workeragentrun.WithAgentRunEventRecorder(agentRunRecorder),
		workeragentrun.WithAgentRunClock(now),
	)
	inner := &workerexecutor.WorkstationBehaviorRouter{
		RuntimeConfig:     runtimeCfg,
		InferenceExecutor: inferenceExecutor,
		AgentRunExecutor:  agentRunExecutor,
	}
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, inner, logger), nil
}

func providerBackedRunner(
	runtimeCfg interfaces.RuntimeConfigLookup,
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerFactory *workerprovider.Factory,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	now func() time.Time,
) (workers.Runner, error) {
	runner, err := newProviderRunner(runtimeCfg, def, logger, invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher, providerFactory)
	if err != nil {
		return nil, err
	}
	if inferenceRecorder == nil {
		return runner, nil
	}
	return wrapRecordingProviderRunner(runner, providerOverride, inferenceRecorder, now), nil
}

func newProviderRunner(
	runtimeCfg interfaces.RuntimeConfigLookup,
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerFactory *workerprovider.Factory,
) (workers.Runner, error) {
	if providerOverride != nil {
		return workers.RunnerFromProvider(providerOverride), nil
	}
	if providerFactory == nil {
		return nil, fmt.Errorf("provider worker factory is required")
	}
	opts := providerRunnerOptions(
		runtimeCfg,
		def,
		logger,
		invocationSkipPermissionsOverride,
		inferenceProgressPublisher,
	)
	return providerFactory.New(opts...)
}

func providerRunnerOptions(
	runtimeCfg interfaces.RuntimeConfigLookup,
	def *interfaces.WorkerConfig,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
) []workerprovider.ScriptWrapProviderOption {
	opts := []workerprovider.ScriptWrapProviderOption{
		workerprovider.WithSkipPermissions(skippermissions.EffectiveSkipPermissions(
			def.SkipPermissions,
			def.Type,
			invocationSkipPermissionsOverride,
		)),
		workerprovider.WithProviderLogger(logger),
	}
	if runtimeCfg != nil {
		if factoryDir := strings.TrimSpace(runtimeCfg.FactoryDir()); factoryDir != "" {
			opts = append(opts, workerprovider.WithAgyFactoryRoot(factoryDir))
		}
	}
	if inferenceProgressPublisher != nil {
		opts = append(opts,
			workerprovider.WithInferenceProgressPublisher(inferenceProgressPublisher),
			workerprovider.WithResponseStreamExecutor(providerstructured.NewExecutor()),
		)
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
	scriptFactory *workerexecutor.ScriptFactory,
	scriptRecorder workers.ScriptEventRecorder,
) (workers.WorkerExecutor, error) {
	scriptOpts := scriptExecutorOptions(runtimeCfg, scriptRecorder)
	if scriptFactory == nil {
		return nil, fmt.Errorf("script worker factory is required")
	}
	scriptExec, err := scriptFactory.New(def, logger, scriptOpts...)
	if err != nil {
		return nil, err
	}
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, scriptExec, logger), nil
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
		WorkerApplication:                       cfg.WorkerApplication,
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
) (*runtimebuild.Service, error) {
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
				workerApplication:     input.WorkerApplication,
				additionalFactoryOpts: input.AdditionalFactoryOpts,
			}
			if progressPublisherFactory != nil {
				bundleInput.inferenceProgressPublisher = progressPublisherFactory(bundleInput.sessionID)
				bundleInput.inferenceProgressPublisherSet = true
			}
			workerApplication, err := workerApplicationWithProgress(bundleInput)
			if err != nil {
				return nil, fmt.Errorf("construct runtime worker application: %w", err)
			}
			bundleInput.workerApplication = workerApplication
			if dispatchCompletionFactory != nil {
				bundleInput.dispatchCompleted = dispatchCompletionFactory(bundleInput.sessionID)
			}
			if startupLocalModels != nil && startupLocalModels.Manager != nil {
				bundleInput.prefetchedLocalModels = *startupLocalModels
				*startupLocalModels = localModelDomain{}
			}
			return buildRuntimeBundle(ctx, bundleInput)
		},
	)
}

// NewRuntimeBuildServiceWithObservers constructs the runtime builder with the
// session-owner callbacks selected by the application composition root.
func NewRuntimeBuildServiceWithObservers(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
	progressPublisherFactory func(string) workerprovider.InferenceProgressPublisher,
	dispatchCompletionFactory func(string) func(string),
) (*runtimebuild.Service, error) {
	return newRuntimeBuildService(
		cfg,
		clock,
		baseLogger,
		localModels,
		inferenceProgressPublisherFactory(progressPublisherFactory),
		dispatchCompletionObserverFactory(dispatchCompletionFactory),
	)
}

func workerApplicationWithProgress(input runtimeBundleBuildInput) (workerapplication.Components, error) {
	if input.workerApplication.ProviderCommandInjected {
		return input.workerApplication, nil
	}
	runner := wrapProviderCommandRunnerForProgress(input, input.providerCommandRunner)
	if runner == nil {
		return input.workerApplication, nil
	}
	return input.workerApplication.WithCommandRunners(runner, nil)
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

func (fs *FactoryService) defaultSessionClosedDuringStartup() bool {
	if fs == nil || runtimeModeOrDefault(fs.cfg.RuntimeMode) != interfaces.RuntimeModeService {
		return false
	}
	return fs.defaultSession() == nil
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
	if session := fs.defaultSession(); session != nil {
		fs.unregisterLiveSession(session.ID)
	} else {
		fs.unregisterLiveSession(defaultFactorySessionID)
	}
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

func ensureServiceBackendScope(cfg *FactoryServiceConfig, logger *zap.Logger) error {
	if cfg == nil {
		return fmt.Errorf("factory service config is required to resolve backend scope")
	}
	if strings.TrimSpace(cfg.ReplayPath) != "" {
		return nil
	}
	if strings.TrimSpace(cfg.BackendScopeID) != "" {
		return nil
	}

	configPath, err := resolveSystemConfigPath(cfg)
	if err != nil {
		return err
	}
	resolved, err := systemconfig.EnsureLocalBackendScope(configPath)
	if err != nil {
		return err
	}
	cfg.BackendScopeID = resolved.BackendScopeID
	if logger != nil {
		logger.Info("resolved backend scope for local backend", zap.String("diagnostics", resolved.DiagnosticsLine()))
	}
	return nil
}

func resolveSystemConfigPath(cfg *FactoryServiceConfig) (string, error) {
	if cfg != nil && strings.TrimSpace(cfg.SystemConfigPath) != "" {
		return strings.TrimSpace(cfg.SystemConfigPath), nil
	}
	homeDir := ""
	if cfg != nil {
		homeDir = strings.TrimSpace(cfg.SystemConfigHomeDir)
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory for backend scope system config: %w", err)
		}
	}
	return defaultpaths.OperatorConfigPath(homeDir), nil
}

func serviceBackendScopeID(cfg *FactoryServiceConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.BackendScopeID)
}

// FactoryServiceConfigFromRuntimeHost maps initializer compose config onto the
// service composition config. The structs are kept layout-compatible during the
// runtimehost migration.
func FactoryServiceConfigFromRuntimeHost(cfg *runtimehost.Config) *FactoryServiceConfig {
	if cfg == nil {
		return nil
	}
	return (*FactoryServiceConfig)(unsafe.Pointer(cfg))
}

// RuntimeHostConfigFromFactoryService maps the legacy CLI composition config
// onto initializer-owned runtime composition while both migration structs are
// kept layout-compatible.
func RuntimeHostConfigFromFactoryService(cfg *FactoryServiceConfig) *runtimehost.Config {
	if cfg == nil {
		return nil
	}
	return (*runtimehost.Config)(unsafe.Pointer(cfg))
}

// ModelService is the compose-facing model API collaborator.
type ModelService = apisurface.ModelAPI

func adaptRuntimeHostCore(core *runtimehost.Core) *FactoryCore {
	if core == nil {
		return nil
	}
	return &FactoryCore{
		cfg:              FactoryServiceConfigFromRuntimeHost(core.ServiceConfig()),
		root:             FactoryServiceRoot{FactoryRootDir: core.FactoryRootDir(), BaseLogger: core.BaseLogger()},
		collaborators:    factoryServiceCollaboratorsFromRuntimeHost(core),
		hostedWorkers:    core.HostedWorkers(),
		clock:            core.Clock(),
		startupBundle:    asRuntimeBundle(core.StartupBundle()),
		logger:           core.Logger(),
		modelAssets:      core.ModelAssetPuller(),
		durableExecution: core.DurableExecution(),
	}
}

// NewFactoryServiceFromRuntimeHostCore wraps the shared runtimehost graph in
// the legacy FactoryService facade without replacing stateful collaborators.
func NewFactoryServiceFromRuntimeHostCore(core *runtimehost.Core) *FactoryService {
	return NewFactoryServiceFromCore(adaptRuntimeHostCore(core))
}

func factoryServiceCollaboratorsFromRuntimeHost(core *runtimehost.Core) FactoryServiceCollaborators {
	if core == nil {
		return FactoryServiceCollaborators{}
	}
	return FactoryServiceCollaborators{
		Sessions:         core.Sessions(),
		LocalModels:      core.LocalModels(),
		RuntimeBuild:     core.RuntimeBuild(),
		WorkersScheduler: core.WorkersScheduler(),
	}
}

// InvocationBootstrap constructs and runs one-shot factory invocation
// session/runtime dependencies without binding a listening HTTP server.
type InvocationBootstrap struct {
	Service *FactoryService
}

// NormalizeInvocationBootstrapConfig returns a copy of cfg shaped for in-process
// one-shot invocation: service-mode runtime, no dashboard renderer, no work-file
// seeding, and no API/dashboard TCP listener.
func NormalizeInvocationBootstrapConfig(cfg *FactoryServiceConfig) *FactoryServiceConfig {
	if cfg == nil {
		return nil
	}
	normalized := *cfg
	normalized.Port = 0
	normalized.APIServerStarter = nil
	normalized.APIServerReady = nil
	normalized.SimpleDashboardRenderer = nil
	normalized.RuntimeMode = interfaces.RuntimeModeService
	normalized.WorkFile = ""
	return &normalized
}

// NewInvocationBootstrap adapts an already-composed service facade for one-shot
// invocation. Composition remains owned by the shared Wire application graph.
func NewInvocationBootstrap(service *FactoryService) (*InvocationBootstrap, error) {
	if service == nil {
		return nil, fmt.Errorf("build invocation bootstrap: service is required")
	}
	return &InvocationBootstrap{Service: service}, nil
}

// Run starts the bootstrap-owned factory session runtime loop.
func (b *InvocationBootstrap) Run(ctx context.Context) error {
	if b == nil || b.Service == nil {
		return fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.Run(ctx)
}

// GetCurrentFactoryForSession exposes the active factory definition for a live
// bootstrap-owned session.
func (b *InvocationBootstrap) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if b == nil || b.Service == nil {
		return factoryapi.Factory{}, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.GetCurrentFactoryForSession(ctx, sessionID)
}

// InvokeModel forwards one-shot model invocation through the bootstrap-owned
// FactoryService model collaborator.
func (b *InvocationBootstrap) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (apisurface.ModelInvocationResult, error) {
	if b == nil || b.Service == nil {
		return apisurface.ModelInvocationResult{}, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.InvokeModel(ctx, modelName, request)
}

// InvokeFactorySession forwards one-shot invocation through the bootstrap-owned
// FactoryService session invoker.
func (b *InvocationBootstrap) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factoryapi.InvocationRequest,
) (apisurface.FactoryInvocationResult, error) {
	if b == nil || b.Service == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.InvokeFactorySession(ctx, sessionID, request)
}

// SubscribeSessionResponseEventsFromLatest forwards canonical response-event
// attachment through the bootstrap-owned FactoryService.
func (b *InvocationBootstrap) SubscribeSessionResponseEventsFromLatest(
	sessionID string,
) (*responseeventstore.Subscription, error) {
	if b == nil || b.Service == nil {
		return nil, fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.SubscribeSessionResponseEventsFromLatest(sessionID)
}

// CloseFactorySession releases a bootstrap-owned live session through the same
// FactoryService session ownership path used by API session lifecycle.
func (b *InvocationBootstrap) CloseFactorySession(ctx context.Context, sessionID string) error {
	if b == nil || b.Service == nil {
		return fmt.Errorf("invocation bootstrap is required")
	}
	return b.Service.CloseFactorySession(ctx, sessionID)
}

// ApplyInvocationBootstrapLocalModelTestFixture wires hermetic ready LOCAL managed-model
// overrides for callers that build an in-process invocation bootstrap, such as
// models CLI offline invoke tests.
func ApplyInvocationBootstrapLocalModelTestFixture(
	cfg *FactoryServiceConfig,
	healthEndpoint string,
	runtime localmodels.Runtime,
	assets localmodels.AssetPuller,
) error {
	if cfg == nil {
		return fmt.Errorf("apply invocation bootstrap local model fixture: config is required")
	}
	cfg.LocalModelRuntimeOverride = runtime
	cfg.ModelAssets = assets
	host, err := newInvocationBootstrapSupervisedModelHost(assets, healthEndpoint)
	if err != nil {
		return err
	}
	cfg.ModelHostOverride = host
	cfg.SkipBuiltInRunnerPrerequisiteValidation = true
	return nil
}

func newInvocationBootstrapSupervisedModelHost(assets localmodels.AssetPuller, healthEndpoint string) (modelhost.Host, error) {
	launcher := &invocationBootstrapFakeProcessLauncher{healthEndpoint: strings.TrimSpace(healthEndpoint)}
	gateway := modelhost.NewLocalAssetGateway(assets)
	host, err := modelhost.NewHost(modelhost.Dependencies{
		AssetPuller: gateway, CacheInspector: gateway, ProcessLauncher: launcher,
		Options: modelhost.Options{
			SourceResolver: modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
			Supervisor: modelhost.SupervisorConfig{
				ReadinessTimeout:    500 * time.Millisecond,
				HealthCheckInterval: 10 * time.Millisecond,
				HealthChecker:       modelhost.HTTPHealthChecker{Path: "/health"},
			},
		}})
	if err != nil {
		return nil, fmt.Errorf("construct invocation bootstrap model host: %w", err)
	}
	return host, nil
}

type invocationBootstrapFakeProcessLauncher struct {
	mu             sync.Mutex
	healthEndpoint string
}

func (f *invocationBootstrapFakeProcessLauncher) Start(_ context.Context, _ modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	return &invocationBootstrapFakeManagedProcess{
		endpoint: f.healthEndpoint,
		stopCh:   make(chan struct{}),
	}, nil
}

type invocationBootstrapFakeManagedProcess struct {
	endpoint string
	stopCh   chan struct{}
}

func (p *invocationBootstrapFakeManagedProcess) HealthEndpoint() string {
	return p.endpoint
}

func (p *invocationBootstrapFakeManagedProcess) Stop(context.Context) error {
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}
	return nil
}

func (p *invocationBootstrapFakeManagedProcess) Wait() error {
	<-p.stopCh
	return nil
}

var _ modelhost.ProcessLauncher = (*invocationBootstrapFakeProcessLauncher)(nil)
var _ modelhost.ManagedProcess = (*invocationBootstrapFakeManagedProcess)(nil)
