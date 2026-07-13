package initializer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/wire"
	"go.uber.org/zap"
)

func TestInitialize_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.Config{Dir: t.TempDir()}

	services, errInit := initializer.Initialize(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, service.FactoryServiceConfigFromRuntimeHost(cfg))

	if services != nil {
		t.Fatal("expected Initialize to return nil services without factory.json")
	}
	if errInit == nil {
		t.Fatal("expected Initialize to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("Initialize error = %q, want %q", errInit, errService)
	}
}

func TestInitialize_RejectsInvalidFactoryConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "invalid",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [],
  "workstations": [{
    "name": "orphan",
    "worker": "does-not-exist",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION"
  }]
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &initializer.Config{Dir: dir}

	services, errInit := initializer.Initialize(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, service.FactoryServiceConfigFromRuntimeHost(cfg))

	if services != nil {
		t.Fatal("expected Initialize to return nil services for invalid workstation worker reference")
	}
	if errInit == nil {
		t.Fatal("expected Initialize to fail for invalid workstation worker reference")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail for invalid workstation worker reference")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("Initialize error = %q, want %q", errInit, errService)
	}
}

func TestInitialize_ComposesDomainServicesWithoutFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	services, err := initializer.Initialize(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if services.Sessions == nil {
		t.Fatal("expected session registry")
	}
	if services.Sessions.Get(factorysessions.DefaultSessionID) == nil {
		t.Fatal("expected default session in registry")
	}
	if services.FactoryDefinition == nil {
		t.Fatal("expected factory definition service")
	}
	if services.Models == nil {
		t.Fatal("expected model service")
	}
	if services.Workers.Logger == nil {
		t.Fatal("expected hosted workers config with logger")
	}
	if services.RuntimeHost == nil {
		t.Fatal("expected runtime host service")
	}

	current, err := services.FactoryDefinition.GetCurrentNamedFactory(ctx)
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	if current.Name == "" {
		t.Fatal("expected current factory name")
	}

	models, err := services.Models.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if models.Results == nil {
		t.Fatal("expected model list payload")
	}
}

func TestInitialize_MatchesBuildFactoryServiceOperatorDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "initializer-operator-defaults",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "executor",
    "type": "MODEL_WORKER",
    "body": "You are the executor."
  }],
  "workstations": [{
    "name": "execute-task",
    "worker": "executor",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "type": "MODEL_WORKSTATION",
    "body": "Implement {{ .WorkID }}."
  }]
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &initializer.Config{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider:       "CODEX",
			WorkerModel:               "gpt-5-codex",
			WorkerModelProviderSource: operatorconfig.SourceFlag,
			WorkerModelSource:         operatorconfig.SourceFlag,
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	}

	services, err := initializer.Initialize(ctx, cfg)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	svc, err := service.BuildFactoryService(ctx, service.FactoryServiceConfigFromRuntimeHost(cfg))
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	worker, ok := services.StartupWorkerConfig("executor")
	if !ok || worker == nil {
		t.Fatal("expected executor worker from initializer startup runtime")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("initializer modelProvider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderCodex)
	}

	serviceWorker, ok := svc.StartupWorkerConfig("executor")
	if !ok || serviceWorker == nil {
		t.Fatal("expected executor worker from service startup runtime")
	}
	if worker.ModelProvider != serviceWorker.ModelProvider || worker.Model != serviceWorker.Model {
		t.Fatalf("initializer worker %+v != service worker %+v", worker, serviceWorker)
	}
}

func TestInitialize_GetCurrentFactoryForSession_DefaultSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	services, err := initializer.Initialize(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	factory, err := services.FactoryDefinition.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if factory.Name == "" {
		t.Fatal("expected default session factory name")
	}
}

func TestAPITransport_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var transport *initializer.APITransport
	if transport.SessionAPISurface() != nil {
		t.Fatal("expected nil session API surface for nil transport")
	}
	if err := transport.Run(context.Background()); err != nil {
		t.Fatalf("Run on nil transport: %v", err)
	}
}

func TestCLITransport_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var transport *initializer.CLITransport
	if transport.Runner() != nil {
		t.Fatal("expected nil runner for nil transport")
	}
}

func TestMCPTransport_NilReceiverSessionClientUsesDefault(t *testing.T) {
	t.Parallel()

	var transport *initializer.MCPTransport
	if transport.SessionClient() == nil {
		t.Fatal("expected default MCP session client for nil transport")
	}
}

func TestServices_NilStartupWorkerConfig(t *testing.T) {
	t.Parallel()

	var services *initializer.Services
	if worker, ok := services.StartupWorkerConfig("worker-a"); worker != nil || ok {
		t.Fatalf("StartupWorkerConfig(nil) = (%v, %v), want (nil, false)", worker, ok)
	}
}

