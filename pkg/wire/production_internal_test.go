package wire

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type recordingRuntimeOwner struct {
	factorysessionexecution.Service
	sessionID string
	records   []interfaces.TokenMutationRecord
}

func TestProductionGraphRetainsDefaultModelServiceAndInjectedAssetEdge(t *testing.T) {
	t.Parallel()

	dir, assets := modelCatalogFixture(t)
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(),
			Clock: productionInternalClock{}, ModelAssets: assets,
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	t.Cleanup(func() { _ = graph.Close() })

	if graph.core == nil || graph.core.ModelService() == nil ||
		graph.core.ModelService() != graph.Models || graph.Models != graph.Transport.Models {
		t.Fatal("production core, graph, and transport did not retain the default model service instance")
	}
	models, err := graph.Transport.Models.ListModels(context.Background())
	if err != nil {
		t.Fatalf("transport model catalog call error = %v", err)
	}
	if len(models.Results) != 1 || assets.inspectCalls != 1 {
		t.Fatalf("catalog results/asset inspections = (%d, %d), want (1, 1)", len(models.Results), assets.inspectCalls)
	}
}

func TestFactoryServiceCompatibilityFacadeUsesWireModelProviderAndInjectedAssetEdge(t *testing.T) {
	t.Parallel()

	dir, assets := modelCatalogFixture(t)
	svc, err := InjectFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir: dir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(),
		Clock: productionInternalClock{}, ModelAssets: assets,
		DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
		RuntimeFileLoggingPolicy:                service.RuntimeFileLoggingPolicyDisabled,
		RuntimeMetricsPolicy:                    service.RuntimeMetricsPolicyDisabled,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("InjectFactoryService() error = %v", err)
	}

	models, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("compatibility model catalog call error = %v", err)
	}
	if !svc.ComposeCollaboratorSnapshot().ModelServiceInitialized || len(models.Results) != 1 || assets.inspectCalls != 1 {
		t.Fatalf("model service/catalog results/asset inspections = (%t, %d, %d), want (true, 1, 1)",
			svc.ComposeCollaboratorSnapshot().ModelServiceInitialized, len(models.Results), assets.inspectCalls)
	}
}

func modelCatalogFixture(t *testing.T) (string, *recordingModelAssets) {
	t.Helper()
	dir := t.TempDir()
	factoryCfg := factoryfixtures.MinimalFactoryConfig()
	factoryCfg["workers"] = []map[string]any{
		{"name": "worker-a", "type": "SCRIPT_WORKER", "command": "test", "body": "Process work."},
		{
			"name": "model-worker", "type": "INFERENCE_WORKER", "modelProvider": "CODEX",
			"model": "OMNIVOICE_Q4_K_M", "modelLocality": interfaces.ModelLocalityLocal,
			"resources": []map[string]any{{"name": "omnivoice-cache", "capacity": 1}},
		},
	}
	factoryCfg["workstations"].([]map[string]any)[0]["type"] = "SCRIPT_RUN"
	factoryCfg["workstations"].([]map[string]any)[0]["body"] = "Run the worker."
	factoryCfg["resources"] = []map[string]any{{
		"name": "omnivoice-cache", "type": "MODEL", "capacity": 1,
		"model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP", "loadPolicy": "ON_DEMAND",
	}}
	factoryfixtures.WriteFactoryJSON(t, dir, factoryCfg)
	return dir, &recordingModelAssets{AssetPuller: localmodels.NewAssetPuller(t.TempDir())}
}

type recordingModelAssets struct {
	localmodels.AssetPuller
	inspectCalls int
}

func (assets *recordingModelAssets) InspectRuntimeCache(
	ctx context.Context,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	modelName string,
) (localmodels.RuntimeCacheInspection, error) {
	assets.inspectCalls++
	return assets.AssetPuller.InspectRuntimeCache(ctx, runtimeCfg, modelName)
}

func TestProductionGraphRetainsCoreModelServiceAndInjectedCatalogEdge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	injected := &injectedModelAPI{}
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), Logger: zap.NewNop(),
			Clock: productionInternalClock{}, ModelAPI: injected,
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	t.Cleanup(func() { _ = graph.Close() })

	if graph.core == nil || graph.core.ModelService() != injected || graph.Models != injected || graph.Transport.Models != injected {
		t.Fatal("production core, graph, and transport did not retain the exact injected model service instance")
	}
	if _, err := graph.Transport.Models.ListModels(context.Background()); err != nil {
		t.Fatalf("transport model catalog call error = %v", err)
	}
	if injected.listCalls != 1 {
		t.Fatalf("injected model catalog calls = %d, want 1", injected.listCalls)
	}
}

