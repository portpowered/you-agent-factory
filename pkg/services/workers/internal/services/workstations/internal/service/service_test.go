package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

type recordingExecutor struct {
	requests []workers.WorkstationExecutionRequest
	result   workers.WorkResult
	err      error
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

type controlledExecutor struct {
	mu       sync.Mutex
	running  int
	max      int
	started  chan string
	releases map[string]chan struct{}
}

type panicOnceExecutor struct {
	mu    sync.Mutex
	calls int
}

func (executor *panicOnceExecutor) Execute(
	_ context.Context,
	_ workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	executor.calls++
	if executor.calls == 1 {
		panic("executor panic")
	}
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

func newControlledExecutor(dispatchIDs ...string) *controlledExecutor {
	executor := &controlledExecutor{
		started:  make(chan string, len(dispatchIDs)),
		releases: make(map[string]chan struct{}, len(dispatchIDs)),
	}
	for _, dispatchID := range dispatchIDs {
		executor.releases[dispatchID] = make(chan struct{})
	}
	return executor
}

func (executor *controlledExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	dispatchID := request.Dispatch.DispatchID
	executor.mu.Lock()
	executor.running++
	if executor.running > executor.max {
		executor.max = executor.running
	}
	release := executor.releases[dispatchID]
	executor.mu.Unlock()
	defer func() {
		executor.mu.Lock()
		executor.running--
		executor.mu.Unlock()
	}()
	executor.started <- dispatchID
	select {
	case <-release:
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

func (executor *controlledExecutor) release(dispatchID string) {
	close(executor.releases[dispatchID])
}

func (executor *blockingExecutor) Execute(
	ctx context.Context,
	_ workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	close(executor.started)
	select {
	case <-executor.release:
		return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
}

func (executor *recordingExecutor) Execute(
	_ context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	executor.requests = append(executor.requests, request)
	return executor.result, executor.err
}

func TestPoolLifecycleMakesOnlyStartedRoutesAvailable(t *testing.T) {
	t.Parallel()

	pool := New()
	if err := pool.route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed route error = %v, want ErrWorkstationPoolUnavailable", err)
	}

	outcome, err := pool.start(context.Background(), []workstations.Route{
		{WorkstationName: "review"},
		{WorkstationName: "implement"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("Start() outcome = %q, want STARTED", outcome)
	}
	if err := pool.route(context.Background(), "review"); err != nil {
		t.Fatalf("started route error = %v", err)
	}
	if err := pool.route(context.Background(), "missing"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("unknown route error = %v, want ErrUnknownWorkstationRoute", err)
	}

	outcome, err = pool.start(context.Background(), []workstations.Route{{WorkstationName: "other"}})
	if err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyRunning {
		t.Fatalf("repeated Start() outcome = %q, want ALREADY_RUNNING", outcome)
	}
	if err := pool.route(context.Background(), "other"); !errors.Is(err, workers.ErrUnknownWorkstationRoute) {
		t.Fatalf("repeated start replaced routes: error = %v", err)
	}

	outcome, err = pool.stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	if err := pool.route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("stopped route error = %v, want ErrWorkstationPoolStopped", err)
	}
	if _, err := pool.start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Start() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func TestPoolTranslatesWorkersRootLifecycleContracts(t *testing.T) {
	t.Parallel()
	pool := New()
	started, err := pool.Start(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation,
		}}},
	)
	if err != nil || started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("Start() = %#v, %v", started, err)
	}
	route, err := pool.Route(
		context.Background(),
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	)
	if err != nil || !route.Available || route.WorkstationName != "review" {
		t.Fatalf("Route() = %#v, %v", route, err)
	}
	stopped, err := pool.Stop(context.Background())
	if err != nil || stopped.Outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() = %#v, %v", stopped, err)
	}
	invalid := New()
	if result, err := invalid.Start(
		context.Background(),
		workers.WorkstationPoolStartRequest{Bindings: []workers.AssembledRuntimeBinding{{
			RoleName: "worker", RoleKind: workers.RuntimeBuildRoleKindWorker,
		}}},
	); !errors.Is(err, workers.ErrInvalidWorkstationPoolStart) ||
		result != (workers.WorkstationPoolStartResult{}) {
		t.Fatalf("invalid Start() = %#v, %v", result, err)
	}
}

func TestPoolStopIsRepeatSafeUnderConcurrentCalls(t *testing.T) {
	t.Parallel()

	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{{WorkstationName: "review"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	const callers = 32
	outcomes := make(chan workers.WorkstationPoolLifecycleOutcome, callers)
	errorsCh := make(chan error, callers)
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			outcome, err := pool.stop(context.Background())
			outcomes <- outcome
			errorsCh <- err
		}()
	}
	wait.Wait()
	close(outcomes)
	close(errorsCh)

	stopped := 0
	alreadyStopped := 0
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent Stop() error = %v", err)
		}
	}
	for outcome := range outcomes {
		switch outcome {
		case workers.WorkstationPoolLifecycleOutcomeStopped:
			stopped++
		case workers.WorkstationPoolLifecycleOutcomeAlreadyStopped:
			alreadyStopped++
		default:
			t.Fatalf("concurrent Stop() outcome = %q", outcome)
		}
	}
	if stopped != 1 || alreadyStopped != callers-1 {
		t.Fatalf("stop outcomes = stopped:%d already:%d", stopped, alreadyStopped)
	}
}

