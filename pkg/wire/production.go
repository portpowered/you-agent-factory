package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/composebridge"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	execution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	transportmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// Inputs contains one explicit domain construction source plus the stdio edges
// required by the MCP transport. Run modes supply Config; MCP-only mode supplies
// MCPExecution. Build copies Config before normalization so graph construction
// does not mutate the caller's configuration object.
type Inputs struct {
	Config       *runtimehost.Config
	MCPExecution execution.Service
	MCPInput     io.Reader
	MCPOutput    io.Writer
}

func provideFactorySessionsRegistry() *factorysessions.Registry {
	return factorysessions.NewRegistry()
}

func provideRuntimeHostConfigFromFactoryService(cfg *service.FactoryServiceConfig) *runtimehost.Config {
	runtimeCfg := service.RuntimeHostConfigFromFactoryService(cfg)
	if runtimeCfg == nil {
		return nil
	}
	copied := *runtimeCfg
	return &copied
}

func provideFactoryServiceFromRuntimeHostCore(
	core *runtimehost.Core,
	cfg *service.FactoryServiceConfig,
) *service.FactoryService {
	serviceShell := service.FactoryServiceShell{Service: service.NewFactoryServiceFromRuntimeHostCore(core)}
	models := core.ModelService()
	svc := service.AttachModelServiceCollaborator(serviceShell, models)
	return service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: svc},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: svc}, cfg),
	)
}

func provideRuntimeHostRoot(cfg *runtimehost.Config) (composebridge.Root, error) {
	if cfg == nil {
		return composebridge.Root{}, fmt.Errorf("runtime host config is required")
	}
	if !cfg.WorkerApplication.Valid() {
		components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{})
		if err != nil {
			return composebridge.Root{}, fmt.Errorf("construct production worker application: %w", err)
		}
		cfg.WorkerApplication = components
	}
	return service.ResolveFactoryServiceRoot(service.FactoryServiceConfigFromRuntimeHost(cfg))
}

func provideRuntimeHostBaseLogger(root composebridge.Root) *zap.Logger {
	return root.BaseLogger
}

func provideRuntimeHostConfigLoad(
	cfg *runtimehost.Config,
	root composebridge.Root,
) (composebridge.ConfigLoad, error) {
	if err := service.EnsureBackendScopeForCompose(cfg, root.BaseLogger); err != nil {
		return composebridge.ConfigLoad{}, err
	}
	return service.LoadFactoryConfigForStartup(cfg, root)
}

func provideRuntimeHostClock(
	cfg *runtimehost.Config,
	load composebridge.ConfigLoad,
) factory.Clock {
	return composebridge.ClockForCompose(cfg, load)
}

func provideRuntimeHostLocalModels(cfg *runtimehost.Config) (composebridge.LocalModelDomain, error) {
	return composebridge.NewLocalModelDomain(cfg)
}

func provideRuntimeHostRuntimeBuild(
	cfg *runtimehost.Config,
	clock factory.Clock,
	logger *zap.Logger,
	localModels composebridge.LocalModelDomain,
	sessions *factorysessions.Registry,
) (runtimeHostBaseBuild, error) {
	build, err := composebridge.NewRuntimeBuildService(cfg, clock, logger, &localModels, sessions)
	return runtimeHostBaseBuild{Service: build}, err
}

type runtimeHostBaseBuild struct{ Service *runtimebuild.Service }

type runtimeHostPersistence struct {
	Choice execution.PersistenceChoice
	Store  runtimepersist.Store
}

func provideRuntimeHostPersistence(cfg *runtimehost.Config, root composebridge.Root) (runtimeHostPersistence, error) {
	projectRoot := durableProjectRoot(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir)
	choice, err := execution.PersistenceChoiceForPolicy(cfg.DurableSessionPersistencePolicy, projectRoot)
	if err != nil {
		return runtimeHostPersistence{}, fmt.Errorf("compose durable session persistence: %w", err)
	}
	return runtimeHostPersistence{Choice: choice, Store: choice.Store()}, nil
}

