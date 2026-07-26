package service

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

func TestPoolCancellationAndCompletionRaceCommitsOneTerminalOutcome(t *testing.T) {
	t.Parallel()

	const schedules = 50
	for index := range schedules {
		dispatchID := fmt.Sprintf("dispatch-race-%d", index)
		executor := newControlledExecutor(dispatchID)
		pool := New()
		startPool(t, pool, workstations.Route{
			WorkstationName: "review",
			Executor:        executor,
			Capacity:        1,
			QueueCapacity:   1,
		})
		completed := dispatchResultAsync(
			pool,
			context.Background(),
			dispatchID,
			"review",
		)
		assertStarted(t, executor, dispatchID)

		start := make(chan struct{})
		cancelled := make(chan struct {
			result workers.WorkstationDispatchCancelResult
			err    error
		}, 1)
		go func() {
			<-start
			result, err := pool.Cancel(
				context.Background(),
				workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID},
			)
			cancelled <- struct {
				result workers.WorkstationDispatchCancelResult
				err    error
			}{result: result, err: err}
		}()
		go func() {
			<-start
			executor.release(dispatchID)
		}()
		close(start)

		dispatch := <-completed
		cancellation := <-cancelled
		switch cancellation.result.Outcome {
		case workers.WorkstationDispatchCancelOutcomeCanceled:
			if cancellation.err != nil {
				t.Fatalf("winning Cancel() error = %v", cancellation.err)
			}
			assertCanceledDispatch(t, dispatch)
		case workers.WorkstationDispatchCancelOutcomeAlreadyTerminal:
			if !errors.Is(cancellation.err, workers.ErrWorkstationDispatchAlreadyTerminal) {
				t.Fatalf("late Cancel() error = %v", cancellation.err)
			}
			if dispatch.err != nil ||
				dispatch.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
				t.Fatalf("winning completion = %#v, %v", dispatch.result, dispatch.err)
			}
		default:
			t.Fatalf("Cancel() outcome = %q", cancellation.result.Outcome)
		}
	}
}

func TestPoolConcurrentLifecycleDispatchCancellationAndCompletion(t *testing.T) {
	t.Parallel()

	schedule := newStressSchedule(t)
	cancellations, starts, stops := schedule.overlap()
	assertStressControlResults(
		t,
		cancellations,
		starts,
		stops,
		len(schedule.dispatchIDs)/3,
	)
	assertStressTerminals(t, schedule.completed, len(schedule.dispatchIDs))
	schedule.executor.assertSafe(t, stressRouteCapacity)
	assertRoutesDrained(t, schedule.ownedRoutes)
}

const (
	stressDispatchesPerRoute = 12
	stressRouteCapacity      = 3
)

type stressSchedule struct {
	pool        *Pool
	routes      []workstations.Route
	ownedRoutes []*routePool
	executor    *stressExecutor
	dispatchIDs []string
	completed   chan dispatchCompletion
}

func newStressSchedule(t *testing.T) stressSchedule {
	t.Helper()
	executor := newStressExecutor()
	pool := New()
	routes := []workstations.Route{
		{
			WorkstationName: "review",
			Executor:        executor,
			Capacity:        stressRouteCapacity,
			QueueCapacity:   stressDispatchesPerRoute,
		},
		{
			WorkstationName: "implement",
			Executor:        executor,
			Capacity:        stressRouteCapacity,
			QueueCapacity:   stressDispatchesPerRoute,
		},
	}
	if _, err := pool.Start(context.Background(), routes); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	total := stressDispatchesPerRoute * len(routes)
	completed := make(chan dispatchCompletion, total)
	dispatchIDs := make([]string, 0, total)
	for _, route := range routes {
		for index := range stressDispatchesPerRoute {
			dispatchID := fmt.Sprintf("%s-%d", route.WorkstationName, index)
			dispatchIDs = append(dispatchIDs, dispatchID)
			go func() {
				result, err := pool.Dispatch(
					context.Background(),
					dispatchRequest(
						dispatchID,
						"transition-"+dispatchID,
						route.WorkstationName,
					),
				)
				completed <- dispatchCompletion{result: result, err: err}
			}()
		}
	}
	waitForAcceptedDispatches(t, pool, total)
	executor.waitForRouteProgress(t, len(routes))
	return stressSchedule{
		pool:        pool,
		routes:      routes,
		ownedRoutes: []*routePool{pool.routes["review"], pool.routes["implement"]},
		executor:    executor,
		dispatchIDs: dispatchIDs,
		completed:   completed,
	}
}

func (schedule stressSchedule) overlap() (<-chan error, <-chan error, <-chan error) {
	race := make(chan struct{})
	cancellations := make(chan error, len(schedule.dispatchIDs)/3)
	for index, dispatchID := range schedule.dispatchIDs {
		if index%3 != 0 {
			continue
		}
		go func() {
			<-race
			_, err := schedule.pool.Cancel(
				context.Background(),
				workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID},
			)
			cancellations <- err
		}()
	}
	starts := make(chan error, 2)
	for range 2 {
		go func() {
			<-race
			_, err := schedule.pool.Start(context.Background(), schedule.routes)
			starts <- err
		}()
	}
	stops := make(chan error, 3)
	for range 3 {
		go func() {
			<-race
			_, err := schedule.pool.Stop(context.Background())
			stops <- err
		}()
	}
	go func() {
		<-race
		close(schedule.executor.release)
	}()
	close(race)
	return cancellations, starts, stops
}

