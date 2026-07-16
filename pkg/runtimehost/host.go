// backendsizecheck:ignore-file compatibility-shell package doc and SessionRuntimeHost remain with Host until dedicated transport host seam splits.
// pkgmaintcheck:ignore-file-lines compatibility-shell package doc and SessionRuntimeHost remain with Host until dedicated transport host seam splits.
// Package service provides factory runtime coordination, session hosting, and
// domain service implementations used by pkg/initializer and legacy callers.
//
// Transport composition (API startup, CLI local in-process startup, and MCP or
// tool-facing entrypoints) must use pkg/initializer as the canonical
// composition root. Root Host and BuildHost remain a
// temporary compatibility shell for in-process runtime hosting and wire
// equivalence tests; they are not the primary application shell for new
// transport wiring. Delete this compatibility layer once domain services fully
// replace the monolithic facade.
package runtimehost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryingest "github.com/portpowered/infinite-you/pkg/factory/ingest"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboardrender"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"

	"go.uber.org/zap"
)

type secretResolver = hostedworkers.SecretResolver

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

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = apisurface.ErrCurrentFactoryNotFound

type localModelCacheLayout = localmodels.CacheLayout
type localModelDomain = LocalModelDomain
type localModelLoadRequest = localmodels.LoadRequest
type localModelInvocationRequest = localmodels.InvocationRequest
type localModelHandle = localmodels.Handle
type localModelRuntime = localmodels.Runtime
type localModelResourceLimiter = localmodels.ResourceLimiter
type managedLocalModelManager = localmodels.Manager

// factoryRuntimeBundle is the runtime host bundle owned by pkg/factory/service.
type factoryRuntimeBundle = factoryservice.Bundle

// liveRuntimeHandle is the single-runtime host handle owned by pkg/factory/service.
type liveRuntimeHandle = factoryservice.Handle

type hostRunState struct {
	ctx       context.Context
	sessionID string
	runtime   *liveRuntimeHandle
}

// Host is an instantiation of a factory along with its runtime
// concerns: file watcher, dashboard, API server. It owns the full lifecycle
// so that CLI and other entry points remain thin wrappers.
//
// Extracted domains are composed explicitly: pkg/factory/sessions owns the live
// session registry, pkg/models/local owns managed model runtime wiring, and
// pkg/workers/hosted owns hosted poller supervision invoked from poller_watcher.
// pkg/workers/service owns poller and cron supervision invoked from poller_watcher.
type Host struct {
	runtimeMu        sync.RWMutex
	activationMu     sync.RWMutex
	runMu            sync.RWMutex
	runState         *hostRunState
	apiServerExit    <-chan error
	core             *Core
	sessions         *factorysessions.Registry
	factorySave      FactorySaveSaver
	sessionGateway   SessionGateway
	runtimeBuild     *runtimebuild.Service
	workersScheduler workerSidecarOwner
	hostedWorkers    hostedworkers.Config
	factoryRootDir   string
	policy           hostCoordinatorPolicy
	// startupBundle holds the built default runtime before Run registers ~default.
	startupBundle            *factoryRuntimeBundle
	cfg                      *Config
	baseLogger               *zap.Logger
	logger                   *zap.Logger
	startTime                time.Time
	clock                    factory.Clock
	modelAssets              modelAssetPuller
	modelService             apisurface.ModelAPI
	durableExecutionAPI      apisurface.DurableSessionAPI
	sessionInvoker           sessioninvocation.SessionInvoker
	coordinator              FactoryCoordinator
	definitions              FactoryDefinitionService
	newSessionResponseStream func() *factorysessions.SessionResponseStream
	durableExecution         factorysessionexecution.Service
}

var _ factory.APIFactory = (*Host)(nil)
var _ factory.WorkMover = (*Host)(nil)
var _ apisurface.ModelAPI = (*Host)(nil)
var _ apisurface.FactorySaveAPI = (*Host)(nil)
var _ apisurface.SessionAPI = (*Host)(nil)
var _ apisurface.WorkAPI = (*Host)(nil)
var _ apisurface.APISurface = (*Host)(nil)
var _ apisurface.SessionAPISurface = (*Host)(nil)

