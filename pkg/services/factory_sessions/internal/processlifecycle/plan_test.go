package processlifecycle

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type planRuntime struct {
	events         []string
	startErr       error
	workerErr      error
	workerStop     factorysessions.RuntimeStop
	runtimeStopErr error
}

func (runtime *planRuntime) Start(context.Context, context.Context) error {
	runtime.events = append(runtime.events, "runtime:start")
	return runtime.startErr
}

func (runtime *planRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	runtime.events = append(runtime.events, "workers:start")
	if runtime.workerStop != nil || runtime.workerErr != nil {
		return runtime.workerStop, runtime.workerErr
	}
	return func(context.Context) error {
		runtime.events = append(runtime.events, "workers:stop")
		return nil
	}, nil
}

func (*planRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (runtime *planRuntime) Stop(context.Context) error {
	runtime.events = append(runtime.events, "runtime:stop")
	return runtime.runtimeStopErr
}

type planComponent struct {
	name   string
	events *[]string
}

type blockingPlanComponent struct {
	started chan struct{}
	stopped chan struct{}
	waitErr error
}

func (component *blockingPlanComponent) Start(context.Context) error {
	close(component.started)
	return nil
}

func (component *blockingPlanComponent) Stop(context.Context) error {
	select {
	case <-component.stopped:
	default:
		close(component.stopped)
	}
	return nil
}

func (component *blockingPlanComponent) Wait(ctx context.Context) error {
	if component.waitErr != nil {
		return component.waitErr
	}
	<-ctx.Done()
	return ctx.Err()
}

func (component *planComponent) Start(context.Context) error {
	*component.events = append(*component.events, component.name+":start")
	return nil
}

func (component *planComponent) Stop(context.Context) error {
	*component.events = append(*component.events, component.name+":stop")
	return nil
}

func (component *planComponent) Wait(context.Context) error {
	*component.events = append(*component.events, component.name+":wait")
	return nil
}

func TestRuntimeLifecycleOwnsActivationAndReverseShutdown(t *testing.T) {
	runtime := &planRuntime{}
	visualization := &planComponent{name: "visualization", events: &runtime.events}
	transport := &planComponent{name: "transport", events: &runtime.events}
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Visualization: visualization,
			Transport:     transport,
		},
		Close: func() error {
			runtime.events = append(runtime.events, "runtime:close")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	if err := lifecycle.NewManager().Run(context.Background(), plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{
		"runtime:start", "workers:start", "visualization:start", "transport:start",
		"transport:wait", "transport:stop", "visualization:stop", "workers:stop",
		"runtime:stop", "runtime:close",
	}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
}

func TestRunCompletionStopsAndJoinsOwnedTransport(t *testing.T) {
	runtime := &planRuntime{}
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	completed := make(chan struct{})
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: transport,
		},
		Completion: func(ctx context.Context) error {
			select {
			case <-transport.started:
				close(completed)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	if err := lifecycle.NewManager().Run(t.Context(), plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-completed:
	default:
		t.Fatal("terminal completion did not run after transport activation")
	}
	select {
	case <-transport.stopped:
	default:
		t.Fatal("terminal completion did not join the owned transport")
	}
}

func TestRunTransportFailureCancelsWaitingCompletion(t *testing.T) {
	transportErr := errors.New("listener startup failed")
	runtime := &planRuntime{}
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
		waitErr: transportErr,
	}
	completionCanceled := make(chan struct{})
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: transport,
		},
		Completion: func(ctx context.Context) error {
			<-ctx.Done()
			close(completionCanceled)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	if err := lifecycle.NewManager().Run(t.Context(), plan); !errors.Is(err, transportErr) {
		t.Fatalf("Run error = %v, want %v", err, transportErr)
	}
	select {
	case <-completionCanceled:
	default:
		t.Fatal("listener failure did not cancel the waiting completion")
	}
}

func TestRuntimeLifecycleLeavesRuntimeUnwindAvailableAfterWorkerStartFailure(t *testing.T) {
	sentinel := errors.New("workers failed")
	runtime := &planRuntime{workerErr: sentinel}
	plan := requiredPlan(t, runtime)
	if err := lifecycle.NewManager().Run(context.Background(), plan); !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want %v", err, sentinel)
	}
	want := []string{"runtime:start", "workers:start", "runtime:stop"}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
}

func TestRuntimeLifecycleUnwindsFailedRuntimeStart(t *testing.T) {
	startErr := errors.New("start failed")
	stopErr := errors.New("stop failed")
	runtime := &planRuntime{startErr: startErr, runtimeStopErr: stopErr}
	plan := requiredPlan(t, runtime)
	err := lifecycle.NewManager().Run(context.Background(), plan)
	if !errors.Is(err, startErr) || !errors.Is(err, stopErr) {
		t.Fatalf("Run error = %v, want start and cleanup causes", err)
	}
	want := []string{"runtime:start", "runtime:stop"}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
}

func TestRuntimeLifecycleInvokesReturnedWorkerStopExactlyOnceOnStartFailure(t *testing.T) {
	sentinel := errors.New("workers failed")
	stops := 0
	runtime := &planRuntime{workerErr: sentinel}
	runtime.workerStop = func(context.Context) error {
		stops++
		runtime.events = append(runtime.events, "workers:stop")
		return nil
	}
	plan := requiredPlan(t, runtime)
	if err := lifecycle.NewManager().Run(context.Background(), plan); !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want %v", err, sentinel)
	}
	if stops != 1 {
		t.Fatalf("worker stop count = %d, want 1", stops)
	}
	want := []string{"runtime:start", "workers:start", "workers:stop", "runtime:stop"}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
}

