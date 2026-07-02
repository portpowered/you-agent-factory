package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
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
