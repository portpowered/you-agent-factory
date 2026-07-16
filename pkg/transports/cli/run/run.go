// Package run implements the agent-factory run command behavior.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	"github.com/portpowered/infinite-you/pkg/transports/cli/batchload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/dashboard"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"github.com/portpowered/infinite-you/pkg/transports/cli/timedisplay"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/google/uuid"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
	"go.uber.org/zap"
)

// RunConfig holds parameters for the run command.
type RunConfig struct {
	Workflow     string
	Continuously bool
	WorkFile     string
	Dir          string
	HomeDir      string // Normalized home for implicit paths; empty preserves legacy direct callers.
	// NamedFactoryName is the canonical --named factory resolved into Dir before startup.
	NamedFactoryName       string
	NamedFactoryResolution *factoryconfig.NamedFactoryResolution // Source and precedence metadata.
	// FactoryConfigPath is the --factory file; Dir is its resolved factory root.
	FactoryConfigPath string
	// InvocationPositionalText is optional --factory text resolved by the shared input contract.
	InvocationPositionalText *string
	// InvocationStdinText is stdin consumed before a one-shot factory invocation.
	InvocationStdinText           *string
	InvocationNormalizedArguments *invocations.NormalizedArguments // Normalized signature-backed inputs.
	RunnerID                      string
	// OperatorDefaults carries resolved operator-level default worker model
	// settings loaded at the CLI boundary.
	OperatorDefaults operatorconfig.ResolvedDefaults
	// ExecutionBaseDir overrides the base directory used to resolve relative
	// runtime execution paths. Empty defaults to the caller's current working
	// directory for CLI-style runs.
	ExecutionBaseDir string
	Bootstrap        bool
	// BindHost is the hostname from --server used in dashboard URLs (for example localhost or 127.0.0.1).
	BindHost string
	Port     int
	// AutoPort resolves Port to the next available local TCP port when the
	// preferred port is unavailable. Explicit port selections should leave this
	// false so operator intent is preserved.
	AutoPort   bool
	RecordPath string
	ReplayPath string
	// DisableDefaultRecording disables the default live-run replay artifact
	// generation for a single invocation.
	DisableDefaultRecording bool
	// RuntimeLogDir overrides the service-owned structured runtime log root.
	// Empty uses the service default under the user's home directory.
	RuntimeLogDir string
	// RuntimeLogConfig controls service-owned structured runtime log rotation.
	RuntimeLogConfig logging.RuntimeLogConfig
	// RuntimeMetricsDir overrides the service-owned structured runtime metrics
	// root. Empty uses the service default under the user's home directory.
	RuntimeMetricsDir string
	// RuntimeMetricsConfig controls service-owned structured runtime metrics
	// rolling behavior.
	RuntimeMetricsConfig logging.RuntimeMetricsConfig
	// MockWorkersEnabled enables deterministic mock-worker execution. When
	// true and MockWorkersConfigPath is empty, the runtime uses the default
	// accept behavior for all worker dispatches.
	MockWorkersEnabled    bool
	MockWorkersConfigPath string
	Verbose               bool
	// TerminalPolicy carries the CLI-resolved quiet/normal/verbose contract for
	// this invocation. When resolved, diagnostics and logger sinks consult it.
	TerminalPolicy terminalpolicy.Policy
	// SuppressDashboardRendering disables the simple stdout dashboard while
	// preserving the normal service-layer run path.
	SuppressDashboardRendering bool
	// CleanInvocation suppresses operator-facing stdout chatter for one-shot
	// result-oriented invocations. It does not disable replay recording.
	CleanInvocation bool
	// JSON emits the clean invocation success result as a single JSON object.
	JSON bool
	// CleanInvocationInputSource describes how a one-shot clean invocation
	// received its primary input payload.
	CleanInvocationInputSource InvocationInputSource
	// Output receives clean invocation and shared factory-invocation success
	// payloads. Nil defaults to stdout.
	Output io.Writer
	// OpenDashboard attempts to open the embedded dashboard URL in a browser.
	OpenDashboard bool
	// StartupOutput receives human-facing startup messages. Nil suppresses
	// startup output for programmatic callers and tests.
	StartupOutput io.Writer
	// Diagnostics receives metadata-only verbose command diagnostics. Nil
	// suppresses diagnostics for programmatic callers and tests.
	Diagnostics io.Writer
	// Stdin provides the CLI stdin stream for shared invocation input
	// resolution. Nil defaults to os.Stdin.
	Stdin io.Reader
	// StdinIsTTY reports whether stdin is an interactive TTY. Nil inspects
	// os.Stdin directly.
	StdinIsTTY func() bool
	// JSONOutput emits the API-shaped InvocationResponse for factory invocation
	// results, including non-success outcomes that return recovery context.
	JSONOutput bool
	// InvocationOutputMode selects stdout behavior for one-shot factory
	// invocations. Empty uses the primary-result-only contract; response-stream
	// attaches to internal SessionResponseStream progress when available.
	InvocationOutputMode string
	// InvocationMetricsRecorder receives invocation counter emissions from the
	// CLI boundary, including pre-runtime source conflicts.
	InvocationMetricsRecorder service.InvocationMetricsRecorder
	// InvocationSkipPermissionsOverride requests an invocation-scoped unsafe
	// permission bypass for agent workers when non-nil. Set from you run
	// --skip-permissions and never written back to persisted factory config.
	InvocationSkipPermissionsOverride *bool
	Logger                            *zap.Logger
}