func TestPoolRejectsInvalidRoutesAndCancelledLifecycleCalls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		routes []workstations.Route
	}{
		{name: "empty"},
		{name: "blank", routes: []workstations.Route{{WorkstationName: " "}}},
		{
			name: "duplicate",
			routes: []workstations.Route{
				{WorkstationName: "review"},
				{WorkstationName: " review "},
			},
		},
		{
			name:   "negative capacity",
			routes: []workstations.Route{{WorkstationName: "review", Capacity: -1}},
		},
		{
			name: "negative queue capacity",
			routes: []workstations.Route{{
				WorkstationName: "review",
				QueueCapacity:   -1,
			}},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New().start(context.Background(), testCase.routes); !errors.Is(err, workers.ErrInvalidWorkstationPoolStart) {
				t.Fatalf("Start() error = %v, want ErrInvalidWorkstationPoolStart", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool := New()
	if _, err := pool.start(ctx, []workstations.Route{{WorkstationName: "review"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Start() error = %v, want context.Canceled", err)
	}
	if _, err := pool.stop(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Stop() error = %v, want context.Canceled", err)
	}
}

func TestPoolCanStopBeforeStartWithoutActivating(t *testing.T) {
	t.Parallel()

	pool := New()
	outcome, err := pool.stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() outcome = %q, want STOPPED", outcome)
	}
	outcome, err = pool.stop(context.Background())
	if err != nil {
		t.Fatalf("repeated Stop() error = %v", err)
	}
	if outcome != workers.WorkstationPoolLifecycleOutcomeAlreadyStopped {
		t.Fatalf("repeated Stop() outcome = %q, want ALREADY_STOPPED", outcome)
	}
}

func TestPoolDispatchesToImmutableRouteBindingWithDetachedAttribution(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{result: workers.WorkResult{
		DispatchID:   "wrong-dispatch",
		TransitionID: "wrong-transition",
		Outcome:      workers.OutcomeAccepted,
		Output:       "done",
	}}
	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{
		{
			WorkstationName: "review",
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDCodex,
				Source:   workers.RunnerSelectionSourceWorkstation,
			},
			Executor: executor,
		},
		{
			WorkstationName: "implement",
			RunnerSelection: workers.ResolvedRunnerSelection{
				RunnerID: workers.RunnerIDGemini,
				Source:   workers.RunnerSelectionSourceFactory,
			},
			Executor: executor,
		},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	request := dispatchRequest("dispatch-review", "transition-review", "review")
	wantExecution := workers.CloneWorkstationExecutionRequest(request.Execution)
	wantExecution.RunnerID = workers.RunnerIDCodex
	wantExecution.RunnerSelectionSource = workers.RunnerSelectionSourceWorkstation
	result, err := pool.Dispatch(context.Background(), request)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	assertExecutionRequests(t, executor.requests, wantExecution)
	assertDispatchResult(
		t,
		result,
		"dispatch-review",
		"transition-review",
		"review",
		workers.OutcomeAccepted,
		"done",
	)

	request.Execution.InputTokens[0] = "mutated-after-call"
	if executor.requests[0].InputTokens[0] != "input-token" {
		t.Fatalf("executor request aliases caller input: %#v", executor.requests[0])
	}

	second := dispatchRequest("dispatch-implement", "transition-implement", "implement")
	if _, err := pool.Dispatch(context.Background(), second); err != nil {
		t.Fatalf("second Dispatch() error = %v", err)
	}
	if got := executor.requests[1]; got.RunnerID != workers.RunnerIDGemini ||
		got.RunnerSelectionSource != workers.RunnerSelectionSourceFactory ||
		got.Dispatch.WorkstationName != "implement" {
		t.Fatalf("second executor request = %#v", got)
	}
}

func assertExecutionRequests(
	t *testing.T,
	got []workers.WorkstationExecutionRequest,
	want workers.WorkstationExecutionRequest,
) {
	t.Helper()
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("executor requests = %#v, want %#v", got, want)
	}
}

func assertDispatchResult(
	t *testing.T,
	got workers.WorkstationDispatchResult,
	dispatchID string,
	transitionID string,
	workstationName string,
	outcome workers.WorkOutcome,
	output string,
) {
	t.Helper()
	if got.DispatchID != dispatchID ||
		got.WorkstationName != workstationName ||
		got.Result.DispatchID != dispatchID ||
		got.Result.TransitionID != transitionID ||
		got.Result.Outcome != outcome ||
		got.Result.Output != output {
		t.Fatalf("Dispatch() result = %#v", got)
	}
}

func TestPoolDispatchReturnsTypedRoutingFailuresWithoutExecutorEntry(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{}
	pool := New()
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-1", "transition-1", "review"),
	); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed Dispatch() error = %v, want ErrWorkstationPoolUnavailable", err)
	}
	if _, err := pool.start(context.Background(), []workstations.Route{
		{WorkstationName: "review", Executor: executor},
		{WorkstationName: "unbound"},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	testCases := []struct {
		name    string
		request workers.WorkstationDispatchRequest
		want    error
	}{
		{
			name:    "unknown route",
			request: dispatchRequest("dispatch-1", "transition-1", "missing"),
			want:    workers.ErrUnknownWorkstationRoute,
		},
		{
			name:    "missing binding",
			request: dispatchRequest("dispatch-2", "transition-2", "unbound"),
			want:    workers.ErrMissingWorkstationBinding,
		},
		{
			name:    "missing dispatch identity",
			request: dispatchRequest("", "transition-3", "review"),
			want:    workers.ErrInvalidWorkstationDispatch,
		},
		{
			name: "mismatched workstation identity",
			request: workers.WorkstationDispatchRequest{
				WorkstationName: "review",
				Execution: workers.WorkstationExecutionRequest{
					Dispatch: work.WorkDispatch{
						DispatchID:      "dispatch-4",
						WorkstationName: "implement",
					},
				},
			},
			want: workers.ErrInvalidWorkstationDispatch,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := pool.Dispatch(context.Background(), testCase.request)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Dispatch() error = %v, want %v", err, testCase.want)
			}
			if !reflect.DeepEqual(result, workers.WorkstationDispatchResult{}) {
				t.Fatalf("Dispatch() result = %#v, want empty", result)
			}
		})
	}
	if len(executor.requests) != 0 {
		t.Fatalf("executor calls = %d, want zero", len(executor.requests))
	}
}

func TestPoolDispatchReturnsAttributedResultWithExecutorFailure(t *testing.T) {
	t.Parallel()

	executeErr := errors.New("executor failed")
	executor := &recordingExecutor{err: executeErr}
	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{{
		WorkstationName: "review",
		Executor:        executor,
	}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-failed", "transition-failed", "review"),
	)
	if !errors.Is(err, executeErr) {
		t.Fatalf("Dispatch() error = %v, want executor failure", err)
	}
	if result.DispatchID != "dispatch-failed" ||
		result.WorkstationName != "review" ||
		result.Result.DispatchID != "dispatch-failed" ||
		result.Result.TransitionID != "transition-failed" {
		t.Fatalf("Dispatch() result = %#v", result)
	}
	if len(executor.requests) != 1 {
		t.Fatalf("executor calls = %d, want one", len(executor.requests))
	}
}

func TestPoolStopCancelsAcceptedDispatchAndRejectsLaterDispatch(t *testing.T) {
	t.Parallel()

	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{{
		WorkstationName: "review",
		Executor:        executor,
	}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	dispatchDone := make(chan error, 1)
	go func() {
		_, err := pool.Dispatch(
			context.Background(),
			dispatchRequest("dispatch-running", "transition-running", "review"),
		)
		dispatchDone <- err
	}()
	<-executor.started

	stopDone := make(chan struct {
		outcome workers.WorkstationPoolLifecycleOutcome
		err     error
	}, 1)
	go func() {
		outcome, err := pool.stop(context.Background())
		stopDone <- struct {
			outcome workers.WorkstationPoolLifecycleOutcome
			err     error
		}{outcome: outcome, err: err}
	}()
	if err := <-dispatchDone; !errors.Is(err, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("Dispatch() error = %v, want ErrWorkstationDispatchCanceled", err)
	}
	stopped := <-stopDone
	if stopped.err != nil ||
		stopped.outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() = %q, %v", stopped.outcome, stopped.err)
	}
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-late", "transition-late", "review"),
	); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Dispatch() after stop error = %v, want ErrWorkstationPoolStopped", err)
	}
}

