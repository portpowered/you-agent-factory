package processlifecycle

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

const (
	runtimeComponentName       = "runtime sidecar"
	workersComponentName       = "workers sidecar"
	visualizationComponentName = "Factory visualization sidecar"
	transportComponentName     = "application transport"
	runtimeResourceName        = "runtime application"
)

// NewLifecyclePlanOperation returns the Factory Sessions-owned application
// lifecycle planner selected once by the canonical injector.
func NewLifecyclePlanOperation() roles.LifecyclePlanOperation {
	return BuildLifecyclePlan
}

// BuildLifecyclePlan declares the complete product activation transaction.
// Initializer consumes the result without knowing which product components it
// contains or why they are ordered this way.
func BuildLifecyclePlan(request roles.LifecyclePlanRequest) (lifecycle.Plan, error) {
	if isNil(request.Runtime) {
		return lifecycle.Plan{}, errors.New("plan Factory Session lifecycle: runtime is required")
	}
	if isNil(request.Components.Transport) {
		return lifecycle.Plan{}, errors.New("plan Factory Session lifecycle: transport is required")
	}

	state := &applicationRuntimeLifecycle{runtime: request.Runtime}
	components := []lifecycle.NamedComponent{
		{
			Name: runtimeComponentName,
			Component: lifecycle.Functions{
				StartFunc: state.startRuntime,
				StopFunc:  state.stopRuntime,
			},
		},
		{
			Name: workersComponentName,
			Component: lifecycle.Functions{
				StartFunc: state.startWorkers,
				StopFunc:  state.stopWorkers,
			},
		},
	}
	if !isNil(request.Components.Visualization) {
		components = append(components, lifecycle.NamedComponent{
			Name: visualizationComponentName, Component: request.Components.Visualization,
		})
	}
	components = append(components, lifecycle.NamedComponent{
		Name: transportComponentName, Component: request.Components.Transport, Primary: true,
	})

	plan := lifecycle.Plan{Components: components}
	if request.Close != nil {
		plan.Resources = []lifecycle.NamedResource{{
			Name: runtimeResourceName, Resource: lifecycle.CloserFunc(request.Close),
		}}
	}
	if err := lifecycle.Validate(plan); err != nil {
		return lifecycle.Plan{}, errors.Join(errors.New("plan Factory Session lifecycle"), err)
	}
	return plan, nil
}

// applicationRuntimeLifecycle owns the state needed to pair process runtime
// and worker-sidecar acquisition with exactly one cleanup operation.
type applicationRuntimeLifecycle struct {
	runtime roles.ProcessRuntime

	mu            sync.Mutex
	runtimeCancel context.CancelFunc
	runtimeActive bool
	workersCancel context.CancelFunc
	stopWorkersFn factorysessions.RuntimeStop
}

func (state *applicationRuntimeLifecycle) startRuntime(ctx context.Context) error {
	state.mu.Lock()
	if state.runtimeActive || state.runtimeCancel != nil {
		state.mu.Unlock()
		return errors.New("start Factory Session runtime: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	state.runtimeCancel = cancel
	state.mu.Unlock()

	if err := state.runtime.Start(ctx, runCtx); err != nil {
		return errors.Join(err, state.stopRuntime(context.Background()))
	}
	state.mu.Lock()
	state.runtimeActive = true
	state.mu.Unlock()
	return nil
}

func (state *applicationRuntimeLifecycle) startWorkers(ctx context.Context) error {
	state.mu.Lock()
	if !state.runtimeActive {
		state.mu.Unlock()
		return errors.New("start Factory Session workers: runtime is not started")
	}
	if state.workersCancel != nil || state.stopWorkersFn != nil {
		state.mu.Unlock()
		return errors.New("start Factory Session workers: already started")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	state.workersCancel = cancel
	state.mu.Unlock()

	stop, err := state.runtime.StartWorkers(workerCtx)
	if err != nil {
		cancel()
		state.mu.Lock()
		state.workersCancel = nil
		state.mu.Unlock()
		if stop != nil {
			err = errors.Join(err, stop(context.Background()))
		}
		return err
	}
	if stop == nil {
		cancel()
		state.mu.Lock()
		state.workersCancel = nil
		state.mu.Unlock()
		return errors.New("start Factory Session workers: stop operation is required")
	}
	state.mu.Lock()
	state.stopWorkersFn = stop
	state.mu.Unlock()
	return nil
}

func (state *applicationRuntimeLifecycle) stopWorkers(ctx context.Context) error {
	state.mu.Lock()
	cancel, stop := state.workersCancel, state.stopWorkersFn
	state.workersCancel, state.stopWorkersFn = nil, nil
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stop == nil {
		return nil
	}
	return stop(ctx)
}

func (state *applicationRuntimeLifecycle) stopRuntime(ctx context.Context) error {
	state.mu.Lock()
	cancel := state.runtimeCancel
	shouldStop := cancel != nil
	state.runtimeCancel = nil
	state.runtimeActive = false
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if !shouldStop {
		return nil
	}
	return state.runtime.Stop(ctx)
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
