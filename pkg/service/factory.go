// backendsizecheck:ignore-file this legacy service orchestration file remains oversized until dedicated refactor work lands.
// pkgmaintcheck:ignore-file-lines legacy runtime orchestration still spans startup, activation, and shutdown boundaries; split it in dedicated follow-up slices instead of risking behavior drift here.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/listeners"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"

	"go.uber.org/zap"
)

// SimpleDashboardRenderInput carries the low-level engine snapshot that powers
// runtime diagnostics together with the dedicated event-first render DTO used
// for dashboard session accounting.
type SimpleDashboardRenderInput struct {
	EngineState interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	RenderData  dashboardrender.SimpleDashboardRenderData
	Now         time.Time
}

// SimpleDashboardRenderer is a callback that formats and prints dashboard
// output. Callers (e.g. CLI) provide their own rendering implementation.
type SimpleDashboardRenderer func(input SimpleDashboardRenderInput)

// APIServerStarter is a callback that starts an API server for the factory.
// It receives the API surface, port, and logger, and should block until ctx is
// cancelled. Callers (e.g. CLI) provide their own implementation to avoid
// import cycles between service and api packages.
type APIServerStarter func(ctx context.Context, runtime apisurface.APISurface, port int, logger *zap.Logger) error

type secretResolver func(ctx context.Context, runtimeCfg interfaces.RuntimeConfigLookup, secretRef string) (string, error)

// ErrFactoryActivationRequiresIdle reports that runtime replacement was
// attempted while the current runtime still had active work.
var ErrFactoryActivationRequiresIdle = apisurface.ErrFactoryActivationRequiresIdle

// ErrInvalidNamedFactoryName reports that the requested named-factory name is
// not a safe canonical layout segment.
var ErrInvalidNamedFactoryName = apisurface.ErrInvalidNamedFactoryName

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be persisted or validated as a runnable runtime config.
var ErrInvalidNamedFactory = apisurface.ErrInvalidNamedFactory

// ErrCurrentNamedFactoryNotFound reports that no durable current-factory
// pointer could be resolved for named-factory readback.
var ErrCurrentNamedFactoryNotFound = apisurface.ErrCurrentNamedFactoryNotFound

type replacementFactoryRuntime struct {
	dir            string
	folderPath     string
	eventHistory   *factory.FactoryEventHistory
	factory        factory.Factory
	listener       *listeners.FileWatcher
	net            *state.Net
	runtimeCfg     *factoryconfig.LoadedFactoryConfig
	modelResources *localModelResourceLimiter
	modelAssets    modelAssetPuller
	localModels    *managedLocalModelManager
	logger         *zap.Logger
	logSink        *logging.RuntimeLogSink
	recording      *replay.Recorder
	recordPath     string
}

type liveRuntimeHandle struct {
	runtime       *replacementFactoryRuntime
	runCancel     context.CancelFunc
	runDone       chan struct{}
	sidecarCancel context.CancelFunc
	sidecars      sync.WaitGroup
	runErrMu      sync.RWMutex
	runErr        error
	sidecarMu     sync.Mutex
}

type serviceRunState struct {
	ctx       context.Context
	sessionID string
	runtime   *liveRuntimeHandle
}

type runtimeBundleBuildInput struct {
	dir                   string
	folderPath            string
	cfg                   *FactoryServiceConfig
	loadedFactoryCfg      *factoryconfig.LoadedFactoryConfig
	logger                *zap.Logger
	clock                 factory.Clock
	recordPath            string
	workflowID            string
	providerOverride      workers.Provider
	providerCommandRunner workers.CommandRunner
	commandRunnerOverride workers.CommandRunner
	additionalFactoryOpts []factory.FactoryOption
}

// FactoryService is an instantiation of a factory along with its runtime
// concerns: file watcher, dashboard, API server. It owns the full lifecycle
// so that CLI and other entry points remain thin wrappers.
type FactoryService struct {
	runtimeMu      sync.RWMutex
	activationMu   sync.RWMutex
	runMu          sync.RWMutex
	runState       *serviceRunState
	sessions       *liveRuntimeSessionManager
	factoryRootDir string
	factory        factory.Factory
	listener       *listeners.FileWatcher
	net            *state.Net
	cfg            *FactoryServiceConfig
	runtimeCfg     *factoryconfig.LoadedFactoryConfig
	eventHistory   *factory.FactoryEventHistory
	baseLogger     *zap.Logger
	logger         *zap.Logger
	startTime      time.Time
	clock          factory.Clock
	recording      *replay.Recorder
	logSink        *logging.RuntimeLogSink
	modelResources *localModelResourceLimiter
	modelAssets    modelAssetPuller
	localModels    *managedLocalModelManager
}

var _ factory.APIFactory = (*FactoryService)(nil)
var _ apisurface.APISurface = (*FactoryService)(nil)
var _ apisurface.SessionAPISurface = (*FactoryService)(nil)

