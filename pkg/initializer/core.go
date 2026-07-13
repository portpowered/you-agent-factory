package initializer

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/wire"
)

// Core is the legacy normalized runtime graph retained while compatibility
// transport constructors migrate to wire.Graph.
type Core = runtimehost.Core

// BuildCore retains legacy transport construction behavior. New process startup
// constructs wire.Graph in pkg/root and passes it to Start.
func BuildCore(ctx context.Context, cfg *Config) (*Core, error) {
	return buildCore(ctx, cfg)
}

// Mode identifies the process adapter that consumes an already-constructed
// application graph.
type Mode string

const (
	ModeAPI Mode = "api"
	ModeCLI Mode = "cli"
	ModeMCP Mode = "mcp"
)

// Application owns lifecycle activation for one constructed graph.
type Application struct {
	graph     *wire.Graph
	started   []namedLifecycle
	primary   namedLifecycle
	stopOnce  sync.Once
	stopError error
}

type namedLifecycle struct {
	name      string
	lifecycle wire.Lifecycle
}

// Start activates the selected mode from graph-owned collaborators. It does
// not construct or replace any graph dependency.
func Start(ctx context.Context, mode Mode, graph *wire.Graph) (*Application, error) {
	if ctx == nil {
		return nil, fmt.Errorf("initialize application: context is required")
	}
	if graph == nil {
		return nil, fmt.Errorf("initialize application: graph is required")
	}
	application := &Application{graph: graph}
	selected, err := lifecyclesForMode(mode, graph)
	if err != nil {
		return nil, fmt.Errorf("initialize application: %w", application.failStart(ctx, "process mode", err))
	}
	for _, component := range selected {
		if err := ctx.Err(); err != nil {
			return nil, application.failStart(ctx, component.name, err)
		}
		if err := component.lifecycle.Start(ctx); err != nil {
			return nil, application.failStart(ctx, component.name, err)
		}
		application.started = append(application.started, component)
	}
	application.primary = selected[len(selected)-1]
	return application, nil
}

type waitableLifecycle interface {
	Wait(context.Context) error
}

// Run blocks on the selected graph-owned transport and then performs the same
// idempotent shutdown used by explicit process teardown. This lets production
// command adapters preserve their existing blocking runner contract without
// reconstructing services behind the transport boundary.
func (a *Application) Run(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waiter, ok := a.primary.lifecycle.(waitableLifecycle)
	if !ok {
		return fmt.Errorf("run application: %s lifecycle is not waitable", a.primary.name)
	}
	runErr := waiter.Wait(ctx)
	shutdownErr := a.Shutdown(context.WithoutCancel(ctx))
	if errors.Is(runErr, context.Canceled) && ctx != nil && ctx.Err() != nil {
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

// Graph returns the exact graph consumed during initialization.
func (a *Application) Graph() *wire.Graph {
	if a == nil {
		return nil
	}
	return a.graph
}

// RuntimeLogDiagnostics preserves the CLI startup diagnostic contract using
// immutable graph metadata rather than exposing the runtime host.
func (a *Application) RuntimeLogDiagnostics() runtimehost.RuntimeLogDiagnostics {
	if a == nil || a.graph == nil {
		return runtimehost.RuntimeLogDiagnostics{}
	}
	return a.graph.RuntimeLog
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
	a.stopOnce.Do(func() {
		a.stopError = a.shutdown(ctx)
	})
	return a.stopError
}

func (a *Application) failStart(ctx context.Context, name string, startErr error) error {
	startupErr := fmt.Errorf("start %s: %w", name, startErr)
	cleanupCtx := context.WithoutCancel(ctx)
	if cleanupErr := a.Shutdown(cleanupCtx); cleanupErr != nil {
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

func lifecyclesForMode(mode Mode, graph *wire.Graph) ([]namedLifecycle, error) {
	var sidecars []namedLifecycle
	if graph.Sidecars.Runtime != nil || graph.Sidecars.Workers != nil || graph.Sidecars.Dashboard != nil {
		sidecars = []namedLifecycle{
			{name: "runtime sidecar", lifecycle: graph.Sidecars.Runtime},
			{name: "workers sidecar", lifecycle: graph.Sidecars.Workers},
			{name: "dashboard sidecar", lifecycle: graph.Sidecars.Dashboard},
		}
	}
	switch mode {
	case ModeAPI:
		return append(sidecars, namedLifecycle{name: "API transport", lifecycle: graph.Transports.API}), nil
	case ModeCLI:
		return append(sidecars, namedLifecycle{name: "CLI transport", lifecycle: graph.Transports.CLI}), nil
	case ModeMCP:
		return []namedLifecycle{{name: "MCP transport", lifecycle: graph.Transports.MCP}}, nil
	default:
		return nil, fmt.Errorf("process mode %q is not supported", mode)
	}
}
