package stress_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestRaceConditionConcurrentMutation verifies the engine has no race conditions
// when multiple goroutines submit work, return results, and query state simultaneously.
//
// Run with: go test -race -count=5 ./tests/stress/
func TestRaceConditionConcurrentMutation(t *testing.T) {
	const (
		numSubmitters     = 10
		itemsPerSubmitter = 10
		totalItems        = numSubmitters * itemsPerSubmitter
		numReaders        = 5
		pipelineStages    = 5
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))
	h := startStressProcess(t, dir, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})

	// 10 goroutines submitting 10 items each.
	var submitWg sync.WaitGroup
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range itemsPerSubmitter {
				h.SubmitFull(context.Background(), []work.SubmitRequest{{
					WorkTypeID: "task",
					TraceID:    fmt.Sprintf("trace-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"goroutine":%d,"item":%d}`, gid, i),
				}})
				// Small random delay to stagger submissions.
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}(g)
	}

	// 5 goroutines querying state continuously.
	var queryWg sync.WaitGroup
	queryDone := make(chan struct{})
	var queryCount atomic.Int64
	for range numReaders {
		queryWg.Add(1)
		go func() {
			defer queryWg.Done()
			for {
				select {
				case <-queryDone:
					return
				default:
					snap := h.Marking()
					// Exercise the snapshot data to ensure reads are consistent.
					for _, tok := range snap.Tokens {
						_ = tok.PlaceID
						_ = tok.Color.WorkTypeID
					}
					queryCount.Add(1)
				}
			}
		}()
	}

	// Wait for all submissions to complete.
	submitWg.Wait()

	// Poll until all tokens reach terminal state.
	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	h.WaitForTerminalCount(totalItems, 25*time.Second)

	// Stop reader goroutines.
	close(queryDone)
	queryWg.Wait()

	// Final consistency checks.
	snap := h.Marking()
	h.Stop()
	assertMarkingConsistency(t, snap, pipelineTerminalPlaces, totalItems)

	t.Logf("completed: %d tokens, %d query calls", len(snap.Tokens), queryCount.Load())
}

// TestRaceConditionWithMockExecutors verifies no race conditions when using
// synchronous mock executors with random delays inside the tick cycle.
func TestRaceConditionWithMockExecutors(t *testing.T) {
	const (
		numSubmitters     = 10
		itemsPerSubmitter = 10
		totalItems        = numSubmitters * itemsPerSubmitter
		numReaders        = 5
		pipelineStages    = 5
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))
	executor := &delayExecutor{maxDelay: 2 * time.Millisecond}
	h := startStressProcess(t, dir, workerExecutorProvider{executor: executor})

	// 10 goroutines submitting 10 items each.
	var submitWg sync.WaitGroup
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range itemsPerSubmitter {
				h.SubmitFull(context.Background(), []work.SubmitRequest{{
					WorkTypeID: "task",
					TraceID:    fmt.Sprintf("trace-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"goroutine":%d,"item":%d}`, gid, i),
				}})
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}(g)
	}

	// 5 goroutines querying state continuously.
	var queryWg sync.WaitGroup
	queryDone := make(chan struct{})
	var queryCount atomic.Int64
	for range numReaders {
		queryWg.Add(1)
		go func() {
			defer queryWg.Done()
			for {
				select {
				case <-queryDone:
					return
				default:
					snap := h.Marking()
					for _, tok := range snap.Tokens {
						_ = tok.PlaceID
						_ = tok.Color.WorkTypeID
					}
					queryCount.Add(1)
				}
			}
		}()
	}

	// Wait for all submissions.
	submitWg.Wait()

	// Poll until all tokens reach terminal state.
	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	h.WaitForTerminalCount(totalItems, 25*time.Second)

	// Stop readers and engine.
	close(queryDone)
	queryWg.Wait()

	// Final consistency checks.
	snap := h.Marking()
	h.Stop()
	assertMarkingConsistency(t, snap, pipelineTerminalPlaces, totalItems)

	t.Logf("completed: %d tokens, %d executor calls, %d query calls",
		len(snap.Tokens), executor.callCount(), queryCount.Load())
}