type factoryServiceRunner interface {
	Run(ctx context.Context) error
}

// RuntimeRunner is the local in-process runtime seam used by CLI startup.
type RuntimeRunner = factoryServiceRunner

type engineStateSnapshotProvider interface {
	GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
}

type cleanInvocationSuccess struct {
	Output       string `json:"output"`
	WorkID       string `json:"workId"`
	WorkTypeName string `json:"workTypeName"`
	TraceID      string `json:"traceId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
}

type cleanInvocationWorkTarget struct {
	WorkID       string
	WorkTypeName string
}

// FactoryServiceBuilder constructs the factory service used by Run.
type FactoryServiceBuilder func(
	context.Context,
	*service.FactoryServiceConfig,
) (RuntimeRunner, error)

// FactoryServiceBuildFunc constructs *service.FactoryService for registration
// from cmd/ when the builder is defined outside pkg/transports/cli/run.
type FactoryServiceBuildFunc func(
	context.Context,
	*service.FactoryServiceConfig,
) (*service.FactoryService, error)

// FactoryServiceBuilderFromService adapts a concrete service constructor for
// SetBuildFactoryService when the builder is defined outside pkg/transports/cli/run.
func FactoryServiceBuilderFromService(build FactoryServiceBuildFunc) FactoryServiceBuilder {
	return func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return build(ctx, cfg)
	}
}

func defaultBuildFactoryService(
	context.Context,
	*service.FactoryServiceConfig,
) (factoryServiceRunner, error) {
	return nil, errors.New("construct local runtime: dependency-injected builder is required")
}

var buildFactoryService FactoryServiceBuilder = defaultBuildFactoryService

// SetBuildFactoryService registers the factory service builder used by Run.
// This compatibility hook does not select a construction path by default;
// process-root executions use BuildApplication with a Wire-owned builder.
func SetBuildFactoryService(builder FactoryServiceBuilder) {
	if builder == nil {
		buildFactoryService = defaultBuildFactoryService
		return
	}
	buildFactoryService = builder
}

const (
	completedPlaceIDSuffix        = "completed"
	failedPlaceIDSuffix           = "failed"
	defaultFactorySessionID       = "~default"
	defaultRecordPathSessionToken = "__factory_session_id__"
)

var bootstrapFactory = func(dir string) error {
	resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(dir)
	if err != nil {
		if errors.Is(err, factoryconfig.ErrFactoryLayoutNotFound) {
			return initcmd.Init(initcmd.InitConfig{Dir: dir})
		}
		return err
	}

	defaultInputDir := filepath.Join(resolvedDir, interfaces.InputsDir, initcmd.DefaultFactoryInputType, interfaces.DefaultChannelName)
	return os.MkdirAll(defaultInputDir, 0o755)
}

var dashboardOpener = openURLInBrowser

var interactiveOutput = isInteractiveOutput
var defaultLiveRunRecordPath = generateDefaultLiveRunRecordPath
var defaultLiveRunRecordTime = time.Now
var defaultLiveRunRecordUUID = uuid.NewString

const dashboardReadyTimeout = 5 * time.Second
const maxAutoPortAttempts = 100

type reservedAPIServerListener struct {
	listener net.Listener
	port     int
	taken    bool
	mu       sync.Mutex
}

type resolvedRunRecordPath struct {
	servicePath   string
	reportedPath  string
	autoGenerated bool
}

func defaultServeFactoryAPIServer(
	ctx context.Context,
	runtime apisurface.APISurface,
	port int,
	logger *zap.Logger,
	listener net.Listener,
) error {
	srv := api.NewServer(runtime, port, logger)
	return srv.Serve(ctx, listener)
}

var serveFactoryAPIServer = defaultServeFactoryAPIServer

var startAPIServer = func(
	ctx context.Context,
	runtime apisurface.APISurface,
	port int,
	logger *zap.Logger,
	markReady func(),
) error {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return serveAPIServer(ctx, runtime, port, logger, markReady, listener)
}

func serveAPIServer(
	ctx context.Context,
	runtime apisurface.APISurface,
	port int,
	logger *zap.Logger,
	markReady func(),
	listener net.Listener,
) error {
	markReady()
	return serveFactoryAPIServer(ctx, runtime, port, logger, listener)
}

func reserveAPIServerListener(port int, autoPort bool) (*reservedAPIServerListener, error) {
	if port <= 0 || !autoPort {
		return nil, nil
	}

	var firstErr error
	for candidate := port; candidate <= 65535 && candidate < port+maxAutoPortAttempts; candidate++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", candidate))
		if err == nil {
			return &reservedAPIServerListener{
				listener: listener,
				port:     candidate,
			}, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr == nil {
		firstErr = fmt.Errorf("invalid preferred port %d", port)
	}
	return nil, fmt.Errorf("resolve open API server port from %d: %w", port, firstErr)
}

func (r *reservedAPIServerListener) Port() int {
	if r == nil {
		return 0
	}
	return r.port
}

func (r *reservedAPIServerListener) Take() net.Listener {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taken {
		return nil
	}
	r.taken = true
	return r.listener
}

func (r *reservedAPIServerListener) CloseIfUnused() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taken {
		return nil
	}
	r.taken = true
	return r.listener.Close()
}

// Run loads a workflow from factory.json and starts the factory via
// FactoryService. The CLI is a thin wrapper — all orchestration logic
// (file watcher, dashboard, API server, engine) lives in the service layer.
func Run(ctx context.Context, cfg RunConfig) error {
	return runWithFactoryServiceBuilder(ctx, cfg, nil)
}

func runWithFactoryServiceBuilder(ctx context.Context, cfg RunConfig, builder FactoryServiceBuilder) error {
	application, err := BuildApplication(ctx, cfg, builder, buildInvocationBootstrap)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

// Application is the already-constructed local-run graph consumed by the
// initializer lifecycle boundary.
type Application struct {
	cfg               RunConfig
	logger            *zap.Logger
	runner            RuntimeRunner
	invocationRequest *factoryapi.InvocationRequest
	invocationRunner  sessionInvocationRunner
	invocationMode    bool
	recordPath        resolvedRunRecordPath
	reservedAPIServer *reservedAPIServerListener
	dashboardReady    <-chan struct{}
}

// BuildApplication resolves run inputs and constructs the runtime graph without
// starting its transport, sidecars, or runtime loop.
func BuildApplication(
	ctx context.Context,
	cfg RunConfig,
	builder FactoryServiceBuilder,
	invocationBuilder InvocationBootstrapBuilder,
) (*Application, error) {
	cfg = normalizeRunInvocationMode(cfg)
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg, invocationRequest, invocationMode, recordPath, err := prepareRunConfig(cfg)
	if err != nil {
		return nil, err
	}

	mockWorkersConfig, err := loadMockWorkersConfig(cfg)
	if err != nil {
		return nil, err
	}

	var reservedAPIServer *reservedAPIServerListener
	if !invocationMode {
		reservedAPIServer, err = reserveAPIServerListener(cfg.Port, cfg.AutoPort)
		if err != nil {
			return nil, err
		}
	}
	requestedPort := cfg.Port
	closeReserved := func() {
		if reservedAPIServer != nil {
			if closeErr := reservedAPIServer.CloseIfUnused(); closeErr != nil {
				logger.Warn("release reserved API server listener failed", zap.Error(closeErr))
			}
		}
	}
	if reservedAPIServer != nil {
		cfg.Port = reservedAPIServer.Port()
	}
	emitNamedFactoryResolutionDiagnostics(cfg, logger)
	emitVerboseStartupDiagnostics(cfg, recordPath, requestedPort)

	if invocationMode {
		return buildInvocationApplication(ctx, cfg, logger, invocationRequest, recordPath, invocationBuilder, mockWorkersConfig)
	}

	dashboardReady := make(chan struct{})
	var dashboardReadyOnce sync.Once
	svcCfg := buildRunServiceConfig(cfg, logger, mockWorkersConfig, reservedAPIServer, dashboardReady, &dashboardReadyOnce)

	if builder == nil {
		builder = buildFactoryService
	}
	factorySvc, err := builder(ctx, svcCfg)
	if err != nil {
		closeReserved()
		return nil, err
	}
	if factorySvc == nil {
		closeReserved()
		return nil, fmt.Errorf("construct local runtime: builder returned nil runner")
	}

	return &Application{
		cfg: cfg, logger: logger, runner: factorySvc, recordPath: recordPath,
		reservedAPIServer: reservedAPIServer, dashboardReady: dashboardReady,
	}, nil
}

// Run starts the lifecycle for an application graph that has already been
// built successfully.
func (application *Application) Run(ctx context.Context) error {
	if application == nil {
		return fmt.Errorf("run local application: graph is required")
	}
	if application.reservedAPIServer != nil {
		defer func() {
			if err := application.reservedAPIServer.CloseIfUnused(); err != nil {
				application.logger.Warn("release reserved API server listener failed", zap.Error(err))
			}
		}()
	}
	if application.invocationMode {
		return runFactoryInvocation(
			ctx, application.cfg, *application.invocationRequest, application.invocationRunner,
		)
	}

	shouldOpenDashboard := emitStartupMessages(
		application.cfg, runtimeLogDiagnosticsForRunner(application.runner), interactiveOutput,
	)
	waitForDashboardOpen := func() {}
	if shouldOpenDashboard {
		waitForDashboardOpen = openDashboardWhenServerReady(
			ctx, application.cfg, application.dashboardReady, dashboardOpener,
		)
	}
	defer waitForDashboardOpen()

	return runFactoryServiceAndEmitResult(ctx, application.cfg, application.runner, application.recordPath)
}

func normalizeRunInvocationMode(cfg RunConfig) RunConfig {
	if !cfg.CleanInvocation {
		return cfg
	}
	cfg.SuppressDashboardRendering = true
	cfg.StartupOutput = nil
	cfg.OpenDashboard = false
	return cfg
}

func prepareRunConfig(cfg RunConfig) (RunConfig, *factoryapi.InvocationRequest, bool, resolvedRunRecordPath, error) {
	if cfg.ExecutionBaseDir == "" {
		if workingDirectory, err := os.Getwd(); err == nil && workingDirectory != "" {
			cfg.ExecutionBaseDir = workingDirectory
		}
	}

	if cfg.Bootstrap {
		if err := bootstrapFactory(cfg.Dir); err != nil {
			return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
		}
	}

	recordPath, err := resolveRecordPathForRun(cfg)
	if err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	cfg.RecordPath = recordPath.servicePath

	invocationRequest, invocationMode, err := resolveFactoryInvocationRequest(cfg)
	if err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	if err := validateInvocationOutputMode(cfg, invocationMode); err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	return cfg, invocationRequest, invocationMode, recordPath, nil
}

func loadMockWorkersConfig(cfg RunConfig) (*factoryconfig.MockWorkersConfig, error) {
	if !cfg.MockWorkersEnabled {
		return nil, nil
	}
	return factoryconfig.LoadMockWorkersConfig(cfg.MockWorkersConfigPath)
}

func resolveRecordPathForRun(cfg RunConfig) (resolvedRunRecordPath, error) {
	if cfg.DisableDefaultRecording && strings.TrimSpace(cfg.RecordPath) != "" {
		return resolvedRunRecordPath{}, fmt.Errorf("--no-record cannot be used with --record")
	}
	if strings.TrimSpace(cfg.RecordPath) != "" {
		return resolvedRunRecordPath{servicePath: cfg.RecordPath}, nil
	}
	if cfg.DisableDefaultRecording || strings.TrimSpace(cfg.ReplayPath) != "" {
		return resolvedRunRecordPath{}, nil
	}
	recordPath, err := defaultLiveRunRecordPathForHome(cfg.HomeDir)
	if err != nil {
		return resolvedRunRecordPath{}, fmt.Errorf("resolve default replay record path: %w", err)
	}
	return resolvedRunRecordPath{
		servicePath:   recordPath,
		reportedPath:  resolveDefaultSessionRecordPath(recordPath),
		autoGenerated: true,
	}, nil
}

func generateDefaultLiveRunRecordPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return generateDefaultLiveRunRecordPathForHome(homeDir)
}

func defaultLiveRunRecordPathForHome(homeDir string) (string, error) {
	if strings.TrimSpace(homeDir) == "" {
		return defaultLiveRunRecordPath()
	}
	return generateDefaultLiveRunRecordPathForHome(homeDir)
}

func generateDefaultLiveRunRecordPathForHome(homeDir string) (string, error) {
	if strings.TrimSpace(homeDir) == "" {
		return "", fmt.Errorf("resolve user home: home directory is required")
	}
	now := defaultLiveRunRecordTime()
	recordingID := fmt.Sprintf(
		"factory-session-%s-%s-%s.json",
		defaultRecordPathSessionToken,
		now.Format("150405"),
		defaultLiveRunRecordUUID(),
	)
	recordingsDir := defaultpaths.RecordingsDatedDir(defaultpaths.RecordingsRoot(homeDir), now)
	return filepath.Join(recordingsDir, recordingID), nil
}

func resolveDefaultSessionRecordPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return strings.ReplaceAll(path, defaultRecordPathSessionToken, defaultFactorySessionID)
}

func runtimeModeForRun(cfg RunConfig) interfaces.RuntimeMode {
	if cfg.Continuously {
		return interfaces.RuntimeModeService
	}
	return interfaces.RuntimeModeBatch
}

func buildRunServiceConfig(
	cfg RunConfig,
	logger *zap.Logger,
	mockWorkersConfig *factoryconfig.MockWorkersConfig,
	reservedAPIServer *reservedAPIServerListener,
	dashboardReady chan struct{},
	dashboardReadyOnce *sync.Once,
) *service.FactoryServiceConfig {
	var apiServerReady chan struct{}
	if cfg.Port > 0 {
		apiServerReady = dashboardReady
	}
	runtimeLogDir := cfg.RuntimeLogDir
	if strings.TrimSpace(runtimeLogDir) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
		runtimeLogDir = defaultpaths.RuntimeLogsRoot(cfg.HomeDir)
	}
	runtimeMetricsDir := cfg.RuntimeMetricsDir
	if strings.TrimSpace(runtimeMetricsDir) == "" && strings.TrimSpace(cfg.HomeDir) != "" {
		runtimeMetricsDir = defaultpaths.RuntimeMetricsRoot(cfg.HomeDir)
	}
	svcCfg := &service.FactoryServiceConfig{
		Dir:                               cfg.Dir,
		RunnerID:                          cfg.RunnerID,
		OperatorDefaults:                  cfg.OperatorDefaults,
		ExecutionBaseDir:                  cfg.ExecutionBaseDir,
		RuntimeMode:                       runtimeModeForRun(cfg),
		SystemConfigHomeDir:               cfg.HomeDir,
		Port:                              cfg.Port,
		Logger:                            logger,
		Verbose:                           cfg.Verbose,
		WorkFile:                          cfg.WorkFile,
		RecordPath:                        cfg.RecordPath,
		ReplayPath:                        cfg.ReplayPath,
		RuntimeLogDir:                     runtimeLogDir,
		RuntimeLogConfig:                  cfg.RuntimeLogConfig,
		RuntimeMetricsDir:                 runtimeMetricsDir,
		RuntimeMetricsConfig:              cfg.RuntimeMetricsConfig,
		WorkflowID:                        cfg.Workflow,
		MockWorkersConfig:                 mockWorkersConfig,
		APIServerStarter:                  runAPIServerStarter(reservedAPIServer, dashboardReady, dashboardReadyOnce),
		InvocationMetricsRecorder:         cfg.InvocationMetricsRecorder,
		InvocationSkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
		APIServerReady:                    apiServerReady,
	}
	if !cfg.SuppressDashboardRendering {
		svcCfg.SimpleDashboardRenderer = renderSimpleDashboard
	}
	return svcCfg
}

func runFactoryServiceAndEmitResult(
	ctx context.Context,
	cfg RunConfig,
	factorySvc factoryServiceRunner,
	recordPath resolvedRunRecordPath,
) error {
	startedAt := time.Now().UTC()
	if cfg.CleanInvocation {
		recordCleanInvocationAttempt()
	}
	err := factorySvc.Run(ctx)
	reportRecordingPathOnShutdown(cfg.StartupOutput, recordPath)
	if cfg.CleanInvocation {
		return emitCleanInvocationOutcome(ctx, cfg, factorySvc, err, startedAt)
	}
	if err != nil {
		return err
	}
	return nil
}

func emitVerboseStartupDiagnostics(cfg RunConfig, recordPath resolvedRunRecordPath, requestedPort int) {
	resolvedFactoryDir := resolveFactoryDirForDiagnostics(cfg.Dir)
	diagnosticsEnabled := terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose)
	clidiag.Printf(
		cfg.Diagnostics,
		diagnosticsEnabled,
		"run startup factoryDir=%q configuredDir=%q runtimeMode=%s workflow=%q mockWorkers=%t mockWorkersConfigPath=%q recording=%s runtimeLogDir=%q runtimeLogRoll=%s runtimeMetricsDir=%q runtimeMetricsRoll=%s dashboardPort=%d requestedDashboardPort=%d autoPort=%s",
		resolvedFactoryDir,
		cfg.Dir,
		runtimeModeForRun(cfg),
		workflowLabel(cfg.Workflow),
		cfg.MockWorkersEnabled,
		cfg.MockWorkersConfigPath,
		recordingDiagnostics(recordPath, cfg.ReplayPath),
		runtimeLogDirLabel(cfg.RuntimeLogDir),
		rollingPolicyDiagnostics(cfg.RuntimeLogConfig.MaxSize, cfg.RuntimeLogConfig.MaxBackups, cfg.RuntimeLogConfig.MaxAge, cfg.RuntimeLogConfig.Compress),
		runtimeMetricsDirLabel(cfg.RuntimeMetricsDir),
		rollingPolicyDiagnostics(cfg.RuntimeMetricsConfig.MaxSize, cfg.RuntimeMetricsConfig.MaxBackups, cfg.RuntimeMetricsConfig.MaxAge, cfg.RuntimeMetricsConfig.Compress),
		cfg.Port,
		requestedPort,
		autoPortDiagnostics(cfg.AutoPort, requestedPort, cfg.Port),
	)
	clidiag.Printf(cfg.Diagnostics, diagnosticsEnabled, "%s", cfg.OperatorDefaults.DiagnosticsLine())
}

func emitNamedFactoryResolutionDiagnostics(cfg RunConfig, logger *zap.Logger) {
	resolution := cfg.NamedFactoryResolution
	if resolution == nil {
		return
	}

	clidiag.Printf(
		cfg.Diagnostics,
		terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose),
		"run named-factory resolution name=%q source=%s resolvedFactoryDir=%q projectRoot=%q globalRoot=%q precedence=%s",
		resolution.Name,
		resolution.Source,
		resolution.FactoryDir,
		resolution.ProjectRoot,
		resolution.GlobalRoot,
		resolution.PrecedenceDecision,
	)
	logger.Info(
		"named factory resolved",
		zap.String("named_factory_name", resolution.Name),
		zap.String("named_factory_resolution_source", string(resolution.Source)),
		zap.String("named_factory_dir", resolution.FactoryDir),
		zap.String("named_factory_project_root", resolution.ProjectRoot),
		zap.String("named_factory_global_root", resolution.GlobalRoot),
		zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
	)
	if resolution.PrecedenceDecision == factoryconfig.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		logger.Info(
			"named factory precedence selected",
			zap.String("named_factory_name", resolution.Name),
			zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
			zap.String("named_factory_resolution_source", string(resolution.Source)),
		)
	}
}

func resolveFactoryDirForDiagnostics(dir string) string {
	resolved, err := factoryconfig.ResolveCurrentFactoryDir(dir)
	if err != nil {
		return "unresolved"
	}
	return resolved
}

func workflowLabel(workflow string) string {
	if strings.TrimSpace(workflow) == "" {
		return "all"
	}
	return workflow
}

func runtimeLogDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func runtimeMetricsDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func rollingPolicyDiagnostics(maxSize, maxBackups, maxAge int, compress bool) string {
	return fmt.Sprintf("size_mb=%d backups=%d age_days=%d compress=%t", maxSize, maxBackups, maxAge, compress)
}

func recordingDiagnostics(recordPath resolvedRunRecordPath, replayPath string) string {
	switch {
	case strings.TrimSpace(replayPath) != "":
		return "replay"
	case strings.TrimSpace(recordPath.servicePath) == "":
		return "disabled"
	case recordPath.autoGenerated:
		return "default"
	default:
		return "explicit"
	}
}

func autoPortDiagnostics(autoPort bool, requestedPort, resolvedPort int) string {
	switch {
	case requestedPort <= 0:
		return "dashboard-disabled"
	case !autoPort:
		return "disabled"
	case requestedPort == resolvedPort:
		return "preferred-available"
	default:
		return "fallback"
	}
}

func runAPIServerStarter(
	reservedAPIServer *reservedAPIServerListener,
	dashboardReady chan struct{},
	dashboardReadyOnce *sync.Once,
) service.APIServerStarter {
	markReady := func() {
		dashboardReadyOnce.Do(func() {
			close(dashboardReady)
		})
	}
	return func(ctx context.Context, runtime apisurface.APISurface, port int, l *zap.Logger) error {
		if reservedAPIServer != nil {
			listener := reservedAPIServer.Take()
			if listener == nil {
				return fmt.Errorf("reserved API server listener for port %d was already used", port)
			}
			return serveAPIServer(ctx, runtime, port, l, markReady, listener)
		}
		return startAPIServer(ctx, runtime, port, l, markReady)
	}
}

func renderSimpleDashboard(input service.SimpleDashboardRenderInput) {
	fmt.Print(dashboard.FormatSimpleDashboardWithRenderData(
		input.EngineState,
		input.RenderData,
		input.Now,
	))
}

func bindDashboardHost(cfg RunConfig) string {
	if strings.TrimSpace(cfg.BindHost) != "" {
		return cfg.BindHost
	}
	return "localhost"
}

// DashboardURL returns the embedded browser dashboard URL for the configured
// local factory server host and port.
func DashboardURL(host string, port int) string {
	if port <= 0 {
		return ""
	}
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	authority := net.JoinHostPort(host, strconv.Itoa(port))
	return "http://" + authority + "/dashboard/ui"
}

func emitStartupMessages(
	cfg RunConfig,
	runtimeLog service.RuntimeLogDiagnostics,
	isInteractive func(io.Writer) bool,
) bool {
	if cfg.StartupOutput == nil {
		return false
	}

	fmt.Fprintf(cfg.StartupOutput, "Factory initiated: %s\n", cfg.Dir)
	if cfg.Bootstrap {
		fmt.Fprintf(cfg.StartupOutput, "Factory directory ready: %s\n", cfg.Dir)
	}
	if cfg.Continuously {
		fmt.Fprintln(cfg.StartupOutput, "Runtime mode: continuous")
	}
	if strings.TrimSpace(runtimeLog.Path) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime log: %s\n", runtimeLog.Path)
		fmt.Fprintf(cfg.StartupOutput, "Runtime log start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.StartTimeUTC))
	}
	if strings.TrimSpace(runtimeLog.MetricsPath) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics: %s\n", runtimeLog.MetricsPath)
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.MetricsStartTimeUTC))
	}
	if cfg.Port <= 0 {
		fmt.Fprintln(cfg.StartupOutput, "Dashboard server disabled")
		return false
	}

	url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
	fmt.Fprintf(cfg.StartupOutput, "Dashboard URL: %s\n", url)
	if !cfg.OpenDashboard || !isInteractive(cfg.StartupOutput) {
		fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open disabled; open %s\n", url)
		return false
	}
	return true
}

func reportRecordingPathOnShutdown(output io.Writer, recordPath resolvedRunRecordPath) {
	if output == nil || !recordPath.autoGenerated || strings.TrimSpace(recordPath.reportedPath) == "" {
		return
	}
	fmt.Fprintf(output, "Recording saved: %s\n", recordPath.reportedPath)
}

func openDashboardWhenServerReady(
	ctx context.Context,
	cfg RunConfig,
	dashboardReady <-chan struct{},
	openDashboard func(context.Context, string) error,
) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(dashboardReadyTimeout)
		defer timer.Stop()

		url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
		ready := false
		select {
		case <-dashboardReady:
			ready = true
		default:
		}
		if !ready {
			select {
			case <-dashboardReady:
			case <-timer.C:
				fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: dashboard server did not become ready\nOpen the dashboard at %s\n", url)
				return
			case <-ctx.Done():
				return
			}
		}

		if err := openDashboard(ctx, url); err != nil {
			fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: %v\nOpen the dashboard at %s\n", err, url)
			return
		}
		fmt.Fprintf(cfg.StartupOutput, "Opening dashboard: %s\n", url)
	}()

	return func() {
		cancel()
		<-done
	}
}

func openURLInBrowser(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func isInteractiveOutput(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// LoadWorkFile reads a canonical FACTORY_REQUEST_BATCH from a JSON file.
func LoadWorkFile(path string) (interfaces.WorkRequest, error) {
	return batchload.LoadFromFile(path)
}

// CountTokenStates counts tokens by their state category based on place ID conventions.
// Place IDs follow the pattern '{work_type_id}:{state_value}'.
// Terminal states contain "completed", failed states contain "failed".
func CountTokenStates(snap *petri.MarkingSnapshot) (wip, completed, failed int) {
	for _, t := range snap.Tokens {
		placeID := t.PlaceID
		// Extract state from place ID (after the last ':').
		state := placeID
		if idx := strings.LastIndexByte(placeID, ':'); idx >= 0 {
			state = placeID[idx+1:]
		}

		switch {
		case isFailedState(state):
			failed++
		case isTerminalState(state):
			completed++
		default:
			wip++
		}
	}
	return
}

func isTerminalState(state string) bool {
	return state == completedPlaceIDSuffix
}

func isFailedState(state string) bool {
	return state == failedPlaceIDSuffix
}

// FormatDuration formats a duration as "Xm" or "Xh Ym".
func FormatDuration(d time.Duration) string {
	return dashboard.FormatDuration(d)
}
