package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/listeners"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"

	"go.uber.org/zap"
)

type preparedServiceRuntime struct {
	loadedFactoryCfg     *factoryconfig.LoadedFactoryConfig
	clock                factory.Clock
	replaySideEffects    *replay.SideEffects
	replaySubmissionHook *replay.SubmissionHook
	replayDeliveryPlan   *replay.CompletionDeliveryPlan
	net                  *state.Net
	eventHistory         *factory.FactoryEventHistory
	workerOpts           []factory.FactoryOption
	recording            *replay.Recorder
}

// BuildFactoryService loads factory.json from the config directory, constructs
// the petri net, factory runtime, file watcher, and session metrics.
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}

	factoryRootDir := cfg.Dir
	logSink, err := prepareServiceLogger(cfg)
	if err != nil {
		return nil, err
	}

	serviceBuilt := false
	defer func() {
		if !serviceBuilt {
			_ = logSink.Close()
		}
	}()

	prepared, err := prepareServiceRuntime(ctx, cfg)
	if err != nil {
		return nil, err
	}
	builtFactory, err := newPreparedFactory(cfg, prepared)
	if err != nil {
		return nil, err
	}
	listener, err := ensureInputsListener(cfg.Dir, prepared.net, builtFactory, cfg.Logger)
	if err != nil {
		return nil, err
	}

	serviceBuilt = true
	return &FactoryService{
		factoryRootDir: factoryRootDir,
		eventHistory:   prepared.eventHistory,
		factory:        builtFactory,
		listener:       listener,
		net:            prepared.net,
		cfg:            cfg,
		runtimeCfg:     prepared.loadedFactoryCfg,
		logger:         cfg.Logger,
		clock:          prepared.clock,
		recording:      prepared.recording,
		logSink:        logSink,
	}, nil
}

func prepareServiceLogger(cfg *FactoryServiceConfig) (*logging.RuntimeLogSink, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	runtimeInstanceID := cfg.RuntimeInstanceID
	if runtimeInstanceID == "" {
		runtimeInstanceID = uuid.NewString()
	}
	logSink, err := logging.BuildRuntimeLogger(logger, runtimeInstanceID, cfg.RuntimeLogDir, cfg.RuntimeLogConfig)
	if err != nil {
		return nil, err
	}
	cfg.RuntimeInstanceID = runtimeInstanceID
	cfg.Logger = logSink.Logger()
	return logSink, nil
}

func prepareServiceRuntime(ctx context.Context, cfg *FactoryServiceConfig) (*preparedServiceRuntime, error) {
	if err := resolveServiceFactoryDir(cfg); err != nil {
		return nil, err
	}

	cfg.Logger.Info("loading factory config", zap.String("dir", cfg.Dir))
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForMode(cfg)
	if err != nil {
		cfg.Logger.Error("failed to load factory config", zap.Error(err))
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	warnPortableBundledReplacementReport(cfg.Logger, "runtime config load replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	warnReplayMetadataMismatches(cfg, replayArtifact, cfg.Logger)

	clock := resolveServiceClock(cfg.Clock, replayArtifact)
	replaySideEffects, replaySubmissionHook, replayDeliveryPlan, err := buildReplayRuntimeHelpers(replayArtifact)
	if err != nil {
		return nil, err
	}
	net, eventHistory, workerOpts, err := buildPreparedRuntimeGraph(ctx, cfg, loadedFactoryCfg, clock, replaySideEffects)
	if err != nil {
		return nil, err
	}
	recording, err := buildPreparedRecorder(cfg, loadedFactoryCfg, clock)
	if err != nil {
		return nil, err
	}

	return &preparedServiceRuntime{
		loadedFactoryCfg:     loadedFactoryCfg,
		clock:                clock,
		replaySideEffects:    replaySideEffects,
		replaySubmissionHook: replaySubmissionHook,
		replayDeliveryPlan:   replayDeliveryPlan,
		net:                  net,
		eventHistory:         eventHistory,
		workerOpts:           workerOpts,
		recording:            recording,
	}, nil
}

func resolveServiceFactoryDir(cfg *FactoryServiceConfig) error {
	if cfg.ReplayPath != "" {
		return nil
	}
	resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
	if err != nil {
		return fmt.Errorf("resolve factory dir: %w", err)
	}
	cfg.Dir = resolvedDir
	return nil
}

func resolveServiceClock(clock factory.Clock, replayArtifact *interfaces.ReplayArtifact) factory.Clock {
	if clock == nil && replayArtifact != nil {
		clock = replay.NewArtifactClock(replayArtifact)
	}
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	return factory.EnsureClock(clock)
}

func buildReplayRuntimeHelpers(replayArtifact *interfaces.ReplayArtifact) (*replay.SideEffects, *replay.SubmissionHook, *replay.CompletionDeliveryPlan, error) {
	if replayArtifact == nil {
		return nil, nil, nil, nil
	}

	replaySideEffects, err := replay.NewSideEffects(replayArtifact)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build replay side effects: %w", err)
	}
	replaySubmissionHook, err := replay.NewSubmissionHook(replayArtifact)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build replay submission hook: %w", err)
	}
	replayDeliveryPlan, err := replay.NewCompletionDeliveryPlan(replayArtifact)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}

	return replaySideEffects, replaySubmissionHook, replayDeliveryPlan, nil
}

