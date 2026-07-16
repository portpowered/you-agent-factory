package wire

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

type processRunnerFunc func(context.Context) error

func (run processRunnerFunc) Run(ctx context.Context) error { return run(ctx) }

type processCommandRunner struct{}

func (*processCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

type processRecordingCommandRunner struct {
	requests []workers.CommandRequest
	result   workers.CommandResult
}

func (r *processRecordingCommandRunner) Run(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	r.requests = append(r.requests, request)
	return r.result, nil
}

type processAgyAllocator struct {
	launches []agypty.ProcessLaunch
}

func (a *processAgyAllocator) Allocate(_ context.Context, launch agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	a.launches = append(a.launches, launch)
	return &processAgySession{}, nil
}

type processAgySession struct{}

func (*processAgySession) Run(context.Context) (agypty.SessionResult, error) {
	return agypty.SessionResult{ExitCode: 0, CleanedText: "agy functional output"}, nil
}
func (*processAgySession) Close() error { return nil }

type processRoundTripper struct{ calls int }

func (r *processRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"data":{"ok":true}}`)),
		Header:     make(http.Header),
	}, nil
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
		false,
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

func TestRootInvocationRunnerConsumesOneWireSessionFoundation(t *testing.T) {
	t.Parallel()

	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	runner, err := buildInvocationRunner(context.Background(), &service.FactoryServiceConfig{
		Dir: factoryDir, ExecutionBaseDir: t.TempDir(), SystemConfigHomeDir: t.TempDir(),
		RuntimeFileLoggingPolicy:                service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    service.RuntimeMetricsPolicyDisabled,
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("buildInvocationRunner() error = %v", err)
	}
	shared, ok := runner.(*service.InvocationBootstrap)
	if !ok || shared.Service == nil {
		t.Fatalf("invocation runner = %T, want service-owned invocation bootstrap", runner)
	}
	owner, ok := shared.Service.DurableExecutionService().(interface{ PersistenceStore() runtimepersist.Store })
	if !ok || owner.PersistenceStore() == nil {
		t.Fatal("root invocation durable execution did not retain its persistence store")
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
			}, BuildMCPExecutionService, false)
			if err != nil {
				t.Fatalf("buildProcessGraph() error = %v", err)
			}
			if built == nil || !built.WorkerApplication.Valid() || built.WorkerApplication.ProviderCommandInjected != (test.wantEdge != nil) {
				t.Fatalf("provider application injection = %v, want %v", built != nil && built.WorkerApplication.ProviderCommandInjected, test.wantEdge != nil)
			}
			if graph == nil || graph.Run == nil {
				t.Fatalf("run graph = %+v, want constructed application", graph)
			}
		})
	}
}

func TestConfigWithFunctionalEdgesCopiesOnlyForReplacement(t *testing.T) {
	t.Parallel()
	injected := &processCommandRunner{}
	cfg := &service.FactoryServiceConfig{}

	production, err := configWithFunctionalEdges(cfg, FunctionalEdges{})
	if err != nil {
		t.Fatalf("configWithFunctionalEdges(production): %v", err)
	}
	if production == cfg || !production.WorkerApplication.Valid() {
		t.Fatalf("production config = %+v, want copied config with constructed worker application", production)
	}

	functional, err := configWithFunctionalEdges(cfg, FunctionalEdges{ProviderCommandRunner: injected})
	if err != nil {
		t.Fatalf("configWithFunctionalEdges(functional): %v", err)
	}
	if functional == cfg || !functional.WorkerApplication.Valid() || !functional.WorkerApplication.ProviderCommandInjected {
		t.Fatalf("functional config = %+v, want copied config with injected provider component", functional)
	}
	if cfg.WorkerApplication.Valid() {
		t.Fatal("functional edge selection mutated the caller-owned config")
	}
}

func TestConfigWithFunctionalEdgesSelectsIndependentWorkerEdges(t *testing.T) {
	t.Parallel()
	providerRunner := &processCommandRunner{}
	scriptRunner := &processCommandRunner{}
	allocator := &agypty.MockAllocator{}

	built, err := configWithFunctionalEdges(&service.FactoryServiceConfig{}, FunctionalEdges{
		ProviderCommandRunner: providerRunner,
		ScriptCommandRunner:   scriptRunner,
		AgyPTYAllocator:       allocator,
	})
	if err != nil {
		t.Fatalf("configWithFunctionalEdges(): %v", err)
	}
	if !built.WorkerApplication.Valid() || !built.WorkerApplication.ProviderCommandInjected {
		t.Fatal("independent worker edges did not produce a functional worker application")
	}
}

func TestConfigWithFunctionalEdgesSelectsIndependentHostedEdges(t *testing.T) {
	t.Parallel()
	httpClient := &http.Client{Timeout: 17 * time.Second}
	clock := clockwork.NewFakeClock()
	resolver := hostedworkers.SecretResolver(func(context.Context, interfaces.RuntimeConfigLookup, string) (string, error) {
		return "functional-secret", nil
	})
	original := &service.FactoryServiceConfig{}

	built, err := configWithFunctionalEdges(original, FunctionalEdges{
		HostedHTTPClient: httpClient, HostedLinearEndpoint: "https://linear.test/graphql",
		HostedSecretResolver: resolver, HostedClock: clock,
	})
	if err != nil {
		t.Fatalf("configWithFunctionalEdges(): %v", err)
	}
	hosted := built.WorkerApplication.Hosted
	if built == original || hosted.HTTPClient != httpClient || hosted.LinearEndpoint != "https://linear.test/graphql" {
		t.Fatalf("hosted HTTP edges were not selected: %+v", built)
	}
	if hosted.SecretResolver == nil || hosted.Clock != clock {
		t.Fatal("hosted secret and clock edges were not selected")
	}
	if original.WorkerApplication.Valid() {
		t.Fatal("hosted functional edge selection mutated caller-owned config")
	}
}

func TestBuildProcessGraphFunctionalWorkerEdgesReachObservableExecution(t *testing.T) {
	t.Parallel()
	factoryDir := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	providerRunner := &processRecordingCommandRunner{}
	scriptRunner := &processRecordingCommandRunner{result: workers.CommandResult{Stdout: []byte("script functional output")}}
	agyAllocator := &processAgyAllocator{}
	httpTransport := &processRoundTripper{}
	httpClient := &http.Client{Transport: httpTransport, Timeout: time.Second}
	hostedClock := clockwork.NewFakeClock()
	probe := &functionalWorkerEdgeProbe{
		t: t, providerRunner: providerRunner, scriptRunner: scriptRunner, agyAllocator: agyAllocator,
		httpTransport: httpTransport, hostedClock: hostedClock,
	}
	resolver := hostedworkers.SecretResolver(func(context.Context, interfaces.RuntimeConfigLookup, string) (string, error) {
		probe.secretCalls++
		return "resolved-functional-secret", nil
	})

	graph, err := buildProcessGraph(context.Background(), startupcli.Request{
		Kind: startupcli.KindRun,
		RunConfig: &runcli.RunConfig{
			Dir: factoryDir, DisableDefaultRecording: true, SuppressDashboardRendering: true,
		},
	}, initializer.ProcessPolicy{
		Mode: initializer.ProcessModeLocalRun, Sidecars: initializer.SidecarPolicy{WorkerScheduler: true},
	}, FunctionalEdges{
		ProviderCommandRunner: providerRunner, ScriptCommandRunner: scriptRunner, AgyPTYAllocator: agyAllocator,
		HostedHTTPClient: httpClient, HostedLinearEndpoint: "https://linear.functional/graphql",
		HostedSecretResolver: resolver, HostedClock: hostedClock,
	}, probe.buildRunner, BuildMCPExecutionService, false)
	if err != nil {
		t.Fatalf("buildProcessGraph() error = %v", err)
	}
	if graph == nil || graph.Run == nil || len(scriptRunner.requests) != 1 {
		t.Fatalf("functional graph = %+v, script calls = %d", graph, len(scriptRunner.requests))
	}
}

type functionalWorkerEdgeProbe struct {
	t              *testing.T
	providerRunner *processRecordingCommandRunner
	scriptRunner   *processRecordingCommandRunner
	agyAllocator   *processAgyAllocator
	httpTransport  *processRoundTripper
	hostedClock    clockwork.Clock
	secretCalls    int
}

func (p *functionalWorkerEdgeProbe) buildRunner(
	_ context.Context,
	cfg *service.FactoryServiceConfig,
	_ initializer.Mode,
) (runcli.RuntimeRunner, error) {
	if err := p.executeScript(cfg); err != nil {
		return nil, err
	}
	if err := p.executeAgy(cfg); err != nil {
		return nil, err
	}
	if err := p.executeHosted(cfg); err != nil {
		return nil, err
	}
	return processRunnerFunc(func(context.Context) error { return nil }), nil
}

func (p *functionalWorkerEdgeProbe) executeScript(cfg *service.FactoryServiceConfig) error {
	script, err := cfg.WorkerApplication.Script.New(
		&interfaces.WorkerConfig{Command: "functional-script"}, logging.NoopLogger{},
	)
	if err != nil {
		return err
	}
	result, err := script.Execute(context.Background(), interfaces.WorkstationExecutionRequest{})
	if err != nil || result.Output != "script functional output" {
		return errors.New("functional script edge did not reach execution")
	}
	return nil
}

func (p *functionalWorkerEdgeProbe) executeAgy(cfg *service.FactoryServiceConfig) error {
	provider, err := cfg.WorkerApplication.Provider.New(workerprovider.WithAgyFactoryRoot(p.t.TempDir()))
	if err != nil {
		return err
	}
	response, err := provider.Execute(context.Background(), interfaces.RunnerExecutionRequest{
		Dispatch: interfaces.WorkDispatch{DispatchID: "functional-agy"}, ModelProvider: string(interfaces.ModelProviderAgy),
		WorkingDirectory: ".", UserMessage: "run through the PTY edge",
	})
	if err != nil || response.Content != "agy functional output" || len(p.agyAllocator.launches) != 1 || len(p.providerRunner.requests) != 0 {
		return errors.New("functional Agy edge did not remain distinct from provider command execution")
	}
	return nil
}

func (p *functionalWorkerEdgeProbe) executeHosted(cfg *service.FactoryServiceConfig) error {
	hosted := cfg.WorkerApplication.Hosted
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, hosted.LinearEndpoint, strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	response, err := hosted.HTTPClient.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	secret, secretErr := hosted.SecretResolver(context.Background(), nil, "linear-token")
	if secretErr != nil || secret != "resolved-functional-secret" || p.httpTransport.calls != 1 || p.secretCalls != 1 || hosted.Clock != p.hostedClock {
		return errors.New("functional hosted edges did not reach execution")
	}
	return nil
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
			name: "runtime", request: MCPExecutionRequest{RuntimeBacked: true, ProjectRoot: copiedSessionExecutionFactory(t)},
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
				if _, ok := runtimeOwnedExecution(service); !ok {
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
					ProjectRoot: copiedSessionExecutionFactory(t),
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
				if _, ok := runtimeOwnedExecution(service); !ok {
					t.Fatalf("service type = %T, want runtime service", service)
				}
			} else if owned, ok := service.(ownedExecutionService); !ok {
				t.Fatalf("service type = %T, want owned fixture service", service)
			} else if _, ok := owned.Service.(*factorysessionexecution.FakeService); !ok {
				t.Fatalf("owned service type = %T, want fixture service", owned.Service)
			}
			if err := service.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
	}
}

func TestRuntimeBackedMCPAndCLIExecutionConsumeOneWireCore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(context.Context, runtimeSessionExecutionCoreBuilder) (factorysessionexecution.Service, error)
	}{
		{
			name: "MCP runtime serve",
			build: func(ctx context.Context, buildCore runtimeSessionExecutionCoreBuilder) (factorysessionexecution.Service, error) {
				return buildMCPExecutionService(ctx, MCPExecutionRequest{
					RuntimeBacked: true, ProjectRoot: copiedSessionExecutionFactory(t),
				}, buildCore)
			},
		},
		{
			name: "CLI session execution",
			build: func(ctx context.Context, buildCore runtimeSessionExecutionCoreBuilder) (factorysessionexecution.Service, error) {
				return buildSessionExecutionService(ctx, sessionexecutioncli.ServiceRequest{
					ExecutionBackendConfig: sessionexecutioncli.ExecutionBackendConfig{
						Provider:    string(factorysessionexecution.ExecutionProviderJavaScriptRuntime),
						ProjectRoot: copiedSessionExecutionFactory(t),
					},
				}, buildCore)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			var core *runtimehost.Core
			service, err := test.build(context.Background(), func(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
				calls++
				built, err := InjectRuntimeCore(ctx, cfg)
				core = built
				return built, err
			})
			if err != nil {
				t.Fatalf("runtime-backed execution build error = %v", err)
			}
			if calls != 1 || core == nil {
				t.Fatalf("runtime core builds = %d, core = %p, want one completed Wire core", calls, core)
			}
			owned, ok := service.(ownedExecutionService)
			if !ok || owned.Service != core.DurableExecution() {
				t.Fatal("runtime-backed compatibility path replaced the Wire-owned durable execution service")
			}
			if owned.graph == nil || owned.graph.SessionRegistry != core.Sessions() || owned.graph.Persistence != core.Persistence() || owned.graph.WorkerProvider != core.RuntimeBuild() {
				t.Fatal("runtime-backed compatibility path did not retain the completed Wire graph")
			}
			owner, ok := owned.Service.(interface{ PersistenceStore() runtimepersist.Store })
			if !ok || owner.PersistenceStore() == nil || owner.PersistenceStore() != core.Persistence() {
				t.Fatal("runtime-backed compatibility path did not retain the Wire-owned persistence store")
			}
			if err := owned.Close(); err != nil {
				t.Fatalf("close retained runtime graph: %v", err)
			}
		})
	}
}

func runtimeOwnedExecution(service factorysessionexecution.Service) (*factorysessionexecution.JavaScriptRuntimeService, bool) {
	owned, ok := service.(ownedExecutionService)
	if !ok {
		return nil, false
	}
	runtime, ok := owned.Service.(*factorysessionexecution.JavaScriptRuntimeService)
	return runtime, ok
}

func copiedSessionExecutionFactory(t *testing.T) string {
	t.Helper()
	return testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory"))
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

func TestBuildProcessGraphRuntimeMCPRetainsCompletedWireGraph(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	process, err := BuildProcessGraph(context.Background(), startupcli.Request{
		Kind: startupcli.KindMCPServe,
		MCP: startupcli.MCPIntent{
			RuntimeBacked: true,
			ProjectRoot:   copiedSessionExecutionFactory(t),
			Stdin:         strings.NewReader(""),
			Stdout:        &output,
		},
	}, initializer.ProcessPolicy{Mode: initializer.ProcessModeMCPServe})
	if err != nil {
		t.Fatalf("BuildProcessGraph() error = %v", err)
	}
	application, ok := process.MCP.(*initializer.Application)
	if !ok {
		t.Fatalf("MCP application type = %T, want initializer application", process.MCP)
	}
	graph, ok := application.Graph().(*Graph)
	if !ok {
		t.Fatalf("MCP graph type = %T, want complete wire graph", application.Graph())
	}
	if graph.SessionRegistry == nil || graph.Persistence == nil || graph.WorkerProvider == nil || graph.DurableExecution == nil {
		t.Fatal("runtime MCP graph dropped a completed Factory Session collaborator")
	}
	if graph.Transport.DurableExecution != graph.DurableExecution {
		t.Fatal("runtime MCP transport did not consume the graph-owned durable execution service")
	}
	if err := graph.Close(); err != nil {
		t.Fatalf("close runtime MCP graph: %v", err)
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
			}, BuildMCPExecutionService, false)
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
