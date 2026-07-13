package wire

import (
	"context"
	"errors"
	"fmt"

	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// BuildProcessGraph constructs the concrete application graph selected by the
// process root without starting transports, sidecars, or runtime loops.
func BuildProcessGraph(ctx context.Context, request startupcli.Request, policy initializer.ProcessPolicy) (*initializer.ProcessGraph, error) {
	return buildProcessGraph(ctx, request, policy, func(
		buildCtx context.Context,
		cfg *service.FactoryServiceConfig,
	) (runcli.RuntimeRunner, error) {
		return BuildCLIRunner(buildCtx, cfg)
	})
}

func buildProcessGraph(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	buildRunner runcli.FactoryServiceBuilder,
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
		application, err := runcli.BuildApplication(ctx, runConfig, buildRunner)
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		return &initializer.ProcessGraph{Policy: policy, Run: application}, nil
	case startupcli.KindMCPServe:
		if policy.Mode != initializer.ProcessModeMCPServe || policy.Sidecars != (initializer.SidecarPolicy{}) {
			return nil, fmt.Errorf("construct MCP graph: incompatible process policy %+v", policy)
		}
		execution, err := mcpcli.ResolveServeService(mcpcli.ServeConfig{
			FixtureCatalogPath: request.MCP.FixtureCatalogPath,
			RuntimeBacked:      request.MCP.RuntimeBacked,
			ProjectRoot:        request.MCP.ProjectRoot,
		})
		if err != nil {
			return nil, fmt.Errorf("construct MCP graph: %w", err)
		}
		graph, err := Build(ctx, Inputs{
			MCPExecution: execution,
			MCPInput:     request.MCP.Stdin,
			MCPOutput:    request.MCP.Stdout,
		})
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
	default:
		return nil, fmt.Errorf("construct process graph: unsupported startup kind %q", request.Kind)
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