// FactoryServiceConfig holds all parameters needed to build and run a factory.
type FactoryServiceConfig struct {
	// Dir is the factory root directory containing factory.json and inputs/.
	Dir string
	// RunnerID sets the factory-level runner override used when a workstation
	// does not declare its own runner selection.
	RunnerID string
	// ExecutionBaseDir overrides the base directory used to resolve relative
	// runtime execution paths such as workstation workingDirectory values.
	// Empty defaults to the loaded factory directory.
	ExecutionBaseDir string
	// RuntimeMode controls whether the runtime exits on idle completion or
	// stays alive until its context is canceled. Empty defaults to batch mode.
	RuntimeMode interfaces.RuntimeMode
	// Port is the REST API server port. 0 disables the API server.
	Port int
	// Logger is the structured logger. Nil uses a production default.
	Logger *zap.Logger
	// Verbose enables additional runtime diagnostic records. The runtime file
	// log remains enabled regardless of this setting.
	Verbose bool
	// RuntimeInstanceID identifies this runtime process for file-backed logs.
	// Empty generates a UUID.
	RuntimeInstanceID string
	// RuntimeLogDir optionally overrides the default ~/.agent-factory/logs
	// directory. Tests use this to keep file-backed logs isolated.
	RuntimeLogDir string
	// RuntimeLogConfig controls bounded runtime file logging behavior.
	// Zero values use defaults that match the package rolling policy.
	RuntimeLogConfig logging.RuntimeLogConfig
	// WorkFile is an optional path to a FACTORY_REQUEST_BATCH JSON file
	// containing initial work to submit when the factory starts.
	WorkFile string
	// RecordPath is an optional path where the service writes a replay artifact
	// for the current run.
	RecordPath string
	// ReplayPath is an optional path to a replay artifact whose embedded config
	// should be used instead of local factory files.
	ReplayPath string
	// WorkflowID is optional metadata recorded into replay artifacts when the
	// caller selected a specific workflow.
	WorkflowID string
	// MockWorkersConfig is the normalized mock-worker run configuration loaded
	// by the CLI when --with-mock-workers is enabled.
	MockWorkersConfig *factoryconfig.MockWorkersConfig
	// RecordFlushInterval controls how often dirty record-mode artifacts are
	// flushed during execution. Empty uses replay.DefaultRecordFlushInterval.
	RecordFlushInterval time.Duration
	// Clock is an optional runtime time source. Replay mode defaults to a
	// deterministic logical clock when no explicit clock is supplied.
	Clock factory.Clock
	// ExtraOptions are additional factory.FactoryOption values applied when
	// constructing the factory (e.g. factory.WithWorkerExecutor for tests).
	ExtraOptions []factory.FactoryOption
	// SimpleDashboardRenderer is an optional callback for rendering dashboard
	// output from the aggregate runtime snapshot and event-first world view.
	// If nil, no dashboard output is produced.
	SimpleDashboardRenderer SimpleDashboardRenderer
	// APIServerStarter is an optional callback that starts an API server.
	// If nil, no API server is started.
	APIServerStarter APIServerStarter
	// ProviderOverride, when non-nil, replaces the default
	// ScriptWrapProvider for MODEL_WORKER executors. This allows tests
	// to inject a mock Provider and exercise the full worker pipeline
	// (prompt rendering, AgentExecutor, stop-token evaluation) without
	// shelling out to a real CLI tool.
	ProviderOverride workers.Provider
	// ProviderCommandRunnerOverride, when non-nil, is injected into the
	// ScriptWrapProvider used by MODEL_WORKER executors. This preserves the
	// real provider request construction while letting tests fake the CLI
	// subprocess boundary and assert command details, env, stdin, stdout,
	// stderr, and exit failures.
	ProviderCommandRunnerOverride workers.CommandRunner
	// SkipBuiltInRunnerPrerequisiteValidation disables PATH-style built-in
	// runner prerequisite checks during startup. Tests that replace execution
	// with mocks or custom executors use this to exercise service wiring
	// without requiring local AI runner binaries to exist.
	SkipBuiltInRunnerPrerequisiteValidation bool
	// WorkstationLoader, when non-nil, is consulted before falling back
	// to disk when loading workstation AGENTS.md files. Returning
	// (nil, nil) from Load signals "no config available" and the
	// workstation is skipped. Tests use this to inject workstation
	// definitions without requiring files on disk.
	WorkstationLoader factoryconfig.WorkstationLoader
	// CommandRunnerOverride, when non-nil, is injected into SCRIPT_WORKER
	// executors instead of the default ExecCommandRunner. This allows
	// tests to mock os/exec at the CommandRunner level while still
	// exercising the full ScriptExecutor pipeline (arg templates, env
	// merging, exit-code routing).
	CommandRunnerOverride workers.CommandRunner
	// HostedPollerHTTPClient, when non-nil, overrides the default HTTP client
	// used by repository-owned hosted pollers such as the built-in Linear
	// integration.
	HostedPollerHTTPClient *http.Client
	// HostedPollerSecretResolver, when non-nil, resolves hosted-worker
	// auth.secretRef values at runtime instead of using the default env/file
	// lookup behavior.
	HostedPollerSecretResolver secretResolver
	// HostedLinearEndpoint overrides the Linear GraphQL endpoint for tests.
	// Empty uses the official default endpoint.
	HostedLinearEndpoint string
	// ModelCacheDir optionally overrides the default managed local-model cache
	// directory under ~/.agent-factory/models.
	ModelCacheDir string
	// LocalModelRuntimeOverride injects a managed local-model runtime for
	// supported LOCAL model workers. Package tests use this to exercise the
	// load/invoke/reuse path without a live embedded backend.
	LocalModelRuntimeOverride localModelRuntime
}

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
		recordPath:            cfg.RecordPath,
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

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (fs *FactoryService) ActivateNamedFactory(ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return err
	}

	sessionID := defaultFactorySessionID
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		sessionID = runState.sessionID
	}
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, rootDir, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}
	return fs.activateReplacementRuntime(ctx, rootDir, name, replacement)
}

