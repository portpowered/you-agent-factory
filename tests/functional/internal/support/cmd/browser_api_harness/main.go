package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"go.uber.org/zap"
)

const startupTimeout = 10 * time.Second

type readyPayload struct {
	APIPort   int    `json:"apiPort"`
	APIOrigin string `json:"apiOrigin"`
	SessionID string `json:"sessionId"`
}

type harnessConfig struct {
	apiPort          int
	executionBaseDir string
	factoryDir       string
	requestID        string
	startMode        string
	workflowFixture  string
	workflowName     string
}

type runningHTTPServer struct {
	server *http.Server
	done   <-chan error
}

func main() {
	cfg := parseHarnessConfig()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := zap.NewNop()
	projectRoot, cleanupProjectRoot, err := prepareWorkflowProjectRoot(
		cfg.executionBaseDir,
		cfg.workflowFixture,
		cfg.workflowName,
	)
	if err != nil {
		fatalf("prepare workflow project root: %v", err)
	}
	defer cleanupProjectRoot()

	svc, handler, serviceDone, err := startFactoryService(ctx, logger, cfg, projectRoot)
	if err != nil {
		fatalf("InjectAPITransport: %v", err)
	}

	httpServer, err := startHTTPServer(cfg.apiPort, handler)
	if err != nil {
		fatalf("start http server: %v", err)
	}

	waitForHTTPReady(cfg.apiPort)

	startRequest := factoryapi.FactorySessionExecutionRequest{
		RequestId: cfg.requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(cfg.workflowName),
		},
	}
	sessionID, err := startSession(ctx, svc, cfg.startMode, startRequest)
	if err != nil {
		fatalf("start durable factory session (%s): %v", cfg.startMode, err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(readyPayload{
		APIPort:   cfg.apiPort,
		APIOrigin: fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort),
		SessionID: sessionID,
	}); err != nil {
		fatalf("encode ready payload: %v", err)
	}

	<-ctx.Done()
	shutdownHarness(httpServer, serviceDone)
}

func parseHarnessConfig() harnessConfig {
	cfg := harnessConfig{
		requestID: "req-browser-runtime-001",
		startMode: "sync",
	}
	flag.IntVar(&cfg.apiPort, "api-port", 0, "port for the real backend API server")
	flag.StringVar(&cfg.executionBaseDir, "execution-base-dir", "", "project root for durable workflow resolution")
	flag.StringVar(&cfg.factoryDir, "factory-dir", "", "factory directory for the live dashboard session")
	flag.StringVar(&cfg.requestID, "request-id", cfg.requestID, "durable session request id")
	flag.StringVar(&cfg.startMode, "start-mode", cfg.startMode, "durable session start mode: sync or async")
	flag.StringVar(&cfg.workflowFixture, "workflow-fixture", "", "runtime testdata workflow fixture filename")
	flag.StringVar(&cfg.workflowName, "workflow-name", "", "workflow name exposed under .claude/workflows")
	flag.Parse()
	validateHarnessConfig(cfg)
	return cfg
}

func validateHarnessConfig(cfg harnessConfig) {
	if cfg.apiPort <= 0 {
		fatalf("expected --api-port > 0")
	}
	if cfg.factoryDir == "" {
		fatalf("expected --factory-dir")
	}
	if cfg.workflowFixture == "" {
		fatalf("expected --workflow-fixture")
	}
	if cfg.workflowName == "" {
		fatalf("expected --workflow-name")
	}
	if cfg.startMode != "sync" && cfg.startMode != "async" {
		fatalf("expected --start-mode to be sync or async")
	}
}