func buildPreparedRuntimeGraph(
	ctx context.Context,
	cfg *FactoryServiceConfig,
	loadedFactoryCfg *factoryconfig.LoadedFactoryConfig,
	clock factory.Clock,
	replaySideEffects *replay.SideEffects,
) (*state.Net, *factory.FactoryEventHistory, []factory.FactoryOption, error) {
	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, loadedFactoryCfg.FactoryConfig())
	if err != nil {
		cfg.Logger.Error("failed to map factory config", zap.Error(err))
		return nil, nil, nil, fmt.Errorf("map factory config: %w", err)
	}

	eventHistory := factory.NewFactoryEventHistory(net, clock.Now, loadedFactoryCfg)
	effectiveRunnerID := effectiveFactoryRunnerID(cfg.RunnerID, loadedFactoryCfg.FactoryConfig())
	eventHistory.SetFactoryRunnerOverride(effectiveRunnerID)
	workerOpts, err := loadWorkersFromConfig(
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		effectiveRunnerID,
		loadedFactoryCfg,
		logging.NewZapLogger(cfg.Logger, cfg.Verbose),
		cfg.SkipBuiltInRunnerPrerequisiteValidation,
		providerOverrideForMode(cfg, replaySideEffects),
		providerCommandRunnerForMode(cfg, loadedFactoryCfg),
		commandRunnerOverrideForMode(cfg, loadedFactoryCfg, replaySideEffects),
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		clock.Now,
	)
	if err != nil {
		cfg.Logger.Error("failed to load workers from config", zap.Error(err))
		return nil, nil, nil, fmt.Errorf("load workers: %w", err)
	}

	return net, eventHistory, workerOpts, nil
}

func buildPreparedRecorder(
	cfg *FactoryServiceConfig,
	loadedFactoryCfg *factoryconfig.LoadedFactoryConfig,
	clock factory.Clock,
) (*replay.Recorder, error) {
	recordingArtifact, err := newRecordingArtifact(
		cfg,
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		loadedFactoryCfg,
		clock,
	)
	if err != nil || recordingArtifact == nil {
		return nil, err
	}

	recording, err := replay.NewRecorder(
		cfg.RecordPath,
		recordingArtifact,
		replay.WithFlushInterval(cfg.RecordFlushInterval),
	)
	if err != nil {
		return nil, fmt.Errorf("create replay recorder: %w", err)
	}
	return recording, nil
}

func newPreparedFactory(cfg *FactoryServiceConfig, prepared *preparedServiceRuntime) (factory.Factory, error) {
	opts := []factory.FactoryOption{
		factory.WithNet(prepared.net),
		factory.WithRuntimeMode(cfg.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(cfg.Logger, cfg.Verbose)),
		factory.WithRuntimeConfig(prepared.loadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(prepared.loadedFactoryCfg.FactoryConfig())),
		factory.WithClock(prepared.clock),
		factory.WithFactoryEventHistory(prepared.eventHistory),
	}
	if cfg.RecordPath != "" {
		opts = append(opts, factory.WithFactoryEventRecorder(func(event factoryapi.FactoryEvent) {
			if prepared.recording != nil {
				prepared.recording.RecordEvent(event)
			}
		}))
	}
	if prepared.replaySubmissionHook != nil {
		opts = append(opts, factory.WithSubmissionHook(prepared.replaySubmissionHook))
	}
	if prepared.replayDeliveryPlan != nil {
		opts = append(opts, factory.WithCompletionDeliveryPlanner(prepared.replayDeliveryPlan))
	}
	opts = append(opts, prepared.workerOpts...)
	opts = append(opts, cfg.ExtraOptions...)

	builtFactory, err := runtime.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}
	return builtFactory, nil
}

func ensureInputsListener(
	factoryDir string,
	net *state.Net,
	builtFactory factory.Factory,
	logger *zap.Logger,
) (*listeners.FileWatcher, error) {
	inputsDir := filepath.Join(factoryDir, interfaces.InputsDir)
	if !dirExists(inputsDir) {
		if err := os.MkdirAll(inputsDir, 0o755); err != nil {
			return nil, fmt.Errorf("create inputs dir: %w", err)
		}
	} else {
		logger.Info("using inputs/ directory", zap.String("dir", inputsDir))
	}

	return listeners.NewFileWatcher(
		inputsDir,
		builtFactory,
		logger,
		listeners.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)),
	), nil
}

