package initializer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

// LocalRuntimeRunner is the already-constructed runtime seam consumed by local
// CLI startup. Dependency construction remains owned by pkg/wire.
type LocalRuntimeRunner interface {
	Run(context.Context) error
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
// graph. Start returns only after the component is ready. Initializer invokes
// Stop at most once, in reverse start order; Stop must cancel and join any work
// the component launched before returning.
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
	mode      Mode
	graph     ApplicationGraph
	selected  []namedLifecycle
	started   []namedLifecycle
	operation sync.Mutex
	runCtx    context.Context
	cancelRun context.CancelFunc
	stopped   bool
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
	primary := selected[len(selected)-1]
	if _, ok := primary.lifecycle.(waitableLifecycle); !ok {
		return nil, fmt.Errorf(
			"initialize application: process mode %q requires %s lifecycle to support join",
			mode,
			primary.name,
		)
	}
	return &Application{mode: mode, graph: graph, selected: selected}, nil
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

// Run starts the selected graph edges, observes all join-capable components,
// and performs the same idempotent shutdown used by explicit process teardown.
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
	return a.waitForTermination()
}

func (a *Application) start(ctx context.Context) error {
	a.startOnce.Do(func() {
		a.operation.Lock()
		defer a.operation.Unlock()
		if a.stopped {
			a.startErr = fmt.Errorf("initialize application: process mode %q was already shut down", a.mode)
			return
		}
		a.runCtx, a.cancelRun = context.WithCancel(ctx)
		for _, component := range a.selected {
			if err := a.runCtx.Err(); err != nil {
				a.startErr = a.failStartLocked(ctx, component.name, err)
				return
			}
			if err := component.lifecycle.Start(a.runCtx); err != nil {
				a.startErr = a.failStartLocked(ctx, component.name, err)
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
	a.operation.Lock()
	defer a.operation.Unlock()
	a.stopOnce.Do(func() { a.stopError = a.shutdownLocked(ctx) })
	return a.stopError
}

func (a *Application) failStartLocked(ctx context.Context, name string, startErr error) error {
	startupErr := fmt.Errorf(
		"initialize application: process mode %q start %s: %w",
		a.mode,
		name,
		startErr,
	)
	a.stopOnce.Do(func() { a.stopError = a.shutdownLocked(context.WithoutCancel(ctx)) })
	if cleanupErr := a.stopError; cleanupErr != nil {
		return errors.Join(startupErr, fmt.Errorf("unwind application startup: %w", cleanupErr))
	}
	return startupErr
}

func (a *Application) shutdownLocked(ctx context.Context) error {
	a.stopped = true
	if a.cancelRun != nil {
		a.cancelRun()
	}
	var result error
	for index := len(a.started) - 1; index >= 0; index-- {
		component := a.started[index]
		if err := component.lifecycle.Stop(ctx); err != nil {
			if isCancellation(err) && a.runCtx != nil && a.runCtx.Err() != nil {
				continue
			}
			result = errors.Join(result, fmt.Errorf("stop %s: %w", component.name, err))
		}
	}
	if err := a.graph.Close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close application graph: %w", err))
	}
	return result
}

type lifecycleResult struct {
	index int
	name  string
	err   error
}

// waitForTermination observes every join-capable component. The first terminal
// exit begins shutdown, but errors are reported in lifecycle-plan order so
// goroutine scheduling cannot change the process result.
func (a *Application) waitForTermination() error {
	results := make(chan lifecycleResult, len(a.started))
	waiterCount := 0
	for index, component := range a.started {
		waiter, ok := component.lifecycle.(waitableLifecycle)
		if !ok {
			continue
		}
		waiterCount++
		go func(index int, component namedLifecycle, waiter waitableLifecycle) {
			results <- lifecycleResult{index: index, name: component.name, err: waiter.Wait(context.Background())}
		}(index, component, waiter)
	}

	collected := make([]lifecycleResult, 0, waiterCount)
	waitResults := 0
	select {
	case result := <-results:
		collected = append(collected, result)
		waitResults++
	case <-a.runCtx.Done():
	}
	if a.cancelRun != nil {
		a.cancelRun()
	}
	shutdownErr := a.Shutdown(context.Background())
	for waitResults < waiterCount {
		collected = append(collected, <-results)
		waitResults++
	}
	if shutdownErr != nil {
		collected = append(collected, lifecycleResult{
			index: len(a.started), name: "shutdown", err: fmt.Errorf("shutdown application: %w", shutdownErr),
		})
	}

	slices.SortFunc(collected, func(left, right lifecycleResult) int {
		return cmp.Compare(left.index, right.index)
	})
	var result error
	for _, lifecycleResult := range collected {
		if lifecycleResult.err == nil || isCancellation(lifecycleResult.err) {
			continue
		}
		if lifecycleResult.name == "shutdown" {
			result = errors.Join(result, lifecycleResult.err)
			continue
		}
		result = errors.Join(result, fmt.Errorf("run %s: %w", lifecycleResult.name, lifecycleResult.err))
	}
	return result
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func lifecyclesForMode(mode Mode, lifecycles ApplicationLifecycles) ([]namedLifecycle, error) {
	var selected []namedLifecycle
	switch mode {
	case ModeAPI:
		selected = []namedLifecycle{
			{name: "runtime sidecar", lifecycle: lifecycles.Runtime},
			{name: "workers sidecar", lifecycle: lifecycles.Workers},
			{name: "dashboard sidecar", lifecycle: lifecycles.Dashboard},
			{name: "API transport", lifecycle: lifecycles.API},
		}
	case ModeCLI:
		selected = []namedLifecycle{
			{name: "runtime sidecar", lifecycle: lifecycles.Runtime},
			{name: "workers sidecar", lifecycle: lifecycles.Workers},
			{name: "dashboard sidecar", lifecycle: lifecycles.Dashboard},
			{name: "CLI transport", lifecycle: lifecycles.CLI},
		}
	case ModeMCP:
		selected = []namedLifecycle{{name: "MCP transport", lifecycle: lifecycles.MCP}}
	default:
		return nil, fmt.Errorf("process mode %q is not supported", mode)
	}

	plan := make([]namedLifecycle, 0, len(selected))
	for _, component := range selected {
		if lifecycleIsNil(component.lifecycle) {
			if component.name == "dashboard sidecar" {
				continue
			}
			return nil, fmt.Errorf("process mode %q requires %s lifecycle", mode, component.name)
		}
		plan = append(plan, component)
	}
	return plan, nil
}

func lifecycleIsNil(lifecycle Lifecycle) bool {
	if lifecycle == nil {
		return true
	}
	value := reflect.ValueOf(lifecycle)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
