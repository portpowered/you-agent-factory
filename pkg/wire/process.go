package wire

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// FunctionalEdges contains the process-owned side-effect boundaries that a
// production-shaped functional graph may replace. Its zero value preserves
// every production edge.
type FunctionalEdges struct {
	ProviderCommandRunner workers.CommandRunner
}

// MCPExecutionRequest contains the transport inputs that select the durable
// Factory Session execution collaborator used by MCP serve.
type MCPExecutionRequest struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
}

// MCPExecutionBuilder constructs an MCP durable execution collaborator before
// the initializer starts the stdio transport.
type MCPExecutionBuilder func(context.Context, MCPExecutionRequest) (factorysessionexecution.Service, error)

// BuildMCPExecutionService constructs the durable execution collaborator
// selected by MCP command inputs. Protocol parsing remains in the transport;
// persistence and runtime construction remain in wire.
func BuildMCPExecutionService(
	_ context.Context,
	request MCPExecutionRequest,
) (factorysessionexecution.Service, error) {
	if request.RuntimeBacked {
		projectRoot, err := resolveSessionExecutionProjectRoot(request.ProjectRoot)
		if err != nil {
			return nil, err
		}
		persistence, err := factorysessionexecution.ProjectPersistence(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("initialize runtime-backed persistence: %w", err)
		}
		service, err := factorysessionexecution.NewExecutionService(
			factorysessionexecution.ExecutionProviderJavaScriptRuntime,
			factorysessionexecution.ServiceConfig{ProjectRoot: projectRoot, Persistence: persistence, Clock: factory.EnsureClock(nil)},
		)
		if err != nil {
			return nil, fmt.Errorf("initialize runtime-backed execution service: %w", err)
		}
		return service, nil
	}
	catalogPath, err := resolveSessionExecutionFixtureCatalog(request.FixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return service, nil
}

// BuildProcessGraph constructs the concrete application graph selected by the
// process root without starting transports, sidecars, or runtime loops.
func BuildProcessGraph(ctx context.Context, request startupcli.Request, policy initializer.ProcessPolicy) (*initializer.ProcessGraph, error) {
	return buildProcessGraph(
		ctx, request, policy, FunctionalEdges{}, buildProcessApplicationRunner,
		BuildMCPExecutionService,
	)
}

// BuildProcessGraphWithFunctionalEdges constructs the same application graph
// as BuildProcessGraph while replacing only explicitly supplied process edges.
// Functional edge selection is applied to a copy of invocation-local config.
func BuildProcessGraphWithFunctionalEdges(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	edges FunctionalEdges,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraph(
		ctx, request, policy, edges, buildProcessApplicationRunner,
		BuildMCPExecutionService,
	)
}

// BuildProcessGraphWithMCPBuilder constructs a process graph with an explicit
// MCP execution collaborator builder. The root uses this seam to make all
// production transport dependencies visible before lifecycle startup.
func BuildProcessGraphWithMCPBuilder(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	buildMCP MCPExecutionBuilder,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraph(
		ctx, request, policy, FunctionalEdges{}, buildProcessApplicationRunner,
		buildMCP,
	)
}

type processRunnerBuilder func(
	context.Context,
	*service.FactoryServiceConfig,
	initializer.Mode,
) (runcli.RuntimeRunner, error)

func buildProcessApplicationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	mode initializer.Mode,
) (runcli.RuntimeRunner, error) {
	return buildApplicationRunner(ctx, cfg, mode)
}

func buildProcessGraph(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	edges FunctionalEdges,
	buildRunner processRunnerBuilder,
	buildMCP MCPExecutionBuilder,
	invocationBuilders ...runcli.InvocationBootstrapBuilder,
) (*initializer.ProcessGraph, error) {
	switch request.Kind {
	case startupcli.KindRun:
		if request.RunConfig == nil {
			return nil, fmt.Errorf("construct run graph: run config is required")
		}
		runConfig, err := applyRunProcessPolicy(*request.RunConfig, policy)
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		applicationMode, err := applicationModeForProcess(policy.Mode)
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		invocationBuilder := runcli.InvocationBootstrapBuilder(buildInvocationRunner)
		if len(invocationBuilders) > 0 {
			invocationBuilder = invocationBuilders[0]
		}
		application, err := runcli.BuildApplication(ctx, runConfig, func(
			buildCtx context.Context,
			cfg *service.FactoryServiceConfig,
		) (runcli.RuntimeRunner, error) {
			return buildRunner(buildCtx, configWithFunctionalEdges(cfg, edges), applicationMode)
		}, func(buildCtx context.Context, cfg *service.FactoryServiceConfig) (runcli.InvocationRunner, error) {
			return invocationBuilder(buildCtx, configWithFunctionalEdges(cfg, edges))
		})
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		return &initializer.ProcessGraph{Policy: policy, Run: application}, nil
	case startupcli.KindMCPServe:
		return buildMCPProcessGraph(ctx, request.MCP, policy, buildMCP)
	default:
		return nil, fmt.Errorf("construct process graph: unsupported startup kind %q", request.Kind)
	}
}

