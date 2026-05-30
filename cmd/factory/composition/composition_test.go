package composition

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestBuildFactoryService_MatchesServiceBuilder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := BuildFactoryService(ctx, nil)
	_, serviceErr := service.BuildFactoryService(ctx, nil)
	if (err == nil) != (serviceErr == nil) {
		t.Fatalf(
			"composition error presence = %v, service.BuildFactoryService = %v",
			err,
			serviceErr,
		)
	}
	if err != nil && serviceErr != nil && err.Error() != serviceErr.Error() {
		t.Fatalf(
			"composition err = %q, service.BuildFactoryService err = %q",
			err,
			serviceErr,
		)
	}
}

func TestBuildFactoryService_ReturnsServiceForMinimalFixture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeMinimalWorkerAgentsMD(t, dir)

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}

	svc, err := BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("BuildFactoryService returned nil service")
	}

	serviceSvc, serviceErr := service.BuildFactoryService(ctx, cfg)
	if serviceErr != nil {
		t.Fatalf("service.BuildFactoryService: %v", serviceErr)
	}
	if serviceSvc == nil {
		t.Fatal("service.BuildFactoryService returned nil service")
	}
}

func writeMinimalWorkerAgentsMD(t *testing.T, factoryDir string) {
	t.Helper()

	workerDir := filepath.Join(factoryDir, "workers", "worker-a")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := []byte(`---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
---
You are a helpful assistant.
`)
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), content, 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	workstationDir := filepath.Join(factoryDir, "workstations", "process")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workstationDir, "AGENTS.md"),
		[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
		0o644,
	); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}