func (fs *FactoryService) buildReplacementFactoryRuntime(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (*replacementFactoryRuntime, error) {
	baseLogger := fs.baseLogger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}

	loadedFactoryCfg, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.cfg.WorkstationLoader)
	if err != nil {
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	runtimeInstanceID := uuid.NewString()
	logSink, err := logging.BuildRuntimeLogger(baseLogger, runtimeInstanceID, fs.cfg.RuntimeLogDir, fs.cfg.RuntimeLogConfig)
	if err != nil {
		return nil, fmt.Errorf("build runtime logger: %w", err)
	}
	logger := newSessionLogger(logSink.Logger(), sessionID, folderPath, loadedFactoryCfg.FactoryDir())
	runtimeBuilt := false
	defer func() {
		if !runtimeBuilt {
			_ = logSink.Close()
		}
	}()
	warnPortableBundledReplacementReport(logger, "named factory activation replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	loadedFactoryCfg.SetRuntimeBaseDir(fs.cfg.ExecutionBaseDir)
	clock := factory.EnsureClock(fs.clock)
	recordPath := sessionScopedRecordPath(fs.cfg.RecordPath, sessionID)
	replacementRuntime, err := buildRuntimeBundle(ctx, runtimeBundleBuildInput{
		dir:                   factoryDir,
		folderPath:            folderPath,
		cfg:                   fs.cfg,
		loadedFactoryCfg:      loadedFactoryCfg,
		logger:                logger,
		clock:                 clock,
		recordPath:            recordPath,
		workflowID:            fs.cfg.WorkflowID,
		providerOverride:      providerOverrideForMode(fs.cfg, nil),
		providerCommandRunner: providerCommandRunnerForMode(fs.cfg, loadedFactoryCfg),
		commandRunnerOverride: commandRunnerOverrideForMode(fs.cfg, loadedFactoryCfg, nil),
	})
	if err != nil {
		return nil, err
	}
	runtimeBuilt = true
	replacementRuntime.logSink = logSink
	return replacementRuntime, nil
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

// Run starts the file watcher, dashboard, API server, and factory engine.
// It blocks until ctx is cancelled or the factory reaches a terminal state.
// portos:func-length-exception owner=agent-factory reason=legacy-service-run-loop review=2026-07-18 removal=split-sidecar-startup-recording-and-engine-shutdown-before-next-service-run-change
func (fs *FactoryService) Run(ctx context.Context) error {
	runCtx, cancelRunSidecars := context.WithCancel(ctx)
	var sidecars sync.WaitGroup
	var currentRuntime *liveRuntimeHandle
	serviceMode := runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService

	defer func() {
		if currentRuntime != nil || fs.logSink == nil {
			return
		}
		if err := fs.logSink.Close(); err != nil {
			fs.logger.Warn("runtime log close failed", zap.Error(err))
		}
	}()
	defer func() {
		cancelRunSidecars()
		fs.clearRunState()
		sidecars.Wait()
	}()
	fs.startRunSidecars(runCtx, &sidecars, serviceMode)
	if err := fs.prepareRunInputs(ctx, serviceMode); err != nil {
		return err
	}
	currentRuntime, err := fs.startDefaultRuntime(ctx, runCtx, serviceMode)
	if err != nil {
		return err
	}
	fs.startAPIServerSidecar(runCtx, &sidecars)
	fs.logServiceStartup()

	err = fs.waitForActiveRuntime(ctx)
	currentRuntime = fs.currentLiveRuntime()
	if stopErr := fs.stopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) && err == nil {
		err = stopErr
	}
	if stopErr := fs.shutdownOtherLiveSessions(currentRuntime); stopErr != nil && err == nil {
		err = stopErr
	}
	fs.clearRunState()
	cancelRunSidecars()
	sidecars.Wait()
	// Print final dashboard.
	if fs.cfg.SimpleDashboardRenderer != nil {
		fs.renderDashboard(ctx)
	}

	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("factory run: %w", err)
	}
	return nil
}

