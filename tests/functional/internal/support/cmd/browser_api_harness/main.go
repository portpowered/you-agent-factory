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
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

const startupTimeout = 10 * time.Second

type readyPayload struct {
	APIPort   int    `json:"apiPort"`
	APIOrigin string `json:"apiOrigin"`
	SessionID string `json:"sessionId"`
}

func main() {
	var apiPort int
	var executionBaseDir string
	var factoryDir string
	var requestID string
	var workflowFixture string
	var workflowName string

	flag.IntVar(&apiPort, "api-port", 0, "port for the real backend API server")
	flag.StringVar(&executionBaseDir, "execution-base-dir", "", "project root for durable workflow resolution")
	flag.StringVar(&factoryDir, "factory-dir", "", "factory directory for the live dashboard session")
	flag.StringVar(&requestID, "request-id", "req-browser-runtime-001", "durable session request id")
	flag.StringVar(&workflowFixture, "workflow-fixture", "", "runtime testdata workflow fixture filename")
	flag.StringVar(&workflowName, "workflow-name", "", "workflow name exposed under .claude/workflows")
	flag.Parse()

	if apiPort <= 0 {
		fatalf("expected --api-port > 0")
	}
	if factoryDir == "" {
		fatalf("expected --factory-dir")
	}
	if workflowFixture == "" {
		fatalf("expected --workflow-fixture")
	}
	if workflowName == "" {
		fatalf("expected --workflow-name")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := zap.NewNop()
	projectRoot, cleanupProjectRoot, err := prepareWorkflowProjectRoot(executionBaseDir, workflowFixture, workflowName)
	if err != nil {
		fatalf("prepare workflow project root: %v", err)
	}
	defer cleanupProjectRoot()

	var handler http.Handler
	readyCh := make(chan struct{})
	serviceCfg := &service.FactoryServiceConfig{
		Dir:                      factoryDir,
		ExecutionBaseDir:         projectRoot,
		Logger:                   logger,
		MockWorkersConfig:        config.NewEmptyMockWorkersConfig(),
		Port:                     apiPort,
		RuntimeFileLoggingPolicy: service.RuntimeFileLoggingPolicyDisabled,
		APIServerStarter: func(ctx context.Context, surface apisurface.APISurface, port int, l *zap.Logger) error {
			handler = api.NewServer(surface, port, l).Handler()
			close(readyCh)
			<-ctx.Done()
			return nil
		},
	}

	svc, err := compose.InjectFactoryService(ctx, serviceCfg)
	if err != nil {
		fatalf("InjectFactoryService: %v", err)
	}

	serviceDone := make(chan error, 1)
	go func() {
		serviceDone <- svc.Run(ctx)
	}()

	select {
	case <-readyCh:
	case err := <-serviceDone:
		fatalf("service exited before API ready: %v", err)
	case <-time.After(startupTimeout):
		fatalf("timed out waiting for API handler readiness")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", apiPort))
	if err != nil {
		fatalf("listen on api port %d: %v", apiPort, err)
	}
	httpServer := &http.Server{
		Handler: handler,
	}
	httpDone := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if err == nil || err == http.ErrServerClosed {
			httpDone <- nil
			return
		}
		httpDone <- err
	}()

	waitForHTTPReady(apiPort)

	started, err := svc.StartDurableFactorySessionSync(ctx, factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
	})
	if err != nil {
		fatalf("StartDurableFactorySessionSync: %v", err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(readyPayload{
		APIPort:   apiPort,
		APIOrigin: fmt.Sprintf("http://127.0.0.1:%d", apiPort),
		SessionID: started.SessionId,
	}); err != nil {
		fatalf("encode ready payload: %v", err)
	}

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), startupTimeout)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	select {
	case <-httpDone:
	case <-time.After(startupTimeout):
	}
	select {
	case <-serviceDone:
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

func strPtr(value string) *string {
	return &value
}
