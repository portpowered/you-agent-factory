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

// ProcessMode is the process behavior selected before graph construction.
type ProcessMode string

const (
	ProcessModeDefaultRun ProcessMode = "default-run"
	ProcessModeLocalRun   ProcessMode = "local-run"
	ProcessModeAPIService ProcessMode = "api-service"
	ProcessModeMCPServe   ProcessMode = "mcp-serve"
)

// SidecarPolicy is the authoritative set of transports and background
// collaborators enabled for one process graph.
type SidecarPolicy struct {
	API             bool
	Dashboard       bool
	WorkerScheduler bool
	Watchers        bool
}

// ProcessPolicy is selected by the process root, applied during construction,
// and validated before lifecycle execution.
type ProcessPolicy struct {
	Mode     ProcessMode
	Sidecars SidecarPolicy
}

// ProcessGraph is the concrete, typed application graph assembled before
// initializer lifecycle execution. Exactly one mode graph must be present.
type ProcessGraph struct {
	Policy ProcessPolicy
	Run    RunApplication
	MCP    MCPApplication
}

// RunProcess owns lifecycle execution for an already-constructed process graph.
func RunProcess(ctx context.Context, graph *ProcessGraph) error {
	if graph == nil {
		return fmt.Errorf("initialize process: application graph is required")
	}
	if err := validateProcessPolicy(graph.Policy); err != nil {
		return fmt.Errorf("initialize process: %w", err)
	}
	switch graph.Policy.Mode {
	case ProcessModeDefaultRun, ProcessModeLocalRun, ProcessModeAPIService:
		if graph.Run == nil || graph.MCP != nil {
			return fmt.Errorf("initialize process: run policy requires exactly one run application")
		}
		return graph.Run.Run(ctx)
	case ProcessModeMCPServe:
		if graph.MCP == nil || graph.Run != nil {
			return fmt.Errorf("initialize process: MCP policy requires exactly one MCP application")
		}
		return graph.MCP.Run(ctx)
	default:
		return fmt.Errorf("initialize process: unsupported process mode %q", graph.Policy.Mode)
	}
}

func validateProcessPolicy(policy ProcessPolicy) error {
	if policy.Sidecars.Dashboard && !policy.Sidecars.API {
		return fmt.Errorf("dashboard sidecar requires API transport")
	}
	switch policy.Mode {
	case ProcessModeDefaultRun, ProcessModeAPIService:
		if !policy.Sidecars.WorkerScheduler || !policy.Sidecars.Watchers {
			return fmt.Errorf("%s policy requires worker scheduler and watchers", policy.Mode)
		}
	case ProcessModeLocalRun:
		if !policy.Sidecars.WorkerScheduler || policy.Sidecars.Watchers {
			return fmt.Errorf("local-run policy requires worker scheduler with watchers disabled")
		}
	case ProcessModeMCPServe:
		if policy.Sidecars != (SidecarPolicy{}) {
			return fmt.Errorf("MCP policy does not permit run sidecars")
		}
	default:
		return fmt.Errorf("unsupported process mode %q", policy.Mode)
	}
	return nil
}
