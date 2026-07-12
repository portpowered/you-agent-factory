package service_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestNormalizeInvocationBootstrapConfig_ForcesNoServerShape(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	starter := func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
		close(ready)
		return nil
	}
	renderer := func(service.SimpleDashboardRenderInput) {}

	cfg := &service.FactoryServiceConfig{
		Port:                    7437,
		RuntimeMode:             interfaces.RuntimeModeBatch,
		WorkFile:                "/tmp/work.json",
		APIServerStarter:        starter,
		APIServerReady:          ready,
		SimpleDashboardRenderer: renderer,
	}

	got := service.NormalizeInvocationBootstrapConfig(cfg)
	if got.Port != 0 {
		t.Fatalf("Port = %d, want 0", got.Port)
	}
	if got.APIServerStarter != nil {
		t.Fatal("APIServerStarter = non-nil, want nil")
	}
	if got.APIServerReady != nil {
		t.Fatal("APIServerReady = non-nil, want nil")
	}
	if got.SimpleDashboardRenderer != nil {
		t.Fatal("SimpleDashboardRenderer = non-nil, want nil")
	}
	if got.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("RuntimeMode = %q, want %q", got.RuntimeMode, interfaces.RuntimeModeService)
	}
	if got.WorkFile != "" {
		t.Fatalf("WorkFile = %q, want empty", got.WorkFile)
	}
}

func TestBuildInvocationBootstrap_LeavesNoFactoryAPIServerListener(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeInvocationBootstrapWorkerAgentsMD(t, dir, "worker-a")
	writeInvocationBootstrapWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	probePort := reserveInvocationBootstrapProbePort(t)
	apiReady := make(chan struct{})
	starterInvoked := make(chan struct{}, 1)
	cfg := &service.FactoryServiceConfig{
		Dir:               dir,
		Port:              probePort,
		RuntimeMode:       interfaces.RuntimeModeBatch,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		APIServerReady:    apiReady,
		APIServerStarter: func(ctx context.Context, _ apisurface.APISurface, port int, _ *zap.Logger) error {
			starterInvoked <- struct{}{}
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return err
			}
			close(apiReady)
			<-ctx.Done()
			return listener.Close()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap, err := service.BuildInvocationBootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildInvocationBootstrap: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- bootstrap.Run(ctx)
	}()

	waitForInvocationBootstrapSessionReady(t, ctx, bootstrap, runErrCh)
	assertFactoryAPIServerPortFree(t, probePort)

	select {
	case <-starterInvoked:
		t.Fatal("APIServerStarter invoked, want no-server bootstrap to skip HTTP listener startup")
	default:
	}

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestInvocationBootstrap_CloseFactorySessionReleasesLiveSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeInvocationBootstrapWorkerAgentsMD(t, dir, "worker-a")
	writeInvocationBootstrapWorkstationAgentsMD(t, dir, "process")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap, err := service.BuildInvocationBootstrap(ctx, &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildInvocationBootstrap: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- bootstrap.Run(ctx)
	}()

	waitForInvocationBootstrapSessionReady(t, ctx, bootstrap, runErrCh)

	if _, err := bootstrap.Service.GetFactorySession(ctx, factorysessions.DefaultSessionID); err != nil {
		t.Fatalf("GetFactorySession before close: %v", err)
	}
	if err := bootstrap.CloseFactorySession(ctx, factorysessions.DefaultSessionID); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if _, err := bootstrap.Service.GetFactorySession(ctx, factorysessions.DefaultSessionID); !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetFactorySession after close = %v, want %v", err, apisurface.ErrFactorySessionNotFound)
	}

	cancel()
	if err := <-runErrCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}

func reserveInvocationBootstrapProbePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}
	return port
}

func waitForInvocationBootstrapSessionReady(
	t *testing.T,
	ctx context.Context,
	bootstrap *service.InvocationBootstrap,
	runErrCh <-chan error,
) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := bootstrap.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID); err == nil {
			return
		} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			t.Fatalf("GetCurrentFactoryForSession: %v", err)
		}

		select {
		case err := <-runErrCh:
			if err == nil || err == context.Canceled {
				t.Fatal("bootstrap runtime stopped before session became ready")
			}
			t.Fatalf("bootstrap runtime failed before session became ready: %v", err)
		case <-ctx.Done():
			t.Fatalf("context canceled before session became ready: %v", ctx.Err())
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for bootstrap session readiness")
}

func assertFactoryAPIServerPortFree(t *testing.T, port int) {
	t.Helper()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("tcp %s accepted a connection, want no factory API/dashboard listener", addr)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("tcp %s still held by bootstrap listener: %v", addr, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close probe listener: %v", err)
	}
}

func writeInvocationBootstrapWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
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

func writeInvocationBootstrapWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
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