func configWithFunctionalEdges(
	cfg *service.FactoryServiceConfig,
	edges FunctionalEdges,
) *service.FactoryServiceConfig {
	if cfg == nil || isNil(edges.ProviderCommandRunner) {
		return cfg
	}
	copied := *cfg
	copied.ProviderCommandRunnerOverride = edges.ProviderCommandRunner
	return &copied
}

func buildMCPProcessGraph(
	ctx context.Context,
	intent startupcli.MCPIntent,
	policy initializer.ProcessPolicy,
	buildMCP MCPExecutionBuilder,
) (*initializer.ProcessGraph, error) {
	if policy.Mode != initializer.ProcessModeMCPServe || policy.Sidecars != (initializer.SidecarPolicy{}) {
		return nil, fmt.Errorf("construct MCP graph: incompatible process policy %+v", policy)
	}
	if buildMCP == nil {
		return nil, fmt.Errorf("construct MCP graph: execution service builder is required")
	}
	execution, err := buildMCP(ctx, MCPExecutionRequest{
		FixtureCatalogPath: intent.FixtureCatalogPath,
		RuntimeBacked:      intent.RuntimeBacked,
		ProjectRoot:        intent.ProjectRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("construct MCP graph: %w", err)
	}
	graph, err := Build(ctx, Inputs{MCPExecution: execution, MCPInput: intent.Stdin, MCPOutput: intent.Stdout})
	if err != nil {
		return nil, fmt.Errorf("construct MCP graph: %w", err)
	}
	application, err := initializer.NewApplication(initializer.ModeMCP, graph)
	if err != nil {
		if cleanupErr := graph.Close(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close rejected MCP application graph: %w", cleanupErr))
		}
		return nil, fmt.Errorf("construct MCP graph: %w", err)
	}
	return &initializer.ProcessGraph{Policy: policy, MCP: application}, nil
}

func buildInvocationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (runcli.InvocationRunner, error) {
	return service.BuildInvocationBootstrap(ctx, cfg)
}

func applicationModeForProcess(mode initializer.ProcessMode) (initializer.Mode, error) {
	switch mode {
	case initializer.ProcessModeDefaultRun, initializer.ProcessModeAPIService:
		return initializer.ModeAPI, nil
	case initializer.ProcessModeLocalRun:
		return initializer.ModeCLI, nil
	default:
		return "", fmt.Errorf("run graph requires a run process mode, got %q", mode)
	}
}

func applyRunProcessPolicy(cfg runcli.RunConfig, policy initializer.ProcessPolicy) (runcli.RunConfig, error) {
	if policy.Sidecars.Dashboard && !policy.Sidecars.API {
		return runcli.RunConfig{}, fmt.Errorf("dashboard sidecar requires API transport")
	}
	switch policy.Mode {
	case initializer.ProcessModeDefaultRun, initializer.ProcessModeAPIService:
		if !policy.Sidecars.WorkerScheduler || !policy.Sidecars.Watchers {
			return runcli.RunConfig{}, fmt.Errorf("%s policy requires worker scheduler and watchers", policy.Mode)
		}
		cfg.Continuously = true
	case initializer.ProcessModeLocalRun:
		if !policy.Sidecars.WorkerScheduler || policy.Sidecars.Watchers {
			return runcli.RunConfig{}, fmt.Errorf("local-run policy requires worker scheduler with watchers disabled")
		}
		cfg.Continuously = false
	default:
		return runcli.RunConfig{}, fmt.Errorf("run graph requires a run process mode, got %q", policy.Mode)
	}
	if !policy.Sidecars.API {
		cfg.Port = 0
	}
	cfg.SuppressDashboardRendering = !policy.Sidecars.Dashboard
	return cfg, nil
}
