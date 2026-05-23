package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/listeners"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// BuildFactoryService loads factory.json from the config directory, constructs
// the petri net, factory runtime, file watcher, and session metrics.
// portos:func-length-exception owner=agent-factory reason=legacy-service-wiring review=2026-07-18 removal=split-replay-recording-worker-and-listener-builders-before-next-service-wiring-change
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	factoryRootDir, baseLogger, logSink, logger, err := buildPrimaryServiceLogger(cfg)
	if err != nil {
		return nil, err
	}
	serviceBuilt := false
	defer func() {
		if !serviceBuilt {
			_ = logSink.Close()
		}
	}()
	if cfg.ReplayPath == "" {
		resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		cfg.Dir = resolvedDir
	}

	logger = newSessionLogger(logger, defaultFactorySessionID, factoryRootDir, cfg.Dir)
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForService(cfg, logger)
	if err != nil {
		return nil, err
	}
	clock := serviceClockForMode(cfg.Clock, replayArtifact)
	replaySideEffects, replayFactoryOpts, err := replayFactoryModeOptions(replayArtifact)
	if err != nil {
		return nil, err
	}
	runtimeBundle, err := buildRuntimeBundle(ctx, runtimeBundleBuildInput{
		dir:                   cfg.Dir,
		folderPath:            factoryRootDir,
		cfg:                   cfg,
		loadedFactoryCfg:      loadedFactoryCfg,
		logger:                logger,
		clock:                 clock,
		recordPath:            sessionScopedRecordPath(cfg.RecordPath, defaultFactorySessionID),
		workflowID:            cfg.WorkflowID,
		providerOverride:      providerOverrideForMode(cfg, replaySideEffects),
		providerCommandRunner: providerCommandRunnerForMode(cfg, loadedFactoryCfg),
		commandRunnerOverride: commandRunnerOverrideForMode(cfg, loadedFactoryCfg, replaySideEffects),
		additionalFactoryOpts: replayFactoryOpts,
	})
	if err != nil {
		return nil, err
	}

	serviceBuilt = true
	return &FactoryService{
		factoryRootDir: factoryRootDir,
		sessions:       newLiveRuntimeSessionManager(),
		eventHistory:   runtimeBundle.eventHistory,
		factory:        runtimeBundle.factory,
		listener:       runtimeBundle.listener,
		net:            runtimeBundle.net,
		cfg:            cfg,
		runtimeCfg:     runtimeBundle.runtimeCfg,
		modelResources: runtimeBundle.modelResources,
		modelAssets:    runtimeBundle.modelAssets,
		localModels:    runtimeBundle.localModels,
		baseLogger:     baseLogger,
		logger:         logger,
		clock:          clock,
		recording:      runtimeBundle.recording,
		logSink:        logSink,
	}, nil
}

func buildPrimaryServiceLogger(cfg *FactoryServiceConfig) (string, *zap.Logger, *logging.RuntimeLogSink, *zap.Logger, error) {
	factoryRootDir := cfg.Dir
	baseLogger := cfg.Logger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	runtimeInstanceID := cfg.RuntimeInstanceID
	if runtimeInstanceID == "" {
		runtimeInstanceID = uuid.NewString()
	}
	logSink, err := logging.BuildRuntimeLogger(baseLogger, runtimeInstanceID, cfg.RuntimeLogDir, cfg.RuntimeLogConfig)
	if err != nil {
		return "", nil, nil, nil, err
	}
	cfg.RuntimeInstanceID = runtimeInstanceID
	cfg.Logger = baseLogger
	return factoryRootDir, baseLogger, logSink, logSink.Logger(), nil
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
	warnPortableBundledReplacementReport(logger, "runtime config load replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
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
	replayDeliveryPlan, err := replay.NewCompletionDeliveryPlan(replayArtifact)
	if err != nil {
		return nil, nil, fmt.Errorf("build replay completion delivery plan: %w", err)
	}
	return replaySideEffects, []factory.FactoryOption{
		factory.WithSubmissionHook(replaySubmissionHook),
		factory.WithCompletionDeliveryPlanner(replayDeliveryPlan),
	}, nil
}

func buildRuntimeBundle(
	ctx context.Context,
	input runtimeBundleBuildInput,
) (*replacementFactoryRuntime, error) {
	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, input.loadedFactoryCfg.FactoryConfig())
	if err != nil {
		input.logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	effectiveFactoryRunnerID := effectiveFactoryRunnerID(input.cfg.RunnerID, input.loadedFactoryCfg.FactoryConfig())
	eventHistory := factory.NewFactoryEventHistory(net, input.clock.Now, input.loadedFactoryCfg)
	eventHistory.SetFactoryRunnerOverride(effectiveFactoryRunnerID)
	modelResources, modelAssets, localModels := newRuntimeLocalModelDependencies(input.cfg)
	workerOpts, err := loadWorkersFromConfig(
		input.loadedFactoryCfg.FactoryDir(),
		input.loadedFactoryCfg.FactoryConfig(),
		effectiveFactoryRunnerID,
		input.loadedFactoryCfg,
		logging.NewZapLogger(input.logger, input.cfg.Verbose),
		input.cfg.SkipBuiltInRunnerPrerequisiteValidation,
		input.providerOverride,
		input.providerCommandRunner,
		input.commandRunnerOverride,
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		eventHistory.RecordModelEvent,
		input.clock.Now,
		modelResources,
		localModels,
	)
	if err != nil {
		input.logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}

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
		factory.WithLogger(logging.NewZapLogger(input.logger, input.cfg.Verbose)),
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
	listener, err := buildRuntimeListener(input.dir, activeFactory, input.logger, net)
	if err != nil {
		return nil, err
	}

	return &replacementFactoryRuntime{
		dir:            input.dir,
		folderPath:     input.folderPath,
		eventHistory:   eventHistory,
		factory:        activeFactory,
		listener:       listener,
		net:            net,
		runtimeCfg:     input.loadedFactoryCfg,
		modelResources: modelResources,
		modelAssets:    modelAssets,
		localModels:    localModels,
		logger:         input.logger,
		recording:      recording,
		recordPath:     input.recordPath,
	}, nil
}

func newRuntimeLocalModelDependencies(cfg *FactoryServiceConfig) (*localModelResourceLimiter, modelAssetPuller, *managedLocalModelManager) {
	modelResources := newLocalModelResourceLimiter()
	modelAssets := newHuggingFaceModelAssetPuller(strings.TrimSpace(cfg.ModelCacheDir))
	localModelRuntime := cfg.LocalModelRuntimeOverride
	if localModelRuntime == nil {
		localModelRuntime = newOmniVoiceLocalRuntime(nil)
	}
	return modelResources, modelAssets, newManagedLocalModelManager(modelAssets, localModelRuntime)
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
		activeFactory,
		logger,
		listeners.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)),
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
