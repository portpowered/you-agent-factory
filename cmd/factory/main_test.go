package main

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestMainExecutesCLI(t *testing.T) {
	original := executeCLI
	t.Cleanup(func() { executeCLI = original })

	called := false
	executeCLI = func() { called = true }
	main()
	if !called {
		t.Fatal("main() did not execute the CLI entrypoint")
	}
}

func TestBuildCLIRunnerStartsRootGraphWithGraphOwnedAPISurface(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	observedSurface := make(chan apisurface.APISurface, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cfg := &service.FactoryServiceConfig{
		Dir:                                     factoryDir,
		SystemConfigHomeDir:                     t.TempDir(),
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    43173,
		Logger:                                  zap.NewNop(),
		RuntimeFileLoggingPolicy:                service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    service.RuntimeMetricsPolicyDisabled,
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
		APIServerStarter: func(ctx context.Context, surface apisurface.APISurface, _ int, _ *zap.Logger) error {
			observedSurface <- surface
			<-ctx.Done()
			return ctx.Err()
		},
	}

	runner, err := buildCLIRunner(ctx, cfg)
	if err != nil {
		t.Fatalf("buildCLIRunner() error = %v", err)
	}
	application, ok := runner.(*initializer.Application)
	if !ok {
		t.Fatalf("buildCLIRunner() type = %T, want *initializer.Application", runner)
	}
	if cfg.Clock != nil {
		t.Fatal("buildCLIRunner() mutated caller-owned clock")
	}
	graph := application.Graph()
	if graph == nil || graph.Runtime.Clock == nil {
		t.Fatal("production root returned graph without normalized explicit clock")
	}
	if graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("production root graph transport does not retain graph-owned service identity")
	}

	select {
	case surface := <-observedSurface:
		if surface != graph.Transport.API {
			t.Fatal("API starter observed a surface other than the graph-owned transport surface")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("graph-owned API transport did not start")
	}

	cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() shutdown error = %v", err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("repeated Shutdown() error = %v", err)
	}
}

func TestBuildCLIRunnerReturnsGraphConstructionFailureBeforeTransportStart(t *testing.T) {
	t.Parallel()

	starts := 0
	runner, err := buildCLIRunner(context.Background(), &service.FactoryServiceConfig{
		Dir:                      t.TempDir(),
		SystemConfigHomeDir:      t.TempDir(),
		RuntimeFileLoggingPolicy: service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:     service.RuntimeMetricsPolicyDisabled,
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			starts++
			return nil
		},
	})
	if runner != nil || err == nil {
		t.Fatalf("buildCLIRunner() = (%v, %v), want construction failure", runner, err)
	}
	if starts != 0 {
		t.Fatalf("transport starts = %d, want zero", starts)
	}
}