func TestPoolEnforcesCapacityBoundedQueueAndFIFO(t *testing.T) {
	t.Parallel()

	executor := newControlledExecutor("dispatch-1", "dispatch-2", "dispatch-3")
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   2,
	})

	results := make(chan error, 3)
	dispatchAsync(pool, "dispatch-1", "review", results)
	assertStarted(t, executor, "dispatch-1")
	dispatchAsync(pool, "dispatch-2", "review", results)
	waitForQueued(t, pool, "review", 1)
	dispatchAsync(pool, "dispatch-3", "review", results)
	waitForQueued(t, pool, "review", 2)

	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-saturated", "transition", "review"),
	); !errors.Is(err, workers.ErrWorkstationSaturated) {
		t.Fatalf("saturated Dispatch() error = %v, want ErrWorkstationSaturated", err)
	}
	executor.release("dispatch-1")
	assertStarted(t, executor, "dispatch-2")
	executor.release("dispatch-2")
	assertStarted(t, executor, "dispatch-3")
	executor.release("dispatch-3")
	assertDispatchErrors(t, results, 3)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.max != 1 {
		t.Fatalf("maximum concurrent executor calls = %d, want 1", executor.max)
	}
}

func TestPoolCapacityGreaterThanOneAndIndependentRoutesProgress(t *testing.T) {
	t.Parallel()

	executor := newControlledExecutor("a-1", "a-2", "a-3", "b-1")
	pool := New()
	if _, err := pool.start(context.Background(), []workstations.Route{
		{
			WorkstationName: "route-a",
			Executor:        executor,
			Capacity:        2,
			QueueCapacity:   1,
		},
		{
			WorkstationName: "route-b",
			Executor:        executor,
			Capacity:        1,
			QueueCapacity:   1,
		},
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	results := make(chan error, 4)
	dispatchAsync(pool, "a-1", "route-a", results)
	assertStarted(t, executor, "a-1")
	dispatchAsync(pool, "a-2", "route-a", results)
	assertStarted(t, executor, "a-2")
	dispatchAsync(pool, "a-3", "route-a", results)
	waitForQueued(t, pool, "route-a", 1)
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("a-saturated", "transition", "route-a"),
	); !errors.Is(err, workers.ErrWorkstationSaturated) {
		t.Fatalf("route-a saturation error = %v, want ErrWorkstationSaturated", err)
	}

	dispatchAsync(pool, "b-1", "route-b", results)
	assertStarted(t, executor, "b-1")
	executor.release("b-1")
	executor.release("a-1")
	assertStarted(t, executor, "a-3")
	executor.release("a-2")
	executor.release("a-3")
	assertDispatchErrors(t, results, 4)

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.max != 3 {
		t.Fatalf("cross-route maximum concurrent executor calls = %d, want 3", executor.max)
	}
}

func TestPoolNormalizesExecutorPanicAndReleasesCapacity(t *testing.T) {
	t.Parallel()

	executor := &panicOnceExecutor{}
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})

	result, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-panic", "transition-panic", "review"),
	)
	if err == nil || !strings.Contains(err.Error(), "workstation executor panic: executor panic") {
		t.Fatalf("Dispatch() panic error = %v", err)
	}
	if result.DispatchID != "dispatch-panic" ||
		result.WorkstationName != "review" ||
		result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		result.Result.DispatchID != "dispatch-panic" ||
		result.Result.TransitionID != "transition-panic" ||
		result.Result.Outcome != workers.OutcomeFailed ||
		result.Result.Error != "workstation executor panic: executor panic" {
		t.Fatalf("Dispatch() panic result = %#v", result)
	}
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-after-panic", "transition-after-panic", "review"),
	); err != nil {
		t.Fatalf("Dispatch() after panic error = %v", err)
	}
}

