package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/ingest"
	"github.com/portpowered/infinite-you/pkg/localmodels"
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

type secretResolver = hostedworkers.SecretResolver

// ErrFactoryActivationRequiresIdle reports that runtime replacement was
// attempted while the current runtime still had active work.
var ErrFactoryActivationRequiresIdle = apisurface.ErrFactoryActivationRequiresIdle

// ErrInvalidNamedFactoryName reports that the requested named-factory name is
// not a safe canonical layout segment.
var ErrInvalidNamedFactoryName = apisurface.ErrInvalidNamedFactoryName

// ErrInvalidNamedFactory reports that the submitted named-factory payload could
// not be persisted or validated as a runnable runtime config.
var ErrInvalidNamedFactory = apisurface.ErrInvalidNamedFactory

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = apisurface.ErrCurrentFactoryNotFound

type localModelCacheLayout = localmodels.CacheLayout
type localModelLoadRequest = localmodels.LoadRequest
type localModelInvocationRequest = localmodels.InvocationRequest
type localModelHandle = localmodels.Handle
type localModelRuntime = localmodels.Runtime
type localModelResourceLimiter = localmodels.ResourceLimiter
type managedLocalModelManager = localmodels.Manager

// factoryRuntimeBundle is the single runtime wiring struct produced by
// buildRuntimeBundle and referenced from liveRuntimeHandle.
type factoryRuntimeBundle struct {
	dir            string
	folderPath     string
	eventHistory   *factoryevents.FactoryEventHistory
	factory        factory.Factory
	listener       *ingest.FileWatcher
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
	runtime       *factoryRuntimeBundle
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
	sessionID             string
	cfg                   *FactoryServiceConfig
	loadedFactoryCfg      *factoryconfig.LoadedFactoryConfig
	baseLogger            *zap.Logger
	runtimeInstanceID     string
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
//
// Extracted domains are composed explicitly: pkg/factorysessions owns the live
// session registry, pkg/localmodels owns managed model runtime wiring, and
// pkg/hostedworkers owns hosted poller supervision invoked from poller_watcher.
type FactoryService struct {
	runtimeMu     sync.RWMutex
	activationMu  sync.RWMutex
	runMu         sync.RWMutex
	runState      *serviceRunState
	apiServerExit <-chan error
	sessions      *factorysessions.Registry
	factorySave   *factorysave.Service
	hostedWorkers hostedworkers.Config
	factoryRootDir string
	// startupBundle holds the built default runtime before Run registers ~default.
	startupBundle *factoryRuntimeBundle
	cfg           *FactoryServiceConfig
	baseLogger    *zap.Logger
	logger        *zap.Logger
	startTime     time.Time
	clock         factory.Clock
	modelAssets   modelAssetPuller
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
	// APIServerReady, when non-nil, is closed by the API starter once the
	// service-mode HTTP surface is reachable. Service-mode startup work waits
	// for this signal so external clients can observe the startup window before
	// the initial WorkFile begins processing.
	APIServerReady <-chan struct{}
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

const serviceModeStartupWorkReadabilityDelay = 250 * time.Millisecond

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (fs *FactoryService) ActivateNamedFactory(ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	sessionID := fs.runSessionID()
	session := fs.sessionByID(sessionID)
	persistRoot, folderPath := fs.namedFactoryActivationPaths(session)

	if err := fs.requireIdleBeforeNamedFactoryActivation(ctx, sessionID, session); err != nil {
		return err
	}

	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(persistRoot, name)
	if err != nil {
		return err
	}

	replacement, err := fs.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}

	return fs.applyNamedFactoryReplacement(ctx, sessionID, session, persistRoot, name, replacement)
}

func (fs *FactoryService) namedFactoryActivationPaths(session *factorysessions.LiveSession) (persistRoot, folderPath string) {
	persistRoot = fs.factoryRootDir
	if persistRoot == "" && fs.cfg != nil {
		persistRoot = fs.cfg.Dir
	}
	folderPath = persistRoot
	if session == nil {
		return persistRoot, folderPath
	}
	persistRoot = sessionFactoryPersistRoot(fs.factoryRootDir, session)
	if trimmed := strings.TrimSpace(session.FolderPath); trimmed != "" {
		folderPath = trimmed
	} else {
		folderPath = persistRoot
	}
	return persistRoot, folderPath
}

func (fs *FactoryService) requireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
) error {
	if session != nil && liveSessionHandle(session) != nil {
		return fs.requireIdleRuntimeForSession(ctx, sessionID)
	}
	return fs.requireIdleRuntime(ctx)
}

