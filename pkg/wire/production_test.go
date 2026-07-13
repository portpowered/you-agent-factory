package wire_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

func TestBuildProductionConstructsRealGraphOnceWithoutStartingLifecycle(t *testing.T) {
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

	graph, err := wire.BuildProduction(context.Background(), wire.ProductionInputs{
		Config:    config,
		MCPInput:  strings.NewReader(""),
		MCPOutput: &mcpOutput,
	})
	if err != nil {
		t.Fatalf("BuildProduction() error = %v", err)
	}
	assertCompleteProductionGraph(t, graph, config, clock)
	assertProductionConstructionIsInert(t, apiStarts, dashboardRenders, &mcpOutput)
	if config.Dir != factoryDir {
		t.Fatalf("BuildProduction() mutated caller config dir to %q", config.Dir)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("ProductionGraph.Close() error = %v", err)
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("second ProductionGraph.Close() error = %v", err)
	}
}

func assertCompleteProductionGraph(
	t *testing.T,
	graph *wire.ProductionGraph,
	config *runtimehost.Config,
	clock productionClock,
) {
	t.Helper()
	if graph == nil || graph.Config == nil {
		t.Fatal("BuildProduction() returned an incomplete graph")
	}
	if graph.Runtime.Logger != config.Logger || graph.Runtime.Clock != clock {
		t.Fatal("production graph did not retain explicit logger and clock identity")
	}
	assertProductionDomainServices(t, graph)
	assertProductionTransportIdentity(t, graph)
}

func assertProductionDomainServices(t *testing.T, graph *wire.ProductionGraph) {
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
}

func assertProductionTransportIdentity(t *testing.T, graph *wire.ProductionGraph) {
	t.Helper()
	if graph.Transport.Models != graph.Models || graph.Transport.Sessions != graph.FactorySessions || graph.Transport.FactoryDefinition != graph.FactoryDefinition || graph.Transport.DurableExecution != graph.DurableExecution {
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

func TestBuildProductionReportsConcreteCoreFailureBeforeLifecycleStart(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	graph, err := wire.BuildProduction(context.Background(), wire.ProductionInputs{
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
		t.Fatal("BuildProduction() returned a graph for a missing factory")
	}
	if err == nil || !strings.Contains(err.Error(), "construct runtime core") {
		t.Fatalf("BuildProduction() error = %v, want concrete runtime-core phase", err)
	}
	if output.Len() != 0 {
		t.Fatal("MCP transport started after graph construction failed")
	}
}

func TestBuildProductionPreservesCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	graph, err := wire.BuildProduction(ctx, wire.ProductionInputs{})
	if graph != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildProduction() = (%v, %v), want nil graph wrapping context.Canceled", graph, err)
	}
}

type productionClock struct{ now time.Time }

func (c productionClock) Now() time.Time { return c.now }

var _ factory.Clock = productionClock{}