func assertStressControlResults(
	t *testing.T,
	cancellations <-chan error,
	starts <-chan error,
	stops <-chan error,
	cancellationCount int,
) {
	t.Helper()
	for range cancellationCount {
		if err := <-cancellations; err != nil &&
			!errors.Is(err, workers.ErrWorkstationDispatchAlreadyTerminal) {
			t.Fatalf("Cancel() error = %v", err)
		}
	}
	for range 2 {
		err := <-starts
		if err != nil && !errors.Is(err, workers.ErrWorkstationPoolStopped) {
			t.Fatalf("overlapping Start() error = %v", err)
		}
	}
	for range 3 {
		if err := <-stops; err != nil {
			t.Fatalf("overlapping Stop() error = %v", err)
		}
	}
}

func assertStressTerminals(t *testing.T, completed <-chan dispatchCompletion, total int) {
	t.Helper()
	terminals := make(map[string]workers.WorkstationDispatchTerminalOutcome, total)
	for range total {
		finished := <-completed
		if finished.result.DispatchID == "" {
			t.Fatalf(
				"Dispatch() returned unattributed terminal result: %#v, %v",
				finished.result,
				finished.err,
			)
		}
		if _, duplicate := terminals[finished.result.DispatchID]; duplicate {
			t.Fatalf("duplicate terminal result for %q", finished.result.DispatchID)
		}
		terminals[finished.result.DispatchID] = finished.result.TerminalOutcome
		switch finished.result.TerminalOutcome {
		case workers.WorkstationDispatchTerminalOutcomeCompleted:
			if finished.err != nil {
				t.Fatalf("completed Dispatch() error = %v", finished.err)
			}
		case workers.WorkstationDispatchTerminalOutcomeCanceled:
			if !errors.Is(finished.err, workers.ErrWorkstationDispatchCanceled) {
				t.Fatalf("cancelled Dispatch() error = %v", finished.err)
			}
		default:
			t.Fatalf("Dispatch() terminal outcome = %q", finished.result.TerminalOutcome)
		}
	}
	if len(terminals) != total {
		t.Fatalf("terminal results = %d, want %d", len(terminals), total)
	}
}

type dispatchCompletion struct {
	result workers.WorkstationDispatchResult
	err    error
}

type stressExecutor struct {
	mu      sync.Mutex
	active  map[string]int
	max     map[string]int
	entries map[string]int
	release chan struct{}
}

func newStressExecutor() *stressExecutor {
	return &stressExecutor{
		active:  make(map[string]int),
		max:     make(map[string]int),
		entries: make(map[string]int),
		release: make(chan struct{}),
	}
}

func (executor *stressExecutor) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	route := request.Dispatch.WorkstationName
	dispatchID := request.Dispatch.DispatchID
	executor.mu.Lock()
	executor.active[route]++
	executor.entries[dispatchID]++
	if executor.active[route] > executor.max[route] {
		executor.max[route] = executor.active[route]
	}
	executor.mu.Unlock()
	defer func() {
		executor.mu.Lock()
		executor.active[route]--
		executor.mu.Unlock()
	}()

	select {
	case <-executor.release:
		return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
	case <-ctx.Done():
		return workers.WorkResult{}, ctx.Err()
	}
}

func (executor *stressExecutor) waitForRouteProgress(t *testing.T, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		executor.mu.Lock()
		routes := len(executor.active)
		executor.mu.Unlock()
		if routes == count {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("executor routes with progress did not reach %d", count)
}

func (executor *stressExecutor) assertSafe(t *testing.T, capacity int) {
	t.Helper()
	executor.mu.Lock()
	defer executor.mu.Unlock()
	for route, maximum := range executor.max {
		if maximum > capacity {
			t.Fatalf("route %q maximum concurrency = %d, want <= %d", route, maximum, capacity)
		}
	}
	for dispatchID, entries := range executor.entries {
		if entries != 1 {
			t.Fatalf("dispatch %q executor entries = %d, want one", dispatchID, entries)
		}
	}
	for route, active := range executor.active {
		if active != 0 {
			t.Fatalf("route %q active executors = %d, want zero", route, active)
		}
	}
}

func waitForAcceptedDispatches(t *testing.T, pool *Pool, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		pool.mu.RLock()
		accepted := len(pool.dispatches)
		pool.mu.RUnlock()
		if accepted == count {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("accepted dispatches did not reach %d", count)
}

func assertRoutesDrained(t *testing.T, routes []*routePool) {
	t.Helper()
	for _, route := range routes {
		route.mu.Lock()
		running, waiting := route.running, len(route.waiting)
		route.mu.Unlock()
		if running != 0 || waiting != 0 {
			t.Fatalf(
				"route accounting = running %d, waiting %d; want drained",
				running,
				waiting,
			)
		}
	}
}

func dispatchResultAsync(
	pool *Pool,
	ctx context.Context,
	dispatchID string,
	route string,
) <-chan dispatchCompletion {
	completed := make(chan dispatchCompletion, 1)
	go func() {
		result, err := pool.Dispatch(
			ctx,
			dispatchRequest(dispatchID, "transition-"+dispatchID, route),
		)
		completed <- dispatchCompletion{result: result, err: err}
	}()
	return completed
}

func assertCanceledDispatch(t *testing.T, completed dispatchCompletion) {
	t.Helper()
	if !errors.Is(completed.err, workers.ErrWorkstationDispatchCanceled) ||
		!errors.Is(completed.err, context.Canceled) {
		t.Fatalf("cancelled Dispatch() error = %v", completed.err)
	}
	if completed.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled ||
		completed.result.Result.Outcome != workers.OutcomeFailed ||
		completed.result.Result.Error != workers.ErrWorkstationDispatchCanceled.Error() {
		t.Fatalf("cancelled Dispatch() result = %#v", completed.result)
	}
}