func (fs *FactoryService) startRunSidecars(runCtx context.Context, sidecars *sync.WaitGroup, serviceMode bool) {
	if !serviceMode {
		fs.startListenerSidecar(runCtx, sidecars, fs.listener, fs.logger)
	}
	fs.startDashboardSidecar(runCtx, sidecars)
}

func (fs *FactoryService) startListenerSidecar(
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	listener *listeners.FileWatcher,
	logger *zap.Logger,
) {
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		if listener == nil {
			return
		}
		if err := listener.Watch(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("file watcher error", zap.Error(err))
		}
	}()
}

func (fs *FactoryService) startDashboardSidecar(runCtx context.Context, sidecars *sync.WaitGroup) {
	fs.startTime = fs.clock.Now()
	if fs.cfg.SimpleDashboardRenderer == nil {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		fs.dashboardLoop(runCtx)
	}()
}

func (fs *FactoryService) prepareRunInputs(ctx context.Context, serviceMode bool) error {
	if !serviceMode {
		if err := fs.preseedCurrentRuntimeInputs(ctx); err != nil {
			return err
		}
	}
	if fs.cfg.WorkFile != "" {
		if err := fs.submitWorkFile(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (fs *FactoryService) startDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	currentRuntime := fs.startLiveRuntime(runCtx, fs.currentRuntimeBundle())
	fs.registerLiveSession(defaultFactorySessionID, currentRuntime, true)
	fs.setRunState(runCtx, defaultFactorySessionID, currentRuntime)
	if err := fs.waitForLiveRuntimeStart(ctx, currentRuntime); err != nil {
		return nil, fs.handleDefaultRuntimeStartFailure(ctx, currentRuntime, err)
	}
	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runCtx, currentRuntime); err != nil {
			fs.clearRunState()
			fs.unregisterLiveSession(defaultFactorySessionID)
			_ = fs.stopLiveRuntime(currentRuntime)
			return nil, err
		}
	}
	return currentRuntime, nil
}

func (fs *FactoryService) handleDefaultRuntimeStartFailure(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	startErr error,
) error {
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

func (fs *FactoryService) startAPIServerSidecar(runCtx context.Context, sidecars *sync.WaitGroup) {
	if fs.cfg.APIServerStarter == nil || fs.cfg.Port <= 0 {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		if err := fs.cfg.APIServerStarter(runCtx, fs, fs.cfg.Port, fs.logger); err != nil {
			fs.logger.Error("API server error", zap.Error(err))
		}
	}()
}

func (fs *FactoryService) logServiceStartup() {
	runtimeLogConfig := fs.logSink.Config()
	fs.logger.Info("factory started",
		zap.String("dir", fs.cfg.Dir),
		zap.String("runtime_log_path", fs.logSink.Path()),
		zap.String("runtime_log_appender", logging.RuntimeLogAppenderZapRollingFile),
		zap.Int("runtime_log_max_size_mb", runtimeLogConfig.MaxSize),
		zap.Int("runtime_log_max_backups", runtimeLogConfig.MaxBackups),
		zap.Int("runtime_log_max_age_days", runtimeLogConfig.MaxAge),
		zap.Bool("runtime_log_compress", runtimeLogConfig.Compress),
		zap.String("runtime_env_log_channel", logging.RuntimeEnvLogChannelRecord),
		zap.String("runtime_success_command_output", logging.RuntimeSuccessCommandOutputPolicy),
		zap.String("runtime_failure_command_output", logging.RuntimeFailureCommandOutputPolicy),
		zap.String("runtime_verbose_command_output", logging.RuntimeVerboseCommandOutputPolicy),
		zap.String("record_command_diagnostics", logging.RuntimeRecordCommandDiagnosticsMode),
		zap.String("runtime_mode", string(runtimeModeOrDefault(fs.cfg.RuntimeMode))),
		zap.Bool("mock-workers", fs.cfg.MockWorkersConfig != nil),
		zap.Int("port", fs.cfg.Port),
	)
}

func (fs *FactoryService) activateReplacementRuntime(
	ctx context.Context,
	rootDir string,
	name string,
	replacement *replacementFactoryRuntime,
) error {
	runState := fs.currentRunState()
	if runState == nil || runState.runtime == nil || runState.ctx == nil {
		return fs.activateReplacementWithoutLiveRuntime(rootDir, name, replacement)
	}

	restoreCurrentSidecars := false
	serviceMode := fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		fs.stopLiveRuntimeSidecars(runState.runtime)
		restoreCurrentSidecars = true
		defer func() {
			if restoreCurrentSidecars {
				fs.restoreLiveRuntimeSidecars(runState)
			}
		}()
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}

	replacementHandle, err := fs.startReplacementRuntime(ctx, runState.ctx, replacement, serviceMode)
	if err != nil {
		return err
	}
	if err := fs.persistReplacementRuntime(rootDir, name, replacementHandle, serviceMode); err != nil {
		return err
	}
	fs.publishFactoryChangeEvent(ctx, runState.runtime, replacement)
	restoreCurrentSidecars = false
	fs.registerLiveSession(runState.sessionID, replacementHandle, true)
	fs.setRunState(runState.ctx, runState.sessionID, replacementHandle)
	if err := fs.stopLiveRuntime(runState.runtime); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior runtime shutdown failed", zap.Error(err))
	}
	return nil
}

