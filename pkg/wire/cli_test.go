package wire

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

func TestInjectCLICommandConstructsFreshCobraTree(t *testing.T) {
	t.Parallel()

	first := InjectCLICommand(cli.RootCommandOptions{})
	second := InjectCLICommand(cli.RootCommandOptions{})
	if first == nil || second == nil {
		t.Fatal("InjectCLICommand() returned a nil command")
	}
	if first == second {
		t.Fatal("InjectCLICommand() reused mutable Cobra command state")
	}
	if first.Name() != "you" {
		t.Fatalf("InjectCLICommand() name = %q, want you", first.Name())
	}
}

func TestInjectWireCoreReturnsCompleteProcessComposition(t *testing.T) {
	t.Parallel()

	core := InjectWireCore()
	if core.BuildCLICommand == nil || core.BuildProcessGraph == nil || core.InitializeProcess == nil ||
		core.BuildMCPExecution == nil || core.BuildSessionExecution == nil || core.BuildModelInvocation == nil ||
		core.BuildWorkerApplication == nil || core.BuildRunSessionExecution == nil {
		t.Fatalf("InjectWireCore() returned incomplete composition: %+v", core)
	}
	if command := core.BuildCLICommand(cli.RootCommandOptions{}); command == nil || command.Name() != "you" {
		t.Fatalf("WireCore.BuildCLICommand() = %v, want you command", command)
	}
}

func TestBuildCLIRunnerConstructsInertGraphApplication(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	starts := 0
	cfg := &service.FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		RuntimeFileLoggingPolicy:                service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    service.RuntimeMetricsPolicyDisabled,
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			starts++
			return nil
		},
	}
	runner, err := BuildCLIRunner(context.Background(), cfg)
	if err != nil {
		t.Fatalf("BuildCLIRunner() error = %v", err)
	}
	application, ok := runner.(*initializer.Application)
	if !ok {
		t.Fatalf("BuildCLIRunner() type = %T, want *initializer.Application", runner)
	}
	graph, ok := application.Graph().(*Graph)
	if !ok || graph == nil {
		t.Fatalf("application graph type = %T, want *wire.Graph", application.Graph())
	}
	if starts != 0 {
		t.Fatalf("API starts during graph construction = %d, want zero", starts)
	}
	if cfg.Clock != nil {
		t.Fatal("BuildCLIRunner() mutated caller-owned clock")
	}
	if graph.Runtime.Clock == nil || graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("constructed graph did not retain normalized inputs and graph-owned service identity")
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestBuildApplicationRunnerAPIModeRunsGraphOwnedSurfaceAndCanonicalSessionID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	observedSurface := make(chan apisurface.APISurface, 1)
	cfg := &service.FactoryServiceConfig{
		Dir:                                     dir,
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
	runner, err := buildApplicationRunner(context.Background(), cfg, initializer.ModeAPI)
	if err != nil {
		t.Fatalf("buildApplicationRunner(API) error = %v", err)
	}
	application := runner.(*initializer.Application)
	graph := application.Graph().(*Graph)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runDone := make(chan error, 1)
	go func() { runDone <- runner.Run(ctx) }()

	var surface apisurface.APISurface
	select {
	case surface = <-observedSurface:
		if surface != graph.Transport.API {
			t.Fatal("API starter observed a surface other than the graph-owned transport surface")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("graph-owned API transport did not start")
	}
	assertCanonicalSessionIDResolves(t, ctx, surface)
	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("Run() shutdown error = %v", err)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("repeated Shutdown() error = %v", err)
	}
}

func assertCanonicalSessionIDResolves(t *testing.T, ctx context.Context, surface apisurface.APISurface) {
	t.Helper()
	sessions, ok := surface.(apisurface.SessionAPISurface)
	if !ok {
		t.Fatalf("graph API surface type = %T, want SessionAPISurface", surface)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		list, err := sessions.ListFactorySessions(ctx)
		if err != nil {
			t.Fatalf("ListFactorySessions() error = %v", err)
		}
		if len(list.Sessions) > 0 {
			canonicalID := list.Sessions[0].Id
			preflight, err := sessions.GetFactorySessionSyncPreflight(ctx, canonicalID, interfaces.FactorySessionSyncPreflightOptions{})
			if err != nil {
				t.Fatalf("GetFactorySessionSyncPreflight(%q) error = %v", canonicalID, err)
			}
			if preflight.ReasonCode != factoryapi.Ok || preflight.FactorySessionId == nil || *preflight.FactorySessionId != canonicalID {
				t.Fatalf("canonical session preflight = %+v, want ok for %q", preflight, canonicalID)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("graph API surface did not publish a live Factory Session")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBuildCLIRunnerReturnsConstructionFailures(t *testing.T) {
	t.Parallel()

	if runner, err := BuildCLIRunner(context.Background(), nil); err == nil || runner != nil {
		t.Fatalf("BuildCLIRunner(nil) = (%T, %v), want nil runner and construction error", runner, err)
	}
	runner, err := BuildCLIRunner(context.Background(), &service.FactoryServiceConfig{
		Dir:                                     t.TempDir(),
		Logger:                                  zap.NewNop(),
		RuntimeFileLoggingPolicy:                service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    service.RuntimeMetricsPolicyDisabled,
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err == nil || runner != nil || !strings.Contains(err.Error(), "construct runtime core") {
		t.Fatalf("BuildCLIRunner(invalid config) = (%T, %v), want concrete graph failure", runner, err)
	}
}
