package processlifecycle

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
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

type cancellationTransitionRuntime struct {
	events []string

	runtimeContext             context.Context
	runtimeAliveAtStartReturn  bool
	workerStopSawRuntimeCancel bool
	workerStarts               int

	startEntered  chan struct{}
	releaseStart  chan struct{}
	workerEntered chan struct{}
	releaseWorker chan struct{}
}

func (runtime *cancellationTransitionRuntime) StartLifecycle(_, runCtx context.Context) error {
	runtime.runtimeContext = runCtx
	runtime.events = append(runtime.events, "runtime:start")
	if runtime.startEntered != nil {
		close(runtime.startEntered)
		<-runtime.releaseStart
	}
	runtime.runtimeAliveAtStartReturn = runCtx.Err() == nil
	return nil
}

func (runtime *cancellationTransitionRuntime) Start(ctx, runCtx context.Context) error {
	return runtime.StartLifecycle(ctx, runCtx)
}

func (runtime *cancellationTransitionRuntime) StartWorkerLifecycle(ctx context.Context) (factorysessions.RuntimeStop, error) {
	runtime.workerStarts++
	runtime.events = append(runtime.events, "workers:start")
	if runtime.workerEntered != nil {
		close(runtime.workerEntered)
		<-runtime.releaseWorker
	}
	return func(context.Context) error {
		runtime.workerStopSawRuntimeCancel = runtime.runtimeContext.Err() != nil
		runtime.events = append(runtime.events, "workers:stop")
		return nil
	}, nil
}

func (runtime *cancellationTransitionRuntime) StartWorkers(ctx context.Context) (factorysessions.RuntimeStop, error) {
	return runtime.StartWorkerLifecycle(ctx)
}

func (*cancellationTransitionRuntime) CompleteStartup(context.Context) error { return nil }

func (*cancellationTransitionRuntime) WaitForRuntime(context.Context) error { return nil }

func (*cancellationTransitionRuntime) RunTransport(context.Context, http.Handler) error { return nil }

func (runtime *cancellationTransitionRuntime) StopLifecycle(context.Context) error {
	runtime.events = append(runtime.events, "runtime:stop")
	return nil
}

func (runtime *cancellationTransitionRuntime) Stop(ctx context.Context) error {
	return runtime.StopLifecycle(ctx)
}

func (*cancellationTransitionRuntime) FailStartup(err error) error { return err }

func (*cancellationTransitionRuntime) CurrentRuntimeBundle() factoryruntime.RuntimeRecord {
	return nil
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

type nonWaitablePlanComponent struct{}

func (*nonWaitablePlanComponent) Start(context.Context) error { return nil }
func (*nonWaitablePlanComponent) Stop(context.Context) error  { return nil }

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
	exerciseBatchColdStartLifecycleCharacterization(t)
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

func TestRuntimeLifecycleRejectsPreActivationCancellationBeforeRuntimeAcquisition(t *testing.T) {
	runtime := &planRuntime{}
	plan := requiredPlan(t, runtime)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := lifecycle.NewManager().Run(ctx, plan); err != nil {
		t.Fatalf("Run error = %v, want canceled activation to unwind quietly", err)
	}
	if len(runtime.events) != 0 {
		t.Fatalf("runtime events = %v, want no runtime acquisition after pre-activation cancellation", runtime.events)
	}
}

func TestRuntimeLifecycleKeepsRuntimeAliveUntilCanceledStartupUnwinds(t *testing.T) {
	runtime := &cancellationTransitionRuntime{
		startEntered: make(chan struct{}),
		releaseStart: make(chan struct{}),
	}
	plan := requiredPlanWithEvents(t, runtime, &runtime.events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- lifecycle.NewManager().Run(ctx, plan) }()

	awaitLifecycleSignal(t, runtime.startEntered)
	cancel()
	close(runtime.releaseStart)
	if err := <-runDone; err != nil {
		t.Fatalf("Run error = %v, want cancellation to unwind cleanly", err)
	}
	if !runtime.runtimeAliveAtStartReturn {
		t.Fatal("runtime context was canceled before the startup transition returned")
	}
	if runtime.workerStarts != 0 {
		t.Fatalf("worker starts = %d, want no worker acquisition after cancellation", runtime.workerStarts)
	}
}

func TestRuntimeLifecycleDrainsWorkerAcquisitionBeforeRuntimeShutdown(t *testing.T) {
	runtime := &cancellationTransitionRuntime{
		workerEntered: make(chan struct{}),
		releaseWorker: make(chan struct{}),
	}
	plan := requiredPlanWithEvents(t, runtime, &runtime.events)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- lifecycle.NewManager().Run(ctx, plan) }()

	awaitLifecycleSignal(t, runtime.workerEntered)
	cancel()
	close(runtime.releaseWorker)
	if err := <-runDone; err != nil {
		t.Fatalf("Run error = %v, want cancellation to unwind cleanly", err)
	}
	if runtime.workerStopSawRuntimeCancel {
		t.Fatal("worker acquisition was stopped after the runtime context was canceled")
	}
}

