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

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestRun_WireBuiltFactoryServiceListsModels(t *testing.T) {
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
	var models factoryapi.ListModelsResponse
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/models")
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Fatalf("read /models body: %v", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(body, &models); err != nil {
			t.Fatalf("decode /models: %v", err)
		}
		break
	}
	if len(models.Results) == 0 {
		t.Fatalf("models list empty after startup, want configured model summaries")
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