// InvocationAPI returns the canonical invocation collaborator used by the
// compatibility facade and composed HTTP surface.
func (h *Host) InvocationAPI() apisurface.InvocationAPI {
	return sessionInvocationAPI{owner: h.sessionInvocationOwner()}
}

// DurableExecutionAPI returns the canonical durable-start collaborator used by
// the compatibility facade and composed HTTP surface.
func (h *Host) DurableExecutionAPI() apisurface.DurableSessionAPI {
	if h != nil && h.durableExecutionAPI != nil {
		return h.durableExecutionAPI
	}
	return factorysession.NewDurableAPI(h.durableExecutionService(), h.requireSessionGateway())
}

// DurableExecutionService exposes the explicitly injected durable collaborator
// for compatibility composition and ownership verification.
func (h *Host) DurableExecutionService() factorysessionexecution.Service {
	if h == nil {
		return nil
	}
	return h.durableExecution
}

type RuntimeFileLoggingPolicy string

const (
	RuntimeFileLoggingPolicyEnabled  RuntimeFileLoggingPolicy = "enabled"
	RuntimeFileLoggingPolicyDisabled RuntimeFileLoggingPolicy = "disabled"
)

type RuntimeMetricsPolicy string

const (
	RuntimeMetricsPolicyEnabled  RuntimeMetricsPolicy = "enabled"
	RuntimeMetricsPolicyDisabled RuntimeMetricsPolicy = "disabled"
)

// Config holds all parameters needed to build and run a factory.
type Config struct {
	// Dir is the factory root directory containing factory.json and inputs/.
	Dir string
	// RunnerID sets the factory-level runner override used when a workstation
	// does not declare its own runner selection.
	RunnerID string
	// OperatorDefaults carries resolved operator-level default worker model
	// settings applied to omitted MODEL_WORKER fields in effective runtime config.
	OperatorDefaults operatorconfig.ResolvedDefaults
	// ExecutionBaseDir overrides the base directory used to resolve relative
	// runtime execution paths such as workstation workingDirectory values.
	// Empty defaults to the loaded factory directory.
	ExecutionBaseDir string
	// DurableSessionPersistencePolicy selects enabled project-local snapshots
	// or explicitly disabled in-memory-only durable execution. Empty defaults
	// to enabled for production-facing behavior.
	DurableSessionPersistencePolicy factorysessionexecution.PersistencePolicy
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
	// BackendScopeID is the stable session/cache namespace; empty local backends persist local-<uuid> during construction.
	BackendScopeID string
	// SystemConfigHomeDir overrides the home directory for backendScopeID system config resolution.
	SystemConfigHomeDir string
	// SystemConfigPath overrides the system config file path for backendScopeID persistence.
	SystemConfigPath string
	// RuntimeLogDir optionally overrides the default
	// ~/.you-agent-factory/logs directory. Tests use this to keep file-backed
	// logs isolated.
	RuntimeLogDir string
	// RuntimeFileLoggingPolicy controls whether the service creates a runtime
	// file sink. Empty defaults to enabled for production-facing behavior.
	RuntimeFileLoggingPolicy RuntimeFileLoggingPolicy
	// RuntimeLogConfig controls bounded runtime file logging behavior.
	// Zero values use defaults that match the package rolling policy.
	RuntimeLogConfig logging.RuntimeLogConfig
	// RuntimeMetricsPolicy controls whether the service creates a runtime
	// metrics sink. Empty defaults to enabled for production-facing behavior.
	RuntimeMetricsPolicy RuntimeMetricsPolicy
	// RuntimeMetricsDir optionally overrides the default
	// ~/.you-agent-factory/metrics directory. Tests use this to keep
	// file-backed metrics isolated.
	RuntimeMetricsDir string
	// RuntimeMetricsConfig controls bounded runtime metrics file behavior.
	// Zero values use defaults that match the runtime log rolling policy.
	RuntimeMetricsConfig platformmetrics.RuntimeMetricsConfig
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
	// InvocationSkipPermissionsOverride, when non-nil, requests an
	// invocation-scoped skip-permissions override for agent workers in this
	// run. It does not mutate persisted factory worker configuration.
	InvocationSkipPermissionsOverride *bool
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
	// InvocationMetricsRecorder, when non-nil, receives invocation boundary
	// counter emissions without including full prompt or result bodies.
	InvocationMetricsRecorder InvocationMetricsRecorder
	// ModelPullMetricsRecorder, when non-nil, receives managed-runtime pull
	// counter emissions without including cache paths or downloaded file bodies.
	ModelPullMetricsRecorder ModelPullMetricsRecorder
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
	// WorkerApplication is the process-composed worker/provider, script, and
	// hosted-worker component shared by every runtime session.
	WorkerApplication workerapplication.Components
	// ModelCacheDir optionally overrides the default managed local-model cache
	// directory under ~/.agent-factory/models.
	ModelCacheDir string
	// LocalModelRuntimeOverride injects a managed local-model runtime for
	// supported LOCAL model workers. Package tests use this to exercise the
	// load/invoke/reuse path without a live embedded backend.
	LocalModelRuntimeOverride localModelRuntime
	// ModelHostOverride replaces the default catalog model host wired into
	// runtime bundles. Tests use this to inject supervised hosts with fake
	// process launchers for hermetic local-model bootstrap invoke coverage.
	ModelHostOverride modelhost.Host
	// FactorySave, when non-nil, replaces the default factorysave.Service
	// collaborator. Tests use this to assert SaveFactoryForSession delegates
	// without running the full save orchestration pipeline.
	FactorySave FactorySaveSaver
	// SessionGateway, when non-nil, replaces the default
	// factory/sessions/service gateway collaborator. Tests use this to assert
	// OpenFactorySession delegates without running the full open pipeline.
	SessionGateway SessionGateway
	// ModelAPI, when non-nil, replaces the default pkg/models/service collaborator.
	// Tests use this to assert model transport methods delegate without running
	// the full managed-runtime pipeline.
	ModelAPI apisurface.ModelAPI
	// ModelAssets, when non-nil, replaces the default localmodels.AssetPuller
	// collaborator wired at service construction. Tests use this to assert
	// PullModel delegates without running managed asset downloads.
	ModelAssets modelAssetPuller
}

