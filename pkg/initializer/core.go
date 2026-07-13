package initializer

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

// Core is the normalized runtime graph composed before transport facades attach.
type Core = runtimehost.Core

// BuildCore loads factory configuration and composes the normalized runtime graph
// through pkg/initializer as the canonical composition entrypoint.
func BuildCore(ctx context.Context, cfg *Config) (*Core, error) {
	return buildCore(ctx, cfg)
}

// RunApplication is a constructed local runtime graph.
type RunApplication interface {
	Run(context.Context) error
}

// MCPApplication is a constructed MCP transport graph.
type MCPApplication interface {
	Run(context.Context) error
}

// ProcessGraph is the concrete, typed application graph assembled before
// initializer lifecycle execution. Exactly one mode graph must be present.
type ProcessGraph struct {
	Run RunApplication
	MCP MCPApplication
}

// RunProcess owns lifecycle execution for an already-constructed process graph.
func RunProcess(ctx context.Context, graph *ProcessGraph) error {
	if graph == nil {
		return fmt.Errorf("initialize process: application graph is required")
	}
	switch {
	case graph.Run != nil && graph.MCP == nil:
		return graph.Run.Run(ctx)
	case graph.MCP != nil && graph.Run == nil:
		return graph.MCP.Run(ctx)
	default:
		return fmt.Errorf("initialize process: application graph must contain exactly one startup mode")
	}
}
