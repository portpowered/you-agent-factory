package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/apisurface"
	"github.com/portpowered/agent-factory/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory"
	factory_context "github.com/portpowered/agent-factory/pkg/factory/context"
	"github.com/portpowered/agent-factory/pkg/factory/projections"
	"github.com/portpowered/agent-factory/pkg/factory/runtime"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/listeners"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/workers"

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
	dir        string
	factory    factory.Factory
	listener   *listeners.FileWatcher
	net        *state.Net
	runtimeCfg *factoryconfig.LoadedFactoryConfig
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
	ctx     context.Context
	runtime *liveRuntimeHandle
}

// FactoryService is an instantiation of a factory along with its runtime
// concerns: file watcher, dashboard, API server. It owns the full lifecycle
// so that CLI and other entry points remain thin wrappers.
type FactoryService struct {
	runtimeMu      sync.RWMutex
	activationMu   sync.RWMutex
	runMu          sync.RWMutex
	runState       *serviceRunState
	factoryRootDir string
	factory        factory.Factory
	listener       *listeners.FileWatcher
	net            *state.Net
	cfg            *FactoryServiceConfig
	runtimeCfg     *factoryconfig.LoadedFactoryConfig
	logger         *zap.Logger
	startTime      time.Time
	clock          factory.Clock
	recording      *replay.Recorder
	logSink        *logging.RuntimeLogSink
}

var _ apisurface.APISurface = (*FactoryService)(nil)

// FactoryServiceConfig holds all parameters needed to build and run a factory.
type FactoryServiceConfig struct {
	// Dir is the factory root directory containing factory.json and inputs/.
	Dir string
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
}

