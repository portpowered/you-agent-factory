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