func TestPoolCancelledWaiterReleasesBoundedQueueAccounting(t *testing.T) {
	t.Parallel()

	executor := newControlledExecutor("dispatch-running", "dispatch-next")
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})

	results := make(chan error, 2)
	dispatchAsync(pool, "dispatch-running", "review", results)
	assertStarted(t, executor, "dispatch-running")
	waitingContext, cancelWaiting := context.WithCancel(context.Background())
	waitingResult := make(chan error, 1)
	go func() {
		_, err := pool.Dispatch(
			waitingContext,
			dispatchRequest("dispatch-cancelled", "transition-cancelled", "review"),
		)
		waitingResult <- err
	}()
	waitForQueued(t, pool, "review", 1)
	cancelWaiting()
	if err := <-waitingResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled queued Dispatch() error = %v, want context.Canceled", err)
	}

	dispatchAsync(pool, "dispatch-next", "review", results)
	waitForQueued(t, pool, "review", 1)
	executor.release("dispatch-running")
	assertStarted(t, executor, "dispatch-next")
	executor.release("dispatch-next")
	assertDispatchErrors(t, results, 2)
}

func TestPoolExplicitCancellationPreventsQueuedExecutorEntry(t *testing.T) {
	t.Parallel()

	executor := newControlledExecutor("dispatch-running", "dispatch-queued")
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})

	running := dispatchResultAsync(pool, context.Background(), "dispatch-running", "review")
	assertStarted(t, executor, "dispatch-running")
	queued := dispatchResultAsync(pool, context.Background(), "dispatch-queued", "review")
	waitForQueued(t, pool, "review", 1)

	cancelled, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-queued"},
	)
	if err != nil || cancelled.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("Cancel() = %#v, %v", cancelled, err)
	}
	repeated, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-queued"},
	)
	if err != nil || repeated.Outcome != workers.WorkstationDispatchCancelOutcomeAlreadyCanceled {
		t.Fatalf("repeated Cancel() = %#v, %v", repeated, err)
	}
	assertCanceledDispatch(t, <-queued)

	executor.release("dispatch-running")
	if completed := <-running; completed.err != nil {
		t.Fatalf("running Dispatch() error = %v", completed.err)
	}
	select {
	case unexpected := <-executor.started:
		t.Fatalf("cancelled queued dispatch entered executor as %q", unexpected)
	default:
	}
}