// BuildFactoryService loads factory.json from the config directory, constructs
// the petri net, factory runtime, file watcher, and session metrics.
// portos:func-length-exception owner=agent-factory reason=legacy-service-wiring review=2026-07-18 removal=split-replay-recording-worker-and-listener-builders-before-next-service-wiring-change
func BuildFactoryService(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	factoryRootDir := cfg.Dir
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
	serviceBuilt := false
	defer func() {
		if !serviceBuilt {
			_ = logSink.Close()
		}
	}()
	logger = logSink.Logger()
	cfg.RuntimeInstanceID = runtimeInstanceID
	cfg.Logger = logger

	if cfg.ReplayPath == "" {
		resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		cfg.Dir = resolvedDir
	}

	logger.Info("loading factory config", zap.String("dir", cfg.Dir))
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForMode(cfg)
	if err != nil {
		logger.Error("failed to load factory config", zap.Error(err))
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	warnReplayMetadataMismatches(cfg, replayArtifact, logger)
	clock := cfg.Clock
	if clock == nil && replayArtifact != nil {
		clock = replay.NewArtifactClock(replayArtifact)
	}
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	clock = factory.EnsureClock(clock)
	var replaySideEffects *replay.SideEffects
	var replaySubmissionHook *replay.SubmissionHook
	var replayDeliveryPlan *replay.CompletionDeliveryPlan
	if replayArtifact != nil {
		replaySideEffects, err = replay.NewSideEffects(replayArtifact)
		if err != nil {
			return nil, fmt.Errorf("build replay side effects: %w", err)
		}
		replaySubmissionHook, err = replay.NewSubmissionHook(replayArtifact)
		if err != nil {
			return nil, fmt.Errorf("build replay submission hook: %w", err)
		}
		replayDeliveryPlan, err = replay.NewCompletionDeliveryPlan(replayArtifact)
		if err != nil {
			return nil, fmt.Errorf("build replay completion delivery plan: %w", err)
		}
	}

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, loadedFactoryCfg.FactoryConfig(), loadedFactoryCfg)
	if err != nil {
		logger.Error("failed to map factory config", zap.Error(err))
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	eventHistory := factory.NewFactoryEventHistory(net, clock.Now, loadedFactoryCfg)
	workerOpts, err := loadWorkersFromConfig(
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		loadedFactoryCfg,
		logging.NewZapLogger(logger, cfg.Verbose),
		providerOverrideForMode(cfg, replaySideEffects),
		providerCommandRunnerForMode(cfg, loadedFactoryCfg),
		commandRunnerOverrideForMode(cfg, loadedFactoryCfg, replaySideEffects),
		eventHistory.RecordScriptEvent,
		eventHistory.RecordInferenceEvent,
		clock.Now,
	)
	if err != nil {
		logger.Error("failed to load workers from config", zap.Error(err))
		return nil, fmt.Errorf("load workers: %w", err)
	}

	recordingArtifact, err := newRecordingArtifact(
		cfg,
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		loadedFactoryCfg,
		clock,
	)
	if err != nil {
		return nil, err
	}
	var recording *replay.Recorder
	if recordingArtifact != nil {
		recording, err = replay.NewRecorder(
			cfg.RecordPath,
			recordingArtifact,
			replay.WithFlushInterval(cfg.RecordFlushInterval),
		)
		if err != nil {
			return nil, fmt.Errorf("create replay recorder: %w", err)
		}
	}

	opts := []factory.FactoryOption{
		factory.WithNet(net),
		factory.WithRuntimeMode(cfg.RuntimeMode),
		factory.WithLogger(logging.NewZapLogger(logger, cfg.Verbose)),
		factory.WithRuntimeConfig(loadedFactoryCfg),
		factory.WithWorkflowContext(runtimeWorkflowContext(loadedFactoryCfg.FactoryConfig())),
		factory.WithClock(clock),
		factory.WithFactoryEventHistory(eventHistory),
	}
	if cfg.RecordPath != "" {
		opts = append(opts, factory.WithFactoryEventRecorder(func(event factoryapi.FactoryEvent) {
			if recording != nil {
				recording.RecordEvent(event)
			}
		}))
	}
	if replaySubmissionHook != nil {
		opts = append(opts, factory.WithSubmissionHook(replaySubmissionHook))
	}
	if replayDeliveryPlan != nil {
		opts = append(opts, factory.WithCompletionDeliveryPlanner(replayDeliveryPlan))
	}
	opts = append(opts, workerOpts...)
	opts = append(opts, cfg.ExtraOptions...)

	f, err := runtime.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create factory: %w", err)
	}

	// Always use the inputs/ directory.
	inputsDir := filepath.Join(cfg.Dir, interfaces.InputsDir)

	var listener *listeners.FileWatcher
	if dirExists(inputsDir) {
		listener = listeners.NewFileWatcher(inputsDir, f, logger, listeners.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)))
		logger.Info("using inputs/ directory", zap.String("dir", inputsDir))
	} else {
		// Create inputs/ for new factories.
		if err := os.MkdirAll(inputsDir, 0o755); err != nil {
			return nil, fmt.Errorf("create inputs dir: %w", err)
		}
		listener = listeners.NewFileWatcher(inputsDir, f, logger, listeners.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)))
	}

	serviceBuilt = true
	return &FactoryService{
		factoryRootDir: factoryRootDir,
		factory:        f,
		listener:       listener,
		net:            net,
		cfg:            cfg,
		runtimeCfg:     loadedFactoryCfg,
		logger:         logger,
		clock:          clock,
		recording:      recording,
		logSink:        logSink,
	}, nil
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

	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %v", ErrInvalidNamedFactory, name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}
	return fs.activateReplacementRuntime(ctx, rootDir, name, replacement)
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
	loadedFactoryCfg.SetRuntimeBaseDir(fs.cfg.ExecutionBaseDir)

	mapper := factoryconfig.ConfigMapper{}
	net, err := mapper.Map(ctx, loadedFactoryCfg.FactoryConfig(), loadedFactoryCfg)
	if err != nil {
		return nil, fmt.Errorf("map factory config: %w", err)
	}

	clock := fs.clock
	if clock == nil {
		clock = factory.EnsureClock(clockwork.NewRealClock())
	}
	eventHistory := factory.NewFactoryEventHistory(net, clock.Now, loadedFactoryCfg)
	workerOpts, err := loadWorkersFromConfig(
		loadedFactoryCfg.FactoryDir(),
		loadedFactoryCfg.FactoryConfig(),
		loadedFactoryCfg,
		logging.NewZapLogger(logger, fs.cfg.Verbose),
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

	inputsDir := filepath.Join(factoryDir, interfaces.InputsDir)
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create inputs dir: %w", err)
	}

	replacementListener := listeners.NewFileWatcher(
		inputsDir,
		replacementFactory,
		logger,
		listeners.WithKnownWorkStates(state.ValidStatesByType(net.WorkTypes)),
	)

	return &replacementFactoryRuntime{
		dir:        factoryDir,
		factory:    replacementFactory,
		listener:   replacementListener,
		net:        net,
		runtimeCfg: loadedFactoryCfg,
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

// Run starts the file watcher, dashboard, API server, and factory engine.
// It blocks until ctx is cancelled or the factory reaches a terminal state.
// portos:func-length-exception owner=agent-factory reason=legacy-service-run-loop review=2026-07-18 removal=split-sidecar-startup-recording-and-engine-shutdown-before-next-service-run-change
func (fs *FactoryService) Run(ctx context.Context) error {
	runCtx, cancelRunSidecars := context.WithCancel(ctx)
	var sidecars sync.WaitGroup
	var currentRuntime *liveRuntimeHandle
	serviceMode := runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService

	defer func() {
		if err := fs.logSink.Close(); err != nil {
			fs.logger.Warn("runtime log close failed", zap.Error(err))
		}
	}()
	defer func() {
		cancelRunSidecars()
		fs.clearRunState()
		sidecars.Wait()
	}()
	if fs.recording != nil {
		fs.recording.Start(runCtx)
		if err := fs.recording.Flush(); err != nil {
			return err
		}
	}

	if !serviceMode {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			if err := fs.listener.Watch(runCtx); err != nil && err != context.Canceled {
				fs.logger.Error("file watcher error", zap.Error(err))
			}
		}()
	}

	// Start API server if configured.
	if fs.cfg.APIServerStarter != nil && fs.cfg.Port > 0 {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			if err := fs.cfg.APIServerStarter(runCtx, fs, fs.cfg.Port, fs.logger); err != nil {
				fs.logger.Error("API server error", zap.Error(err))
			}
		}()
	}

	// Start dashboard loop if a renderer is provided.
	fs.startTime = fs.clock.Now()
	if fs.cfg.SimpleDashboardRenderer != nil {
		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			fs.dashboardLoop(runCtx)
		}()
	}

	if !serviceMode {
		if err := fs.preseedCurrentRuntimeInputs(ctx); err != nil {
			return err
		}
	}

	// Submit initial work if specified.
	if fs.cfg.WorkFile != "" {
		if err := fs.submitWorkFile(ctx); err != nil {
			return err
		}
	}

	currentRuntime = fs.startLiveRuntime(runCtx, fs.currentRuntimeBundle())
	fs.setRunState(runCtx, currentRuntime)
	if err := fs.waitForLiveRuntimeStart(ctx, currentRuntime); err != nil {
		fs.clearRunState()
		_ = fs.stopLiveRuntime(currentRuntime)
		return fmt.Errorf("start runtime: %w", err)
	}
	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runCtx, currentRuntime); err != nil {
			fs.clearRunState()
			_ = fs.stopLiveRuntime(currentRuntime)
			return err
		}
	}

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

	err := fs.waitForActiveRuntime(ctx)
	currentRuntime = fs.currentLiveRuntime()
	if stopErr := fs.stopLiveRuntime(currentRuntime); stopErr != nil && stopErr != context.Canceled && err == nil {
		err = stopErr
	}
	fs.clearRunState()
	cancelRunSidecars()
	sidecars.Wait()
	if fs.recording != nil {
		fs.recording.Finish(fs.clock.Now().UTC())
	}
	if writeErr := fs.writeRecording(); writeErr != nil {
		return writeErr
	}
	if fs.recording != nil {
		if recordErr := fs.recording.Err(); recordErr != nil {
			return recordErr
		}
	}

	// Print final dashboard.
	if fs.cfg.SimpleDashboardRenderer != nil {
		fs.renderDashboard(ctx)
	}

	if err != nil && err != context.Canceled {
		return fmt.Errorf("factory run: %w", err)
	}
	return nil
}