func provideRuntimeHostDurableExecution(
	cfg *runtimehost.Config,
	root composebridge.Root,
	clock factory.Clock,
	persistence runtimeHostPersistence,
) (execution.Service, error) {
	projectRoot := durableProjectRoot(cfg.ExecutionBaseDir, cfg.Dir, root.FactoryRootDir)
	operatorConfig, err := loadRuntimeHostOperatorConfig(cfg)
	if err != nil {
		return nil, err
	}
	workerPresetIDs := make(map[string]struct{}, len(operatorConfig.WorkerPresets))
	workerPresets := make(map[string]workflowruntime.WorkerPreset, len(operatorConfig.WorkerPresets))
	for _, preset := range operatorConfig.WorkerPresets {
		workerPresetIDs[preset.ID] = struct{}{}
		workerPresets[preset.ID] = workflowruntime.WorkerPreset{
			ModelProvider: preset.ModelProvider, Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
		}
	}
	return execution.NewExecutionService(execution.ExecutionProviderJavaScriptRuntime, execution.ServiceConfig{
		ProjectRoot: projectRoot, Provider: cfg.ProviderOverride,
		ProviderExecutor: providerexecution.NewExecutor(cfg.ProviderOverride),
		Persistence:      persistence.Choice, Clock: clock, WorkerPresetIDs: workerPresetIDs,
		WorkerSettings: workflowruntime.WorkerSettingsConfig{
			Presets: workerPresets, DefaultModelProvider: operatorConfig.Defaults.WorkerModelProvider,
			DefaultModel: operatorConfig.Defaults.WorkerModel,
		},
	})
}

func provideRuntimeHostRecordingBuild(base runtimeHostBaseBuild, owner execution.Service) (*runtimebuild.Service, error) {
	recorder, ok := owner.(interface {
		RecordPetriTokenMutations(string, []interfaces.TokenMutationRecord) error
	})
	if !ok {
		return nil, fmt.Errorf("compose runtime host core: durable execution owner does not record Petri mutations")
	}
	configured, err := base.Service.WithPetriMutationRecorder(recorder.RecordPetriTokenMutations)
	if err != nil {
		return nil, fmt.Errorf("compose runtime host core: %w", err)
	}
	return configured, nil
}

func durableProjectRoot(executionBaseDir, configuredDir, factoryRootDir string) string {
	for _, candidate := range []string{executionBaseDir, configuredDir, factoryRootDir} {
		if root := strings.TrimSpace(candidate); root != "" {
			return root
		}
	}
	return ""
}

func loadRuntimeHostOperatorConfig(cfg *runtimehost.Config) (operatorconfig.FileConfig, error) {
	configPath := strings.TrimSpace(cfg.SystemConfigPath)
	if configPath == "" {
		homeDir := strings.TrimSpace(cfg.SystemConfigHomeDir)
		if homeDir == "" {
			var err error
			homeDir, err = os.UserHomeDir()
			if err != nil {
				return operatorconfig.FileConfig{}, fmt.Errorf("resolve operator config home: %w", err)
			}
		}
		configPath = defaultpaths.OperatorConfigPath(homeDir)
	}
	loaded, err := operatorconfig.LoadFileConfig(configPath)
	if err != nil {
		return operatorconfig.FileConfig{}, fmt.Errorf("compose durable session worker presets: %w", err)
	}
	return loaded, nil
}

func provideRuntimeHostHostedWorkers(
	cfg *runtimehost.Config,
	_ *zap.Logger,
	_ factory.Clock,
) hostedworkers.Config {
	return cfg.WorkerApplication.Hosted
}

func provideRuntimeHostWorkers(
	cfg *runtimehost.Config,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) *workersservice.Service {
	return composebridge.NewWorkersScheduler(cfg, clock, logger, hostedWorkers)
}

func provideRuntimeHostCollaborators(
	sessions *factorysessions.Registry,
	localModels composebridge.LocalModelDomain,
	runtimeBuild *runtimebuild.Service,
	workers *workersservice.Service,
	durableExecution execution.Service,
	persistence runtimeHostPersistence,
) composebridge.Collaborators {
	return composebridge.Collaborators{
		Sessions: sessions, LocalModels: localModels,
		RuntimeBuild: runtimeBuild, WorkersScheduler: workers,
		DurableExecution: durableExecution, Persistence: persistence.Choice,
	}
}

type runtimeHostCoreWithoutModels struct{ Core *runtimehost.Core }