func (fs *FactoryService) applyNamedFactoryReplacement(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	persistRoot string,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	if session != nil && liveSessionHandle(session) != nil {
		if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
			return err
		}
		if err := factoryconfig.WriteCurrentFactoryPointer(persistRoot, name); err != nil {
			return err
		}
		return fs.replaceSessionRuntime(ctx, session, name, replacement)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}
	return fs.activateReplacementWithoutLiveRuntime(persistRoot, name, replacement)
}

func (fs *FactoryService) buildReplacementFactoryRuntime(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (*factoryRuntimeBundle, error) {
	baseLogger := fs.baseLogger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}

	loadedFactoryCfg, err := configload.LoadRuntimeConfigFromFactoryDir(factoryDir, fs.cfg.WorkstationLoader)
	if err != nil {
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	logger := newSessionLogger(baseLogger, sessionID, folderPath, loadedFactoryCfg.FactoryDir())
	warnPortableBundledReplacementReport(logger, "named factory activation replaced portable bundled files", loadedFactoryCfg.PortableBundledFileReplacements())
	loadedFactoryCfg.SetRuntimeBaseDir(fs.cfg.ExecutionBaseDir)
	clock := factory.EnsureClock(fs.clock)
	recordPath := sessionScopedRecordPath(fs.cfg.RecordPath, sessionID)
	return buildRuntimeBundle(ctx, runtimeBundleBuildInput{
		dir:                   factoryDir,
		folderPath:            folderPath,
		sessionID:             sessionID,
		cfg:                   fs.cfg,
		loadedFactoryCfg:      loadedFactoryCfg,
		baseLogger:            baseLogger,
		runtimeInstanceID:     uuid.NewString(),
		clock:                 clock,
		recordPath:            recordPath,
		workflowID:            fs.cfg.WorkflowID,
		providerOverride:      providerOverrideForMode(fs.cfg, nil),
		providerCommandRunner: providerCommandRunnerForMode(fs.cfg, loadedFactoryCfg),
		commandRunnerOverride: commandRunnerOverrideForMode(fs.cfg, loadedFactoryCfg, nil),
	})
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
		if currentRuntime != nil {
			return
		}
		if bundle := fs.startupRuntimeBundle(); bundle != nil && bundle.logSink != nil {
			if err := bundle.logSink.Close(); err != nil {
				fs.logger.Warn("runtime log close failed", zap.Error(err))
			}
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
	currentRuntime, err := fs.startRunRuntime(ctx, runCtx, &sidecars, serviceMode)
	if err != nil {
		return err
	}

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

func (fs *FactoryService) startRunRuntime(
	ctx context.Context,
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	currentRuntime, err := fs.startDefaultRuntime(ctx, runCtx, serviceMode)
	if err != nil {
		return nil, err
	}
	fs.startAPIServerSidecar(runCtx, sidecars)
	if err := fs.waitForServiceModeStartupWorkReadability(ctx, serviceMode); err != nil {
		return nil, fs.failServiceModeStartup(currentRuntime, err)
	}
	if err := fs.submitServiceModeWorkFile(ctx, currentRuntime, serviceMode); err != nil {
		return nil, err
	}
	fs.logServiceStartup()
	return currentRuntime, nil
}

func (fs *FactoryService) startRunSidecars(runCtx context.Context, sidecars *sync.WaitGroup, serviceMode bool) {
	if !serviceMode {
		var listener *ingest.FileWatcher
		if bundle := fs.currentRuntimeBundle(); bundle != nil {
			listener = bundle.listener
		}
		fs.startListenerSidecar(runCtx, sidecars, listener, fs.logger)
	}
	fs.startDashboardSidecar(runCtx, sidecars)
}

func (fs *FactoryService) startListenerSidecar(
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	listener *ingest.FileWatcher,
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
	if fs.cfg.WorkFile != "" && !serviceMode {
		if err := fs.submitWorkFile(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (fs *FactoryService) submitServiceModeWorkFile(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	serviceMode bool,
) error {
	if !serviceMode || fs.cfg.WorkFile == "" {
		return nil
	}
	if err := fs.submitWorkFile(ctx); err != nil {
		return fs.failServiceModeStartup(currentRuntime, err)
	}
	return nil
}

func (fs *FactoryService) startDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	runtimeBundle := fs.currentRuntimeBundle()
	currentRuntime := fs.startLiveRuntime(runCtx, runtimeBundle)
	fs.registerLiveSession(
		defaultFactorySessionID,
		currentRuntime,
		defaultSessionTargetFromRuntimeBundle(runtimeBundle, fs.factoryRootDir),
		true,
	)
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
		fs.apiServerExit = nil
		return
	}
	apiServerExit := make(chan error, 1)
	fs.apiServerExit = apiServerExit
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		err := fs.cfg.APIServerStarter(runCtx, fs, fs.cfg.Port, fs.logger)
		apiServerExit <- err
		close(apiServerExit)
		if err != nil {
			fs.logger.Error("API server error", zap.Error(err))
		}
	}()
}

func (fs *FactoryService) logServiceStartup() {
	bundle := fs.currentRuntimeBundle()
	if bundle == nil || bundle.logSink == nil {
		return
	}
	runtimeLogConfig := bundle.logSink.Config()
	fs.logger.Info("factory started",
		zap.String("dir", fs.cfg.Dir),
		zap.String("runtime_log_path", bundle.logSink.Path()),
		zap.String("runtime_log_root", bundle.logSink.RootDir()),
		zap.String("runtime_log_start_time_utc", runtimeLogStartTimeString(bundle.logSink.StartTimeUTC())),
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

func (fs *FactoryService) activateReplacementWithoutLiveRuntime(
	rootDir string,
	name string,
	replacement *factoryRuntimeBundle,
) error {
	if err := configpersist.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		return err
	}
	fs.setStartupBundle(replacement)
	fs.syncActiveSessionDir(replacement)
	return nil
}

func (fs *FactoryService) requireIdleRuntime(ctx context.Context) error {
	sessionID := fs.runSessionID()
	if session := fs.sessionByID(sessionID); session != nil && liveSessionHandle(session) != nil {
		return fs.requireIdleRuntimeForSession(ctx, sessionID)
	}

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

func (fs *FactoryService) currentRuntimeBundle() *factoryRuntimeBundle {
	if fs == nil {
		return nil
	}
	if runState := fs.currentRunState(); runState != nil && runState.runtime != nil {
		return runState.runtime.runtime
	}
	if session := fs.defaultSession(); session != nil {
		if handle := liveSessionHandle(session); handle != nil {
			return handle.runtime
		}
	}
	return fs.startupRuntimeBundle()
}

func (fs *FactoryService) publishFactoryChangeEvent(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	replacement *factoryRuntimeBundle,
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

func (fs *FactoryService) startLiveRuntime(ctx context.Context, runtimeBundle *factoryRuntimeBundle) *liveRuntimeHandle {
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
	for _, sessionID := range fs.sessions.IDs() {
		session := fs.sessionByID(sessionID)
		if session == nil {
			continue
		}
		handle := liveSessionHandle(session)
		if handle == except {
			continue
		}
		if handle != nil {
			if err := fs.stopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
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