func TestSessionRuntimeHost_NilReceiverMethodsAreSafe(t *testing.T) {
	t.Parallel()

	var host *initializer.SessionRuntimeHost
	if host.SessionAPISurface() != nil {
		t.Fatal("expected nil session API surface for nil host")
	}
	if err := host.Run(context.Background()); err != nil {
		t.Fatalf("Run on nil host: %v", err)
	}
	if host.LocalRuntimeRunner() != nil {
		t.Fatal("expected nil local runtime runner for nil host")
	}
	if host.CompatibilityServiceShell() != nil {
		t.Fatal("expected nil compatibility shell for nil host")
	}
}

func TestStartConsumesGraphAndActivatesSelectedMode(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		mode      initializer.Mode
		wantStart []string
	}{
		{name: "API", mode: initializer.ModeAPI, wantStart: []string{"runtime", "workers", "dashboard", "api"}},
		{name: "CLI", mode: initializer.ModeCLI, wantStart: []string{"runtime", "workers", "dashboard", "cli"}},
		{name: "MCP", mode: initializer.ModeMCP, wantStart: []string{"mcp"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newApplicationFixture()
			application, err := initializer.Start(context.Background(), test.mode, fixture.graph)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if application.Graph() != fixture.graph {
				t.Fatal("Start() replaced the constructed graph")
			}
			if !reflect.DeepEqual(fixture.starts, test.wantStart) {
				t.Fatalf("start order = %v, want %v", fixture.starts, test.wantStart)
			}

			if err := application.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown() error = %v", err)
			}
			wantStops := reversed(test.wantStart)
			if !reflect.DeepEqual(fixture.stops, wantStops) {
				t.Fatalf("stop order = %v, want %v", fixture.stops, wantStops)
			}
			if err := application.Shutdown(context.Background()); err != nil {
				t.Fatalf("second Shutdown() error = %v", err)
			}
			if !reflect.DeepEqual(fixture.stops, wantStops) {
				t.Fatalf("second shutdown changed stops to %v", fixture.stops)
			}
		})
	}
}

func TestStartFailureUnwindsActivatedCollaborators(t *testing.T) {
	t.Parallel()

	fixture := newApplicationFixture()
	cause := errors.New("listener unavailable")
	fixture.api.startErr = cause

	application, err := initializer.Start(context.Background(), initializer.ModeAPI, fixture.graph)
	if application != nil || !errors.Is(err, cause) {
		t.Fatalf("Start() = (%v, %v), want nil application wrapping cause", application, err)
	}
	if want := []string{"dashboard", "workers", "runtime"}; !reflect.DeepEqual(fixture.stops, want) {
		t.Fatalf("stop order = %v, want %v", fixture.stops, want)
	}
}

func TestApplicationRunWaitsForSelectedTransportAndShutsDownOnce(t *testing.T) {
	t.Parallel()

	fixture := newApplicationFixture()
	done := make(chan struct{})
	waitable := &waitableApplicationLifecycle{
		applicationLifecycle: applicationLifecycle{name: "cli", starts: &fixture.starts, stops: &fixture.stops},
		done:                 done,
	}
	fixture.graph.Transports.CLI = waitable
	application, err := initializer.Start(context.Background(), initializer.ModeCLI, fixture.graph)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- application.Run(context.Background()) }()
	close(done)
	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := fixture.stops, []string{"cli", "dashboard", "workers", "runtime"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stop order = %v, want %v", got, want)
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if len(fixture.stops) != 4 {
		t.Fatalf("stop calls after repeated shutdown = %v, want exactly once", fixture.stops)
	}
}

type applicationFixture struct {
	graph  *wire.Graph
	starts []string
	stops  []string
	api    *applicationLifecycle
}

func newApplicationFixture() *applicationFixture {
	fixture := &applicationFixture{}
	lifecycle := func(name string) *applicationLifecycle {
		return &applicationLifecycle{name: name, starts: &fixture.starts, stops: &fixture.stops}
	}
	fixture.api = lifecycle("api")
	fixture.graph = &wire.Graph{
		Transports: wire.TransportLifecycles{API: fixture.api, CLI: lifecycle("cli"), MCP: lifecycle("mcp")},
		Sidecars: wire.SidecarLifecycles{
			Runtime:   lifecycle("runtime"),
			Workers:   lifecycle("workers"),
			Dashboard: lifecycle("dashboard"),
		},
	}
	return fixture
}

type applicationLifecycle struct {
	name     string
	starts   *[]string
	stops    *[]string
	startErr error
}

type waitableApplicationLifecycle struct {
	applicationLifecycle
	done chan struct{}
}

func (l *waitableApplicationLifecycle) Wait(ctx context.Context) error {
	select {
	case <-l.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *applicationLifecycle) Start(context.Context) error {
	*l.starts = append(*l.starts, l.name)
	return l.startErr
}

func (l *applicationLifecycle) Stop(context.Context) error {
	*l.stops = append(*l.stops, l.name)
	return nil
}

func reversed(values []string) []string {
	result := make([]string, len(values))
	for index := range values {
		result[len(values)-1-index] = values[index]
	}
	return result
}
