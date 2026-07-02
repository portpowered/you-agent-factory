package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestInjectRuntimeRunner_MatchesServiceBuildFactoryServiceInvalidConfig(t *testing.T) {
	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir: t.TempDir(),
	}

	_, errWire := compose.InjectRuntimeRunner(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg)

	if errWire == nil {
		t.Fatal("expected InjectRuntimeRunner to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected service.BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errWire.Error() {
		t.Fatalf("service.BuildFactoryService error = %q, want %q", errService, errWire)
	}
}

func TestRun_InitializerStartupFailureReturnsActionableError(t *testing.T) {
	restoreBuilder := setInitializerRuntimeBuilder(t)
	defer restoreBuilder()

	dir := t.TempDir()
	err := Run(context.Background(), RunConfig{
		Dir:                        dir,
		Port:                       0,
		SuppressDashboardRendering: true,
		Logger:                     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected Run to fail without factory.json")
	}

	_, errService := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if errService == nil {
		t.Fatal("expected service.BuildFactoryService to fail without factory.json")
	}
	if err.Error() != errService.Error() {
		t.Fatalf("Run startup error = %q, want %q", err, errService)
	}
}

func TestInjectRuntimeRunner_RunCompletesBatchWithMockWorkers(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeCLITransportTestWorkerAgentsMD(t, dir, "worker-a")
	writeCLITransportTestWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	taskPath := filepath.Join(t.TempDir(), "work.json")
	workRequest := interfaces.WorkRequest{
		Type: interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "cli-transport-smoke",
			WorkID:     "cli-transport-smoke",
			WorkTypeID: "task",
			TraceID:    "cli-transport-smoke-trace",
			Payload:    "exercise initializer-backed CLI batch run",
		}},
	}
	workData, err := json.Marshal(workRequest)
	if err != nil {
		t.Fatalf("marshal work file: %v", err)
	}
	if err := os.WriteFile(taskPath, workData, 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	restoreBuilder := setInitializerRuntimeBuilder(t)
	defer restoreBuilder()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, RunConfig{
			Dir:                        dir,
			WorkFile:                   taskPath,
			MockWorkersEnabled:         true,
			SuppressDashboardRendering: true,
			Port:                       0,
			Logger:                     zap.NewNop(),
		})
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for initializer-backed batch run")
	}
}

func TestInjectRuntimeRunner_CleanInvocationReturnsPrimaryResult(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)

	restoreBuilder := setInitializerRuntimeBuilder(t)
	defer restoreBuilder()

	output, err := runWithCapturedStdout(t, RunConfig{
		Dir:                     dir,
		Port:                    0,
		WorkFile:                workFile,
		MockWorkersEnabled:      true,
		CleanInvocation:         true,
		DisableDefaultRecording: true,
		Logger:                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if output != "mock worker accepted" {
		t.Fatalf("stdout = %q, want primary clean invocation output", output)
	}
}

func TestBuildFactoryService_OverrideableWithoutWire(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return errors.New("stub run")
			},
		}, nil
	}

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if err != nil {
		t.Fatalf("override buildFactoryService: %v", err)
	}

	err = Run(context.Background(), RunConfig{
		Dir:                        t.TempDir(),
		SuppressDashboardRendering: true,
	})
	if err == nil || err.Error() != "stub run" {
		t.Fatalf("Run with stub builder = %v, want stub run", err)
	}
}

func TestInjectAPITransport_MatchesServiceBuildFactoryServiceInvalidConfig(t *testing.T) {
	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir: t.TempDir(),
	}

	_, errAPI := compose.InjectAPITransport(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg)

	if errAPI == nil {
		t.Fatal("expected InjectAPITransport to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected service.BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errAPI.Error() {
		t.Fatalf("service.BuildFactoryService error = %q, want %q", errService, errAPI)
	}
}

func TestInjectAPITransport_RunServesSessionModelAndFactoryEndpoints(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeAPITransportTestWorkerAgentsMD(t, dir, "worker-a")
	writeAPITransportTestWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}

	originalBuilder := buildFactoryService
	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	defer func() {
		buildFactoryService = originalBuilder
		startAPIServer = originalStartAPIServer
		serveFactoryAPIServer = originalServeFactoryAPIServer
	}()

	serveFactoryAPIServer = compose.ServeAPIServer
	startAPIServer = func(
		ctx context.Context,
		runtime apisurface.APISurface,
		bindPort int,
		logger *zap.Logger,
		markReady func(),
	) error {
		apiListener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", bindPort))
		if listenErr != nil {
			return listenErr
		}
		return serveAPIServer(ctx, runtime, bindPort, logger, markReady, apiListener)
	}

	buildFactoryService = func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return compose.InjectRuntimeRunner(ctx, cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- Run(ctx, RunConfig{
			Dir:                        dir,
			Continuously:               true,
			MockWorkersEnabled:         true,
			Port:                       port,
			SuppressDashboardRendering: true,
			Logger:                     zap.NewNop(),
		})
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForAPIComposeEndpoint(t, baseURL+"/models", http.StatusOK)
	waitForAPIComposeEndpoint(t, baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/factory", http.StatusOK)
	waitForAPIComposeEndpoint(t, baseURL+"/factory-sessions", http.StatusOK)

	var status factoryapi.StatusResponse
	waitForAPIComposeJSON(t, baseURL+"/status", &status)
	if status.FactoryState == "" {
		t.Fatalf("GET /status factory_state empty after polling %s/status", baseURL)
	}

	cancel()
	select {
	case err := <-runErrCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Run to exit after cancel")
	}
}

func waitForAPIComposeEndpoint(t *testing.T, url string, wantStatus int) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s body: %v", url, readErr)
		}
		if resp.StatusCode != wantStatus {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if resp.StatusCode == http.StatusOK && len(body) == 0 {
			t.Fatalf("GET %s returned empty body", url)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
		return
	}
	t.Fatalf("GET %s did not return %d within timeout", url, wantStatus)
}

func waitForAPIComposeJSON(t *testing.T, url string, dest any) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s body: %v", url, readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, dest); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
		return
	}
	t.Fatalf("GET %s did not return decodable JSON within timeout", url)
}

func writeAPITransportTestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeAPITransportTestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeCLITransportTestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeCLITransportTestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

// setInitializerRuntimeBuilder wires Run through the same initializer transport
// composition used by cmd/factory/main.go (InjectRuntimeRunner).
func setInitializerRuntimeBuilder(t *testing.T) func() {
	t.Helper()

	originalBuilder := buildFactoryService
	buildFactoryService = func(ctx context.Context, cfg *initializer.Config) (factoryServiceRunner, error) {
		runner, err := compose.InjectRuntimeRunner(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if runner == nil {
			return nil, errors.New("initializer runtime runner missing")
		}
		return runner, nil
	}
	return func() {
		buildFactoryService = originalBuilder
	}
}
