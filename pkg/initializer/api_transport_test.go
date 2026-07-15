package initializer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func TestInitializeAPITransport_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.Config{Dir: t.TempDir()}

	_, errInit := initializer.InitializeAPITransport(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, service.FactoryServiceConfigFromRuntimeHost(cfg))

	if errInit == nil {
		t.Fatal("expected InitializeAPITransport to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("InitializeAPITransport error = %q, want %q", errInit, errService)
	}
}

func TestInitializeAPITransport_ComposesHandlerDependenciesWithoutFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	transport, err := initializer.InitializeAPITransport(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("InitializeAPITransport: %v", err)
	}
	if transport.Services == nil || transport.Host == nil {
		t.Fatal("expected initializer transport bundle")
	}
	if transport.Host.SessionAPISurface() == nil {
		t.Fatal("expected session runtime host with attached collaborators")
	}
	if shell := transport.Host.CompatibilityServiceShell(); shell == nil {
		t.Fatal("expected compatibility service shell for legacy harness callbacks")
	}
	if transport.Host.RuntimeHost() == nil {
		t.Fatal("expected authoritative runtime host behind session runtime shell")
	}
	surface := transport.SessionAPISurface()
	if surface == nil {
		t.Fatal("expected session API surface")
	}
	if _, ok := surface.(apisurface.SessionAPISurface); !ok {
		t.Fatal("expected SessionAPISurface implementation")
	}
	assertComposedDurableCapabilities(t, surface)
	if _, ok := surface.(*runtimehost.Host); ok {
		t.Fatal("API transport surface must not be the runtime lifecycle host")
	}
	if _, ok := surface.(*service.FactoryService); ok {
		t.Fatal("API transport surface must not be the FactoryService compatibility facade")
	}
	if transport.Services.Models == nil || transport.Services.FactoryDefinition == nil {
		t.Fatal("expected initializer-produced model and factory-definition services")
	}
	if transport.Services.Models != transport.Host.ModelService() {
		t.Fatal("expected API transport and runtime host to share one model service")
	}
}

func assertComposedDurableCapabilities(t *testing.T, surface apisurface.SessionAPISurface) {
	t.Helper()
	if _, ok := surface.(apisurface.DurableSessionLifecycleAPI); !ok {
		t.Fatal("expected composed surface to preserve durable lifecycle routes")
	}
	if _, ok := surface.(apisurface.DurableSessionProjectionAPI); !ok {
		t.Fatal("expected composed surface to preserve durable projection routes")
	}
}

func TestInitializeAPITransport_ServesSessionModelAndFactoryEndpoints(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeInitializerAPITestWorkerAgentsMD(t, dir, "worker-a")
	writeInitializerAPITestWorkstationAgentsMD(t, dir, "process")
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	var startedSurface apisurface.APISurface
	svcCfg := &initializer.Config{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Port:              port,
		Logger:            zap.NewNop(),
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, bindPort int, l *zap.Logger) error {
			startedSurface = runtime
			apiListener, listenErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", bindPort))
			if listenErr != nil {
				return listenErr
			}
			close(ready)
			return compose.ServeAPIServer(ctx, runtime, bindPort, l, apiListener)
		},
	}

	transport, err := compose.InjectAPITransport(ctx, svcCfg)
	if err != nil {
		t.Fatalf("InjectAPITransport: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- transport.Run(ctx)
	}()

	select {
	case <-ready:
	case err := <-runErrCh:
		t.Fatalf("Run exited before API server ready: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for API server starter")
	}
	if startedSurface != transport.SessionAPISurface() {
		t.Fatal("API server starter did not receive the composed session API surface")
	}
	if _, ok := startedSurface.(*runtimehost.Host); ok {
		t.Fatal("API server starter received the runtime lifecycle host")
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForInitializerAPIEndpoint(t, baseURL+"/models", http.StatusOK)
	waitForInitializerAPIEndpoint(t, baseURL+"/factory-sessions/"+factorysessions.DefaultSessionID+"/factory", http.StatusOK)
	waitForInitializerAPIEndpoint(t, baseURL+"/factory-sessions", http.StatusOK)

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func waitForInitializerAPIEndpoint(t *testing.T, url string, wantStatus int) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
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

func writeInitializerAPITestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
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

func writeInitializerAPITestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
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
