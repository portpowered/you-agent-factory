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
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/api"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboard"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

// RunConfig holds parameters for the run command.
type RunConfig struct {
	Workflow     string
	Continuously bool
	WorkFile     string
	Dir          string
	// ExecutionBaseDir overrides the base directory used to resolve relative
	// runtime execution paths. Empty defaults to the caller's current working
	// directory for CLI-style runs.
	ExecutionBaseDir string
	Bootstrap        bool
	Port             int
	// AutoPort resolves Port to the next available local TCP port when the
	// preferred port is unavailable. Explicit port selections should leave this
	// false so operator intent is preserved.
	AutoPort   bool
	RecordPath string
	ReplayPath string
	// RuntimeLogDir overrides the service-owned structured runtime log
	// directory. Empty uses the service default under the user's home directory.
	RuntimeLogDir string
	// RuntimeLogConfig controls service-owned structured runtime log rotation.
	RuntimeLogConfig logging.RuntimeLogConfig
	// MockWorkersEnabled enables deterministic mock-worker execution. When
	// true and MockWorkersConfigPath is empty, the runtime uses the default
	// accept behavior for all worker dispatches.
	MockWorkersEnabled    bool
	MockWorkersConfigPath string
	Verbose               bool
	// SuppressDashboardRendering disables the simple stdout dashboard while
	// preserving the normal service-layer run path.
	SuppressDashboardRendering bool
	// OpenDashboard attempts to open the embedded dashboard URL in a browser.
	OpenDashboard bool
	// StartupOutput receives human-facing startup messages. Nil suppresses
	// startup output for programmatic callers and tests.
	StartupOutput io.Writer
	Logger        *zap.Logger
}

type factoryServiceRunner interface {
	Run(ctx context.Context) error
}

var buildFactoryService = func(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (factoryServiceRunner, error) {
	return service.BuildFactoryService(ctx, cfg)
}

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

const dashboardReadyTimeout = 5 * time.Second
const maxAutoPortAttempts = 100

type reservedAPIServerListener struct {
	listener net.Listener
	port     int
	taken    bool
	mu       sync.Mutex
}

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
	srv := api.NewServer(runtime, port, logger)
	return srv.Serve(ctx, listener)
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
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.ExecutionBaseDir == "" {
		if workingDirectory, err := os.Getwd(); err == nil && workingDirectory != "" {
			cfg.ExecutionBaseDir = workingDirectory
		}
	}

	if cfg.Bootstrap {
		if err := bootstrapFactory(cfg.Dir); err != nil {
			return err
		}
	}

	var mockWorkersConfig *factoryconfig.MockWorkersConfig
	if cfg.MockWorkersEnabled {
		loadedMockWorkersConfig, err := factoryconfig.LoadMockWorkersConfig(cfg.MockWorkersConfigPath)
		if err != nil {
			return err
		}
		mockWorkersConfig = loadedMockWorkersConfig
	}

	reservedAPIServer, err := reserveAPIServerListener(cfg.Port, cfg.AutoPort)
	if err != nil {
		return err
	}
	if reservedAPIServer != nil {
		defer reservedAPIServer.CloseIfUnused()
		cfg.Port = reservedAPIServer.Port()
	}

	dashboardReady := make(chan struct{})
	var dashboardReadyOnce sync.Once
	svcCfg := buildRunServiceConfig(cfg, logger, mockWorkersConfig, reservedAPIServer, dashboardReady, &dashboardReadyOnce)

	factorySvc, err := buildFactoryService(ctx, svcCfg)
	if err != nil {
		return err
	}

	shouldOpenDashboard := emitStartupMessages(cfg)
	waitForDashboardOpen := func() {}
	if shouldOpenDashboard {
		waitForDashboardOpen = openDashboardWhenServerReady(ctx, cfg, dashboardReady)
	}
	defer waitForDashboardOpen()

	return factorySvc.Run(ctx)
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
	svcCfg := &service.FactoryServiceConfig{
		Dir:               cfg.Dir,
		ExecutionBaseDir:  cfg.ExecutionBaseDir,
		RuntimeMode:       runtimeModeForRun(cfg),
		Port:              cfg.Port,
		Logger:            logger,
		Verbose:           cfg.Verbose,
		WorkFile:          cfg.WorkFile,
		RecordPath:        cfg.RecordPath,
		ReplayPath:        cfg.ReplayPath,
		RuntimeLogDir:     cfg.RuntimeLogDir,
		RuntimeLogConfig:  cfg.RuntimeLogConfig,
		WorkflowID:        cfg.Workflow,
		MockWorkersConfig: mockWorkersConfig,
		APIServerStarter:  runAPIServerStarter(reservedAPIServer, dashboardReady, dashboardReadyOnce),
	}
	if !cfg.SuppressDashboardRendering {
		svcCfg.SimpleDashboardRenderer = renderSimpleDashboard
	}
	return svcCfg
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

// DashboardURL returns the embedded browser dashboard URL for the configured
// local factory server port.
func DashboardURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/dashboard/ui", port)
}

func emitStartupMessages(cfg RunConfig) bool {
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
	if cfg.Port <= 0 {
		fmt.Fprintln(cfg.StartupOutput, "Dashboard server disabled")
		return false
	}

	url := DashboardURL(cfg.Port)
	fmt.Fprintf(cfg.StartupOutput, "Dashboard URL: %s\n", url)
	if !cfg.OpenDashboard || !interactiveOutput(cfg.StartupOutput) {
		fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open disabled; open %s\n", url)
		return false
	}
	return true
}

func openDashboardWhenServerReady(ctx context.Context, cfg RunConfig, dashboardReady <-chan struct{}) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(dashboardReadyTimeout)
		defer timer.Stop()

		url := DashboardURL(cfg.Port)
		select {
		case <-dashboardReady:
			if err := dashboardOpener(ctx, url); err != nil {
				fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: %v\n", err)
				fmt.Fprintf(cfg.StartupOutput, "Open the dashboard at %s\n", url)
				return
			}
			fmt.Fprintf(cfg.StartupOutput, "Opening dashboard: %s\n", url)
		case <-timer.C:
			fmt.Fprintln(cfg.StartupOutput, "Dashboard auto-open unavailable: dashboard server did not become ready")
			fmt.Fprintf(cfg.StartupOutput, "Open the dashboard at %s\n", url)
		case <-ctx.Done():
		}
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
	data, err := os.ReadFile(path)
	if err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("read %s: %w", path, err)
	}
	req, err := factory.ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return req, nil
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
	return state == string(interfaces.StateCompleted)
}

func isFailedState(state string) bool {
	return state == string(interfaces.StateFailed)
}

// FormatDuration formats a duration as "Xm" or "Xh Ym".
func FormatDuration(d time.Duration) string {
	return dashboard.FormatDuration(d)
}
