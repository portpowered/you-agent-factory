package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/composebridge"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	initializerdashboard "github.com/portpowered/infinite-you/pkg/initializer/dashboard"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	transportmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/transports/mcp/server"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// Inputs contains one explicit domain construction source plus the stdio edges
// required by the MCP transport. Run modes supply Config; MCP-only mode supplies
// MCPExecution. Build copies Config before normalization so graph construction
// does not mutate the caller's configuration object.
type Inputs struct {
	Config       *runtimehost.Config
	MCPExecution factorysessionexecution.Service
	MCPInput     io.Reader
	MCPOutput    io.Writer
}

func provideFactoryServiceRoot(cfg *service.FactoryServiceConfig) (service.FactoryServiceRoot, error) {
	if cfg == nil {
		return service.FactoryServiceRoot{}, fmt.Errorf("factory service config is required")
	}
	if !cfg.WorkerApplication.Valid() {
		components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{})
		if err != nil {
			return service.FactoryServiceRoot{}, fmt.Errorf("construct production worker application: %w", err)
		}
		cfg.WorkerApplication = components
	}
	return service.ResolveFactoryServiceRoot(cfg)
}

func provideBaseLogger(root service.FactoryServiceRoot) *zap.Logger {
	return root.BaseLogger
}

func provideFactorySessionsRegistry() *factorysessions.Registry {
	return service.NewFactorySessionsRegistry()
}

func provideLocalModelDomain(cfg *service.FactoryServiceConfig) service.LocalModelDomain {
	return service.NewLocalModelDomain(cfg)
}

func provideFactoryConfigLoad(
	cfg *service.FactoryServiceConfig,
	root service.FactoryServiceRoot,
) (service.FactoryConfigLoadResult, error) {
	return service.LoadFactoryConfigForCompose(cfg, root)
}

func provideServiceClock(
	cfg *service.FactoryServiceConfig,
	load service.FactoryConfigLoadResult,
) factory.Clock {
	return service.ServiceClockForCompose(cfg, load)
}

func provideRuntimeBuildService(
	cfg *service.FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels service.LocalModelDomain,
	sessions *factorysessions.Registry,
) *runtimebuild.Service {
	domain := localModels
	return service.NewRuntimeBuildService(cfg, clock, baseLogger, &domain, sessions)
}

func provideWorkersSchedulerService(
	cfg *service.FactoryServiceConfig,
	clock factory.Clock,
	logger *zap.Logger,
	hostedWorkers hostedworkers.Config,
) *workersservice.Service {
	return service.NewWorkersSchedulerService(cfg, clock, logger, hostedWorkers)
}

func provideFactoryServiceCollaborators(
	sessions *factorysessions.Registry,
	localModels service.LocalModelDomain,
	runtimeBuild *runtimebuild.Service,
	workersScheduler *workersservice.Service,
) service.FactoryServiceCollaborators {
	return service.NewFactoryServiceCollaboratorsFromParts(sessions, localModels, runtimeBuild, workersScheduler)
}

func provideHostedWorkersConfig(
	cfg *service.FactoryServiceConfig,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return service.NewHostedWorkersConfig(cfg, logger, clock)
}

func provideFactoryService(
	core *service.FactoryCore,
	cfg *service.FactoryServiceConfig,
) *service.FactoryService {
	serviceShell := service.FactoryServiceShell{Service: service.NewFactoryServiceFromCore(core)}
	svc := service.AttachModelServiceCollaborator(serviceShell, service.ProvideModelServiceCollaborator(serviceShell, cfg))
	return service.AttachFactorySaveCollaborator(
		service.FactoryServiceShell{Service: svc},
		service.ProvideFactorySaveCollaborator(service.FactoryServiceShell{Service: svc}, cfg),
	)
}

func provideFactoryCore(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	root service.FactoryServiceRoot,
	collaborators service.FactoryServiceCollaborators,
	load service.FactoryConfigLoadResult,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*service.FactoryCore, error) {
	return service.ComposeFactoryCore(ctx, cfg, root, collaborators, load, clock, hostedWorkers)
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
	if !cfg.WorkerApplication.Valid() {
		components, err := workerapplication.New(cfg.Logger, workerapplication.Edges{})
		if err != nil {
			return nil, fmt.Errorf("build production application graph: %w", err)
		}
		cfg.WorkerApplication = components
	}
	core, err := composebridge.BuildCore(ctx, &cfg)
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
	models := composeModelService(core, host, cfg)
	host = runtimehost.AttachModelServiceCollaborator(shell, models)
	shell = runtimehost.HostShell{Host: host}
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
		Config:            bundle.RuntimeCfg,
		Runtime:           runtimeInputs,
		RuntimeLog:        host.RuntimeLogDiagnostics(),
		Models:            models,
		Workers:           core.WorkersScheduler(),
		WorkerProvider:    core.RuntimeBuild(),
		SessionRegistry:   core.Sessions(),
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

func composeModelService(
	core *runtimehost.Core,
	host *runtimehost.Host,
	cfg *runtimehost.Config,
) apisurface.ModelAPI {
	if cfg != nil && cfg.ModelAPI != nil {
		return cfg.ModelAPI
	}
	if core == nil || host == nil {
		return modelsservice.New(modelsservice.Dependencies{})
	}

	var metrics modelsservice.PullMetricsRecorder
	runnerID := ""
	if cfg != nil && cfg.ModelPullMetricsRecorder != nil {
		metrics = modelPullMetricsAdapter{inner: cfg.ModelPullMetricsRecorder}
	}
	if cfg != nil {
		runnerID = cfg.RunnerID
	}
	now := time.Now
	if core.Clock() != nil {
		now = core.Clock().Now
	}
	return modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig:           host.CurrentModelRuntimeConfig,
		ModelHost:               core.ModelHost(),
		ModelAssetPuller:        core.ModelAssetPuller(),
		Logger:                  core.Logger(),
		Clock:                   now,
		ModelPullMetrics:        metrics,
		ModelInvocationExecutor: host.BuildModelInvocationExecutor,
		FactoryRunnerID:         runnerID,
	})
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