func (fs *FactoryService) activateReplacementWithoutLiveRuntime(
	rootDir string,
	name string,
	replacement *replacementFactoryRuntime,
) error {
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		return err
	}
	fs.swapActiveRuntime(replacement)
	return nil
}

func (fs *FactoryService) startReplacementRuntime(
	ctx context.Context,
	runCtx context.Context,
	replacement *replacementFactoryRuntime,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	replacementHandle := fs.startLiveRuntime(runCtx, replacement)
	if err := fs.waitForLiveRuntimeStart(ctx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return nil, fmt.Errorf("start replacement runtime: %w", err)
	}
	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runCtx, replacementHandle); err != nil {
			_ = fs.stopLiveRuntime(replacementHandle)
			return nil, fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}
	return replacementHandle, nil
}

func (fs *FactoryService) persistReplacementRuntime(
	rootDir string,
	name string,
	replacementHandle *liveRuntimeHandle,
	serviceMode bool,
) error {
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		if serviceMode {
			fs.stopLiveRuntimeSidecars(replacementHandle)
		}
		_ = fs.stopLiveRuntime(replacementHandle)
		return err
	}
	return nil
}

func (fs *FactoryService) requireIdleRuntime(ctx context.Context) error {
	snapshot, err := fs.GetEngineStateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("read current runtime status: %w", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		return fmt.Errorf("%w: current runtime status is %s", ErrFactoryActivationRequiresIdle, snapshot.RuntimeStatus)
	}
	if snapshotHasActiveWork(snapshot) {
		return fmt.Errorf("%w: current runtime has active work", ErrFactoryActivationRequiresIdle)
	}
	return nil
}

func snapshotHasActiveWork(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 {
		return true
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if snapshot.Topology == nil {
			return true
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}
	return false
}

func (fs *FactoryService) currentRuntimeBundle() *replacementFactoryRuntime {
	if fs == nil {
		return nil
	}
	if currentSession := fs.currentSession(); currentSession != nil && currentSession.handle != nil {
		return currentSession.handle.runtime
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	if fs.factory == nil {
		return nil
	}
	return &replacementFactoryRuntime{
		dir:            fs.cfg.Dir,
		folderPath:     fs.factoryRootDir,
		eventHistory:   fs.eventHistory,
		factory:        fs.factory,
		listener:       fs.listener,
		net:            fs.net,
		runtimeCfg:     fs.runtimeCfg,
		modelResources: fs.modelResources,
		modelAssets:    fs.modelAssets,
		localModels:    fs.localModels,
		logger:         fs.logger,
		logSink:        fs.logSink,
		recording:      fs.recording,
		recordPath:     fs.cfg.RecordPath,
	}
}

func (fs *FactoryService) publishFactoryChangeEvent(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	replacement *replacementFactoryRuntime,
) {
	if replacement == nil || replacement.eventHistory == nil {
		return
	}

	payload, ok := replacementFactoryChangePayload(replacement.eventHistory.Events())
	if !ok {
		return
	}

	eventTime := factory.EnsureClock(fs.clock).Now()
	replacement.eventHistory.RecordFactoryChange(1, payload, eventTime)

	if currentRuntime == nil || currentRuntime.runtime == nil || currentRuntime.runtime.eventHistory == nil {
		return
	}

	snapshot, err := currentRuntime.runtime.factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		fs.logger.Warn("read current runtime tick for factory-change event failed", zap.Error(err))
		return
	}
	currentRuntime.runtime.eventHistory.RecordFactoryChange(snapshot.TickCount+1, payload, eventTime)
}

func replacementFactoryChangePayload(events []factoryapi.FactoryEvent) (factoryapi.FactoryChangeEventPayload, bool) {
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			return factoryapi.FactoryChangeEventPayload{}, false
		}
		return factoryapi.FactoryChangeEventPayload{
			Factory:         payload.Factory,
			Metadata:        payload.Metadata,
			SourceDirectory: payload.SourceDirectory,
		}, true
	}
	return factoryapi.FactoryChangeEventPayload{}, false
}

