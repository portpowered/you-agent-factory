package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/ingest"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// BuildFactoryService loads factory.json from the config directory, constructs
// the petri net, factory runtime, file watcher, and session metrics.
// portos:func-length-exception owner=agent-factory reason=legacy-service-wiring review=2026-07-18 removal=split-replay-recording-worker-and-listener-builders-before-next-service-wiring-change
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	factoryRootDir, baseLogger, err := resolveFactoryServiceRoot(cfg)
	if err != nil {
		return nil, err
	}
	serviceBuilt := false
	var runtimeBundle *factoryRuntimeBundle
	defer func() {
		if !serviceBuilt && runtimeBundle != nil && runtimeBundle.logSink != nil {
			_ = runtimeBundle.logSink.Close()
		}
	}()
	if cfg.ReplayPath == "" {
		resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		resolvedDir, err = factorysessions.AbsolutizeFactoryDirectory(resolvedDir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		cfg.Dir = resolvedDir
	}

	logger := runtimebuild.NewSessionLogger(baseLogger, defaultFactorySessionID, factoryRootDir, cfg.Dir)
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForService(cfg, logger)
	if err != nil {
		return nil, err
	}
	clock := serviceClockForMode(cfg.Clock, replayArtifact)
	replaySideEffects, replayFactoryOpts, err := replayFactoryModeOptions(replayArtifact)
	if err != nil {
		return nil, err
	}
	collaborators := newFactoryServiceCollaborators(cfg, clock, baseLogger)
	runtimeBundleAny, err := collaborators.runtimeBuild.BuildFromLoadedConfig(ctx, runtimebuild.BuildInput{
		Dir:                   cfg.Dir,
		FolderPath:            factoryRootDir,
		SessionID:             defaultFactorySessionID,
		LoadedFactoryCfg:      loadedFactoryCfg,
		BaseLogger:            baseLogger,
		RuntimeInstanceID:     cfg.RuntimeInstanceID,
		Clock:                 clock,
		RecordPath:            runtimebuild.SessionScopedRecordPath(cfg.RecordPath, defaultFactorySessionID),
		WorkflowID:            cfg.WorkflowID,
		ProviderOverride:      providerOverrideForMode(cfg, replaySideEffects),
		ProviderCommandRunner: providerCommandRunnerForMode(cfg, loadedFactoryCfg),
		CommandRunnerOverride: commandRunnerOverrideForMode(cfg, loadedFactoryCfg, replaySideEffects),
		AdditionalFactoryOpts: replayFactoryOpts,
	})
	if err != nil {
		return nil, err
	}
	runtimeBundle = asRuntimeBundle(runtimeBundleAny)

	serviceBuilt = true
	fs := &FactoryService{
		factoryRootDir: factoryRootDir,
		sessions:       collaborators.sessions,
		hostedWorkers:  buildHostedWorkersConfig(cfg, runtimeBundle.logger, clock),
		startupBundle:  runtimeBundle,
		cfg:            cfg,
		modelAssets:    wireModelAssetPuller(cfg, collaborators.localModels.assets),
		baseLogger:     baseLogger,
		logger:         runtimeBundle.logger,
		clock:          clock,
		runtimeBuild:   collaborators.runtimeBuild,
	}
	fs.factorySave = wireFactorySaveCollaborator(fs, cfg)
	return fs, nil
}

type factoryServiceCollaborators struct {
	sessions     *factorysessions.Registry
	localModels  localModelDomain
	runtimeBuild *runtimebuild.Service
}

func wireModelAssetPuller(cfg *FactoryServiceConfig, production modelAssetPuller) modelAssetPuller {
	if cfg != nil && cfg.ModelAssets != nil {
		return cfg.ModelAssets
	}
	return production
}