func provideRuntimeHostCore(
	ctx context.Context,
	cfg *runtimehost.Config,
	root composebridge.Root,
	collaborators composebridge.Collaborators,
	load composebridge.ConfigLoad,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (runtimeHostCoreWithoutModels, error) {
	core, err := composebridge.ComposeCore(ctx, cfg, root, collaborators, load, clock, hostedWorkers)
	return runtimeHostCoreWithoutModels{Core: core}, err
}

func provideRuntimeModelServiceDependencies(
	inert runtimeHostCoreWithoutModels,
	cfg *runtimehost.Config,
) (modelsservice.Dependencies, error) {
	core := inert.Core
	if core == nil || core.Clock() == nil {
		return modelsservice.Dependencies{}, errors.New("construct model service dependencies: runtime core and clock are required")
	}
	host := runtimehost.NewHostFromCore(core)
	var metrics modelsservice.PullMetricsRecorder
	if cfg != nil && cfg.ModelPullMetricsRecorder != nil {
		metrics = modelPullMetricsAdapter{inner: cfg.ModelPullMetricsRecorder}
	}
	runnerID := ""
	if cfg != nil {
		runnerID = cfg.RunnerID
	}
	return modelsservice.Dependencies{
		RuntimeConfig:           host.CurrentModelRuntimeConfig,
		ModelHost:               core.ModelHost(),
		ModelAssetPuller:        core.ModelAssetPuller(),
		Logger:                  core.Logger(),
		Clock:                   core.Clock().Now,
		ModelPullMetrics:        metrics,
		ModelInvocationExecutor: host.BuildModelInvocationExecutor,
		FactoryRunnerID:         runnerID,
	}, nil
}

func provideRuntimeModelService(
	deps modelsservice.Dependencies,
	cfg *runtimehost.Config,
) (apisurface.ModelAPI, error) {
	if cfg != nil && !isNil(cfg.ModelAPI) {
		return cfg.ModelAPI, nil
	}
	models, err := modelsservice.NewService(deps)
	if err != nil {
		return nil, fmt.Errorf("construct model service: %w", err)
	}
	return models, nil
}

func provideRuntimeHostCoreWithModels(
	inert runtimeHostCoreWithoutModels,
	models apisurface.ModelAPI,
) (*runtimehost.Core, error) {
	if inert.Core == nil || isNil(models) {
		return nil, errors.New("construct runtime core: model service is required")
	}
	return runtimehost.AttachModelService(inert.Core, models), nil
}

// Build eagerly constructs the real runtime core, domain services,
// and transport lifecycle handles without starting listeners or goroutines.
func Build(ctx context.Context, inputs Inputs) (*Graph, error) {
	if err := validateProductionInputs(ctx, inputs); err != nil {
		return nil, fmt.Errorf("build production application graph: %w", err)
	}
	if inputs.Config == nil {
		return assembleMCPGraph(inputs)
	}

	cfg := *inputs.Config
	core, err := InjectRuntimeCore(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("build production application graph: construct runtime core: %w", err)
	}
	resources := &resourceSet{}
	if bundle := core.StartupBundle(); bundle != nil {
		resources.add("runtime core", closeFunc(func() error {
			return composebridge.CloseRuntimeBundleSinks(bundle.LogSink, bundle.MetricsSink)
		}))
	}

	graph, err := assembleProductionGraph(core, &cfg, inputs, resources)
	if err != nil {
		return nil, failProductionBuild(resources, err)
	}
	return graph, nil
}

func validateProductionInputs(ctx context.Context, inputs Inputs) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case inputs.Config == nil && isNil(inputs.MCPExecution):
		return errors.New("config or MCP execution service is required")
	case inputs.Config != nil && !isNil(inputs.MCPExecution):
		return errors.New("config and MCP execution service are mutually exclusive")
	case inputs.Config != nil && inputs.Config.Logger == nil:
		return errors.New("config.logger is required")
	case inputs.Config != nil && isNil(inputs.Config.Clock):
		return errors.New("config.clock is required")
	case inputs.MCPInput == nil:
		return errors.New("mcpInput is required")
	case inputs.MCPOutput == nil:
		return errors.New("mcpOutput is required")
	default:
		return nil
	}
}