func TestPoolExplicitAndCallerCancellationConvergeForRunningDispatches(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		cancel func(*Pool, context.CancelFunc) error
	}{
		{
			name: "explicit",
			cancel: func(pool *Pool, _ context.CancelFunc) error {
				result, err := pool.Cancel(
					context.Background(),
					workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-explicit"},
				)
				if err == nil && result.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
					return fmt.Errorf("Cancel() outcome = %q", result.Outcome)
				}
				return err
			},
		},
		{
			name: "caller context",
			cancel: func(_ *Pool, cancel context.CancelFunc) error {
				cancel()
				return nil
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			dispatchID := "dispatch-" + strings.ReplaceAll(test.name, " ", "-")
			executor := newControlledExecutor(dispatchID)
			pool := New()
			startPool(t, pool, workstations.Route{
				WorkstationName: "review",
				Executor:        executor,
				Capacity:        1,
				QueueCapacity:   1,
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			completed := dispatchResultAsync(pool, ctx, dispatchID, "review")
			assertStarted(t, executor, dispatchID)
			if err := test.cancel(pool, cancel); err != nil {
				t.Fatalf("cancel dispatch: %v", err)
			}
			assertCanceledDispatch(t, <-completed)
			executor.mu.Lock()
			running := executor.running
			executor.mu.Unlock()
			if running != 0 {
				t.Fatalf("executor activity after cancellation = %d", running)
			}
		})
	}
}

