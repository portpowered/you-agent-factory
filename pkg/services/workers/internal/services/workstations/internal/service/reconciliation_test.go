package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

func TestPoolProcessGoneReleasesRouteAndCommitsFailedDispatch(t *testing.T) {
	pool := New()
	executor := &processGoneSequenceExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})

	first := dispatchResultAsync(pool, context.Background(), "gone", "review")
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("orphaned executor did not start")
	}

	record := dispatchRecordForTest(t, pool, "gone")
	observer := dispatchProcessObserver{record: record}
	observer.ProcessStarted(platformprocess.ProcessInfo{PID: 41})
	observer.ProcessExited(platformprocess.ProcessInfo{PID: 41})
	observer.ProcessExited(platformprocess.ProcessInfo{PID: 41})

	completed := receiveDispatch(t, first)
	if !errors.Is(completed.err, workers.ErrWorkstationDispatchProcessGone) {
		t.Fatalf("process-gone Dispatch() error = %v, want ErrWorkstationDispatchProcessGone", completed.err)
	}
	if completed.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		completed.result.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonProcessGone {
		t.Fatalf("process-gone Dispatch() result = %#v, want one FAILED PROCESS_GONE terminal", completed.result)
	}
	if completed.result.Result.FailureMetadata == nil ||
		completed.result.Result.FailureMetadata.Family != workers.WorkFailureFamilyRetryable {
		t.Fatalf("process-gone failure metadata = %#v, want retryable", completed.result.Result.FailureMetadata)
	}
	if completed.result.Result.Diagnostics == nil ||
		completed.result.Result.Diagnostics.Metadata[workers.ProviderResponseMetadataFailureClassification] != "process_gone" {
		t.Fatalf("process-gone diagnostics = %#v, want bounded process_gone classification", completed.result.Result.Diagnostics)
	}

	second := dispatchResultAsync(pool, context.Background(), "after-gone", "review")
	secondResult := receiveDispatch(t, second)
	if secondResult.err != nil || secondResult.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("dispatch after process gone = %#v, %v; route capacity was not released", secondResult.result, secondResult.err)
	}
	if _, err := pool.stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestPoolTimeoutReconciliationReleasesRouteAndCommitsRetryableFailure(t *testing.T) {
	pool := New()
	executor := &processGoneSequenceExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   1,
	})

	first := dispatchResultAsync(pool, context.Background(), "timeout", "review")
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("overdue executor did not start")
	}

	firstCancel, err := pool.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{
		DispatchID: "timeout",
		Reason:     workers.WorkstationDispatchCancelReasonTimeout,
	})
	assertTimeoutCancel(t, firstCancel, err, workers.WorkstationDispatchCancelOutcomeReconciled)
	repeatedCancel, repeatedErr := pool.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{
		DispatchID: "timeout",
		Reason:     workers.WorkstationDispatchCancelReasonTimeout,
	})
	assertTimeoutCancel(t, repeatedCancel, repeatedErr, workers.WorkstationDispatchCancelOutcomeAlreadyReconciled)

	completed := receiveDispatch(t, first)
	assertTimeoutDispatch(t, completed)

	second := dispatchResultAsync(pool, context.Background(), "after-timeout", "review")
	secondResult := receiveDispatch(t, second)
	assertCompletedDispatch(t, secondResult, "dispatch after timeout")
	if _, err := pool.stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func assertTimeoutCancel(t *testing.T, result workers.WorkstationDispatchCancelResult, err error, want workers.WorkstationDispatchCancelOutcome) {
	t.Helper()
	if err != nil || result.Outcome != want {
		t.Fatalf("timeout reconciliation = %#v, %v; want %s", result, err, want)
	}
}

func assertTimeoutDispatch(t *testing.T, completed dispatchCompletion) {
	t.Helper()
	if !errors.Is(completed.err, workers.ErrWorkstationDispatchTimeout) {
		t.Fatalf("timeout Dispatch() error = %v, want ErrWorkstationDispatchTimeout", completed.err)
	}
	if completed.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
		completed.result.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonTimeout {
		t.Fatalf("timeout Dispatch() result = %#v, want one FAILED EXECUTION_TIMEOUT terminal", completed.result)
	}
	metadata := completed.result.Result.FailureMetadata
	if metadata == nil || metadata.Family != workers.WorkFailureFamilyRetryable || metadata.Type != workers.WorkFailureTypeTimeout {
		t.Fatalf("timeout failure metadata = %#v, want retryable timeout", metadata)
	}
	diagnostics := completed.result.Result.Diagnostics
	if diagnostics == nil || diagnostics.Metadata[workers.ProviderResponseMetadataFailureClassification] != "execution_timeout" {
		t.Fatalf("timeout diagnostics = %#v, want bounded execution_timeout classification", diagnostics)
	}
}

func assertCompletedDispatch(t *testing.T, completed dispatchCompletion, label string) {
	t.Helper()
	if completed.err != nil || completed.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("%s = %#v, %v; want completed", label, completed.result, completed.err)
	}
}