func (fs *FactoryService) activateReplacementRuntime(
	ctx context.Context,
	rootDir string,
	name string,
	replacement *replacementFactoryRuntime,
) error {
	runState := fs.currentRunState()
	if runState == nil || runState.runtime == nil || runState.ctx == nil {
		if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
			return err
		}
		fs.swapActiveRuntime(replacement)
		return nil
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

	replacementHandle := fs.startLiveRuntime(runState.ctx, replacement)
	if err := fs.waitForLiveRuntimeStart(ctx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return fmt.Errorf("start replacement runtime: %w", err)
	}

	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runState.ctx, replacementHandle); err != nil {
			_ = fs.stopLiveRuntime(replacementHandle)
			return fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		if serviceMode {
			fs.stopLiveRuntimeSidecars(replacementHandle)
		}
		_ = fs.stopLiveRuntime(replacementHandle)
		return err
	}

	restoreCurrentSidecars = false
	fs.swapActiveRuntime(replacement)
	fs.setRunState(runState.ctx, replacementHandle)
	if err := fs.stopLiveRuntime(runState.runtime); err != nil && err != context.Canceled {
		fs.logger.Warn("prior runtime shutdown failed", zap.Error(err))
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
	return nil
}

func (fs *FactoryService) currentRuntimeBundle() *replacementFactoryRuntime {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	if fs.factory == nil {
		return nil
	}
	return &replacementFactoryRuntime{
		dir:        fs.cfg.Dir,
		factory:    fs.factory,
		listener:   fs.listener,
		net:        fs.net,
		runtimeCfg: fs.runtimeCfg,
	}
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
			if err := handle.runtime.listener.Watch(sidecarCtx); err != nil && err != context.Canceled {
				fs.logger.Error("file watcher error", zap.Error(err))
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
	return handle.wait()
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
			if errors.Is(startCtx.Err(), context.Canceled) {
				_ = handle.wait()
				return handle.result()
			}
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

func (fs *FactoryService) waitForActiveRuntime(ctx context.Context) error {
	for {
		handle := fs.currentLiveRuntime()
		if handle == nil {
			return nil
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
	fs.runtimeMu.Lock()
	defer fs.runtimeMu.Unlock()
	fs.factory = runtimeBundle.factory
	fs.listener = runtimeBundle.listener
	fs.net = runtimeBundle.net
	fs.runtimeCfg = runtimeBundle.runtimeCfg
	fs.cfg.Dir = runtimeBundle.dir
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

func (fs *FactoryService) setRunState(ctx context.Context, runtime *liveRuntimeHandle) {
	fs.runMu.Lock()
	defer fs.runMu.Unlock()
	if ctx == nil || runtime == nil {
		fs.runState = nil
		return
	}
	fs.runState = &serviceRunState{
		ctx:     ctx,
		runtime: runtime,
	}
}

func (fs *FactoryService) clearRunState() {
	fs.setRunState(nil, nil)
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

// CreateNamedFactory persists one named-factory payload under the canonical
// layout and activates it through the idle-only runtime swap path.
func (fs *FactoryService) CreateNamedFactory(ctx context.Context, namedFactory factoryapi.NamedFactory) (factoryapi.NamedFactory, error) {
	if fs == nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("factory service is required")
	}
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	if err := factoryconfig.ValidateNamedFactoryName(string(namedFactory.Name)); err != nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("%w: %v", ErrInvalidNamedFactoryName, err)
	}

	payload, err := json.Marshal(namedFactory.Factory)
	if err != nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("marshal factory payload: %w", err)
	}

	if _, err := factoryconfig.PersistNamedFactory(rootDir, string(namedFactory.Name), payload); err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
			return factoryapi.NamedFactory{}, factoryconfig.ErrNamedFactoryAlreadyExists
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return factoryapi.NamedFactory{}, fmt.Errorf("%w: %v", ErrInvalidNamedFactory, err)
		default:
			return factoryapi.NamedFactory{}, err
		}
	}

	if err := fs.ActivateNamedFactory(ctx, string(namedFactory.Name)); err != nil {
		return factoryapi.NamedFactory{}, err
	}
	return fs.GetCurrentNamedFactory(ctx)
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

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (fs *FactoryService) GetCurrentNamedFactory(_ context.Context) (factoryapi.NamedFactory, error) {
	if fs == nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("factory service is required")
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	name, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return factoryapi.NamedFactory{}, ErrCurrentNamedFactoryNotFound
		}
		return factoryapi.NamedFactory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("resolve current named factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("load current named factory %q: %w", name, err)
	}

	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		current.FactoryConfig(),
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.NamedFactory{}, fmt.Errorf("serialize current named factory: %w", err)
	}
	return factoryapi.NamedFactory{
		Name:    factoryapi.FactoryName(name),
		Factory: generatedFactory,
	}, nil
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
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.factory
}