func (fs *FactoryService) startLiveRuntime(ctx context.Context, runtimeBundle *replacementFactoryRuntime) *liveRuntimeHandle {
	if runtimeBundle == nil {
		return nil
	}
	runCtx, runCancel := context.WithCancel(ctx)
	handle := &liveRuntimeHandle{
		runtime:   runtimeBundle,
		runCancel: runCancel,
		runDone:   make(chan struct{}),
	}
	if runtimeBundle.recording != nil {
		runtimeBundle.recording.Start(runCtx)
		if err := runtimeBundle.recording.Flush(); err != nil {
			handle.setRunResult(err)
			return handle
		}
	}
	go func() {
		handle.setRunResult(runtimeBundle.factory.Run(runCtx))
	}()
	return handle
}

func (fs *FactoryService) startLiveRuntimeSidecars(ctx context.Context, handle *liveRuntimeHandle) error {
	if handle == nil || handle.runtime == nil {
		return fmt.Errorf("runtime handle is required")
	}

	handle.sidecarMu.Lock()
	defer handle.sidecarMu.Unlock()
	if handle.sidecarCancel != nil {
		return nil
	}

	sidecarCtx, sidecarCancel := context.WithCancel(ctx)
	handle.sidecarCancel = sidecarCancel
	if handle.runtime.listener != nil {
		handle.sidecars.Add(1)
		go func() {
			defer handle.sidecars.Done()
			if err := handle.runtime.listener.Watch(sidecarCtx); err != nil && !errors.Is(err, context.Canceled) {
				handle.runtime.runtimeLogger().Error("file watcher error", zap.Error(err))
			}
		}()
	}

	fs.startCronWatchersForRuntime(
		sidecarCtx,
		&handle.sidecars,
		handle.runtime.runtimeCfg.FactoryDir(),
		handle.runtime.runtimeCfg.FactoryConfig(),
		handle.runtime.runtimeCfg,
		submitWorkRequestWithFactory(handle.runtime.factory),
	)
	fs.startPollerWatchersForRuntime(
		sidecarCtx,
		&handle.sidecars,
		handle.runtime.runtimeCfg.FactoryConfig(),
		handle.runtime.runtimeCfg,
		submitWorkRequestWithFactory(handle.runtime.factory),
	)
	if handle.runtime.listener != nil {
		if err := handle.runtime.listener.PreseedInputs(sidecarCtx); err != nil {
			sidecarCancel()
			handle.sidecars.Wait()
			handle.sidecarCancel = nil
			return fmt.Errorf("preseed inputs: %w", err)
		}
	}
	return nil
}

func submitWorkRequestWithFactory(activeFactory factory.Factory) workRequestSubmitter {
	if activeFactory == nil {
		return nil
	}
	return func(ctx context.Context, request interfaces.WorkRequest) error {
		_, err := activeFactory.SubmitWorkRequest(ctx, request)
		return err
	}
}

func (fs *FactoryService) currentRuntimeSubmitter() workRequestSubmitter {
	return submitWorkRequestWithFactory(fs.currentFactory())
}

func (fs *FactoryService) preseedCurrentRuntimeInputs(ctx context.Context) error {
	runtimeBundle := fs.currentRuntimeBundle()
	if runtimeBundle == nil || runtimeBundle.listener == nil {
		return nil
	}
	if err := runtimeBundle.listener.PreseedInputs(ctx); err != nil {
		return fmt.Errorf("preseed inputs: %w", err)
	}
	return nil
}

func (fs *FactoryService) stopLiveRuntimeSidecars(handle *liveRuntimeHandle) {
	if handle == nil {
		return
	}
	handle.sidecarMu.Lock()
	cancel := handle.sidecarCancel
	handle.sidecarCancel = nil
	handle.sidecarMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	handle.sidecars.Wait()
}

func (fs *FactoryService) restoreLiveRuntimeSidecars(runState *serviceRunState) {
	if runState == nil || runState.ctx == nil || runState.runtime == nil {
		return
	}
	if err := fs.startLiveRuntimeSidecars(runState.ctx, runState.runtime); err != nil {
		fs.logger.Error("restore prior runtime sidecars failed", zap.Error(err))
	}
}

func (fs *FactoryService) stopLiveRuntime(handle *liveRuntimeHandle) error {
	if handle == nil {
		return nil
	}
	fs.stopLiveRuntimeSidecars(handle)
	if handle.runCancel != nil {
		handle.runCancel()
	}
	return errors.Join(handle.wait(), fs.finalizeRuntimeArtifacts(handle.runtime))
}

func (fs *FactoryService) shutdownOtherLiveSessions(except *liveRuntimeHandle) error {
	if fs == nil || fs.sessions == nil {
		return nil
	}
	var errs []error
	for _, sessionID := range fs.sessions.ids() {
		session := fs.sessionByID(sessionID)
		if session == nil {
			continue
		}
		if session.handle == except {
			continue
		}
		if session.handle != nil {
			if err := fs.stopLiveRuntime(session.handle); err != nil && !errors.Is(err, context.Canceled) {
				errs = append(errs, err)
			}
		}
		fs.unregisterLiveSession(sessionID)
	}
	return errors.Join(errs...)
}

