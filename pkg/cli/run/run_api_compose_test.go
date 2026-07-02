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
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

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

func setInitializerAPIRuntimeBuilder(t *testing.T) func() {
	t.Helper()

	originalBuilder := buildFactoryService
	buildFactoryService = func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
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