func newFactoryServiceCollaborators(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
) factoryServiceCollaborators {
	startupLocalModels := newRuntimeLocalModelDependencies(cfg)
	return factoryServiceCollaborators{
		sessions:     factorysessions.NewRegistry(),
		localModels:  startupLocalModels,
		runtimeBuild: newRuntimeBuildService(cfg, clock, baseLogger, &startupLocalModels),
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
	Path         string
	RootDir      string
	StartTimeUTC time.Time
}

// RuntimeLogDiagnostics returns the selected runtime log metadata for startup
// diagnostics without exposing the sink writer.
func (fs *FactoryService) RuntimeLogDiagnostics() RuntimeLogDiagnostics {
	bundle := fs.currentRuntimeBundle()
	if bundle == nil || bundle.logSink == nil {
		return RuntimeLogDiagnostics{}
	}
	return RuntimeLogDiagnostics{
		Path:         bundle.logSink.Path(),
		RootDir:      bundle.logSink.RootDir(),
		StartTimeUTC: bundle.logSink.StartTimeUTC(),
	}
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
	logSink, runtimeInstanceID, err := buildRuntimeLogSink(input.cfg, input.baseLogger, input.runtimeInstanceID)
	if err != nil {
		return nil, err
	}
	bundleBuilt := false
	defer func() {
		if !bundleBuilt && logSink != nil {
			_ = logSink.Close()
		}
	}()
	if input.cfg != nil && runtimeInstanceID != "" {
		input.cfg.RuntimeInstanceID = runtimeInstanceID
	}
	sessionID := strings.TrimSpace(input.sessionID)
	if sessionID == "" {
		sessionID = defaultFactorySessionID
	}
	logger := runtimebuild.NewSessionLogger(logSink.Logger(), sessionID, input.folderPath, input.dir)

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, input.loadedFactoryCfg.FactoryConfig())
	if err != nil {
		logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(input.cfg.RunnerID, input.loadedFactoryCfg.FactoryConfig())
	eventHistory := factoryevents.NewFactoryEventHistory(net, input.clock.Now, input.loadedFactoryCfg)
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	localModels := input.prefetchedLocalModels
	if localModels.manager == nil {
		localModels = newRuntimeLocalModelDependencies(input.cfg)
	}
	workerOpts, err := loadRuntimeBundleWorkerOptions(input, logger, effectiveFactoryRunnerID, eventHistory, localModels)
	if err != nil {
		return nil, err
	}

	bundleBuilt = true
	return assembleRuntimeBundle(input, logger, logSink, net, eventHistory, localModels, workerOpts)
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
		logging.NewZapLogger(logger, input.cfg.Verbose),
		input.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		input.providerOverride,
		input.providerCommandRunner,
		input.commandRunnerOverride,
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		eventHistory.RecordModelEvent,
		input.clock.Now,
		localModels.resources,
		localModels.manager,
	)
	if err != nil {
		logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}
	return workerOpts, nil
}

