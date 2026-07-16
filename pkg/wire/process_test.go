package wire

import (
	"bytes"
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
)

type processRunnerFunc func(context.Context) error

func (run processRunnerFunc) Run(ctx context.Context) error { return run(ctx) }

type processCommandRunner struct{}

func (*processCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

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

func TestBuildProcessGraphWithFunctionalEdgesConstructsSharedRunGraph(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	graph, err := BuildProcessGraphWithFunctionalEdges(context.Background(), startupcli.Request{
		Kind: startupcli.KindRun,
		RunConfig: &runcli.RunConfig{
			Dir: factoryDir, Port: 0, DisableDefaultRecording: true,
			MockWorkersEnabled: true, SuppressDashboardRendering: true,
		},
	}, initializer.ProcessPolicy{
		Mode:     initializer.ProcessModeLocalRun,
		Sidecars: initializer.SidecarPolicy{WorkerScheduler: true},
	}, FunctionalEdges{ProviderCommandRunner: &processCommandRunner{}})
	if err != nil {
		t.Fatalf("BuildProcessGraphWithFunctionalEdges() error = %v", err)
	}
	if graph == nil || graph.Run == nil || graph.MCP != nil {
		t.Fatalf("functional run graph = %+v, want shared run application", graph)
	}
	if err := initializer.RunProcess(context.Background(), graph); err != nil {
		t.Fatalf("RunProcess() error = %v", err)
	}
}

func TestBuildProcessGraphUsesInjectedInvocationBuilder(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	text := "sentinel invocation"
	runConfig := runcli.RunConfig{
		Dir: factoryDir, InvocationPositionalText: &text, StdinIsTTY: func() bool { return true },
		DisableDefaultRecording: true, SuppressDashboardRendering: true,
	}
	wantErr := errors.New("sentinel invocation builder")
	calls := 0
	graph, err := buildProcessGraph(context.Background(), startupcli.Request{
		Kind: startupcli.KindRun, RunConfig: &runConfig,
	}, initializer.ProcessPolicy{Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true}},
		FunctionalEdges{},
		func(context.Context, *service.FactoryServiceConfig, initializer.Mode) (runcli.RuntimeRunner, error) {
			t.Fatal("runtime builder called for one-shot invocation")
			return nil, nil
		},
		BuildMCPExecutionService,
		func(context.Context, *service.FactoryServiceConfig) (runcli.InvocationRunner, error) {
			calls++
			return nil, wantErr
		},
	)
	if graph != nil || !errors.Is(err, wantErr) {
		t.Fatalf("buildProcessGraph() = (%+v, %v), want injected builder error", graph, err)
	}
	if calls != 1 {
		t.Fatalf("invocation builder calls = %d, want 1", calls)
	}
}

func TestBuildProcessGraphSelectsOnlySuppliedFunctionalEdge(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	policy := initializer.ProcessPolicy{
		Mode:     initializer.ProcessModeLocalRun,
		Sidecars: initializer.SidecarPolicy{WorkerScheduler: true},
	}
	injected := &processCommandRunner{}

	tests := []struct {
		name     string
		edges    FunctionalEdges
		wantEdge workers.CommandRunner
	}{
		{name: "production defaults", edges: FunctionalEdges{}},
		{name: "functional provider runner", edges: FunctionalEdges{ProviderCommandRunner: injected}, wantEdge: injected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var built *service.FactoryServiceConfig
			graph, err := buildProcessGraph(context.Background(), startupcli.Request{
				Kind: startupcli.KindRun,
				RunConfig: &runcli.RunConfig{
					Dir: factoryDir, DisableDefaultRecording: true, SuppressDashboardRendering: true,
				},
			}, policy, test.edges, func(
				_ context.Context,
				cfg *service.FactoryServiceConfig,
				_ initializer.Mode,
			) (runcli.RuntimeRunner, error) {
				built = cfg
				return processRunnerFunc(func(context.Context) error { return nil }), nil
			}, BuildMCPExecutionService)
			if err != nil {
				t.Fatalf("buildProcessGraph() error = %v", err)
			}
			if built == nil || built.ProviderCommandRunnerOverride != test.wantEdge {
				t.Fatalf("provider command runner = %T, want %T", built.ProviderCommandRunnerOverride, test.wantEdge)
			}
			if graph == nil || graph.Run == nil {
				t.Fatalf("run graph = %+v, want constructed application", graph)
			}
		})
	}
}

func TestConfigWithFunctionalEdgesCopiesOnlyForReplacement(t *testing.T) {
	t.Parallel()
	configured := &processCommandRunner{}
	injected := &processCommandRunner{}
	cfg := &service.FactoryServiceConfig{ProviderCommandRunnerOverride: configured}

	production := configWithFunctionalEdges(cfg, FunctionalEdges{})
	if production != cfg || production.ProviderCommandRunnerOverride != configured {
		t.Fatalf("production config = %+v, want original config and provider runner", production)
	}

	functional := configWithFunctionalEdges(cfg, FunctionalEdges{ProviderCommandRunner: injected})
	if functional == cfg || functional.ProviderCommandRunnerOverride != injected {
		t.Fatalf("functional config = %+v, want copied config with injected provider runner", functional)
	}
	if cfg.ProviderCommandRunnerOverride != configured {
		t.Fatal("functional edge selection mutated the caller-owned config")
	}
}