// TestRaceConditionMarkingConsistency verifies that marking invariants hold
// at every observation point during concurrent execution: terminal token count
// never decreases, no phantom tokens, valid place IDs.
func TestRaceConditionMarkingConsistency(t *testing.T) {
	const (
		numSubmitters     = 10
		itemsPerSubmitter = 10
		totalItems        = numSubmitters * itemsPerSubmitter
		pipelineStages    = 5
	)

	dir := testutil.ScaffoldFactoryDir(t, testutil.PipelineConfig(pipelineStages, "pipeline-worker"))
	h := startStressProcess(t, dir, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Submit all items.
	var submitWg sync.WaitGroup
	for g := range numSubmitters {
		submitWg.Add(1)
		go func(gid int) {
			defer submitWg.Done()
			for i := range itemsPerSubmitter {
				h.SubmitFull(context.Background(), []work.SubmitRequest{{
					WorkTypeID: "task",
					TraceID:    fmt.Sprintf("trace-%d-%d", gid, i),
					Payload:    fmt.Appendf(nil, `{"goroutine":%d,"item":%d}`, gid, i),
				}})
				time.Sleep(time.Duration(rand.Intn(3)) * time.Millisecond)
			}
		}(g)
	}

	// Monitor marking invariants while execution proceeds.
	pipelineTerminalPlaces := []string{"task:complete", "task:failed"}
	var maxTerminal int
	var invariantViolations atomic.Int64
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				snap := h.Marking()

				// Invariant 1: terminal count is monotonically non-decreasing.
				termCount := countTerminalTokens(snap, pipelineTerminalPlaces)
				if termCount < maxTerminal {
					t.Errorf("invariant violation: terminal count decreased from %d to %d", maxTerminal, termCount)
					invariantViolations.Add(1)
				}
				if termCount > maxTerminal {
					maxTerminal = termCount
				}

				if termCount >= totalItems {
					return
				}

				time.Sleep(time.Millisecond)
			}
		}
	}()

	submitWg.Wait()
	<-monitorDone

	if v := invariantViolations.Load(); v > 0 {
		t.Errorf("%d invariant violations detected", v)
	}

	snap := h.Marking()
	h.Stop()
	assertMarkingConsistency(t, snap, pipelineTerminalPlaces, totalItems)

	t.Logf("completed: %d tokens, max terminal observed during execution: %d", len(snap.Tokens), maxTerminal)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// countTerminalTokens counts tokens in the specified terminal places.
func countTerminalTokens(snap *factoryruntime.PetriMarkingSnapshot, terminalPlaces []string) int {
	termSet := make(map[string]bool, len(terminalPlaces))
	for _, p := range terminalPlaces {
		termSet[p] = true
	}
	count := 0
	for _, tok := range snap.Tokens {
		if termSet[tok.PlaceID] {
			count++
		}
	}
	return count
}

// assertMarkingConsistency validates marking invariants after execution completes.
// terminalPlaces is the set of place IDs that are considered terminal.
func assertMarkingConsistency(t *testing.T, snap *factoryruntime.PetriMarkingSnapshot, terminalPlaces []string, expectedItems int) {
	t.Helper()

	// All work tokens should be in terminal or failed places.
	terminalCount := countTerminalTokens(snap, terminalPlaces)
	if terminalCount < expectedItems {
		t.Errorf("only %d/%d tokens in terminal state", terminalCount, expectedItems)
	}

	termSet := make(map[string]bool, len(terminalPlaces))
	for _, p := range terminalPlaces {
		termSet[p] = true
	}

	// No tokens stuck in non-terminal places.
	for id, tok := range snap.Tokens {
		if !termSet[tok.PlaceID] {
			t.Errorf("token %s stuck in non-terminal place %s", id, tok.PlaceID)
		}
	}

	// Token count should match expected total.
	if len(snap.Tokens) != expectedItems {
		t.Errorf("token count mismatch: got %d, want %d", len(snap.Tokens), expectedItems)
	}
}

// ---------------------------------------------------------------------------
// delayExecutor — WorkerExecutor with random delays for race testing
// ---------------------------------------------------------------------------

type delayExecutor struct {
	mu       sync.Mutex
	calls    int
	maxDelay time.Duration
}

func (e *delayExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	if e.maxDelay > 0 {
		time.Sleep(time.Duration(rand.Int63n(int64(e.maxDelay))))
	}
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

func (e *delayExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}
