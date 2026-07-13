package wire

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
)

type processRunnerFunc func(context.Context) error

func (run processRunnerFunc) Run(ctx context.Context) error { return run(ctx) }

func TestBuildProcessGraphConstructsRunBeforeLifecycle(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	runConfig := runcli.RunConfig{
		Dir: factoryDir, Port: 0, DisableDefaultRecording: true,
		MockWorkersEnabled: true, SuppressDashboardRendering: true,
	}
	graph, err := BuildProcessGraph(context.Background(), startupcli.Request{
		Kind: startupcli.KindRun, RunConfig: &runConfig,
	}, initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}})
	if err != nil {
		t.Fatalf("BuildProcessGraph() error = %v", err)
	}
	if graph == nil || graph.Run == nil || graph.MCP != nil {
		t.Fatalf("run graph = %+v, want one constructed run application", graph)
	}
	if err := initializer.RunProcess(context.Background(), graph); err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
}

func TestBuildProcessGraphConstructsMCPBeforeLifecycle(t *testing.T) {
	t.Parallel()
	fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	var output bytes.Buffer
	graph, err := BuildProcessGraph(context.Background(), startupcli.Request{
		Kind: startupcli.KindMCPServe,
		MCP: startupcli.MCPIntent{
			FixtureCatalogPath: fixturePath,
			Stdin:              strings.NewReader(""),
			Stdout:             &output,
		},
	}, initializer.ProcessPolicy{Mode: initializer.ProcessModeMCPServe})
	if err != nil {
		t.Fatalf("BuildProcessGraph() error = %v", err)
	}
	if graph == nil || graph.MCP == nil || graph.Run != nil {
		t.Fatalf("MCP graph = %+v, want one constructed MCP application", graph)
	}
	application, ok := graph.MCP.(*initializer.Application)
	if !ok {
		t.Fatalf("MCP application type = %T, want initializer-owned application", graph.MCP)
	}
	applicationGraph, ok := application.Graph().(*Graph)
	if !ok {
		t.Fatalf("MCP lifecycle graph type = %T, want concrete wire graph", application.Graph())
	}
	if applicationGraph.DurableExecution == nil || applicationGraph.Transport.DurableExecution != applicationGraph.DurableExecution {
		t.Fatal("MCP transport does not retain the graph-owned durable execution service")
	}
	if applicationGraph.Lifecycles().MCP != applicationGraph.Transports.MCP {
		t.Fatal("MCP application does not consume the exact graph transport lifecycle")
	}
	if output.Len() != 0 {
		t.Fatal("MCP graph construction started serving before initializer activation")
	}
}

func TestBuildProcessGraphReturnsModeConstructionFailures(t *testing.T) {
	t.Parallel()
	tests := []startupcli.Request{
		{Kind: startupcli.KindRun},
		{Kind: startupcli.KindRun, RunConfig: &runcli.RunConfig{Dir: t.TempDir(), DisableDefaultRecording: true}},
		{Kind: startupcli.KindMCPServe, MCP: startupcli.MCPIntent{FixtureCatalogPath: filepath.Join(t.TempDir(), "missing.json")}},
		{Kind: startupcli.Kind("unknown")},
	}
	for _, request := range tests {
		policy := initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}}
		if request.Kind == startupcli.KindMCPServe {
			policy = initializer.ProcessPolicy{Mode: initializer.ProcessModeMCPServe}
		}
		graph, err := BuildProcessGraph(context.Background(), request, policy)
		if err == nil || graph != nil {
			t.Fatalf("BuildProcessGraph(%q) = (%+v, %v), want nil graph and construction error", request.Kind, graph, err)
		}
	}
}

func TestBuildProcessGraphAppliesRootPolicyBeforeConstructionAndLifecycle(t *testing.T) {
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	port := reserveProcessTestPort(t)
	tests := []struct {
		name          string
		policy        initializer.ProcessPolicy
		wantMode      interfaces.RuntimeMode
		wantPort      int
		wantDashboard bool
	}{
		{
			name: "local suppresses API dashboard and watchers",
			policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{
				WorkerScheduler: true,
			}},
			wantMode: interfaces.RuntimeModeBatch,
		},
		{
			name: "service enables API dashboard and watchers",
			policy: initializer.ProcessPolicy{Mode: initializer.ProcessModeAPIService, Sidecars: initializer.SidecarPolicy{
				API: true, Dashboard: true, WorkerScheduler: true, Watchers: true,
			}},
			wantMode: interfaces.RuntimeModeService, wantPort: port, wantDashboard: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := runcli.RunConfig{
				Dir: factoryDir, Port: port, DisableDefaultRecording: true,
				MockWorkersEnabled: true, SuppressDashboardRendering: !test.wantDashboard,
			}
			var built *service.FactoryServiceConfig
			var builtMode initializer.Mode
			graph, err := buildProcessGraph(context.Background(), startupcli.Request{
				Kind: startupcli.KindRun, RunConfig: &cfg,
			}, test.policy, func(_ context.Context, cfg *service.FactoryServiceConfig, mode initializer.Mode) (runcli.RuntimeRunner, error) {
				built = cfg
				builtMode = mode
				return processRunnerFunc(func(context.Context) error { return nil }), nil
			})
			if err != nil {
				t.Fatalf("buildProcessGraph() error = %v", err)
			}
			if built == nil || built.RuntimeMode != test.wantMode || built.Port != test.wantPort {
				t.Fatalf("constructed service config = %+v, want mode %q port %d", built, test.wantMode, test.wantPort)
			}
			if got := built.SimpleDashboardRenderer != nil; got != test.wantDashboard {
				t.Fatalf("dashboard renderer enabled = %t, want %t", got, test.wantDashboard)
			}
			wantApplicationMode := initializer.ModeCLI
			if test.policy.Mode == initializer.ProcessModeAPIService {
				wantApplicationMode = initializer.ModeAPI
			}
			if builtMode != wantApplicationMode {
				t.Fatalf("initializer application mode = %q, want %q", builtMode, wantApplicationMode)
			}
			if graph.Policy != test.policy {
				t.Fatalf("graph policy = %+v, want %+v", graph.Policy, test.policy)
			}
			if err := initializer.RunProcess(context.Background(), graph); err != nil {
				t.Fatalf("RunProcess() error = %v", err)
			}
		})
	}
}

func reserveProcessTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved port: %v", err)
	}
	return port
}