func (fs *FactoryService) currentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	return fs.runtimeCfg
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

// loadWorkersFromConfig instantiates worker executors from the loaded runtime config.
// Workers missing AGENTS.md keep the existing noop behavior so topology-only tests continue to work.
func loadWorkersFromConfig(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	logger logging.Logger,
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
	for _, workerCfg := range factoryCfg.Workers {
		logger.Debug("loading worker", "worker", workerCfg.Name)
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || def == nil || def.Type == "" {
			logger.Debug("no AGENTS.md for worker; using noop executor", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, &workers.NoopExecutor{}))
			continue
		}
		executor := buildWorkerExecutor(runtimeCfg, workerCfg.Name, logger, providerOverride, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, now)
		if executor != nil {
			logger.Info("loaded worker", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, executor))
		} else {
			// Continue regardless, intentional to deal with badly configured workers.
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
			RuntimeConfig: runtimeCfg,
			Renderer:      &workers.DefaultPromptRenderer{},
			Logger:        logger,
		}))
	}
	return opts, nil
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workerName string,
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
		var provider workers.Provider
		if providerOverride != nil {
			provider = providerOverride
		} else {
			var providerOpts []workers.ScriptWrapProviderOption
			providerOpts = append(providerOpts, workers.WithSkipPermissions(def.SkipPermissions))
			providerOpts = append(providerOpts, workers.WithProviderLogger(logger))
			if providerCommandRunner != nil {
				providerOpts = append(providerOpts, workers.WithProviderCommandRunner(providerCommandRunner))
			}
			provider = workers.NewScriptWrapProvider(providerOpts...)
		}
		if inferenceRecorder != nil {
			provider = workers.NewRecordingProvider(
				provider,
				inferenceRecorder,
				workers.WithRecordingProviderClock(now),
			)
		}

		agentOpts := []workers.AgentExecutorOption{
			workers.WithLogger(logger),
		}
		agentExec := workers.NewAgentExecutor(runtimeCfg, provider, agentOpts...)
		return &workers.WorkstationExecutor{
			RuntimeConfig: runtimeCfg,
			Executor:      agentExec,
			Renderer:      &workers.DefaultPromptRenderer{},
			Logger:        logger,
		}
	case interfaces.WorkstationTypeLogical:
		// LOGICAL_MOVE workers pass input token colors through without calling any LLM.
		return &workers.WorkstationExecutor{
			RuntimeConfig: runtimeCfg,
			Renderer:      &workers.DefaultPromptRenderer{},
			Logger:        logger,
		}
	case interfaces.WorkerTypeScript:
		var scriptOpts []workers.ScriptExecutorOption
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
			RuntimeConfig: runtimeCfg,
			Executor:      scriptExec,
			Renderer:      &workers.DefaultPromptRenderer{},
			Logger:        logger,
		}
	default:
		return nil
	}
}