const hostModeStartupWorkReadabilityDelay = 250 * time.Millisecond

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (fs *Host) ActivateNamedFactory(ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	return fs.requireCoordinator().ActivateNamedFactory(ctx, name)
}

func (c *runtimeCoordinator) ActivateNamedFactory(ctx context.Context, name string) error {
	fs := c.host
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	svc := fs.definitionService()
	if svc == nil {
		return fmt.Errorf("factory definition service is required")
	}
	return svc.ActivateNamedFactory(ctx, name)
}

func (fs *Host) namedFactoryActivationPaths(session *factorysessions.LiveSession) (persistRoot, folderPath string) {
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

func (fs *Host) requireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
) error {
	if session != nil && liveSessionHandle(session) != nil {
		return fs.requireIdleRuntimeForSession(ctx, sessionID)
	}
	return fs.requireIdleRuntime(ctx)
}

func (fs *Host) applyNamedFactoryReplacement(
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
		return fs.ReplaceSessionRuntime(ctx, session, name, replacement)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}
	return fs.activateReplacementWithoutLiveRuntime(persistRoot, name, replacement)
}

func (fs *Host) buildReplacementFactoryRuntime(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (*factoryRuntimeBundle, error) {
	if fs == nil || fs.runtimeBuild == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	bundle, err := fs.runtimeBuild.BuildReplacement(
		ctx,
		folderPath,
		factoryDir,
		sessionID,
		fs.replacementExecutionBaseDir(folderPath, factoryDir, sessionID),
	)
	if err != nil {
		return nil, err
	}
	return asRuntimeBundle(bundle), nil
}

func (fs *Host) replacementExecutionBaseDir(folderPath string, factoryDir string, sessionID string) string {
	if session := fs.sessionByID(sessionID); session != nil {
		if executionBaseDir := strings.TrimSpace(session.ExecutionBaseDir); executionBaseDir != "" {
			return executionBaseDir
		}
	}
	if folderPath = strings.TrimSpace(folderPath); folderPath != "" {
		return folderPath
	}
	if factoryDir = strings.TrimSpace(factoryDir); factoryDir != "" {
		return factoryDir
	}
	if fs != nil {
		return strings.TrimSpace(fs.coordinatorPolicy().executionBaseDir)
	}
	return ""
}

// Run starts runtime-owned sidecars and the factory engine. Process-level
// presentation sidecars are composed and owned outside Host.
// It blocks until ctx is cancelled or the factory reaches a terminal state.
// portos:func-length-exception owner=agent-factory reason=legacy-service-run-loop review=2026-07-18 removal=split-sidecar-startup-recording-and-engine-shutdown-before-next-service-run-change
func (fs *Host) Run(ctx context.Context) error {
	return fs.run(ctx, fs)
}

// RunWithAPISurface starts the runtime lifecycle while supplying independently
// composed HTTP handler dependencies to the configured API server starter.
func (fs *Host) RunWithAPISurface(ctx context.Context, surface apisurface.APISurface) error {
	return fs.run(ctx, surface)
}

func (fs *Host) run(ctx context.Context, surface apisurface.APISurface) error {
	fs.startTime = fs.clock.Now()
	runCtx, cancelRunSidecars := context.WithCancel(ctx)
	var sidecars sync.WaitGroup
	var currentRuntime *liveRuntimeHandle
	serviceMode := runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService

	defer func() {
		if currentRuntime != nil {
			return
		}
		if bundle := fs.startupRuntimeBundle(); bundle != nil {
			if err := closeRuntimeBundleSinks(bundle.LogSink, bundle.MetricsSink); err != nil {
				fs.logger.Warn("runtime artifact close failed", zap.Error(err))
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
	currentRuntime, err := fs.startRunRuntime(ctx, runCtx, &sidecars, serviceMode, surface)
	if err != nil {
		return err
	}

	err = fs.waitForActiveRuntime(ctx)
	currentRuntime = fs.currentLiveRuntime()
	if stopErr := fs.StopLiveRuntime(currentRuntime); stopErr != nil && !errors.Is(stopErr, context.Canceled) && err == nil {
		err = stopErr
	}
	if stopErr := fs.ShutdownOtherLiveSessions(currentRuntime); stopErr != nil && err == nil {
		err = stopErr
	}
	fs.clearRunState()
	cancelRunSidecars()
	sidecars.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("factory run: %w", err)
	}
	return nil
}

func (fs *Host) startRunRuntime(
	ctx context.Context,
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	serviceMode bool,
	surface apisurface.APISurface,
) (*liveRuntimeHandle, error) {
	currentRuntime, err := fs.StartDefaultRuntime(ctx, runCtx, serviceMode)
	if err != nil {
		return nil, err
	}
	fs.startAPIServerSidecar(runCtx, sidecars, surface)
	if err := fs.waitForServiceModeStartupWorkReadability(ctx, serviceMode); err != nil {
		return nil, fs.failServiceModeStartup(currentRuntime, err)
	}
	if err := fs.submitServiceModeWorkFile(ctx, currentRuntime, serviceMode); err != nil {
		return nil, err
	}
	fs.logServiceStartup()
	return currentRuntime, nil
}

func (fs *Host) startRunSidecars(runCtx context.Context, sidecars *sync.WaitGroup, serviceMode bool) {
	if !serviceMode {
		var listener *factoryingest.FileWatcher
		if bundle := fs.currentRuntimeBundle(); bundle != nil {
			listener = bundle.Listener
		}
		fs.startListenerSidecar(runCtx, sidecars, listener, fs.logger)
	}
}

func (fs *Host) startListenerSidecar(
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	listener *factoryingest.FileWatcher,
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

func (fs *Host) prepareRunInputs(ctx context.Context, serviceMode bool) error {
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

func (fs *Host) submitServiceModeWorkFile(
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

func (fs *Host) StartDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	return fs.requireCoordinator().StartDefaultRuntime(ctx, runCtx, serviceMode)
}

func (c *runtimeCoordinator) StartDefaultRuntime(
	ctx context.Context,
	runCtx context.Context,
	serviceMode bool,
) (*liveRuntimeHandle, error) {
	fs := c.host
	runtimeBundle := fs.currentRuntimeBundle()
	currentRuntime := fs.startLiveRuntime(runCtx, runtimeBundle)
	registeredSessionID := fs.registerLiveSession(
		DefaultFactorySessionID,
		currentRuntime,
		defaultSessionTargetFromRuntimeBundle(runtimeBundle, fs.factoryRootDir),
		true,
	)
	fs.clearStartupBundle()
	fs.setRunState(runCtx, registeredSessionID, currentRuntime)
	if err := fs.waitForLiveRuntimeStart(ctx, currentRuntime); err != nil {
		return nil, fs.handleDefaultRuntimeStartFailure(ctx, currentRuntime, err)
	}
	if serviceMode {
		if err := fs.StartLiveRuntimeSidecars(runCtx, currentRuntime); err != nil {
			if fs.defaultSessionClosedDuringStartup() {
				return nil, nil
			}
			fs.clearRunState()
			fs.unregisterLiveSession(DefaultFactorySessionID)
			_ = fs.StopLiveRuntime(currentRuntime)
			return nil, err
		}
	}
	return currentRuntime, nil
}

func (fs *Host) startAPIServerSidecar(
	runCtx context.Context,
	sidecars *sync.WaitGroup,
	surface apisurface.APISurface,
) {
	if fs.cfg.APIServerStarter == nil || fs.cfg.Port <= 0 {
		fs.apiServerExit = nil
		return
	}
	apiServerExit := make(chan error, 1)
	fs.apiServerExit = apiServerExit
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		err := fs.cfg.APIServerStarter(runCtx, surface, fs.cfg.Port, fs.logger)
		apiServerExit <- err
		close(apiServerExit)
		if err != nil {
			fs.logger.Error("API server error", zap.Error(err))
		}
	}()
}

func (fs *Host) logServiceStartup() {
	bundle := fs.currentRuntimeBundle()
	if bundle == nil || bundle.LogSink == nil {
		return
	}
	runtimeLogConfig := bundle.LogSink.Config()
	fs.logger.Info("factory started",
		zap.String("dir", fs.cfg.Dir),
		zap.String("runtime_log_path", bundle.LogSink.Path()),
		zap.String("runtime_log_root", bundle.LogSink.RootDir()),
		zap.String("runtime_log_start_time_utc", runtimeLogStartTimeString(bundle.LogSink.StartTimeUTC())),
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

func (fs *Host) activateReplacementWithoutLiveRuntime(
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

func (fs *Host) requireIdleRuntime(ctx context.Context) error {
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

func (fs *Host) currentRuntimeBundle() *factoryRuntimeBundle {
	if fs == nil {
		return nil
	}
	if runState := fs.currentRunState(); runState != nil && runState.runtime != nil {
		return runState.runtime.Bundle
	}
	if session := fs.defaultSession(); session != nil {
		if handle := liveSessionHandle(session); handle != nil {
			return handle.Bundle
		}
	}
	return fs.startupRuntimeBundle()
}

func (fs *Host) publishFactoryChangeEvent(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	replacement *factoryRuntimeBundle,
) {
	if replacement == nil || replacement.EventHistory == nil {
		return
	}

	payload, ok := replacementFactoryChangePayload(replacement.EventHistory.CanonicalEvents())
	if !ok {
		return
	}

	eventTime := factory.EnsureClock(fs.clock).Now()
	replacement.EventHistory.RecordFactoryChange(1, payload, eventTime)

	if currentRuntime == nil || currentRuntime.Bundle == nil || currentRuntime.Bundle.EventHistory == nil {
		return
	}

	snapshot, err := currentRuntime.Bundle.Factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		fs.logger.Warn("read current runtime tick for factory-change event failed", zap.Error(err))
		return
	}
	currentRuntime.Bundle.EventHistory.RecordFactoryChange(snapshot.TickCount+1, payload, eventTime)
}

func (fs *Host) startLiveRuntime(ctx context.Context, runtimeBundle *factoryRuntimeBundle) *liveRuntimeHandle {
	return factoryservice.Start(ctx, runtimeBundle)
}

func (fs *Host) StartLiveRuntimeSidecars(ctx context.Context, handle *liveRuntimeHandle) error {
	return fs.requireCoordinator().StartLiveRuntimeSidecars(ctx, handle)
}

func (c *runtimeCoordinator) StartLiveRuntimeSidecars(ctx context.Context, handle *liveRuntimeHandle) error {
	fs := c.host
	if handle == nil || handle.Bundle == nil {
		return fmt.Errorf("runtime handle is required")
	}

	handle.SidecarMu.Lock()
	defer handle.SidecarMu.Unlock()
	if handle.SidecarCancel != nil {
		return nil
	}

	sidecarCtx, sidecarCancel := context.WithCancel(ctx)
	handle.SidecarCancel = sidecarCancel
	handle.Sidecars.Add(1)
	go func() {
		defer handle.Sidecars.Done()
		factoryservice.ObserveRuntimeMetrics(sidecarCtx, handle)
	}()
	if handle.Bundle.Listener != nil {
		handle.Sidecars.Add(1)
		go func() {
			defer handle.Sidecars.Done()
			if err := handle.Bundle.Listener.Watch(sidecarCtx); err != nil && !errors.Is(err, context.Canceled) {
				handle.Bundle.RuntimeLogger().Error("file watcher error", zap.Error(err))
			}
		}()
	}

	if err := fs.startSchedulerSidecarsForRuntime(
		sidecarCtx,
		&handle.Sidecars,
		handle.Bundle.RuntimeCfg.FactoryDir(),
		handle.Bundle.RuntimeCfg.FactoryConfig(),
		handle.Bundle.RuntimeCfg,
		submitWorkRequestWithFactory(handle.Bundle.Factory),
	); err != nil {
		sidecarCancel()
		handle.Sidecars.Wait()
		handle.SidecarCancel = nil
		return fmt.Errorf("attach worker sidecars: %w", err)
	}
	if handle.Bundle.Listener != nil {
		if err := handle.Bundle.Listener.PreseedInputs(sidecarCtx); err != nil {
			sidecarCancel()
			handle.Sidecars.Wait()
			handle.SidecarCancel = nil
			return fmt.Errorf("preseed inputs: %w", err)
		}
	}
	return nil
}

func (fs *Host) StopLiveRuntimeSidecars(handle *liveRuntimeHandle) {
	fs.requireCoordinator().StopLiveRuntimeSidecars(handle)
}

func (c *runtimeCoordinator) StopLiveRuntimeSidecars(handle *liveRuntimeHandle) {
	factoryservice.StopSidecars(handle)
}

func (fs *Host) StopLiveRuntime(handle *liveRuntimeHandle) error {
	return fs.requireCoordinator().StopLiveRuntime(handle)
}

func (c *runtimeCoordinator) StopLiveRuntime(handle *liveRuntimeHandle) error {
	fs := c.host
	if handle == nil {
		return nil
	}
	return factoryservice.Stop(handle, fs.clock)
}

func (fs *Host) ShutdownOtherLiveSessions(except *liveRuntimeHandle) error {
	return fs.requireCoordinator().ShutdownOtherLiveSessions(except)
}

func (c *runtimeCoordinator) ShutdownOtherLiveSessions(except *liveRuntimeHandle) error {
	fs := c.host
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
			if err := fs.StopLiveRuntime(handle); err != nil && !errors.Is(err, context.Canceled) {
				errs = append(errs, err)
			}
		}
		fs.unregisterLiveSession(sessionID)
	}
	return errors.Join(errs...)
}

func (fs *Host) waitForLiveRuntimeStart(ctx context.Context, handle *liveRuntimeHandle) error {
	return factoryservice.WaitForStart(ctx, handle)
}

func isCanceledServiceStartup(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(err, context.Canceled)
}

func (fs *Host) waitForActiveRuntime(ctx context.Context) error {
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
			_ = handle.Wait()
		case <-handle.RunDone:
		}
		if fs.currentLiveRuntime() != handle {
			continue
		}
		if runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService &&
			fs.sessions != nil && fs.sessions.Count() == 0 {
			continue
		}
		return handle.Result()
	}
}
