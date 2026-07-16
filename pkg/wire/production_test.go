package wire_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

func TestBuildConstructsRealGraphOnceWithoutStartingLifecycle(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	clock := productionClock{now: time.Date(2026, time.July, 13, 9, 0, 0, 0, time.UTC)}
	var apiStarts, dashboardRenders int
	config := &runtimehost.Config{
		Dir:                                     factoryDir,
		Logger:                                  zap.NewNop(),
		Clock:                                   clock,
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			apiStarts++
			return nil
		},
		SimpleDashboardRenderer: func(runtimehost.SimpleDashboardRenderInput) {
			dashboardRenders++
		},
	}
	var mcpOutput bytes.Buffer

	graph, err := wire.Build(context.Background(), wire.Inputs{
		Config:    config,
		MCPInput:  strings.NewReader(""),
		MCPOutput: &mcpOutput,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	assertCompleteGraph(t, graph, config, clock)
	assertProductionConstructionIsInert(t, apiStarts, dashboardRenders, &mcpOutput)
	if config.Dir != factoryDir {
		t.Fatalf("Build() mutated caller config dir to %q", config.Dir)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("Graph.Close() error = %v", err)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("second Graph.Close() error = %v", err)
	}
}

func TestInjectRuntimeCoreSharesSessionFoundationWithCompatibilityConsumers(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	starts := 0
	core, err := wire.InjectRuntimeCore(context.Background(), &runtimehost.Config{
		Dir: factoryDir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(), Clock: productionClock{},
		RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			starts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("InjectRuntimeCore() error = %v", err)
	}
	if starts != 0 {
		t.Fatalf("runtime core construction started API lifecycle %d times, want zero", starts)
	}
	if core.Sessions() == nil || core.Sessions().Count() != 1 || core.RuntimeBuild() == nil {
		t.Fatal("runtime core omitted its single initialized registry or runtime-build service")
	}
	owner, ok := core.DurableExecution().(interface{ PersistenceStore() runtimepersist.Store })
	if !ok || core.Persistence() == nil || owner.PersistenceStore() != core.Persistence() {
		t.Fatal("runtime core durable execution did not retain the Wire-owned persistence store")
	}
	host := runtimehost.NewHostFromCore(core)
	if host.DurableExecutionService() != core.DurableExecution() {
		t.Fatal("compatibility consumers replaced the Wire-owned durable execution service")
	}
}

func assertCompleteGraph(
	t *testing.T,
	graph *wire.Graph,
	config *runtimehost.Config,
	clock productionClock,
) {
	t.Helper()
	if graph == nil || graph.Config == nil {
		t.Fatal("Build() returned an incomplete graph")
	}
	if graph.Runtime.Logger != config.Logger || graph.Runtime.Clock != clock {
		t.Fatal("production graph did not retain explicit logger and clock identity")
	}
	assertProductionDomainServices(t, graph)
	assertProductionModelService(t, graph)
	assertProductionTransportIdentity(t, graph)
}

func assertProductionModelService(t *testing.T, graph *wire.Graph) {
	t.Helper()
	models, err := graph.Models.ListModels(context.Background())
	if err != nil {
		t.Fatalf("production model collaborator ListModels() error = %v", err)
	}
	if models.Results == nil {
		t.Fatal("production model collaborator returned nil catalog results")
	}
}

func assertProductionDomainServices(t *testing.T, graph *wire.Graph) {
	t.Helper()
	if graph.Models == nil || graph.Workers == nil || graph.WorkerProvider == nil || graph.SessionRegistry == nil {
		t.Fatal("production graph omitted a model, worker, provider, or Factory Session collaborator")
	}
	if graph.FactorySessions == nil || graph.FactoryDefinition == nil || graph.DurableExecution == nil {
		t.Fatal("production graph omitted a Factory Session domain service")
	}
	if graph.Transports.API == nil || graph.Transports.CLI == nil || graph.Transports.MCP == nil {
		t.Fatal("production graph omitted a transport lifecycle")
	}
	if graph.Sidecars.Runtime == nil || graph.Sidecars.Workers == nil || graph.Sidecars.Dashboard == nil {
		t.Fatal("production graph omitted a runtime, worker/watcher, or dashboard lifecycle")
	}
}

func assertProductionTransportIdentity(t *testing.T, graph *wire.Graph) {
	t.Helper()
	if graph.Transport.Models != graph.Models || graph.Transport.FactorySessions != graph.FactorySessions || graph.Transport.FactoryDefinition != graph.FactoryDefinition || graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("production transport dependencies do not share graph-owned collaborator identity")
	}
}

func assertProductionConstructionIsInert(
	t *testing.T,
	apiStarts int,
	dashboardRenders int,
	mcpOutput *bytes.Buffer,
) {
	t.Helper()
	if apiStarts != 0 || dashboardRenders != 0 || mcpOutput.Len() != 0 {
		t.Fatalf("construction activated lifecycle work: API=%d dashboard=%d MCP-bytes=%d", apiStarts, dashboardRenders, mcpOutput.Len())
	}
}

func TestBuildReportsConcreteCoreFailureBeforeLifecycleStart(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	graph, err := wire.Build(context.Background(), wire.Inputs{
		Config: &runtimehost.Config{
			Dir:                      filepath.Join(t.TempDir(), "missing"),
			Logger:                   zap.NewNop(),
			Clock:                    productionClock{},
			RuntimeFileLoggingPolicy: runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:     runtimehost.RuntimeMetricsPolicyDisabled,
		},
		MCPInput:  strings.NewReader(""),
		MCPOutput: &output,
	})
	if graph != nil {
		t.Fatal("Build() returned a graph for a missing factory")
	}
	if err == nil || !strings.Contains(err.Error(), "construct runtime core") {
		t.Fatalf("Build() error = %v, want concrete runtime-core phase", err)
	}
	if output.Len() != 0 {
		t.Fatal("MCP transport started after graph construction failed")
	}
}

func TestBuildRejectsInvalidPersistenceBeforeLifecycleStart(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	var apiStarts int
	var output bytes.Buffer
	graph, err := wire.Build(context.Background(), wire.Inputs{
		Config: &runtimehost.Config{
			Dir: factoryDir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(), Clock: productionClock{},
			DurableSessionPersistencePolicy:         "unsupported",
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
			APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
				apiStarts++
				return nil
			},
		},
		MCPInput: strings.NewReader(""), MCPOutput: &output,
	})
	if graph != nil {
		t.Fatal("Build() returned a graph for invalid persistence policy")
	}
	var validation *factorysessionexecution.ValidationError
	if err == nil || !strings.Contains(err.Error(), "compose durable session persistence") || !errors.As(err, &validation) || validation.Field != "persistence.policy" {
		t.Fatalf("Build() error = %v, want actionable persistence policy context", err)
	}
	if apiStarts != 0 || output.Len() != 0 {
		t.Fatalf("construction failure started lifecycle work: API=%d MCP-bytes=%d", apiStarts, output.Len())
	}
}