func TestPoolLateCancellationDoesNotReplaceCommittedResult(t *testing.T) {
	t.Parallel()

	executor := &recordingExecutor{
		result: workers.WorkResult{Outcome: workers.OutcomeAccepted},
	}
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
	})
	result, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-complete", "transition-complete", "review"),
	)
	if err != nil || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("Dispatch() = %#v, %v", result, err)
	}
	cancelled, err := pool.Cancel(
		context.Background(),
		workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-complete"},
	)
	if !errors.Is(err, workers.ErrWorkstationDispatchAlreadyTerminal) ||
		cancelled.Outcome != workers.WorkstationDispatchCancelOutcomeAlreadyTerminal {
		t.Fatalf("late Cancel() = %#v, %v", cancelled, err)
	}
}

func TestPoolStopCancelsQueuedAndRunningDispatchesBeforeReturning(t *testing.T) {
	t.Parallel()

	executor := newControlledExecutor("dispatch-running", "dispatch-queued")
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})
	running := dispatchResultAsync(pool, context.Background(), "dispatch-running", "review")
	assertStarted(t, executor, "dispatch-running")
	queued := dispatchResultAsync(pool, context.Background(), "dispatch-queued", "review")
	waitForQueued(t, pool, "review", 1)

	outcome, err := pool.stop(context.Background())
	if err != nil || outcome != workers.WorkstationPoolLifecycleOutcomeStopped {
		t.Fatalf("Stop() = %q, %v", outcome, err)
	}
	assertCanceledDispatch(t, <-running)
	assertCanceledDispatch(t, <-queued)
	executor.mu.Lock()
	active := executor.running
	executor.mu.Unlock()
	if active != 0 {
		t.Fatalf("executor activity after Stop() = %d", active)
	}
	if _, err := pool.Dispatch(
		context.Background(),
		dispatchRequest("dispatch-late", "transition-late", "review"),
	); !errors.Is(err, workers.ErrWorkstationPoolStopped) {
		t.Fatalf("Dispatch() after Stop() error = %v", err)
	}
}

func startPool(t *testing.T, pool *Pool, route workstations.Route) {
	t.Helper()
	if _, err := pool.start(context.Background(), []workstations.Route{route}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func dispatchAsync(pool *Pool, dispatchID string, route string, results chan<- error) {
	go func() {
		_, err := pool.Dispatch(
			context.Background(),
			dispatchRequest(dispatchID, "transition-"+dispatchID, route),
		)
		results <- err
	}()
}

func assertStarted(t *testing.T, executor *controlledExecutor, dispatchID string) {
	t.Helper()
	select {
	case got := <-executor.started:
		if got != dispatchID {
			t.Fatalf("executor start = %q, want %q", got, dispatchID)
		}
	case <-time.After(time.Second):
		t.Fatalf("executor did not start %q", dispatchID)
	}
}

func waitForQueued(t *testing.T, pool *Pool, routeName string, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		route := pool.routes[routeName]
		route.mu.Lock()
		queued := len(route.waiting)
		route.mu.Unlock()
		if queued == count {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("route %q did not reach queued count %d", routeName, count)
}

func assertDispatchErrors(t *testing.T, results <-chan error, count int) {
	t.Helper()
	for range count {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("Dispatch() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Dispatch() did not complete")
		}
	}
}

func dispatchRequest(
	dispatchID string,
	transitionID string,
	workstationName string,
) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: workstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch: work.WorkDispatch{
				DispatchID:               dispatchID,
				TransitionID:             transitionID,
				WorkerType:               "selected-worker",
				WorkstationName:          workstationName,
				ProjectID:                "project-1",
				CurrentChainingTraceID:   "trace-current",
				PreviousChainingTraceIDs: []string{"trace-previous"},
				Execution: work.ExecutionMetadata{
					RequestID: "request-1",
					TraceID:   "trace-1",
					WorkIDs:   []string{"work-1"},
				},
				InputTokens: []any{"dispatch-token"},
				InputBindings: map[string][]string{
					"source": {"work-1"},
				},
			},
			WorkerType:         "selected-worker",
			WorkstationType:    "INFERENCE_RUN",
			ProjectID:          "project-1",
			FactorySessionID:   "session-1",
			InputTokens:        []any{"input-token"},
			ModelOperation:     "summarize",
			SystemPrompt:       "system",
			UserMessage:        "user",
			OutputSchema:       `{"type":"string"}`,
			EnvVars:            map[string]string{"KEY": "value"},
			ProcessEnvironment: []string{"PATH=value"},
			Worktree:           "worktree",
			WorkingDirectory:   "working-directory",
		},
	}
}