type injectedModelAPI struct{ listCalls int }

func (api *injectedModelAPI) ListModels(context.Context) (factoryapi.ListModelsResponse, error) {
	api.listCalls++
	return factoryapi.ListModelsResponse{}, nil
}

func (*injectedModelAPI) GetModel(context.Context, string) (factoryapi.ModelDetail, error) {
	return factoryapi.ModelDetail{}, nil
}

func (*injectedModelAPI) PullModel(context.Context, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (*injectedModelAPI) InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return apisurface.ModelInvocationResult{}, nil
}

func (owner *recordingRuntimeOwner) RecordPetriTokenMutations(
	sessionID string,
	records []interfaces.TokenMutationRecord,
) error {
	owner.sessionID = sessionID
	owner.records = append([]interfaces.TokenMutationRecord(nil), records...)
	return nil
}

func TestRuntimeHostRecordingBuildUsesGraphOwnedDurableExecution(t *testing.T) {
	t.Parallel()

	var built runtimebuild.SessionBuildSpec
	base, err := runtimebuild.New(runtimebuild.Config{}, factory.EnsureClock(nil), zap.NewNop(), func(
		_ context.Context,
		spec runtimebuild.SessionBuildSpec,
	) (any, error) {
		built = spec
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("runtimebuild.New: %v", err)
	}
	owner := &recordingRuntimeOwner{Service: factorysessionexecution.NewFakeService()}
	configured, err := base.WithPetriMutationRecorder(owner.RecordPetriTokenMutations)
	if err != nil {
		t.Fatalf("WithPetriMutationRecorder: %v", err)
	}
	if _, err := configured.Build(context.Background(), runtimebuild.SessionBuildSpec{SessionID: "root-session"}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	cfg := &factory.FactoryConfig{}
	for _, option := range built.AdditionalFactoryOpts {
		option(cfg)
	}
	want := []interfaces.TokenMutationRecord{{TransitionID: "completed"}}
	if cfg.PetriMutationRecorder == nil {
		t.Fatal("configured runtime build omitted the durable execution recorder")
	}
	if err := cfg.PetriMutationRecorder("root-session", want); err != nil {
		t.Fatalf("PetriMutationRecorder: %v", err)
	}
	if owner.sessionID != "root-session" || len(owner.records) != 1 || owner.records[0].TransitionID != "completed" {
		t.Fatalf("recorded mutations = (%q, %#v), want graph-owned root-session completion", owner.sessionID, owner.records)
	}
}

func TestProductionGraphSidecarsStartAndStopThroughInitializer(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	transportStarted := make(chan struct{})
	var dashboardRenders int
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), RuntimeMode: interfaces.RuntimeModeService,
			Port: 43174, Logger: zap.NewNop(), Clock: productionInternalClock{},
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
			APIServerStarter: func(ctx context.Context, _ apisurface.APISurface, _ int, _ *zap.Logger) error {
				close(transportStarted)
				<-ctx.Done()
				return ctx.Err()
			},
			SimpleDashboardRenderer: func(runtimehost.SimpleDashboardRenderInput) { dashboardRenders++ },
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	recorder := &lifecycleOrder{}
	graph.Sidecars.Runtime = recorder.wrap("runtime", graph.Sidecars.Runtime)
	graph.Sidecars.Workers = recorder.wrap("workers", graph.Sidecars.Workers)
	graph.Sidecars.Dashboard = recorder.wrap("dashboard", graph.Sidecars.Dashboard)
	graph.Transports.CLI = recorder.wrap("cli", graph.Transports.CLI)
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- application.Run(ctx) }()
	select {
	case <-transportStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("graph-owned transport did not start")
	}
	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("Application.Run() error = %v", err)
	}
	if got, want := recorder.started(), []string{"runtime", "workers", "dashboard", "cli"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("production start order = %v, want %v", got, want)
	}
	if got, want := recorder.stopped(), []string{"cli", "dashboard", "workers", "runtime"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("production stop order = %v, want %v", got, want)
	}
	if dashboardRenders != 1 {
		t.Fatalf("dashboard render count = %d, want one final render", dashboardRenders)
	}
}