func TestConfigWithFunctionalEdgesSelectsIndependentWorkerEdges(t *testing.T) {
	t.Parallel()
	providerRunner := &processCommandRunner{}
	scriptRunner := &processCommandRunner{}
	allocator := &agypty.MockAllocator{}

	built := configWithFunctionalEdges(&service.FactoryServiceConfig{}, FunctionalEdges{
		ProviderCommandRunner: providerRunner,
		ScriptCommandRunner:   scriptRunner,
		AgyPTYAllocator:       allocator,
	})
	if built.ProviderCommandRunnerOverride != providerRunner {
		t.Fatal("provider command edge was not selected")
	}
	if built.CommandRunnerOverride != scriptRunner {
		t.Fatal("script command edge was not selected")
	}
	if built.AgyPTYAllocatorOverride != allocator {
		t.Fatal("Agy PTY edge was not selected independently")
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

func TestBuildMCPExecutionServiceSelectsRequestedBackingService(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		request     MCPExecutionRequest
		wantRuntime bool
	}{
		{
			name: "fixture",
			request: MCPExecutionRequest{FixtureCatalogPath: testutil.MustRepoPath(
				t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json",
			)},
		},
		{
			name: "runtime", request: MCPExecutionRequest{RuntimeBacked: true, ProjectRoot: t.TempDir()},
			wantRuntime: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := BuildMCPExecutionService(context.Background(), test.request)
			if err != nil {
				t.Fatalf("BuildMCPExecutionService() error = %v", err)
			}
			if test.wantRuntime {
				if _, ok := service.(*factorysessionexecution.JavaScriptRuntimeService); !ok {
					t.Fatalf("service type = %T, want runtime service", service)
				}
			} else if _, ok := service.(*factorysessionexecution.FakeService); !ok {
				t.Fatalf("service type = %T, want fixture service", service)
			}
		})
	}
}

func TestBuildSessionExecutionServiceSelectsRequestedBackingService(t *testing.T) {
	t.Parallel()
	fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	tests := []struct {
		name        string
		request     sessionexecutioncli.ServiceRequest
		wantRuntime bool
	}{
		{name: "explicit fixture", request: sessionexecutioncli.ServiceRequest{FixtureCatalogPath: fixturePath}},
		{name: "repository fixture default", request: sessionexecutioncli.ServiceRequest{}},
		{
			name: "runtime",
			request: sessionexecutioncli.ServiceRequest{
				ExecutionBackendConfig: sessionexecutioncli.ExecutionBackendConfig{
					Provider:    string(factorysessionexecution.ExecutionProviderJavaScriptRuntime),
					ProjectRoot: t.TempDir(),
				},
			},
			wantRuntime: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := BuildSessionExecutionService(context.Background(), test.request)
			if err != nil {
				t.Fatalf("BuildSessionExecutionService() error = %v", err)
			}
			if test.wantRuntime {
				if _, ok := service.(*factorysessionexecution.JavaScriptRuntimeService); !ok {
					t.Fatalf("service type = %T, want runtime service", service)
				}
			} else if _, ok := service.(*factorysessionexecution.FakeService); !ok {
				t.Fatalf("service type = %T, want fixture service", service)
			}
		})
	}
}

func TestBuildSessionExecutionServiceRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()
	service, err := BuildSessionExecutionService(context.Background(), sessionexecutioncli.ServiceRequest{
		ExecutionBackendConfig: sessionexecutioncli.ExecutionBackendConfig{Provider: "unsupported"},
	})
	if err == nil || service != nil || !strings.Contains(err.Error(), "use fake or javascript-runtime") {
		t.Fatalf("BuildSessionExecutionService() = (%T, %v), want actionable unsupported-provider error", service, err)
	}
}

func TestBuildProcessGraphUsesExactInjectedMCPExecutionService(t *testing.T) {
	t.Parallel()
	injected := factorysessionexecution.NewFakeService()
	var output bytes.Buffer
	calls := 0
	graph, err := BuildProcessGraphWithMCPBuilder(context.Background(), startupcli.Request{
		Kind: startupcli.KindMCPServe,
		MCP:  startupcli.MCPIntent{RuntimeBacked: true, ProjectRoot: t.TempDir(), Stdin: strings.NewReader(""), Stdout: &output},
	}, initializer.ProcessPolicy{Mode: initializer.ProcessModeMCPServe}, func(_ context.Context, request MCPExecutionRequest) (factorysessionexecution.Service, error) {
		calls++
		if !request.RuntimeBacked || strings.TrimSpace(request.ProjectRoot) == "" {
			t.Fatalf("MCP execution request = %+v, want runtime-backed project root", request)
		}
		return injected, nil
	})
	if err != nil {
		t.Fatalf("BuildProcessGraphWithMCPBuilder() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("MCP execution builder calls = %d, want 1", calls)
	}
	application, ok := graph.MCP.(*initializer.Application)
	if !ok {
		t.Fatalf("MCP application type = %T, want initializer application", graph.MCP)
	}
	wired, ok := application.Graph().(*Graph)
	if !ok || wired.DurableExecution != injected || wired.Transport.DurableExecution != injected {
		t.Fatalf("wired MCP execution = %+v, want exact injected service %p", wired, injected)
	}
	if err := initializer.RunProcess(context.Background(), graph); err != nil {
		t.Fatalf("RunProcess() error = %v", err)
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
			}, test.policy, FunctionalEdges{}, func(_ context.Context, cfg *service.FactoryServiceConfig, mode initializer.Mode) (runcli.RuntimeRunner, error) {
				built = cfg
				builtMode = mode
				return processRunnerFunc(func(context.Context) error { return nil }), nil
			}, BuildMCPExecutionService)
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
