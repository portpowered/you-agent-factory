package wire

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func TestBuildCLIRunnerUsesServiceCompatibilityForDashboardSuppressedRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	runner, err := BuildCLIRunner(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("BuildCLIRunner() error = %v", err)
	}
	if _, ok := runner.(*service.FactoryService); !ok {
		t.Fatalf("dashboard-suppressed runner type = %T, want *service.FactoryService", runner)
	}
}

func TestBuildCLIRunnerUsesInitializerGraphForDashboardEnabledRuns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	runner, err := BuildCLIRunner(context.Background(), &service.FactoryServiceConfig{
		Dir:                     dir,
		SimpleDashboardRenderer: func(service.SimpleDashboardRenderInput) {},
	})
	if err != nil {
		t.Fatalf("BuildCLIRunner() error = %v", err)
	}
	if runner == nil {
		t.Fatal("BuildCLIRunner() returned nil runner")
	}
	if _, legacy := runner.(*service.FactoryService); legacy {
		t.Fatal("dashboard-enabled runner used service compatibility path")
	}
}

func TestBuildCLIRunnerReturnsConstructionFailures(t *testing.T) {
	t.Parallel()

	if _, err := BuildCLIRunner(context.Background(), nil); err == nil {
		t.Fatal("BuildCLIRunner(nil) error = nil, want construction error")
	}
	runner, err := BuildCLIRunner(context.Background(), &service.FactoryServiceConfig{
		Dir:                     t.TempDir(),
		SimpleDashboardRenderer: func(service.SimpleDashboardRenderInput) {},
	})
	if err == nil || runner != nil {
		t.Fatalf("BuildCLIRunner(invalid dashboard config) = (%T, %v), want nil runner and error", runner, err)
	}
}
