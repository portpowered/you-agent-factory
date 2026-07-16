package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/internal/functionalhost"
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

func main() {
	cfg := parseHarnessConfig()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runHarness(ctx, cfg, os.Stdout); err != nil {
		fatalf("%v", err)
	}
}

func runHarness(ctx context.Context, cfg harnessConfig, stdout io.Writer) (runErr error) {
	projectRoot, cleanupProjectRoot, err := prepareWorkflowProjectRoot(
		cfg.executionBaseDir,
		cfg.workflowFixture,
		cfg.workflowName,
	)
	if err != nil {
		return fmt.Errorf("prepare workflow project root: %w", err)
	}
	defer cleanupProjectRoot()

	host, err := functionalhost.StartFunctionalHTTPServer(ctx, functionalhost.FunctionalHTTPServerConfig{
		Address:          fmt.Sprintf("127.0.0.1:%d", cfg.apiPort),
		ExecutionBaseDir: projectRoot,
		FactoryDir:       cfg.factoryDir,
		UseMockWorkers:   true,
	})
	if err != nil {
		return fmt.Errorf("start composed functional HTTP host: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), startupTimeout)
		defer shutdownCancel()
		runErr = errors.Join(runErr, host.Shutdown(shutdownCtx))
	}()

	startRequest := factoryapi.FactorySessionExecutionRequest{
		RequestId: cfg.requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(cfg.workflowName),
		},
	}
	sessionID, err := startSession(ctx, host.URL(), cfg.startMode, startRequest)
	if err != nil {
		return fmt.Errorf("start durable factory session (%s): %w", cfg.startMode, err)
	}

	if err := json.NewEncoder(stdout).Encode(readyPayload{
		APIPort:   cfg.apiPort,
		APIOrigin: host.URL(),
		SessionID: sessionID,
	}); err != nil {
		return fmt.Errorf("encode ready payload: %w", err)
	}

	<-ctx.Done()
	return nil
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

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func startSession(
	ctx context.Context,
	baseURL string,
	startMode string,
	request factoryapi.FactorySessionExecutionRequest,
) (string, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal session request: %w", err)
	}
	path := "/factory-sessions/sync"
	if startMode == "async" {
		path = "/factory-sessions/async"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: startupTimeout}).Do(req)
	if err != nil {
		return "", fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("POST %s: status %s", path, resp.Status)
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		return "", fmt.Errorf("decode session response: %w", err)
	}
	return started.SessionId, nil
}

func strPtr(value string) *string {
	return &value
}