func assembleMCPGraph(inputs Inputs) (*Graph, error) {
	mcp, err := mcpserver.New(mcpserver.Options{
		Client: mcpfactorysession.NewClientWithService(inputs.MCPExecution),
	})
	if err != nil {
		return nil, fmt.Errorf("build production application graph: construct MCP transport dependencies: %w", err)
	}
	transport := newRunnerLifecycle(func(runCtx context.Context) error {
		return mcp.ServeStdio(runCtx, inputs.MCPInput, inputs.MCPOutput)
	})
	return &Graph{
		DurableExecution: inputs.MCPExecution,
		Transport: TransportDependencies{
			DurableExecution: inputs.MCPExecution,
		},
		Transports: TransportLifecycles{MCP: transport},
		resources:  &resourceSet{},
	}, nil
}

func assembleProductionGraph(
	core *runtimehost.Core,
	cfg *runtimehost.Config,
	inputs Inputs,
	resources *resourceSet,
) (*Graph, error) {
	if core == nil || core.StartupBundle() == nil {
		return nil, errors.New("construct runtime core: startup runtime bundle is required")
	}
	host := runtimehost.NewHostFromCore(core)
	shell := runtimehost.HostShell{Host: host}
	models := core.ModelService()
	if isNil(models) {
		return nil, errors.New("construct production graph: runtime core model service is required")
	}
	host = runtimehost.AttachFactorySaveCollaborator(shell, runtimehost.ProvideFactorySaveCollaborator(shell, cfg))
	applicationRuntime, err := runtimehost.NewApplicationRuntime(host)
	if err != nil {
		return nil, fmt.Errorf("construct runtime lifecycle: %w", err)
	}

	sessions := host.SessionAPI()
	definition := host.FactoryDefinitionAPI()
	apiSurface, err := transportmapping.NewSessionAPISurface(
		sessions,
		models,
		definition,
		host.InvocationAPI(),
		host.DurableExecutionAPI(),
	)
	if err != nil {
		return nil, fmt.Errorf("construct API transport dependencies: %w", err)
	}
	mcp, err := mcpserver.New(mcpserver.Options{
		Client: mcpfactorysession.NewClientWithService(core.DurableExecution()),
	})
	if err != nil {
		return nil, fmt.Errorf("construct MCP transport dependencies: %w", err)
	}

	bundle := core.StartupBundle()
	runtimeInputs := RuntimeInputs{
		FactoryRootDir:   core.FactoryRootDir(),
		ExecutionBaseDir: cfg.ExecutionBaseDir,
		Logger:           core.BaseLogger(),
		Clock:            core.Clock(),
	}
	if runtimeInputs.ExecutionBaseDir == "" {
		runtimeInputs.ExecutionBaseDir = core.FactoryRootDir()
	}
	transport := TransportDependencies{
		API:               apiSurface,
		Models:            models,
		FactoryDefinition: definition,
		FactorySessions:   sessions,
		DurableExecution:  core.DurableExecution(),
	}
	sidecars, err := buildProductionSidecars(cfg, host, applicationRuntime)
	if err != nil {
		return nil, err
	}
	return &Graph{
		core:              core,
		Config:            bundle.RuntimeCfg,
		Runtime:           runtimeInputs,
		RuntimeLog:        host.RuntimeLogDiagnostics(),
		Models:            models,
		Workers:           core.WorkersScheduler(),
		WorkerProvider:    core.RuntimeBuild(),
		SessionRegistry:   core.Sessions(),
		Persistence:       core.Persistence(),
		FactorySessions:   sessions,
		FactoryDefinition: definition,
		DurableExecution:  core.DurableExecution(),
		Transport:         transport,
		Transports: TransportLifecycles{
			API: newRunnerLifecycle(func(runCtx context.Context) error {
				return applicationRuntime.RunTransport(runCtx, apiSurface)
			}),
			CLI: newRunnerLifecycle(func(runCtx context.Context) error {
				return applicationRuntime.RunTransport(runCtx, apiSurface)
			}),
			MCP: newRunnerLifecycle(func(runCtx context.Context) error {
				return mcp.ServeStdio(runCtx, inputs.MCPInput, inputs.MCPOutput)
			}),
		},
		Sidecars:  sidecars,
		resources: resources,
	}, nil
}

type modelPullMetricsAdapter struct {
	inner runtimehost.ModelPullMetricsRecorder
}

func (a modelPullMetricsAdapter) RecordModelPullMetric(metric modelsservice.PullMetric) {
	a.inner.RecordModelPullMetric(runtimehost.InvocationMetric{Name: metric.Name, Labels: metric.Labels})
}