func assembleRuntimeBundle(
	input runtimeBundleBuildInput,
	logger *zap.Logger,
	logSink *logging.RuntimeLogSink,
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

	opts := []factory.FactoryOption{
		factory.WithNet(net),
		factory.WithRuntimeMode(input.cfg.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(logger, input.cfg.Verbose)),
		factory.WithRuntimeConfig(input.loadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(input.loadedFactoryCfg.FactoryConfig())),
		factory.WithClock(input.clock),
		factory.WithFactoryEventHistory(eventHistory),
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

	return &factoryRuntimeBundle{
		dir:            input.dir,
		folderPath:     input.folderPath,
		eventHistory:   eventHistory,
		factory:        activeFactory,
		listener:       listener,
		net:            net,
		runtimeCfg:     input.loadedFactoryCfg,
		modelResources: localModels.resources,
		modelAssets:    localModels.assets,
		localModels:    localModels.manager,
		logger:         logger,
		logSink:        logSink,
		recording:      recording,
		recordPath:     input.recordPath,
	}, nil
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
	logSink, err := logging.BuildRuntimeLogger(baseLogger, runtimeInstanceID, cfg.RuntimeLogDir, cfg.RuntimeLogConfig)
	if err != nil {
		return nil, runtimeInstanceID, fmt.Errorf("build runtime logger: %w", err)
	}
	return logSink, runtimeInstanceID, nil
}

// localModelDomain wires pkg/localmodels runtime dependencies constructed at
// service build time and copied onto each factoryRuntimeBundle.
type localModelDomain struct {
	resources *localModelResourceLimiter
	assets    modelAssetPuller
	manager   *managedLocalModelManager
}

func newRuntimeLocalModelDependencies(cfg *FactoryServiceConfig) localModelDomain {
	modelResources := newLocalModelResourceLimiter()
	modelAssets := newModelAssetPuller(strings.TrimSpace(cfg.ModelCacheDir))
	localModelRuntime := cfg.LocalModelRuntimeOverride
	if localModelRuntime == nil {
		localModelRuntime = newOmniVoiceLocalRuntime(nil)
	}
	return localModelDomain{
		resources: modelResources,
		assets:    modelAssets,
		manager:   newManagedLocalModelManager(modelAssets, localModelRuntime),
	}
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

func providerOverrideForMode(cfg *FactoryServiceConfig, sideEffects *replay.SideEffects) workers.Provider {
	if cfg.ProviderOverride != nil || sideEffects == nil {
		return cfg.ProviderOverride
	}
	return sideEffects
}

func commandRunnerOverrideForMode(
	cfg *FactoryServiceConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	sideEffects *replay.SideEffects,
) workers.CommandRunner {
	next := cfg.CommandRunnerOverride
	if next == nil && sideEffects != nil {
		next = sideEffects
	}
	if cfg.MockWorkersConfig == nil {
		return next
	}
	return &workers.MockWorkerCommandRunner{
		Config:        cfg.MockWorkersConfig,
		RuntimeConfig: runtimeCfg,
		Next:          next,
	}
}

func providerCommandRunnerForMode(cfg *FactoryServiceConfig, runtimeCfg interfaces.RuntimeDefinitionLookup) workers.CommandRunner {
	if cfg.MockWorkersConfig == nil {
		return cfg.ProviderCommandRunnerOverride
	}
	return &workers.MockWorkerCommandRunner{
		Config:        cfg.MockWorkersConfig,
		RuntimeConfig: runtimeCfg,
		Next:          cfg.ProviderCommandRunnerOverride,
	}
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
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	providerOverride workerprovider.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelResources *localModelResourceLimiter,
	localModels *managedLocalModelManager,
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
		executor := buildWorkerExecutor(runtimeCfg, factoryCfg, workerCfg.Name, factoryRunnerID, logger, providerOverride, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, modelRecorder, now, modelResources, localModels)
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
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}))
	}
	return opts, nil
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
	factoryRunnerID string,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelResources *localModelResourceLimiter,
	localModels *managedLocalModelManager,
) workers.WorkerExecutor {
	def, ok := runtimeCfg.Worker(workerName)
	if !ok {
		return nil
	}

	switch def.Type {
	case interfaces.WorkerTypeModel:
		var runner workers.Runner
		if providerOverride != nil {
			runner = workers.RunnerFromProvider(providerOverride)
		} else {
			var providerOpts []workerprovider.ScriptWrapProviderOption
			providerOpts = append(providerOpts, workerprovider.WithSkipPermissions(def.SkipPermissions))
			providerOpts = append(providerOpts, workerprovider.WithProviderLogger(logger))
			if providerCommandRunner != nil {
				providerOpts = append(providerOpts, workerprovider.WithProviderCommandRunner(providerCommandRunner))
			}
			runner = workerprovider.NewScriptWrapProvider(providerOpts...)
		}
		if inferenceRecorder != nil {
			if providerOverride != nil {
				provider := workerprovider.NewRecordingProvider(
					providerOverride,
					inferenceRecorder,
					workerprovider.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			} else if providerRunner, ok := runner.(*workerprovider.ScriptWrapProvider); ok {
				provider := workerprovider.NewRecordingProvider(
					providerRunner,
					inferenceRecorder,
					workerprovider.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			}
		}

		agentOpts := []workerexecutor.AgentExecutorOption{
			workerexecutor.WithLogger(logger),
		}
		runner = localModels.WrapRunner(runner, runtimeCfg, factoryCfg, def)
		runner = modelResources.WrapRunner(runner, factoryCfg, def)
		runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
		agentExec := workerexecutor.NewAgentExecutorWithRunner(runtimeCfg, runner, agentOpts...)
		return &workerexecutor.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        agentExec,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkstationTypeLogical:
		return &workerexecutor.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkerTypeScript:
		var scriptOpts []workerexecutor.ScriptExecutorOption
		if runtimeCfg != nil && runtimeCfg.FactoryDir() != "" {
			scriptOpts = append(scriptOpts, workerexecutor.WithScriptFactoryDir(runtimeCfg.FactoryDir()))
		}
		if scriptRecorder != nil {
			scriptOpts = append(scriptOpts, workerexecutor.WithScriptEventRecorder(scriptRecorder))
		}
		var scriptExec workers.WorkstationRequestExecutor
		if cmdRunner != nil {
			scriptExec = workerexecutor.NewScriptExecutorWithRunner(def, cmdRunner, logger, scriptOpts...)
		} else {
			scriptExec = workerexecutor.NewScriptExecutor(def, logger, scriptOpts...)
		}
		return &workerexecutor.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        scriptExec,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	default:
		return nil
	}
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
	return runtimebuild.Config{
		ExecutionBaseDir:                        cfg.ExecutionBaseDir,
		RunnerID:                                cfg.RunnerID,
		RuntimeMode:                             cfg.RuntimeMode,
		Verbose:                                 cfg.Verbose,
		RuntimeInstanceID:                       cfg.RuntimeInstanceID,
		RuntimeLogDir:                           cfg.RuntimeLogDir,
		RuntimeLogConfig:                        cfg.RuntimeLogConfig,
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
) *runtimebuild.Service {
	return runtimebuild.New(
		runtimeBuildConfigFromService(cfg),
		clock,
		baseLogger,
		func(ctx context.Context, input runtimebuild.BuildInput) (any, error) {
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
			if startupLocalModels != nil && startupLocalModels.manager != nil {
				bundleInput.prefetchedLocalModels = *startupLocalModels
				*startupLocalModels = localModelDomain{}
			}
			return buildRuntimeBundle(ctx, bundleInput)
		},
	)
}

func asRuntimeBundle(bundle any) *factoryRuntimeBundle {
	if bundle == nil {
		return nil
	}
	return bundle.(*factoryRuntimeBundle)
}