func TestRuntimeLifecycleFailsClosedWhenWorkersReturnNoStopOperation(t *testing.T) {
	runtime := &planRuntime{}
	// A non-nil error-free return with no stop operation is an incomplete
	// acquisition contract and must unwind the already-started runtime.
	plan := requiredPlanWithWorkerResult(t, runtime, nil, nil)
	err := lifecycle.NewManager().Run(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "stop operation is required") {
		t.Fatalf("Run error = %v, want missing worker stop operation", err)
	}
	want := []string{"runtime:start", "workers:start", "runtime:stop"}
	if !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("events = %v, want %v", runtime.events, want)
	}
}

func TestBuildLifecyclePlanFailsClosedWithoutRuntimeOrTransport(t *testing.T) {
	transport := &planComponent{}
	var typedNilRuntime *planRuntime
	for name, request := range map[string]roles.LifecyclePlanRequest{
		"runtime":           {Components: factorysessions.BoundProcessComponents{Transport: transport}},
		"typed nil runtime": {Runtime: typedNilRuntime, Components: factorysessions.BoundProcessComponents{Transport: transport}},
		"transport":         {Runtime: &planRuntime{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildLifecyclePlan(request); err == nil {
				t.Fatal("BuildLifecyclePlan error = nil")
			}
		})
	}
}

func TestDirectJavaScriptLifecycleRunsCompletionAndClosesExecution(t *testing.T) {
	var events []string
	plan, err := BuildDirectJavaScriptLifecyclePlan(
		nil,
		func(context.Context) error {
			events = append(events, "completion")
			return nil
		},
		func() error {
			events = append(events, "close")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("BuildDirectJavaScriptLifecyclePlan: %v", err)
	}
	if err := lifecycle.NewManager().Run(t.Context(), plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"completion", "close"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestDirectJavaScriptLifecycleJoinsOwnedTransport(t *testing.T) {
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	completed := make(chan struct{})
	plan, err := BuildDirectJavaScriptLifecyclePlan(
		transport,
		func(ctx context.Context) error {
			select {
			case <-transport.started:
				close(completed)
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildDirectJavaScriptLifecyclePlan: %v", err)
	}
	if err := lifecycle.NewManager().Run(t.Context(), plan); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-completed:
	default:
		t.Fatal("terminal completion did not run after transport activation")
	}
	select {
	case <-transport.stopped:
	default:
		t.Fatal("terminal completion did not join the owned transport")
	}
}

func TestDirectJavaScriptLifecycleRejectsMissingCompletion(t *testing.T) {
	if _, err := BuildDirectJavaScriptLifecyclePlan(nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "completion is required") {
		t.Fatalf("BuildDirectJavaScriptLifecyclePlan error = %v", err)
	}
	if operation := NewLifecyclePlanOperation(); operation == nil {
		t.Fatal("NewLifecyclePlanOperation returned nil")
	}
}

func requiredPlan(t *testing.T, runtime *planRuntime) lifecycle.Plan {
	t.Helper()
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: &planComponent{name: "transport", events: &runtime.events},
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	return plan
}

type workerResultRuntime struct {
	*planRuntime
	stop factorysessions.RuntimeStop
	err  error
}

func (runtime *workerResultRuntime) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	runtime.events = append(runtime.events, "workers:start")
	return runtime.stop, runtime.err
}

func requiredPlanWithWorkerResult(
	t *testing.T,
	runtime *planRuntime,
	stop factorysessions.RuntimeStop,
	err error,
) lifecycle.Plan {
	t.Helper()
	wrapped := &workerResultRuntime{planRuntime: runtime, stop: stop, err: err}
	plan, planErr := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: wrapped,
		Components: factorysessions.BoundProcessComponents{
			Transport: &planComponent{name: "transport", events: &runtime.events},
		},
	})
	if planErr != nil {
		t.Fatalf("BuildLifecyclePlan: %v", planErr)
	}
	return plan
}