func buildProductionSidecars(
	cfg *runtimehost.Config,
	host *runtimehost.Host,
	runtime *runtimehost.ApplicationRuntime,
) (SidecarLifecycles, error) {
	sidecars := SidecarLifecycles{
		Runtime: lifecycleFuncs{start: runtime.StartRuntime, stop: runtime.StopRuntime},
		Workers: lifecycleFuncs{start: runtime.StartWorkers, stop: runtime.StopWorkers},
	}
	if cfg.SimpleDashboardRenderer == nil {
		return sidecars, nil
	}
	dashboard, err := initializerdashboard.NewDashboardSidecar(initializerdashboard.DashboardSidecarConfig{
		Reader: initializerdashboard.NewRuntimeDashboardReader(host),
		Renderer: initializerdashboard.DashboardRendererFunc(func(input initializerdashboard.DashboardRenderInput) {
			cfg.SimpleDashboardRenderer(runtimehost.SimpleDashboardRenderInput{
				EngineState: input.EngineState, RenderData: input.RenderData, Now: input.Now,
			})
		}),
		Timing: initializerdashboard.ClockTiming{Clock: factory.EnsureClock(cfg.Clock)},
		ReportError: func(err error) {
			logger := cfg.Logger
			if logger == nil {
				logger = zap.NewNop()
			}
			logger.Error("simple dashboard render failed", zap.Error(err))
		},
	})
	if err != nil {
		return SidecarLifecycles{}, fmt.Errorf("construct dashboard sidecar: %w", err)
	}
	sidecars.Dashboard = newDashboardLifecycle(dashboard)
	return sidecars, nil
}

func failProductionBuild(resources *resourceSet, constructionErr error) error {
	result := fmt.Errorf("build production application graph: %w", constructionErr)
	if cleanupErr := resources.Close(); cleanupErr != nil {
		return errors.Join(result, fmt.Errorf("cleanup after production graph construction failure: %w", cleanupErr))
	}
	return result
}

type closeFunc func() error

func (fn closeFunc) Close() error { return fn() }

type lifecycleFuncs struct {
	start func(context.Context) error
	stop  func(context.Context) error
}

func (l lifecycleFuncs) Start(ctx context.Context) error { return l.start(ctx) }
func (l lifecycleFuncs) Stop(ctx context.Context) error  { return l.stop(ctx) }

type dashboardLifecycle struct {
	runner    *runnerLifecycle
	dashboard *initializerdashboard.DashboardSidecar
}

func newDashboardLifecycle(dashboard *initializerdashboard.DashboardSidecar) *dashboardLifecycle {
	return &dashboardLifecycle{runner: newRunnerLifecycle(dashboard.Run), dashboard: dashboard}
}

func (l *dashboardLifecycle) Start(ctx context.Context) error { return l.runner.Start(ctx) }

func (l *dashboardLifecycle) Stop(ctx context.Context) error {
	err := l.runner.Stop(ctx)
	l.dashboard.RenderFinal(ctx)
	return err
}

type runnerLifecycle struct {
	run      func(context.Context) error
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	runErr   error
	stopOnce sync.Once
	stopErr  error
}

func newRunnerLifecycle(run func(context.Context) error) *runnerLifecycle {
	return &runnerLifecycle{run: run}
}

func (l *runnerLifecycle) Start(ctx context.Context) error {
	if l == nil || l.run == nil {
		return errors.New("start transport lifecycle: runner is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done != nil {
		return errors.New("start transport lifecycle: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.done = make(chan struct{})
	go func() {
		err := l.run(runCtx)
		l.mu.Lock()
		l.runErr = err
		close(l.done)
		l.mu.Unlock()
	}()
	return nil
}

// Wait blocks until the lifecycle runner exits or ctx is canceled. It does not
// stop the runner; initializer owns cancellation and shutdown ordering.
func (l *runnerLifecycle) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	done := l.done
	l.mu.Unlock()
	if done == nil {
		return errors.New("wait for transport lifecycle: not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *runnerLifecycle) Stop(context.Context) error {
	if l == nil {
		return nil
	}
	l.stopOnce.Do(func() {
		l.mu.Lock()
		cancel, done := l.cancel, l.done
		l.mu.Unlock()
		if cancel == nil || done == nil {
			return
		}
		cancel()
		<-done
		l.mu.Lock()
		err := l.runErr
		l.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			l.stopErr = err
		}
	})
	return l.stopErr
}
