package wire

import (
	"context"
	"fmt"

	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
)

// BuildProcessGraph constructs the concrete application graph selected by the
// process root without starting transports, sidecars, or runtime loops.
func BuildProcessGraph(ctx context.Context, request startupcli.Request) (*initializer.ProcessGraph, error) {
	switch request.Kind {
	case startupcli.KindRun:
		if request.RunConfig == nil {
			return nil, fmt.Errorf("construct run graph: run config is required")
		}
		application, err := runcli.BuildApplication(ctx, *request.RunConfig, func(
			buildCtx context.Context,
			cfg *service.FactoryServiceConfig,
		) (runcli.RuntimeRunner, error) {
			return BuildCLIRunner(buildCtx, cfg)
		})
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		return &initializer.ProcessGraph{Run: application}, nil
	case startupcli.KindMCPServe:
		application, err := mcpcli.BuildServeApplication(mcpcli.ServeConfig{
			FixtureCatalogPath: request.MCP.FixtureCatalogPath,
			RuntimeBacked:      request.MCP.RuntimeBacked,
			ProjectRoot:        request.MCP.ProjectRoot,
			Stdin:              request.MCP.Stdin,
			Stdout:             request.MCP.Stdout,
		})
		if err != nil {
			return nil, fmt.Errorf("construct MCP graph: %w", err)
		}
		return &initializer.ProcessGraph{MCP: application}, nil
	default:
		return nil, fmt.Errorf("construct process graph: unsupported startup kind %q", request.Kind)
	}
}