func TestProductionWorkerSidecarUsesGraphSchedulerAndConfigBeforeTransport(t *testing.T) {
	dir := t.TempDir()
	factoryConfig := factoryfixtures.MinimalFactoryConfig()
	factoryConfig["workstations"] = append(factoryConfig["workstations"].([]map[string]any), map[string]any{
		"name":     "scheduled-task",
		"behavior": "CRON",
		"worker":   "worker-a",
		"cron":     map[string]any{"schedule": "0 * * * *"},
		"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
	})
	factoryfixtures.WriteFactoryJSON(t, dir, factoryConfig)

	logCore, observedLogs := observer.New(zap.InfoLevel)
	transportStarted := make(chan struct{})
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), RuntimeMode: interfaces.RuntimeModeService,
			Port: 43176, Logger: zap.New(logCore), Clock: productionInternalClock{},
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
			APIServerStarter: func(ctx context.Context, _ apisurface.APISurface, _ int, _ *zap.Logger) error {
				registered := observedLogs.FilterMessage("cron watcher registered").All()
				if len(registered) != 1 || registered[0].ContextMap()["workstation"] != "scheduled-task" {
					return fmt.Errorf("transport started before graph worker scheduler was ready: logs=%v", observedLogs.All())
				}
				close(transportStarted)
				<-ctx.Done()
				return ctx.Err()
			},
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	workerScheduler := graph.Workers
	if workerScheduler == nil || graph.Sidecars.Workers == nil {
		t.Fatal("production graph omitted its worker scheduler or worker lifecycle")
	}

	application, err := initializer.NewApplication(initializer.ModeAPI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if application.Graph() != graph || graph.Workers != workerScheduler {
		t.Fatal("initializer replaced the graph or worker scheduler instance")
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- application.Run(ctx) }()
	select {
	case <-transportStarted:
	case err := <-runDone:
		t.Fatalf("Application.Run() returned before transport startup: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("graph-owned transport did not start after worker readiness")
	}
	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("Application.Run() error = %v", err)
	}
}

func TestProductionMCPModeLeavesWorkerSidecarsInactive(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), RuntimeMode: interfaces.RuntimeModeService,
			Logger: zap.NewNop(), Clock: productionInternalClock{},
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	recorder := &lifecycleOrder{}
	graph.Sidecars.Runtime = recorder.wrap("runtime", graph.Sidecars.Runtime)
	graph.Sidecars.Workers = recorder.wrap("workers", graph.Sidecars.Workers)
	graph.Sidecars.Dashboard = recorder.wrap("dashboard", graph.Sidecars.Dashboard)

	application, err := initializer.NewApplication(initializer.ModeMCP, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Application.Run() error = %v", err)
	}
	if starts, stops := recorder.started(), recorder.stopped(); len(starts) != 0 || len(stops) != 0 {
		t.Fatalf("MCP run sidecar effects = starts %v stops %v, want none", starts, stops)
	}
}

func TestProductionCLIModePreservesTransportFailureAfterDashboardShutdown(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	var dashboardRenders int
	graph, err := Build(context.Background(), Inputs{
		Config: &runtimehost.Config{
			Dir: dir, SystemConfigHomeDir: t.TempDir(), RuntimeMode: interfaces.RuntimeModeService,
			Port: 43175, Logger: zap.NewNop(), Clock: productionInternalClock{},
			DurableSessionPersistencePolicy:         factorysessionexecution.PersistencePolicyDisabled,
			RuntimeFileLoggingPolicy:                runtimehost.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:                    runtimehost.RuntimeMetricsPolicyDisabled,
			SkipBuiltInRunnerPrerequisiteValidation: true,
			SimpleDashboardRenderer:                 func(runtimehost.SimpleDashboardRenderInput) { dashboardRenders++ },
		},
		MCPInput: strings.NewReader(""), MCPOutput: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	transportErr := errors.New("CLI runner failed")
	cli := newRunnerLifecycle(func(context.Context) error { return transportErr })
	graph.Transports.CLI = cli
	application, err := initializer.NewApplication(initializer.ModeCLI, graph)
	if err != nil {
		t.Fatalf("NewApplication() error = %v", err)
	}
	if application.Graph() != graph || graph.Transports.CLI != cli {
		t.Fatal("CLI mode replaced the supplied graph or CLI lifecycle")
	}
	if err := application.Run(context.Background()); !errors.Is(err, transportErr) {
		t.Fatalf("Application.Run() error = %v, want CLI runner failure", err)
	}
	if dashboardRenders != 1 {
		t.Fatalf("dashboard render count = %d, want one final render after CLI failure", dashboardRenders)
	}
}

func TestProductionDashboardDisabledOmitsEveryDashboardLifecycleEffect(t *testing.T) {
	var runtime runtimehost.ApplicationRuntime
	sidecars, err := buildProductionSidecars(
		&runtimehost.Config{SimpleDashboardRenderer: nil},
		nil,
		&runtime,
	)
	if err != nil {
		t.Fatalf("buildProductionSidecars() error = %v", err)
	}
	if sidecars.Dashboard != nil {
		t.Fatalf("dashboard lifecycle = %T, want nil when rendering is disabled", sidecars.Dashboard)
	}
}

func TestDashboardLifecycleJoinsPeriodicLoopBeforeFinalRender(t *testing.T) {
	ticker := &joiningDashboardTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}
	finalRendered := make(chan struct{}, 1)
	sidecar, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader: dashboardReaderFunc(func(context.Context, time.Time) (initializerdashboard.DashboardRenderInput, error) {
			select {
			case <-ticker.stopped:
				return initializerdashboard.DashboardRenderInput{}, nil
			default:
				return initializerdashboard.DashboardRenderInput{}, errors.New("final render preceded dashboard join")
			}
		}),
		Renderer: dashboardRendererFunc(func(initializerdashboard.DashboardRenderInput) {
			finalRendered <- struct{}{}
		}),
		Timing: dashboardTiming{ticker: ticker},
	})
	if err != nil {
		t.Fatalf("NewDashboardSidecar() error = %v", err)
	}
	lifecycle := newDashboardLifecycle(sidecar)
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-finalRendered:
	default:
		t.Fatal("dashboard lifecycle did not render after joining its periodic loop")
	}
}