func (fs *FactoryService) buildReplacementFactoryRuntime(ctx context.Context, factoryDir string) (*replacementFactoryRuntime, error) {
	logger := fs.logger
	if logger == nil {
		logger = zap.NewNop()
	}

	loadedFactoryCfg, err := factoryconfig.LoadRuntimeConfig(factoryDir, fs.cfg.WorkstationLoader)
	if err != nil {
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	warnPortableBundledReplacementReport(logger, "named factory activation replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	loadedFactoryCfg.SetRuntimeBaseDir(fs.cfg.ExecutionBaseDir)

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, loadedFactoryCfg.FactoryConfig())
	if err != nil {
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	clock := fs.clock
	if clock == nil {
		clock = factory.EnsureClock(clockwork.NewRealClock())
	}
	eventHistory := factory.NewFactoryEventHistory(net, clock.Now, loadedFactoryCfg)
	effectiveRunnerID := effectiveFactoryRunnerID(fs.cfg.RunnerID, loadedFactoryCfg.FactoryConfig())
	eventHistory.SetFactoryRunnerOverride(effectiveRunnerID)
	workerOpts, err := loadWorkersFromConfig(
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		effectiveRunnerID,
		loadedFactoryCfg,
		logging.NewZapLogger(logger, fs.cfg.Verbose),
		fs.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		providerOverrideForMode(fs.cfg, nil),
		providerCommandRunnerForMode(fs.cfg, loadedFactoryCfg),
		commandRunnerOverrideForMode(fs.cfg, loadedFactoryCfg, nil),
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		clock.Now,
	)
	if err != nil {
		return nil, fmt.Errorf("load workers: %w", err)
	}

	opts := []factory.FactoryOption{
		factory.WithNet(net),
		factory.WithRuntimeMode(fs.cfg.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(logger, fs.cfg.Verbose)),
		factory.WithRuntimeConfig(loadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(loadedFactoryCfg.FactoryConfig())),
		factory.WithClock(clock),
		factory.WithFactoryEventHistory(eventHistory),
	}
	opts = append(opts, workerOpts...)
	opts = append(opts, fs.cfg.ExtraOptions...)

	replacementFactory, err := runtime.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}

	replacementListener, err := ensureInputsListener(factoryDir, net, replacementFactory, logger)
	if err != nil {
		return nil, err
	}

	return &replacementFactoryRuntime{
		dir:          factoryDir,
		eventHistory: eventHistory,
		factory:      replacementFactory,
		listener:     replacementListener,
		net:          net,
		runtimeCfg:   loadedFactoryCfg,
	}, nil
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

func validateReplayModeConfig(cfg *FactoryServiceConfig) error {
	if cfg == nil {
		return fmt.Errorf("factory service config is required")
	}
	if cfg.RecordPath != "" && cfg.ReplayPath != "" {
		return fmt.Errorf("--record and --replay cannot be used together")
	}
	return nil
}

func loadFactoryConfigForMode(cfg *FactoryServiceConfig) (*factoryconfig.LoadedFactoryConfig, *interfaces.ReplayArtifact, error) {
	if cfg.ReplayPath == "" {
		loaded, err := factoryconfig.LoadRuntimeConfig(cfg.Dir, cfg.WorkstationLoader)
		if loaded != nil {
			loaded.SetRuntimeBaseDir(cfg.ExecutionBaseDir)
		}
		return loaded, nil, err
	}
	artifact, err := replay.Load(cfg.ReplayPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load replay artifact: %w", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(artifact.Factory)
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded replay config: %w", err)
	}
	loaded, err := factoryconfig.NewLoadedFactoryConfig(runtimeCfg.FactoryDir(), runtimeCfg.Factory, runtimeCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("build embedded replay config: %w", err)
	}
	loaded.SetRuntimeBaseDir(cfg.ExecutionBaseDir)
	return loaded, artifact, nil
}

func warnReplayMetadataMismatches(cfg *FactoryServiceConfig, artifact *interfaces.ReplayArtifact, logger *zap.Logger) {
	if artifact == nil || cfg == nil || cfg.Dir == "" {
		return
	}
	current, err := factoryconfig.LoadRuntimeConfig(cfg.Dir, cfg.WorkstationLoader)
	if err != nil {
		return
	}
	currentFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		current.FactoryConfig(),
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(cfg.WorkflowID),
	)
	if err != nil {
		return
	}
	for _, warning := range replay.FactoryMetadataWarnings(artifact.Factory, currentFactory) {
		logger.Warn("replay artifact metadata differs from current checkout",
			zap.String("category", replay.DivergenceCategoryConfigMismatch),
			zap.String("metadata_key", warning.Key),
			zap.String("artifact", warning.Artifact),
			zap.String("current", warning.Current),
		)
	}
}

func warnPortableBundledReplacementReport(
	logger *zap.Logger,
	message string,
	replacements []factoryconfig.PortableBundledFileReplacement,
) {
	if logger == nil || len(replacements) == 0 {
		return
	}
	targets := make([]string, 0, len(replacements))
	for _, replacement := range replacements {
		targets = append(targets, replacement.TargetPath)
	}
	logger.Warn(message, zap.Strings("target_paths", targets))
}

func runtimeWorkflowContext(cfg *interfaces.FactoryConfig) *factory_context.FactoryContext {
	projectID := factory_context.DefaultProjectID
	if cfg != nil && cfg.Project != "" {
		projectID = factory_context.ResolveProjectID(cfg.Project, nil, nil)
	}
	return &factory_context.FactoryContext{
		ProjectID: projectID,
		EnvVars:   make(map[string]string),
	}
}

func newRecordingArtifact(
	cfg *FactoryServiceConfig,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeDefinitionLookup,
	clock factory.Clock,
) (*interfaces.ReplayArtifact, error) {
	if cfg.RecordPath == "" {
		return nil, nil
	}
	now := factory.EnsureClock(clock).Now().UTC()
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		factoryDir,
		factoryCfg,
		runtimeCfg,
		replay.WithGeneratedFactorySourceDirectory(factoryDir),
		replay.WithGeneratedFactoryWorkflowID(cfg.WorkflowID),
	)
	if err != nil {
		return nil, fmt.Errorf("build replay artifact config: %w", err)
	}
	return replay.NewEventLogArtifactFromFactory(now, generatedFactory, &interfaces.ReplayWallClockMetadata{
		StartedAt: now,
	}, interfaces.ReplayDiagnostics{})
}

