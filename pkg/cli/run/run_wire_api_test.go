package run

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
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestRun_WireBuiltFactoryServiceServesStatus(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeRunWireTestWorkerAgentsMD(t, dir, "worker-a")
	writeRunWireTestWorkstationAgentsMD(t, dir, "process")
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

	originalStartAPIServer := startAPIServer
	originalServeFactoryAPIServer := serveFactoryAPIServer
	defer func() {
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
	var status factoryapi.StatusResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/status")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read /status body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, &status); err != nil {
			t.Fatalf("decode /status: %v", err)
		}
		break
	}
	if status.FactoryState == "" {
		t.Fatalf("GET /status factory_state empty after polling %s/status", baseURL)
	}

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func writeRunWireTestWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
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

func writeRunWireTestWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
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