type lifecycleOrder struct {
	mu     sync.Mutex
	starts []string
	stops  []string
}

func (o *lifecycleOrder) wrap(name string, lifecycle Lifecycle) Lifecycle {
	recording := &recordingProductionLifecycle{order: o, name: name, lifecycle: lifecycle}
	waiter, ok := lifecycle.(interface{ Wait(context.Context) error })
	if !ok {
		return recording
	}
	return &recordingWaitableProductionLifecycle{recordingProductionLifecycle: recording, waiter: waiter}
}

type recordingProductionLifecycle struct {
	order     *lifecycleOrder
	name      string
	lifecycle Lifecycle
}

func (l *recordingProductionLifecycle) Start(ctx context.Context) error {
	l.order.mu.Lock()
	l.order.starts = append(l.order.starts, l.name)
	l.order.mu.Unlock()
	return l.lifecycle.Start(ctx)
}

func (l *recordingProductionLifecycle) Stop(ctx context.Context) error {
	l.order.mu.Lock()
	l.order.stops = append(l.order.stops, l.name)
	l.order.mu.Unlock()
	return l.lifecycle.Stop(ctx)
}

type recordingWaitableProductionLifecycle struct {
	*recordingProductionLifecycle
	waiter interface{ Wait(context.Context) error }
}

func (l *recordingWaitableProductionLifecycle) Wait(ctx context.Context) error {
	return l.waiter.Wait(ctx)
}

func (o *lifecycleOrder) started() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.starts...)
}

func (o *lifecycleOrder) stopped() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.stops...)
}

