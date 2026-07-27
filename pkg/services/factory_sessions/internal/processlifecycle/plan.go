package processlifecycle

import (
	"context"
	"errors"
	"fmt"
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
	transport := request.Components.Transport
	if request.Completion != nil {
		transport = newCompletionTransport(transport, request.Completion)
	}
	components = append(components, lifecycle.NamedComponent{
		Name: transportComponentName, Component: transport, Primary: true,
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

// BuildDirectJavaScriptLifecyclePlan owns the smaller lifecycle transaction
// used by a raw workflow-file run. The durable execution service is shared by
// the terminal CLI operation and optional HTTP transport, then closed exactly
// once after both have joined.
func BuildDirectJavaScriptLifecyclePlan(
	transport lifecycle.Component,
	completion func(context.Context) error,
	closeExecution func() error,
) (lifecycle.Plan, error) {
	if completion == nil {
		return lifecycle.Plan{}, errors.New("plan direct JavaScript lifecycle: completion is required")
	}
	primary := lifecycle.Component(lifecycle.NewRunner(completion))
	if !isNil(transport) {
		primary = newCompletionTransport(transport, completion)
	}
	plan := lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: transportComponentName, Component: primary, Primary: true,
	}}}
	if closeExecution != nil {
		plan.Resources = []lifecycle.NamedResource{{
			Name: "direct JavaScript execution", Resource: lifecycle.CloserFunc(closeExecution),
		}}
	}
	if err := lifecycle.Validate(plan); err != nil {
		return lifecycle.Plan{}, errors.Join(
			errors.New("plan direct JavaScript lifecycle"),
			err,
		)
	}
	return plan, nil
}

// completionTransport keeps the API transport alive for one terminal run
// operation. Whichever side finishes first cancels and joins the other, so
// listener startup failure cannot strand a completion waiting for readiness
// and terminal one-shot completion cannot leave the listener behind.
type completionTransport struct {
	transport  lifecycle.Component
	completion *lifecycle.Runner

	mu     sync.Mutex
	runCtx context.Context
	cancel context.CancelFunc
}

type completionTransportResult struct {
	name string
	err  error
}

func newCompletionTransport(
	transport lifecycle.Component,
	completion func(context.Context) error,
) *completionTransport {
	return &completionTransport{
		transport:  transport,
		completion: lifecycle.NewRunner(completion),
	}
}

func (component *completionTransport) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	component.mu.Lock()
	component.runCtx = runCtx
	component.cancel = cancel
	component.mu.Unlock()
	if err := component.transport.Start(runCtx); err != nil {
		cancel()
		return err
	}
	if err := component.completion.Start(runCtx); err != nil {
		cancel()
		return errors.Join(err, component.transport.Stop(context.Background()))
	}
	return nil
}

func (component *completionTransport) Wait(ctx context.Context) error {
	transport, ok := component.transport.(lifecycle.Waiter)
	if !ok {
		return errors.New("application transport is not waitable")
	}
	component.mu.Lock()
	runCtx := component.runCtx
	component.mu.Unlock()
	if runCtx == nil {
		runCtx = ctx
	}
	results := make(chan completionTransportResult, 2)
	go func() {
		results <- completionTransportResult{name: "transport", err: transport.Wait(runCtx)}
	}()
	go func() {
		results <- completionTransportResult{name: "completion", err: component.completion.Wait(runCtx)}
	}()

	first := <-results
	component.cancelRun()
	second := <-results
	return joinCompletionTransportResults(first, second)
}

func (component *completionTransport) Stop(ctx context.Context) error {
	component.cancelRun()
	return errors.Join(
		component.completion.Stop(ctx),
		component.transport.Stop(ctx),
	)
}

func (component *completionTransport) cancelRun() {
	component.mu.Lock()
	cancel := component.cancel
	component.runCtx = nil
	component.cancel = nil
	component.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func joinCompletionTransportResults(first, second completionTransportResult) error {
	var errs []error
	for _, candidate := range []completionTransportResult{first, second} {
		if candidate.err != nil && !errors.Is(candidate.err, context.Canceled) {
			errs = append(errs, fmt.Errorf("%s: %w", candidate.name, candidate.err))
		}
	}
	return errors.Join(errs...)
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