func TestPoolQueuedTimeoutReconciliationRemovesReleasedQueueEntry(t *testing.T) {
	executor := &processGoneSequenceExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
		Capacity:        1,
		QueueCapacity:   2,
	})

	first := dispatchResultAsync(pool, context.Background(), "queued-first", "review")
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("first executor did not start")
	}
	second := dispatchResultAsync(pool, context.Background(), "queued-timeout", "review")
	waitForQueued(t, pool, "review", 1)

	result, err := pool.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{
		DispatchID: "queued-timeout",
		Reason:     workers.WorkstationDispatchCancelReasonTimeout,
	})
	if err != nil || result.Outcome != workers.WorkstationDispatchCancelOutcomeReconciled {
		t.Fatalf("queued timeout reconciliation = %#v, %v; want RECONCILED", result, err)
	}
	queuedResult := receiveDispatch(t, second)
	if !errors.Is(queuedResult.err, workers.ErrWorkstationDispatchTimeout) {
		t.Fatalf("queued timeout error = %v, want ErrWorkstationDispatchTimeout", queuedResult.err)
	}
	waitForQueued(t, pool, "review", 0)

	close(executor.release)
	firstResult := receiveDispatch(t, first)
	if firstResult.err != nil {
		t.Fatalf("first dispatch error = %v, want nil", firstResult.err)
	}
	third := dispatchResultAsync(pool, context.Background(), "queued-after-timeout", "review")
	thirdResult := receiveDispatch(t, third)
	if thirdResult.err != nil || thirdResult.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("dispatch after queued timeout = %#v, %v; want completed", thirdResult.result, thirdResult.err)
	}
	if _, err := pool.stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

type processGoneSequenceExecutor struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (executor *processGoneSequenceExecutor) Execute(
	ctx context.Context,
	_ workers.WorkstationExecutionRequest,
) (workers.WorkResult, error) {
	first := false
	executor.once.Do(func() {
		first = true
		close(executor.started)
	})
	if first {
		select {
		case <-executor.release:
		case <-ctx.Done():
			return workers.WorkResult{}, ctx.Err()
		}
	}
	return workers.WorkResult{Outcome: workers.OutcomeAccepted}, nil
}

func TestPoolProcessGoneAndCompletionRaceCommitsExactlyOneTerminal(t *testing.T) {
	const schedules = 50
	for index := range schedules {
		dispatchID := "process-race-" + string(rune('a'+index%26))
		executor := newControlledExecutor(dispatchID)
		pool := New()
		startPool(t, pool, workstations.Route{
			WorkstationName: "review",
			Executor:        executor,
			Capacity:        1,
			QueueCapacity:   1,
		})
		completed := dispatchResultAsync(pool, context.Background(), dispatchID, "review")
		assertStarted(t, executor, dispatchID)
		record := dispatchRecordForTest(t, pool, dispatchID)
		observer := dispatchProcessObserver{record: record}
		observer.ProcessStarted(platformprocess.ProcessInfo{PID: index + 1})

		start := make(chan struct{})
		var race sync.WaitGroup
		race.Add(2)
		go func() {
			defer race.Done()
			<-start
			observer.ProcessExited(platformprocess.ProcessInfo{PID: index + 1})
		}()
		go func() {
			defer race.Done()
			<-start
			executor.release(dispatchID)
		}()
		close(start)
		race.Wait()

		result := receiveDispatch(t, completed)
		if result.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted &&
			result.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed {
			t.Fatalf("race terminal outcome = %q, want COMPLETED or FAILED", result.result.TerminalOutcome)
		}
		if result.result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed &&
			result.result.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonProcessGone {
			t.Fatalf("race failed result = %#v, want PROCESS_GONE reason", result.result)
		}
		if _, err := pool.stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}
}

func TestPoolTimeoutAndCompletionRaceCommitsExactlyOneTerminal(t *testing.T) {
	const schedules = 50
	for index := range schedules {
		dispatchID := "timeout-race-" + string(rune('a'+index%26))
		executor := newControlledExecutor(dispatchID)
		pool := New()
		startPool(t, pool, workstations.Route{
			WorkstationName: "review",
			Executor:        executor,
			Capacity:        1,
			QueueCapacity:   1,
		})
		completed := dispatchResultAsync(pool, context.Background(), dispatchID, "review")
		assertStarted(t, executor, dispatchID)

		start := make(chan struct{})
		var race sync.WaitGroup
		race.Add(2)
		go func() {
			defer race.Done()
			<-start
			_, _ = pool.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{
				DispatchID: dispatchID,
				Reason:     workers.WorkstationDispatchCancelReasonTimeout,
			})
		}()
		go func() {
			defer race.Done()
			<-start
			executor.release(dispatchID)
		}()
		close(start)
		race.Wait()

		result := receiveDispatch(t, completed)
		switch result.result.ReconciliationReason {
		case "":
			if result.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted || result.err != nil {
				t.Fatalf("normal timeout race result = %#v, %v; want completed", result.result, result.err)
			}
		case workers.WorkstationDispatchReconciliationReasonTimeout:
			if result.result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeFailed ||
				!errors.Is(result.err, workers.ErrWorkstationDispatchTimeout) {
				t.Fatalf("timeout race result = %#v, %v; want timeout failure", result.result, result.err)
			}
		default:
			t.Fatalf("timeout race reconciliation reason = %q, want empty or EXECUTION_TIMEOUT", result.result.ReconciliationReason)
		}
		if _, err := pool.stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}
}

func dispatchRecordForTest(t *testing.T, pool *Pool, dispatchID string) *dispatchRecord {
	t.Helper()
	pool.mu.RLock()
	record := pool.dispatches[dispatchID]
	pool.mu.RUnlock()
	if record != nil {
		return record
	}
	t.Fatalf("dispatch %q was not accepted", dispatchID)
	return nil
}

func receiveDispatch(t *testing.T, completed <-chan dispatchCompletion) dispatchCompletion {
	t.Helper()
	select {
	case result := <-completed:
		return result
	case <-time.After(time.Second):
		t.Fatal("Dispatch() did not publish a terminal result")
		return dispatchCompletion{}
	}
}