func TestValidateProductionInputsReportsEachMissingEdge(t *testing.T) {
	t.Parallel()

	valid := func() Inputs {
		return Inputs{
			Config:    &runtimehost.Config{Logger: zap.NewNop(), Clock: productionInternalClock{}},
			MCPInput:  strings.NewReader(""),
			MCPOutput: &bytes.Buffer{},
		}
	}
	tests := []struct {
		name string
		ctx  func() context.Context
		edit func(*Inputs)
		want string
	}{
		{name: "nil context", ctx: func() context.Context { return nil }, edit: func(*Inputs) {}, want: "context is required"},
		{name: "canceled context", ctx: canceledProductionContext, edit: func(*Inputs) {}, want: context.Canceled.Error()},
		{name: "construction source", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config = nil }, want: "config or MCP execution service is required"},
		{name: "ambiguous construction source", ctx: context.Background, edit: func(inputs *Inputs) {
			inputs.MCPExecution = &factorysessionexecution.FakeService{}
		}, want: "config and MCP execution service are mutually exclusive"},
		{name: "logger", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config.Logger = nil }, want: "config.logger is required"},
		{name: "clock", ctx: context.Background, edit: func(inputs *Inputs) { inputs.Config.Clock = (*productionInternalClock)(nil) }, want: "config.clock is required"},
		{name: "MCP input", ctx: context.Background, edit: func(inputs *Inputs) { inputs.MCPInput = nil }, want: "mcpInput is required"},
		{name: "MCP output", ctx: context.Background, edit: func(inputs *Inputs) { inputs.MCPOutput = nil }, want: "mcpOutput is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			inputs := valid()
			test.edit(&inputs)
			if err := validateProductionInputs(test.ctx(), inputs); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateProductionInputs() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAssembleProductionGraphRejectsMissingStartupBundle(t *testing.T) {
	t.Parallel()

	for _, core := range []*runtimehost.Core{nil, {}} {
		graph, err := assembleProductionGraph(core, &runtimehost.Config{}, Inputs{}, &resourceSet{})
		if graph != nil || err == nil || !strings.Contains(err.Error(), "startup runtime bundle is required") {
			t.Fatalf("assembleProductionGraph() = (%v, %v), want missing bundle error", graph, err)
		}
	}
}

func TestFailProductionBuildRetainsConstructionAndCleanupErrors(t *testing.T) {
	t.Parallel()

	constructionErr := errors.New("transport construction failed")
	cleanupErr := errors.New("runtime sink close failed")
	resources := &resourceSet{}
	resources.add("runtime core", &recordingCloser{err: cleanupErr})
	err := failProductionBuild(resources, constructionErr)
	if !errors.Is(err, constructionErr) || !errors.Is(err, cleanupErr) {
		t.Fatalf("failProductionBuild() error = %v, want construction and cleanup causes", err)
	}

	if err := failProductionBuild(&resourceSet{}, constructionErr); !errors.Is(err, constructionErr) {
		t.Fatalf("failProductionBuild() without cleanup error = %v, want construction cause", err)
	}
}

func TestRunnerLifecycleWaitAndStopBehavior(t *testing.T) {
	t.Parallel()

	var nilLifecycle *runnerLifecycle
	if err := nilLifecycle.Start(context.Background()); err == nil {
		t.Fatal("nil runner lifecycle Start() succeeded")
	}
	if err := nilLifecycle.Wait(context.Background()); err != nil {
		t.Fatalf("nil runner lifecycle Wait() error = %v", err)
	}
	if err := nilLifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("nil runner lifecycle Stop() error = %v", err)
	}

	lifecycle := newRunnerLifecycle(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if err := lifecycle.Wait(context.Background()); err == nil {
		t.Fatal("Wait() before Start() succeeded")
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := lifecycle.Start(context.Background()); err == nil {
		t.Fatal("second Start() succeeded")
	}
	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if err := lifecycle.Wait(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait(canceled context) error = %v, want context.Canceled", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := lifecycle.Wait(nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait(nil) after stop error = %v, want runner cancellation", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() error = %v", err)
	}
}

func TestRunnerLifecycleReturnsRunnerFailureFromWaitAndStop(t *testing.T) {
	t.Parallel()

	cause := errors.New("listener failed")
	lifecycle := newRunnerLifecycle(func(context.Context) error { return cause })
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := lifecycle.Wait(context.Background()); !errors.Is(err, cause) {
		t.Fatalf("Wait() error = %v, want runner cause", err)
	}
	if err := lifecycle.Stop(context.Background()); !errors.Is(err, cause) {
		t.Fatalf("Stop() error = %v, want runner cause", err)
	}
}

func canceledProductionContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type productionInternalClock struct{}

func (productionInternalClock) Now() time.Time { return time.Unix(0, 0).UTC() }

type dashboardReaderFunc func(context.Context, time.Time) (initializerdashboard.DashboardRenderInput, error)

func (fn dashboardReaderFunc) ReadDashboard(
	ctx context.Context,
	now time.Time,
) (initializerdashboard.DashboardRenderInput, error) {
	return fn(ctx, now)
}

type dashboardRendererFunc func(initializerdashboard.DashboardRenderInput)

func (fn dashboardRendererFunc) RenderDashboard(input initializerdashboard.DashboardRenderInput) {
	fn(input)
}

type dashboardTiming struct {
	ticker initializerdashboard.DashboardTicker
}

func (dashboardTiming) Now() time.Time { return time.Unix(0, 0).UTC() }

func (timing dashboardTiming) NewTicker(time.Duration) initializerdashboard.DashboardTicker {
	return timing.ticker
}

type joiningDashboardTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (ticker *joiningDashboardTicker) C() <-chan time.Time { return ticker.ticks }

func (ticker *joiningDashboardTicker) Stop() {
	ticker.once.Do(func() { close(ticker.stopped) })
}
