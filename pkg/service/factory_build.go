// backendsizecheck:ignore-file service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
// pkgmaintcheck:ignore-file-lines service runtime build wiring keeps log, metrics, worker, and recorder assembly co-located until dedicated bundle seams split.
package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/workers/executor/agentrun"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

type (
	factoryRuntimeBundle      = factoryservice.Bundle
	liveRuntimeHandle         = factoryservice.Handle
	modelAssetPuller          = localmodels.AssetPuller
	localModelRuntime         = localmodels.Runtime
	localModelResourceLimiter = localmodels.ResourceLimiter
	managedLocalModelManager  = localmodels.Manager
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
	prefetchedLocalModels         LocalModelDomain
	inferenceProgressPublisher    workerprovider.InferenceProgressPublisher
	inferenceProgressPublisherSet bool
	dispatchCompleted             func(string)
}

// NewStartupLiveSessionHandle constructs the default session handle attached
// during startup core composition.
func NewStartupLiveSessionHandle(bundle *factoryservice.Bundle, spec *runtimebuild.SessionBuildSpec) any {
	return runtimehost.NewStartupLiveSessionHandle(bundle, spec)
}

func wireModelAssetPuller(cfg *FactoryServiceConfig, production modelAssetPuller) modelAssetPuller {
	if cfg != nil && cfg.ModelAssets != nil {
		return cfg.ModelAssets
	}
	return production
}

// WireModelAssetPullerForCompose selects the model asset collaborator used by
// pkg/factorycore startup composition.
func WireModelAssetPullerForCompose(cfg *FactoryServiceConfig, production localmodels.AssetPuller) localmodels.AssetPuller {
	return wireModelAssetPuller(cfg, production)
}

// CloseRuntimeBundleSinksForCompose closes startup bundle sinks when core
// composition fails before the runtime graph is fully wired.
func CloseRuntimeBundleSinksForCompose(logSink *logging.RuntimeLogSink, metricsSink *logging.RuntimeMetricsSink) error {
	return closeRuntimeBundleSinks(logSink, metricsSink)
}

// ReplayFactoryModeOptionsForCompose builds replay side-effect options for core
// composition.
func ReplayFactoryModeOptionsForCompose(
	replayArtifact *interfaces.ReplayArtifact,
) (*replay.SideEffects, []factory.FactoryOption, error) {
	return replayFactoryModeOptions(replayArtifact)
}

// AsRuntimeBundleForCompose converts a runtime-build product into the startup
// bundle used by core composition.
func AsRuntimeBundleForCompose(bundle any) *factoryservice.Bundle {
	return asRuntimeBundle(bundle)
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

func loadFactoryConfigForService(
	cfg *FactoryServiceConfig,
	logger *zap.Logger,
) (*factoryconfig.LoadedFactoryConfig, *interfaces.ReplayArtifact, error) {
	logger.Info("loading factory config", zap.String("dir", cfg.Dir))
	loadedFactoryCfg, replayArtifact, err := runtimehost.LoadFactoryConfigForMode(cfg)
	if err != nil {
		logger.Error("failed to load factory config", zap.Error(err))
		return nil, nil, fmt.Errorf("load factory config: %w", err)
	}
	runtimebuild.WarnPortableBundledReplacementReport(logger, "runtime config load replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	runtimehost.WarnReplayMetadataMismatches(cfg, replayArtifact, logger)
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
		sessionID = runtimehost.DefaultFactorySessionID
	}
	localModels := input.prefetchedLocalModels
	if localModels.Manager == nil {
		localModels = NewLocalModelDomain(input.cfg)
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
	workerOpts, err := loadWorkersFromConfig(
		input.loadedFactoryCfg.FactoryDir(),
		input.loadedFactoryCfg.FactoryConfig(),
		effectiveFactoryRunnerID,
		input.loadedFactoryCfg,
		runtimehost.RuntimeWorkflowContext(input.loadedFactoryCfg.FactoryConfig(), input.sessionID),
		logging.NewZapLogger(logger, input.cfg.Verbose),
		input.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		input.providerOverride,
		input.inferenceProgressPublisher,
		wrapProviderCommandRunnerForProgress(input, input.providerCommandRunner),
		input.commandRunnerOverride,
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

func newRuntimeLocalModelDependencies(cfg *FactoryServiceConfig) LocalModelDomain {
	return NewLocalModelDomain(cfg)
}

func newLocalModelResourceLimiter() *localModelResourceLimiter {
	return localmodels.NewResourceLimiter(localModelHooks())
}

func newManagedLocalModelManager(assetPuller modelAssetPuller, runtime localModelRuntime) *managedLocalModelManager {
	return localmodels.NewManager(assetPuller, runtime, localModelHooks())
}

func localModelHooks() localmodels.Hooks {
	return localmodels.Hooks{
		MarkResourceWaitStarted:  runtimehost.MarkModelExecutionResourceWaitStarted,
		MarkResourceWaitFinished: runtimehost.MarkModelExecutionResourceWaitFinished,
		MarkLoadRequested:        runtimehost.MarkModelExecutionLoadRequested,
		MarkLoadFinished:         runtimehost.MarkModelExecutionLoadFinished,
		MarkLoadReused:           runtimehost.MarkModelExecutionLoadReused,
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
	modelRecorder runtimehost.ModelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
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
		executor := buildWorkerExecutor(runtimeCfg, factoryCfg, workerCfg.Name, factoryRunnerID, workflowContext, logger, providerOverride, inferenceProgressPublisher, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, modelRecorder, agentRunRecorder, now, modelDomain)
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
	modelRecorder runtimehost.ModelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
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
			agentRunRecorder,
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
	modelRecorder runtimehost.ModelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
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
	runner = modelDomain.Resources.WrapRunner(runner, factoryCfg, def)
	runner = runtimehost.NewRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
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
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, inner, logger)
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
	progressPublisherFactory runtimehost.InferenceProgressPublisherFactory,
	dispatchCompletionFactory runtimehost.DispatchCompletionObserverFactory,
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
			if startupLocalModels != nil && startupLocalModels.Manager != nil {
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
		Labels: runtimehost.CloneMetricLabels(labels),
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