func (fs *FactoryService) writeRecording() error {
	if fs.recording == nil {
		return nil
	}
	return fs.recording.Flush()
}

func runtimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
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
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	now func() time.Time,
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
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, &workers.NoopExecutor{}))
			continue
		}
		executor := buildWorkerExecutor(runtimeCfg, workerCfg.Name, factoryRunnerID, logger, providerOverride, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, now)
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
		opts = append(opts, factory.WithWorkerExecutor(workstationCfg.Name, &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Renderer:        &workers.DefaultPromptRenderer{},
			Logger:          logger,
		}))
	}
	return opts, nil
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workerName string,
	factoryRunnerID string,
	logger logging.Logger,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	now func() time.Time,
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
			var providerOpts []workers.ScriptWrapProviderOption
			providerOpts = append(providerOpts, workers.WithSkipPermissions(def.SkipPermissions))
			providerOpts = append(providerOpts, workers.WithProviderLogger(logger))
			if providerCommandRunner != nil {
				providerOpts = append(providerOpts, workers.WithProviderCommandRunner(providerCommandRunner))
			}
			runner = workers.NewScriptWrapProvider(providerOpts...)
		}
		if inferenceRecorder != nil {
			if providerOverride != nil {
				provider := workers.NewRecordingProvider(
					providerOverride,
					inferenceRecorder,
					workers.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			} else if providerRunner, ok := runner.(*workers.ScriptWrapProvider); ok {
				provider := workers.NewRecordingProvider(
					providerRunner,
					inferenceRecorder,
					workers.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			}
		}

		agentOpts := []workers.AgentExecutorOption{
			workers.WithLogger(logger),
		}
		agentExec := workers.NewAgentExecutorWithRunner(runtimeCfg, runner, agentOpts...)
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        agentExec,
			Renderer:        &workers.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkstationTypeLogical:
		// LOGICAL_MOVE workers pass input token colors through without calling any LLM.
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Renderer:        &workers.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkerTypeScript:
		var scriptOpts []workers.ScriptExecutorOption
		if runtimeCfg != nil && runtimeCfg.FactoryDir() != "" {
			scriptOpts = append(scriptOpts, workers.WithScriptFactoryDir(runtimeCfg.FactoryDir()))
		}
		if scriptRecorder != nil {
			scriptOpts = append(scriptOpts, workers.WithScriptEventRecorder(scriptRecorder))
		}
		var scriptExec workers.WorkstationRequestExecutor
		if cmdRunner != nil {
			scriptExec = workers.NewScriptExecutorWithRunner(def, cmdRunner, logger, scriptOpts...)
		} else {
			scriptExec = workers.NewScriptExecutor(def, logger, scriptOpts...)
		}
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        scriptExec,
			Renderer:        &workers.DefaultPromptRenderer{},
			Logger:          logger,
		}
	default:
		return nil
	}
}