func TestRuntimeLifecycleFlushesRecordingAfterProducerShutdown(t *testing.T) {
	runtime := &planRuntime{}
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	flushEntered := make(chan struct{})
	releaseFlush := make(chan struct{})
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: transport,
		},
		OrderlyStop: func(context.Context) error {
			close(flushEntered)
			<-releaseFlush
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- lifecycle.NewManager().Run(ctx, plan) }()
	awaitLifecycleSignal(t, transport.started)
	cancel()
	awaitLifecycleSignal(t, flushEntered)
	if want := []string{"runtime:start", "workers:start", "workers:stop", "runtime:stop"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("producer shutdown events = %v, want %v before flush returns", runtime.events, want)
	}
	select {
	case err := <-done:
		t.Fatalf("lifecycle returned before recording flush completed: %v", err)
	default:
	}
	close(releaseFlush)
	if err := <-done; err != nil {
		t.Fatalf("lifecycle run: %v", err)
	}
}

func TestRuntimeLifecyclePropagatesOrderlyRecordingFlushFailure(t *testing.T) {
	flushErr := errors.New("recording flush failed")
	runtime := &planRuntime{}
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: transport,
		},
		OrderlyStop: func(context.Context) error { return flushErr },
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- lifecycle.NewManager().Run(ctx, plan) }()
	awaitLifecycleSignal(t, transport.started)
	cancel()
	if err := <-done; !errors.Is(err, flushErr) {
		t.Fatalf("lifecycle error = %v, want recording flush failure", err)
	}
}

func TestRuntimeLifecycleLeavesOrderlyFlushUnsetWhenRecordingIsDisabled(t *testing.T) {
	runtime := &planRuntime{}
	transport := &blockingPlanComponent{
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}
	plan := requiredPlanWithTransport(t, runtime, transport)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- lifecycle.NewManager().Run(ctx, plan) }()
	awaitLifecycleSignal(t, transport.started)
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("lifecycle without recording flush: %v", err)
	}
}

func awaitLifecycleSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatal("timed out waiting for lifecycle transition")
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

func TestDirectJavaScriptLifecycleRejectsNonWaitableTransport(t *testing.T) {
	plan, err := BuildDirectJavaScriptLifecyclePlan(
		&nonWaitablePlanComponent{},
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildDirectJavaScriptLifecyclePlan: %v", err)
	}
	err = lifecycle.NewManager().Run(t.Context(), plan)
	if err == nil || !strings.Contains(err.Error(), "application transport is not waitable") {
		t.Fatalf("Run error = %v, want non-waitable transport failure", err)
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

func TestLifecycleStateRejectsDuplicateStartsAndNoopStops(t *testing.T) {
	runtime := &planRuntime{}
	state := &applicationRuntimeLifecycle{runtime: runtime}
	ctx := context.Background()

	if err := state.startWorkers(ctx); err == nil || !strings.Contains(err.Error(), "runtime is not started") {
		t.Fatalf("startWorkers before runtime = %v, want runtime-not-started", err)
	}
	if err := state.stopWorkers(ctx); err != nil {
		t.Fatalf("stopWorkers without acquisition: %v", err)
	}
	if err := state.startRuntime(ctx); err != nil {
		t.Fatalf("startRuntime: %v", err)
	}
	if err := state.startRuntime(ctx); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("duplicate startRuntime = %v, want already-started", err)
	}
	if err := state.startWorkers(ctx); err != nil {
		t.Fatalf("startWorkers: %v", err)
	}
	if err := state.startWorkers(ctx); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("duplicate startWorkers = %v, want already-started", err)
	}
	if err := state.stopWorkers(ctx); err != nil {
		t.Fatalf("stopWorkers: %v", err)
	}
	if err := state.stopWorkers(ctx); err != nil {
		t.Fatalf("second stopWorkers: %v", err)
	}
	if err := state.stopRuntime(ctx); err != nil {
		t.Fatalf("stopRuntime: %v", err)
	}
	if err := state.stopRuntime(ctx); err != nil {
		t.Fatalf("second stopRuntime: %v", err)
	}
}

func TestJoinCompletionTransportResultsPreservesOperationalErrors(t *testing.T) {
	transportErr := errors.New("transport failed")
	completionErr := errors.New("completion failed")
	err := joinCompletionTransportResults(
		completionTransportResult{name: "transport", err: transportErr},
		completionTransportResult{name: "completion", err: context.Canceled},
	)
	if !errors.Is(err, transportErr) || strings.Contains(err.Error(), "completion failed") {
		t.Fatalf("joined error = %v, want transport failure without cancellation", err)
	}
	err = joinCompletionTransportResults(
		completionTransportResult{name: "transport", err: transportErr},
		completionTransportResult{name: "completion", err: completionErr},
	)
	if !errors.Is(err, transportErr) || !errors.Is(err, completionErr) {
		t.Fatalf("joined errors = %v, want both operational failures", err)
	}
}

func TestIsNilHandlesNonNilValue(t *testing.T) {
	if isNil(42) {
		t.Fatal("isNil(42) = true, want false")
	}
}

func requiredPlan(t *testing.T, runtime *planRuntime) lifecycle.Plan {
	return requiredPlanWithEvents(t, runtime, &runtime.events)
}

func requiredPlanWithTransport(
	t *testing.T,
	runtime *planRuntime,
	transport lifecycle.Component,
) lifecycle.Plan {
	t.Helper()
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: transport,
		},
	})
	if err != nil {
		t.Fatalf("BuildLifecyclePlan: %v", err)
	}
	return plan
}

func requiredPlanWithEvents(t *testing.T, runtime roles.ProcessRuntime, events *[]string) lifecycle.Plan {
	t.Helper()
	plan, err := BuildLifecyclePlan(roles.LifecyclePlanRequest{
		Runtime: runtime,
		Components: factorysessions.BoundProcessComponents{
			Transport: &planComponent{name: "transport", events: events},
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
