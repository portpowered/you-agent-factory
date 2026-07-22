// Package lifecycle owns process component activation, joining, reverse-order
// shutdown, and resource cleanup.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

// Component is an inert process component.
type Component interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// Waiter is implemented by the primary blocking component in a plan.
type Waiter interface {
	Wait(context.Context) error
}

// NamedComponent gives lifecycle failures stable operational context.
type NamedComponent struct {
	Name      string
	Component Component
	Primary   bool
}

// NamedResource gives cleanup failures stable operational context.
type NamedResource struct {
	Name     string
	Resource io.Closer
}

// Plan declares one process activation. Components start in declaration order,
// stop in reverse order, and resources close after all components stop.
type Plan struct {
	Components []NamedComponent
	Resources  []NamedResource
}

// Manager executes lifecycle plans. It contains no service-selection policy.
type Manager struct{}

func NewManager() *Manager { return &Manager{} }

// Close releases one owned resource and preserves the construction or runtime
// failure that caused cleanup. All initializer cleanup paths use this function
// so reverse-order closing and error annotation have one implementation.
func Close(name string, resource io.Closer, cause error) error {
	return closeResources([]NamedResource{{
		Name: name, Resource: resource,
	}}, cause)
}

// CloseResources releases an ordered resource set in reverse order while
// preserving the failure that caused cleanup.
func CloseResources(resources []NamedResource, cause error) error {
	return closeResources(resources, cause)
}

// Close releases constructed resources through the same reverse-order error
// aggregation used by Run.
func (*Manager) Close(resources []NamedResource, cause error) error {
	return closeResources(resources, cause)
}

// Run executes the complete lifecycle transaction.
func (*Manager) Run(ctx context.Context, plan Plan) error {
	return run(ctx, plan, nil)
}

// RunWithReady executes a plan and invokes ready after every component starts.
func (*Manager) RunWithReady(ctx context.Context, plan Plan, ready func()) error {
	return run(ctx, plan, ready)
}

// Validate verifies that a plan is complete without starting or closing it.
// Opening edges use this to fail closed before handing lifecycle ownership to
// the manager.
func Validate(plan Plan) error {
	_, err := validate(plan)
	return err
}

func run(ctx context.Context, plan Plan, ready func()) error {
	if ctx == nil {
		ctx = context.Background()
	}
	primary, err := validate(plan)
	if err != nil {
		return closeResources(plan.Resources, err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	started := make([]NamedComponent, 0, len(plan.Components))
	for _, candidate := range plan.Components {
		if err := candidate.Component.Start(runCtx); err != nil {
			startErr := fmt.Errorf("start %s: %w", candidate.Name, err)
			return finish(runCtx, started, plan.Resources, startErr)
		}
		started = append(started, candidate)
	}
	if ready != nil {
		ready()
	}

	waitErr := primary.Component.(Waiter).Wait(runCtx)
	cancel()
	return finish(context.Background(), started, plan.Resources, waitErr)
}

func validate(plan Plan) (NamedComponent, error) {
	var primary NamedComponent
	primaryCount := 0
	for _, candidate := range plan.Components {
		if candidate.Name == "" {
			return NamedComponent{}, errors.New("lifecycle component name is required")
		}
		if isNil(candidate.Component) {
			return NamedComponent{}, fmt.Errorf("lifecycle component %q is required", candidate.Name)
		}
		if candidate.Primary {
			primary = candidate
			primaryCount++
		}
	}
	if primaryCount != 1 {
		return NamedComponent{}, fmt.Errorf("lifecycle plan requires exactly one primary component, got %d", primaryCount)
	}
	if _, ok := primary.Component.(Waiter); !ok {
		return NamedComponent{}, fmt.Errorf("primary lifecycle component %q is not waitable", primary.Name)
	}
	return primary, nil
}

func finish(ctx context.Context, started []NamedComponent, resources []NamedResource, cause error) error {
	errs := make([]error, 0, len(started)+len(resources)+1)
	if cause != nil && !errors.Is(cause, context.Canceled) {
		errs = append(errs, cause)
	}
	for index := len(started) - 1; index >= 0; index-- {
		candidate := started[index]
		if err := candidate.Component.Stop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, fmt.Errorf("stop %s: %w", candidate.Name, err))
		}
	}
	if err := closeResources(resources, nil); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func closeResources(resources []NamedResource, cause error) error {
	errs := make([]error, 0, len(resources)+1)
	if cause != nil {
		errs = append(errs, cause)
	}
	for index := len(resources) - 1; index >= 0; index-- {
		resource := resources[index]
		if isNil(resource.Resource) {
			continue
		}
		if err := resource.Resource.Close(); err != nil {
			name := resource.Name
			if name == "" {
				name = "resource"
			}
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Runner adapts one blocking function to Component and Waiter.
type Runner struct {
	run func(context.Context) error

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	runErr  error
	started bool
}

type RunnerFactory func(func(context.Context) error) *Runner

func NewRunner(run func(context.Context) error) *Runner { return &Runner{run: run} }

func (r *Runner) Start(ctx context.Context) error {
	if r == nil || r.run == nil {
		return errors.New("runner function is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("runner already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.started = true
	go func() {
		err := r.run(runCtx)
		r.mu.Lock()
		r.runErr = err
		close(r.done)
		r.mu.Unlock()
	}()
	return nil
}

func (r *Runner) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	done := r.done
	r.mu.Unlock()
	if done == nil {
		return errors.New("runner not started")
	}
	select {
	case <-done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runner) Stop(context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	<-done
	r.mu.Lock()
	defer r.mu.Unlock()
	if errors.Is(r.runErr, context.Canceled) {
		return nil
	}
	return r.runErr
}

// Functions adapts explicit service start and stop operations.
type Functions struct {
	StartFunc func(context.Context) error
	StopFunc  func(context.Context) error
}

// CloserFunc adapts cleanup functions to io.Closer.
type CloserFunc func() error

func (fn CloserFunc) Close() error {
	if fn == nil {
		return nil
	}
	return fn()
}

func (f Functions) Start(ctx context.Context) error {
	if f.StartFunc == nil {
		return nil
	}
	return f.StartFunc(ctx)
}

func (f Functions) Stop(ctx context.Context) error {
	if f.StopFunc == nil {
		return nil
	}
	return f.StopFunc(ctx)
}