func (fs *FactoryService) waitForLiveRuntimeStart(ctx context.Context, handle *liveRuntimeHandle) error {
	if handle == nil || handle.runtime == nil {
		return fmt.Errorf("runtime handle is required")
	}

	startCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startCtx.Done():
			if handle.completed() {
				return handle.result()
			}
			return startCtx.Err()
		case <-handle.runDone:
			return handle.result()
		case <-ticker.C:
			snap, err := handle.runtime.factory.GetEngineStateSnapshot(context.Background())
			if err != nil {
				continue
			}
			if snap.FactoryState == string(interfaces.FactoryStateRunning) {
				return nil
			}
		}
	}
}

func isCanceledServiceStartup(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(err, context.Canceled)
}

func (fs *FactoryService) waitForActiveRuntime(ctx context.Context) error {
	for {
		handle := fs.currentLiveRuntime()
		if handle == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(25 * time.Millisecond):
				continue
			}
		}
		select {
		case <-ctx.Done():
			_ = handle.wait()
		case <-handle.runDone:
		}
		if fs.currentLiveRuntime() != handle {
			continue
		}
		return handle.result()
	}
}

func (fs *FactoryService) swapActiveRuntime(runtimeBundle *replacementFactoryRuntime) {
	if runtimeBundle == nil {
		fs.clearActiveRuntime()
		return
	}
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.eventHistory = runtimeBundle.eventHistory
	fs.factory = runtimeBundle.factory
	fs.listener = runtimeBundle.listener
	fs.net = runtimeBundle.net
	fs.runtimeCfg = runtimeBundle.runtimeCfg
	fs.modelResources = runtimeBundle.modelResources
	fs.modelAssets = runtimeBundle.modelAssets
	fs.localModels = runtimeBundle.localModels
	fs.cfg.Dir = runtimeBundle.dir
}

func (fs *FactoryService) clearActiveRuntime() {
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.eventHistory = nil
	fs.factory = nil
	fs.listener = nil
	fs.net = nil
	fs.runtimeCfg = nil
	fs.modelResources = nil
	fs.modelAssets = nil
	fs.localModels = nil
	if fs.cfg != nil && strings.TrimSpace(fs.factoryRootDir) != "" {
		fs.cfg.Dir = fs.factoryRootDir
	}
}

func (fs *FactoryService) currentRunState() *serviceRunState {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	return fs.runState
}

func (fs *FactoryService) currentLiveRuntime() *liveRuntimeHandle {
	fs.runMu.RLock()
	defer fs.runMu.RUnlock()
	if fs.runState == nil {
		return nil
	}
	return fs.runState.runtime
}

func (fs *FactoryService) setRunState(ctx context.Context, sessionID string, runtime *liveRuntimeHandle) {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	if ctx == nil {
		fs.runState = nil
		return
	}
	fs.runState = &serviceRunState{
		ctx:       ctx,
		sessionID: sessionID,
		runtime:   runtime,
	}
}

func (fs *FactoryService) clearRunState() {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	fs.runState = nil
}

func (h *liveRuntimeHandle) completed() bool {
	if h == nil {
		return true
	}
	select {
	case <-h.runDone:
		return true
	default:
		return false
	}
}

func (h *liveRuntimeHandle) result() error {
	if h == nil {
		return nil
	}
	h.runErrMu.RLock()
	defer h.runErrMu.RUnlock()
	return h.runErr
}

func (h *liveRuntimeHandle) setRunResult(err error) {
	h.runErrMu.Lock()
	h.runErr = err
	h.runErrMu.Unlock()
	close(h.runDone)
}

func (h *liveRuntimeHandle) wait() error {
	if h == nil {
		return nil
	}
	<-h.runDone
	return h.result()
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

func (fs *FactoryService) finalizeRuntimeArtifacts(runtimeBundle *replacementFactoryRuntime) error {
	if runtimeBundle == nil {
		return nil
	}
	var errs []error
	if runtimeBundle.recording != nil {
		runtimeBundle.recording.Finish(factory.EnsureClock(fs.clock).Now().UTC())
		if err := runtimeBundle.recording.Flush(); err != nil {
			errs = append(errs, err)
		}
		if err := runtimeBundle.recording.Err(); err != nil {
			errs = append(errs, err)
		}
	}
	if runtimeBundle.logSink != nil {
		if err := runtimeBundle.logSink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

func sessionScopedRecordPath(basePath string, sessionID string) string {
	if strings.TrimSpace(basePath) == "" || sessionID == defaultFactorySessionID {
		return basePath
	}
	ext := filepath.Ext(basePath)
	base := strings.TrimSuffix(basePath, ext)
	return base + "." + sessionID + ext
}

func (r *replacementFactoryRuntime) runtimeLogger() *zap.Logger {
	if r == nil || r.logger == nil {
		return zap.NewNop()
	}
	return r.logger
}

func runtimeModeOrDefault(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}

// SubmitWorkRequest submits a canonical work request batch to the factory.
func (fs *FactoryService) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	fs.activationMu.RLock()
	defer fs.activationMu.RUnlock()

	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("factory service runtime is not available")
	}
	return activeFactory.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical factory event history followed by
// live events from the current service-owned runtime.
func (fs *FactoryService) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	stream, err := activeFactory.SubscribeFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe factory events: %w", err)
	}
	return stream, nil
}

