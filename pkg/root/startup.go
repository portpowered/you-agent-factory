package root

import (
	"context"
	"fmt"

	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
)

// Mode is the process behavior selected by the root after command parsing.
type Mode string

const (
	ModeDefaultRun Mode = "default-run"
	ModeLocalRun   Mode = "local-run"
	ModeAPIService Mode = "api-service"
	ModeMCPServe   Mode = "mcp-serve"
)

// SidecarPolicy records which existing long-lived collaborators the selected
// command permits. Lifecycle ownership remains with the initializer.
type SidecarPolicy struct {
	API             bool
	Dashboard       bool
	WorkerScheduler bool
	Watchers        bool
}

// GraphRequest is the narrow construction input selected by the process root.
type GraphRequest struct {
	Mode      Mode
	Sidecars  SidecarPolicy
	Construct startupcli.Construct
}

// ApplicationGraph is the opaque constructed collaborator handed to the
// initializer. Root never inspects or retains runtime-domain state.
type ApplicationGraph interface {
	Lifecycle() startupcli.Lifecycle
}

// GraphBuilder constructs one application graph before lifecycle startup.
type GraphBuilder interface {
	Build(context.Context, GraphRequest) (ApplicationGraph, error)
}

// Initialization is the complete root-to-initializer lifecycle handoff.
type Initialization struct {
	Mode     Mode
	Sidecars SidecarPolicy
	Graph    ApplicationGraph
}

// Initializer starts and owns the lifecycle of an already-constructed graph.
type Initializer interface {
	Run(context.Context, Initialization) error
}

// Dependencies are the only construction and lifecycle capabilities retained
// by the process root.
type Dependencies struct {
	GraphBuilder GraphBuilder
	Initializer  Initializer
}

func executeStartup(ctx context.Context, request startupcli.Request, dependencies Dependencies) error {
	mode, sidecars, err := selectMode(request)
	if err != nil {
		return err
	}
	graph, err := dependencies.GraphBuilder.Build(ctx, GraphRequest{
		Mode: mode, Sidecars: sidecars, Construct: request.Construct,
	})
	if err != nil {
		return fmt.Errorf("construct %s application graph: %w", mode, err)
	}
	if graph == nil {
		return fmt.Errorf("construct %s application graph: builder returned nil graph", mode)
	}
	return dependencies.Initializer.Run(ctx, Initialization{
		Mode: mode, Sidecars: sidecars, Graph: graph,
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

type applicationGraph struct{ lifecycle startupcli.Lifecycle }

func (graph applicationGraph) Lifecycle() startupcli.Lifecycle { return graph.lifecycle }

type graphBuilder struct{}

func (graphBuilder) Build(ctx context.Context, request GraphRequest) (ApplicationGraph, error) {
	if request.Construct == nil {
		return nil, fmt.Errorf("startup constructor is required")
	}
	lifecycle, err := request.Construct(ctx)
	if err != nil {
		return nil, err
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("startup constructor returned nil lifecycle")
	}
	return applicationGraph{lifecycle: lifecycle}, nil
}

type lifecycleInitializer struct{}

func (lifecycleInitializer) Run(ctx context.Context, initialization Initialization) error {
	return initialization.Graph.Lifecycle().Run(ctx)
}

func normalizeDependencies(dependencies Dependencies) Dependencies {
	if dependencies.GraphBuilder == nil {
		dependencies.GraphBuilder = graphBuilder{}
	}
	if dependencies.Initializer == nil {
		dependencies.Initializer = lifecycleInitializer{}
	}
	return dependencies
}
