package root

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// Mode is the process behavior selected by the root after command parsing.
type Mode = initializer.ProcessMode

const (
	ModeDefaultRun = initializer.ProcessModeDefaultRun
	ModeLocalRun   = initializer.ProcessModeLocalRun
	ModeAPIService = initializer.ProcessModeAPIService
	ModeMCPServe   = initializer.ProcessModeMCPServe
)

// SidecarPolicy records which existing long-lived collaborators the selected
// command permits. Lifecycle ownership remains with the initializer.
type SidecarPolicy = initializer.SidecarPolicy

// GraphRequest is the narrow construction input selected by the process root.
type GraphRequest struct {
	Policy  initializer.ProcessPolicy
	Startup startupcli.Request
}

// ApplicationGraph is the typed process graph handed to the initializer. Root
// never inspects or retains its runtime-domain collaborators.
type ApplicationGraph = initializer.ProcessGraph

// GraphBuilder constructs one application graph before lifecycle startup.
type GraphBuilder interface {
	Build(context.Context, GraphRequest) (*ApplicationGraph, error)
}

// Initialization is the complete root-to-initializer lifecycle handoff.
type Initialization struct {
	Graph *ApplicationGraph
}

// Initializer starts and owns the lifecycle of an already-constructed graph.
type Initializer interface {
	Run(context.Context, Initialization) error
}

// Dependencies are the only construction and lifecycle capabilities retained
// by the process root.
type Dependencies struct {
	GraphBuilder          GraphBuilder
	Initializer           Initializer
	BuildSessionExecution sessionexecutioncli.ServiceBuilder
	BuildModelInvocation  modelscli.InvocationBuilder
	BuildMCPExecution     wire.MCPExecutionBuilder
	FunctionalEdges       wire.FunctionalEdges
}

type processDependencies struct {
	core         wire.WireCore
	graphBuilder GraphBuilder
	initializer  Initializer
}

func executeStartup(ctx context.Context, request startupcli.Request, dependencies processDependencies) error {
	mode, sidecars, err := selectMode(request)
	if err != nil {
		return err
	}
	graph, err := dependencies.graphBuilder.Build(ctx, GraphRequest{
		Policy: initializer.ProcessPolicy{Mode: mode, Sidecars: sidecars}, Startup: request,
	})
	if err != nil {
		return fmt.Errorf("construct %s application graph: %w", mode, err)
	}
	if graph == nil {
		return fmt.Errorf("construct %s application graph: builder returned nil graph", mode)
	}
	return dependencies.initializer.Run(ctx, Initialization{
		Graph: graph,
	})
}

func selectMode(request startupcli.Request) (Mode, SidecarPolicy, error) {
	switch request.Kind {
	case startupcli.KindMCPServe:
		return ModeMCPServe, SidecarPolicy{}, nil
	case startupcli.KindRun:
		sidecars := SidecarPolicy{
			API: request.Run.APIEnabled, Dashboard: request.Run.DashboardEnabled,
			WorkerScheduler: request.Run.WorkerSidecarsEnabled,
			Watchers:        request.Run.Continuous,
		}
		switch {
		case request.Run.DefaultInvocation:
			return ModeDefaultRun, sidecars, nil
		case request.Run.Continuous:
			return ModeAPIService, sidecars, nil
		default:
			return ModeLocalRun, sidecars, nil
		}
	default:
		return "", SidecarPolicy{}, fmt.Errorf("select process mode: unsupported startup kind %q", request.Kind)
	}
}

func dependenciesFromWireCore(core wire.WireCore, overrides Dependencies) processDependencies {
	runtimeMCPGraph := overrides.BuildMCPExecution == nil
	if overrides.BuildMCPExecution != nil {
		core.BuildMCPExecution = overrides.BuildMCPExecution
	}
	if overrides.BuildSessionExecution != nil {
		core.BuildSessionExecution = overrides.BuildSessionExecution
	} else {
		core.BuildSessionExecution = wire.SessionExecutionBuilderWithEdges(
			core.BuildWorkerApplication,
			overrides.FunctionalEdges,
		)
	}
	if overrides.BuildModelInvocation != nil {
		core.BuildModelInvocation = overrides.BuildModelInvocation
	}
	graphBuilder := overrides.GraphBuilder
	if graphBuilder == nil {
		graphBuilder = productionGraphBuilder{
			buildGraph: core.BuildProcessGraph,
			dependencies: wire.ProcessGraphDependencies{
				BuildMCPExecution:      core.BuildMCPExecution,
				BuildWorkerApplication: core.BuildWorkerApplication,
				BuildSessionExecution:  core.BuildRunSessionExecution,
				FunctionalEdges:        overrides.FunctionalEdges,
				RuntimeMCPGraph:        runtimeMCPGraph,
			},
		}
	}
	processInitializer := overrides.Initializer
	if processInitializer == nil {
		processInitializer = productionInitializer{initialize: core.InitializeProcess}
	}
	return processDependencies{core: core, graphBuilder: graphBuilder, initializer: processInitializer}
}
