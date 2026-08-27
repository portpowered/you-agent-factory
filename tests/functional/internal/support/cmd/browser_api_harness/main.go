package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	startupTimeout           = 10 * time.Second
	projectWorkflowDirectory = ".claude/workflows"
	startupDiagnosticPrefix  = "[browser-api-harness] phase="
)

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

func main() {
	cfg := parseHarnessConfig()
	startupPhase("process-started")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	startupPhase("workflow-project-preparation-started")
	projectRoot, cleanupProjectRoot, err := prepareWorkflowProjectRoot(
		cfg.executionBaseDir,
		cfg.workflowFixture,
		cfg.workflowName,
	)
	if err != nil {
		fatalf("prepare workflow project root: %v", err)
	}
	defer cleanupProjectRoot()
	startupPhase("workflow-project-preparation-complete")

	ready := make(chan struct{})
	edges := serviceedges.Edges{
		APIServerStarter: func(serverCtx context.Context, request platformhttpserver.StartRequest) error {
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: cfg.apiPort})
			}
			return serveInjectedHTTP(serverCtx, cfg.apiPort, request.Handler, ready)
		},
	}
	startupPhase("root-process-build-started")
	process, err := support.BuildProcessWithContext(ctx, edges)
	if err != nil {
		fatalf("build root process: %v", err)
	}
	startupPhase("root-process-build-complete")
	processDone := make(chan error, 1)
	go func() {
		startupPhase("root-process-execution-started")
		processDone <- process.Execute(root.Input{
			Args: []string{
				"you", "run",
				"--dir", cfg.factoryDir,
				"--continuously",
				"--with-server",
				"--server", fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort),
				"--with-mock-workers",
				"--quiet",
				"--no-record",
			},
			Env:              os.Environ(),
			Stdin:            os.Stdin,
			Stdout:           io.Discard,
			Stderr:           os.Stderr,
			Context:          ctx,
			WorkingDirectory: projectRoot,
		})
	}()
	select {
	case <-ready:
	case err := <-processDone:
		fatalf("root process exited before API ready: %v", err)
	case <-time.After(startupTimeout):
		fatalf("timed out waiting for root process API")
	}

	startupPhase("api-bound")
	waitForHTTPReady(cfg.apiPort)
	startupPhase("api-ready")

	startRequest := factoryapi.FactorySessionExecutionRequest{
		RequestId: cfg.requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(cfg.workflowName),
		},
	}
	sessionID, err := startSession(ctx, cfg.apiPort, cfg.startMode, startRequest)
	if err != nil {
		fatalf("start durable factory session (%s): %v", cfg.startMode, err)
	}
	startupPhase("durable-session-started")

	if err := json.NewEncoder(os.Stdout).Encode(readyPayload{
		APIPort:   cfg.apiPort,
		APIOrigin: fmt.Sprintf("http://127.0.0.1:%d", cfg.apiPort),
		SessionID: sessionID,
	}); err != nil {
		fatalf("encode ready payload: %v", err)
	}
	startupPhase("ready-payload-written")

	<-ctx.Done()
	waitForExit(processDone)
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

func startupPhase(phase string) {
	_, _ = fmt.Fprintf(os.Stderr, "%s%s\n", startupDiagnosticPrefix, phase)
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

func serveInjectedHTTP(ctx context.Context, apiPort int, handler http.Handler, ready chan<- struct{}) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", apiPort))
	if err != nil {
		return fmt.Errorf("listen on api port %d: %w", apiPort, err)
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
	close(ready)
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	return <-httpDone
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
	workflowDir := filepath.Join(projectRoot, projectWorkflowDirectory)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join("tests", "fixtures", "javascript_runtime", workflowFixture))
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
	apiPort int,
	startMode string,
	request factoryapi.FactorySessionExecutionRequest,
) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/factory-sessions/%s", apiPort, startMode)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		return "", fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, payload)
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		return "", err
	}
	return started.SessionId, nil
}

func strPtr(value string) *string {
	return &value
}