func startFactoryService(
	ctx context.Context,
	logger *zap.Logger,
	cfg harnessConfig,
	projectRoot string,
) (*runtimehost.Host, http.Handler, <-chan error, error) {
	var handler http.Handler
	readyCh := make(chan struct{})
	serviceCfg := &initializer.Config{
		Dir:                      cfg.factoryDir,
		ExecutionBaseDir:         projectRoot,
		Logger:                   logger,
		MockWorkersConfig:        config.NewEmptyMockWorkersConfig(),
		Port:                     cfg.apiPort,
		RuntimeFileLoggingPolicy: runtimehost.RuntimeFileLoggingPolicyDisabled,
		APIServerStarter: func(ctx context.Context, surface apisurface.APISurface, port int, l *zap.Logger) error {
			handler = api.NewServer(surface, port, l).Handler()
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}

	transport, err := compose.InjectAPITransport(ctx, serviceCfg)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := transport.Host.CompatibilityServiceShell()
	if svc == nil {
		return nil, nil, nil, fmt.Errorf("initializer API transport missing session runtime host")
	}

	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- transport.Run(ctx)
	}()

	if err := waitForAPIHandler(readyCh, serviceDone); err != nil {
		return nil, nil, nil, err
	}

	return svc, handler, serviceDone, nil
}

func waitForAPIHandler(readyCh <-chan struct{}, serviceDone <-chan error) error {
	select {
	case <-readyCh:
		return nil
	case err := <-serviceDone:
		return fmt.Errorf("service exited before API ready: %w", err)
	case <-time.After(startupTimeout):
		return fmt.Errorf("timed out waiting for API handler readiness")
	}
}

func startHTTPServer(apiPort int, handler http.Handler) (*runningHTTPServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", apiPort))
	if err != nil {
		return nil, fmt.Errorf("listen on api port %d: %w", apiPort, err)
	}
	httpServer := &http.Server{Handler: handler}
	httpDone := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err == nil || err == http.ErrServerClosed {
			httpDone <- nil
			return
		}
		httpDone <- err
	}()
	return &runningHTTPServer{
		server: httpServer,
		done:   httpDone,
	}, nil
}

func shutdownHarness(httpServer *runningHTTPServer, serviceDone <-chan error) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer shutdownCancel()
	_ = httpServer.server.Shutdown(shutdownCtx)
	waitForExit(httpServer.done)
	waitForExit(serviceDone)
}

func waitForExit(done <-chan error) {
	select {
	case <-done:
	case <-time.After(startupTimeout):
	}
}

func prepareWorkflowProjectRoot(
	configuredRoot, workflowFixture, workflowName string,
) (string, func(), error) {
	if configuredRoot != "" {
		if err := installWorkflowFixture(configuredRoot, workflowFixture, workflowName); err != nil {
			return "", nil, err
		}
		return configuredRoot, func() {}, nil
	}

	projectRoot, err := os.MkdirTemp("", "browser-api-harness-*")
	if err != nil {
		return "", nil, err
	}
	if err := installWorkflowFixture(projectRoot, workflowFixture, workflowName); err != nil {
		_ = os.RemoveAll(projectRoot)
		return "", nil, err
	}
	return projectRoot, func() {
		_ = os.RemoveAll(projectRoot)
	}, nil
}

func installWorkflowFixture(projectRoot, workflowFixture, workflowName string) error {
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join("pkg", "orchestrators", "javascript", "runtime", "testdata", workflowFixture))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600)
}

func waitForHTTPReady(apiPort int) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(startupTimeout)
	url := fmt.Sprintf("http://127.0.0.1:%d/status", apiPort)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	fatalf("timed out waiting for %s", url)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func startSession(
	ctx context.Context,
	svc *runtimehost.Host,
	startMode string,
	request factoryapi.FactorySessionExecutionRequest,
) (string, error) {
	if startMode == "async" {
		started, err := svc.StartDurableFactorySessionAsync(ctx, request)
		if err != nil {
			return "", err
		}
		return started.SessionId, nil
	}

	started, err := svc.StartDurableFactorySessionSync(ctx, request)
	if err != nil {
		return "", err
	}
	return started.SessionId, nil
}

func strPtr(value string) *string {
	return &value
}