func workTypeNames(workTypes *[]factoryapi.WorkType) []string {
	if workTypes == nil {
		return nil
	}
	names := make([]string, 0, len(*workTypes))
	for _, workType := range *workTypes {
		names = append(names, workType.Name)
	}
	return names
}

func workerNames(workers *[]factoryapi.Worker) []string {
	if workers == nil {
		return nil
	}
	names := make([]string, 0, len(*workers))
	for _, worker := range *workers {
		names = append(names, worker.Name)
	}
	return names
}

func resourceNames(resources *[]factoryapi.Resource) []string {
	if resources == nil {
		return nil
	}
	names := make([]string, 0, len(*resources))
	for _, resource := range *resources {
		names = append(names, resource.Name)
	}
	return names
}

func workstationNames(workstations *[]factoryapi.Workstation) []string {
	if workstations == nil {
		return nil
	}
	names := make([]string, 0, len(*workstations))
	for _, workstation := range *workstations {
		names = append(names, workstation.Name)
	}
	return names
}

func workStateSet(workTypes *[]factoryapi.WorkType) map[string]bool {
	states := make(map[string]bool)
	if workTypes == nil {
		return states
	}
	for _, workType := range *workTypes {
		for _, state := range workType.States {
			states[workType.Name+":"+state.Name] = true
		}
	}
	return states
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func editableFactoryErrorTarget(kind, id, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: kind}
	if id != "" {
		target.Id = &id
	}
	if field != "" {
		target.Field = &field
	}
	return target
}

// WaitToComplete returns a channel that is closed when all tokens reach
// terminal or failed places and no dispatches are in flight. Delegates to
// the underlying factory's termination signal.
func (fs *FactoryService) WaitToComplete() <-chan struct{} {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return activeFactory.WaitToComplete()
}

// GetEngineStateSnapshot returns the factory boundary's aggregate
// observability snapshot.
func (fs *FactoryService) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	snap, err := activeFactory.GetEngineStateSnapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("get engine state snapshot: %w", err)
	}
	return snap, nil
}

// Pause pauses the current runtime instance.
func (fs *FactoryService) Pause(ctx context.Context) error {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if err := activeFactory.Pause(ctx); err != nil {
		return fmt.Errorf("pause factory: %w", err)
	}
	return nil
}

// GetFactoryEvents returns the canonical factory event history.
func (fs *FactoryService) GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error) {
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return nil, fmt.Errorf("factory service runtime is not available")
	}
	events, err := activeFactory.GetFactoryEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("get factory events: %w", err)
	}
	return events, nil
}

func (fs *FactoryService) submitWorkFile(ctx context.Context) error {
	data, err := os.ReadFile(fs.cfg.WorkFile)
	if err != nil {
		return fmt.Errorf("read work file %s: %w", fs.cfg.WorkFile, err)
	}
	workRequest, err := factory.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return fmt.Errorf("parse work file %s: %w", fs.cfg.WorkFile, err)
	}
	activeFactory := fs.currentFactory()
	if activeFactory == nil {
		return fmt.Errorf("factory service runtime is not available")
	}
	if _, err := activeFactory.SubmitWorkRequest(ctx, workRequest); err != nil {
		return fmt.Errorf("submit initial work: %w", err)
	}
	fs.logger.Info("submitted initial work", zap.String("file", fs.cfg.WorkFile))
	return nil
}

func (fs *FactoryService) currentFactory() factory.Factory {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.factory
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.factory
}

func (fs *FactoryService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if fs == nil {
		return nil
	}
	if compatibilitySession := fs.compatibilitySession(); compatibilitySession != nil && compatibilitySession.handle != nil && compatibilitySession.handle.runtime != nil {
		return compatibilitySession.handle.runtime.runtimeCfg
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.runtimeCfg
}

func (fs *FactoryService) compatibilitySession() *liveFactorySession {
	if fs == nil {
		return nil
	}
	return fs.defaultSession()
}

func (fs *FactoryService) workflowID() string {
	if fs == nil || fs.cfg == nil {
		return ""
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.cfg.WorkflowID
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
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
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
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, &workers.NoopExecutor{}))
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
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
	factoryRunnerID string,
	logger logging.Logger,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
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
		runner = localModels.wrapRunner(runner, runtimeCfg, factoryCfg, def)
		runner = modelResources.wrapRunner(runner, factoryCfg, def)
		runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
		agentExec := workers.NewAgentExecutorWithRunner(runtimeCfg, runner, agentOpts...)
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        agentExec,
			Renderer:        &workers.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkstationTypeLogical:
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
