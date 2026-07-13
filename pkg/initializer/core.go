package initializer

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

// Lifecycle is the narrow activation contract supplied by an application
// graph. Initializer owns invocation and shutdown ordering.
type Lifecycle interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// ApplicationLifecycles names the graph-owned process edges initializer may
// activate for a selected application mode.
type ApplicationLifecycles struct {
	API       Lifecycle
	CLI       Lifecycle
	MCP       Lifecycle
	Runtime   Lifecycle
	Workers   Lifecycle
	Dashboard Lifecycle
}

// ApplicationGraph is the narrow lifecycle view of an already-constructed
// graph. Domain collaborators remain owned by the concrete graph package.
type ApplicationGraph interface {
	Close() error
	Lifecycles() ApplicationLifecycles
	RuntimeLogMetadata() runtimehost.RuntimeLogDiagnostics
}

// Mode identifies the graph transport selected for one local application.
type Mode string

const (
	ModeAPI Mode = "api"
	ModeCLI Mode = "cli"
	ModeMCP Mode = "mcp"
)

// Application owns activation and shutdown for one constructed graph.
type Application struct {
	graph     ApplicationGraph
	selected  []namedLifecycle
	started   []namedLifecycle
	primary   namedLifecycle
	startOnce sync.Once
	startErr  error
	stopOnce  sync.Once
	stopError error
}

type namedLifecycle struct {
	name      string
	lifecycle Lifecycle
}

// NewApplication validates a graph lifecycle plan without starting it.
func NewApplication(mode Mode, graph ApplicationGraph) (*Application, error) {
	if graph == nil {
		return nil, fmt.Errorf("initialize application: graph is required")
	}
	selected, err := lifecyclesForMode(mode, graph.Lifecycles())
	if err != nil {
		return nil, fmt.Errorf("initialize application: %w", err)
	}
	return &Application{graph: graph, selected: selected, primary: selected[len(selected)-1]}, nil
}

// Start activates the selected mode immediately for compatibility callers.
func Start(ctx context.Context, mode Mode, graph ApplicationGraph) (*Application, error) {
	if ctx == nil {
		return nil, fmt.Errorf("initialize application: context is required")
	}
	application, err := NewApplication(mode, graph)
	if err != nil {
		return nil, err
	}
	if err := application.start(ctx); err != nil {
		return nil, err
	}
	return application, nil
}

type waitableLifecycle interface {
	Wait(context.Context) error
}

// Run starts the selected graph edges, waits for the primary transport, and
// performs the same idempotent shutdown used by explicit process teardown.
func (a *Application) Run(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.start(ctx); err != nil {
		return err
	}
	waiter, ok := a.primary.lifecycle.(waitableLifecycle)
	if !ok {
		return fmt.Errorf("run application: %s lifecycle is not waitable", a.primary.name)
	}
	runErr := waiter.Wait(ctx)
	shutdownErr := a.Shutdown(context.WithoutCancel(ctx))
	if errors.Is(runErr, context.Canceled) && ctx.Err() != nil {
		runErr = nil
	}
	if runErr != nil {
		runErr = fmt.Errorf("run %s: %w", a.primary.name, runErr)
	}
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("shutdown application: %w", shutdownErr)
	}
	return errors.Join(runErr, shutdownErr)
}

func (a *Application) start(ctx context.Context) error {
	a.startOnce.Do(func() {
		for _, component := range a.selected {
			if err := ctx.Err(); err != nil {
				a.startErr = a.failStart(ctx, component.name, err)
				return
			}
			if err := component.lifecycle.Start(ctx); err != nil {
				a.startErr = a.failStart(ctx, component.name, err)
				return
			}
			a.started = append(a.started, component)
		}
	})
	return a.startErr
}

// Graph returns the exact lifecycle graph consumed during initialization.
func (a *Application) Graph() ApplicationGraph {
	if a == nil {
		return nil
	}
	return a.graph
}

// RuntimeLogDiagnostics preserves startup diagnostics as immutable graph
// metadata rather than exposing a runtime host or service facade.
func (a *Application) RuntimeLogDiagnostics() runtimehost.RuntimeLogDiagnostics {
	if a == nil || a.graph == nil {
		return runtimehost.RuntimeLogDiagnostics{}
	}
	return a.graph.RuntimeLogMetadata()
}

// Shutdown stops activated collaborators in reverse order and then releases
// graph construction resources. Repeated calls return the first result.
func (a *Application) Shutdown(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a.stopOnce.Do(func() { a.stopError = a.shutdown(ctx) })
	return a.stopError
}

func (a *Application) failStart(ctx context.Context, name string, startErr error) error {
	startupErr := fmt.Errorf("initialize application: start %s: %w", name, startErr)
	if cleanupErr := a.Shutdown(context.WithoutCancel(ctx)); cleanupErr != nil {
		return errors.Join(startupErr, fmt.Errorf("unwind application startup: %w", cleanupErr))
	}
	return startupErr
}

func (a *Application) shutdown(ctx context.Context) error {
	var result error
	for index := len(a.started) - 1; index >= 0; index-- {
		component := a.started[index]
		if err := component.lifecycle.Stop(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("stop %s: %w", component.name, err))
		}
	}
	if err := a.graph.Close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close application graph: %w", err))
	}
	return result
}

func lifecyclesForMode(mode Mode, lifecycles ApplicationLifecycles) ([]namedLifecycle, error) {
	var sidecars []namedLifecycle
	for _, sidecar := range []namedLifecycle{
		{name: "runtime sidecar", lifecycle: lifecycles.Runtime},
		{name: "workers sidecar", lifecycle: lifecycles.Workers},
		{name: "dashboard sidecar", lifecycle: lifecycles.Dashboard},
	} {
		if sidecar.lifecycle != nil {
			sidecars = append(sidecars, sidecar)
		}
	}
	switch mode {
	case ModeAPI:
		return append(sidecars, namedLifecycle{name: "API transport", lifecycle: lifecycles.API}), nil
	case ModeCLI:
		return append(sidecars, namedLifecycle{name: "CLI transport", lifecycle: lifecycles.CLI}), nil
	case ModeMCP:
		return []namedLifecycle{{name: "MCP transport", lifecycle: lifecycles.MCP}}, nil
	default:
		return nil, fmt.Errorf("process mode %q is not supported", mode)
	}
}