func TestBuildSharesConstructedPersistenceAndRoundTripsSnapshot(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, factoryDir, factoryfixtures.MinimalFactoryConfig())
	graph, err := wire.Build(context.Background(), wire.Inputs{
		Config: &runtimehost.Config{
			Dir: factoryDir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(), Clock: productionClock{},
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer func() { _ = graph.Close() }()
	owner, ok := graph.DurableExecution.(interface{ PersistenceStore() runtimepersist.Store })
	if !ok || graph.Persistence == nil || owner.PersistenceStore() != graph.Persistence {
		t.Fatal("durable execution did not retain the graph-owned persistence store")
	}
	if graph.SessionRegistry == nil || graph.SessionRegistry.Count() != 1 {
		t.Fatal("graph did not retain its single initialized live session registry")
	}
	sessionID := "dur-sess-cccccccccccccccccccccccccccccccc"
	payload := []byte(`{"status":"COMPLETED"}`)
	if err := graph.Persistence.Save(sessionID, payload); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := graph.Persistence.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(loaded) != string(payload) {
		t.Fatalf("loaded payload = %s, want %s", loaded, payload)
	}
}

func TestBuildPreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	graph, err := wire.Build(ctx, wire.Inputs{})
	if graph != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Build() = (%v, %v), want nil graph wrapping context.Canceled", graph, err)
	}
}

type productionClock struct{ now time.Time }

func (c productionClock) Now() time.Time { return c.now }

var _ factory.Clock = productionClock{}
