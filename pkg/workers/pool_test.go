package workers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// mockExecutor implements WorkerExecutor for testing.
type mockExecutor struct {
	fn func(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error)
}

func (m *mockExecutor) Execute(ctx context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return m.fn(ctx, dispatch)
}

func TestWorkerPool_DispatchAndResult(t *testing.T) {
	pool := NewWorkerPool(nil)

	executor := &mockExecutor{
		fn: func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
			return interfaces.WorkResult{
				TransitionID: d.TransitionID,
				Outcome:      interfaces.OutcomeAccepted,
			}, nil
		},
	}

	pool.Register("test-worker", executor)
	pool.Start()
	defer pool.Stop()

	dispatch := interfaces.WorkDispatch{
		TransitionID: "tr-1",
	}
	ok := pool.Dispatch("test-worker", dispatch)
	if !ok {
		t.Fatal("expected dispatch to succeed")
	}

	select {
	case result := <-pool.ResultCh():
		if result.TransitionID != "tr-1" {
			t.Errorf("expected transition ID tr-1, got %s", result.TransitionID)
		}
		if result.Outcome != interfaces.OutcomeAccepted {
			t.Errorf("expected ACCEPTED, got %s", result.Outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func TestWorkerPool_DispatchPreservesExecutionMetadataForExecutor(t *testing.T) {
	pool := NewWorkerPool(nil)
	seen := make(chan interfaces.ExecutionMetadata, 1)

	executor := &mockExecutor{
		fn: func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
			seen <- d.Execution
			return interfaces.WorkResult{
				DispatchID:   d.DispatchID,
				TransitionID: d.TransitionID,
				Outcome:      interfaces.OutcomeAccepted,
			}, nil
		},
	}

	pool.Register("test-worker", executor)
	pool.Start()
	defer pool.Stop()

	want := interfaces.ExecutionMetadata{
		DispatchCreatedTick: 10,
		CurrentTick:         11,
		RequestID:           "request-1",
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	if !pool.Dispatch("test-worker", interfaces.WorkDispatch{
		DispatchID:   "d-1",
		TransitionID: "transition-1",
		Execution:    want,
	}) {
		t.Fatal("expected dispatch to succeed")
	}

	select {
	case got := <-seen:
		assertExecutionMetadataEqual(t, want, got)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for executor metadata")
	}
	select {
	case <-pool.ResultCh():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func TestWorkerPool_DispatchUnknownType(t *testing.T) {
	pool := NewWorkerPool(nil)

	ok := pool.Dispatch("nonexistent", interfaces.WorkDispatch{TransitionID: "tr-1"})
	if ok {
		t.Fatal("expected dispatch to unknown worker type to return false")
	}
}

func TestWorkerRunner_ExecutorError(t *testing.T) {
	pool := NewWorkerPool(nil)

	executor := &mockExecutor{
		fn: func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
			return interfaces.WorkResult{}, fmt.Errorf("connection refused")
		},
	}

	pool.Register("error-worker", executor)
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("error-worker", interfaces.WorkDispatch{
		TransitionID: "tr-err",
	})

	select {
	case result := <-pool.ResultCh():
		if result.Outcome != interfaces.OutcomeFailed {
			t.Errorf("expected FAILED, got %s", result.Outcome)
		}
		if result.Error != "connection refused" {
			t.Errorf("expected 'connection refused', got %q", result.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error result")
	}
}

func TestWorkerRunner_ExecutorPanic(t *testing.T) {
	pool := NewWorkerPool(nil)

	executor := &mockExecutor{
		fn: func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
			panic("simulated panic")
		},
	}

	pool.Register("panic-worker", executor)
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("panic-worker", interfaces.WorkDispatch{
		DispatchID:   "d-panic",
		TransitionID: "tr-panic",
	})

	select {
	case result := <-pool.ResultCh():
		if result.Outcome != interfaces.OutcomeFailed {
			t.Errorf("expected FAILED, got %s", result.Outcome)
		}
		if result.DispatchID != "d-panic" {
			t.Errorf("expected dispatch ID d-panic, got %s", result.DispatchID)
		}
		if result.Error == "" {
			t.Fatal("expected panic-derived error message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panic result")
	}
}

func TestWorkerPool_MultipleWorkerTypes(t *testing.T) {
	pool := NewWorkerPool(nil)

	makeExecutor := func(suffix string) *mockExecutor {
		return &mockExecutor{
			fn: func(ctx context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
				return interfaces.WorkResult{
					TransitionID: d.TransitionID,
					Outcome:      interfaces.OutcomeAccepted,
					Feedback:     suffix,
				}, nil
			},
		}
	}

	pool.Register("worker-a", makeExecutor("a"))
	pool.Register("worker-b", makeExecutor("b"))
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("worker-a", interfaces.WorkDispatch{TransitionID: "tr-a"})
	pool.Dispatch("worker-b", interfaces.WorkDispatch{TransitionID: "tr-b"})

	results := map[string]string{}
	for range 2 {
		select {
		case r := <-pool.ResultCh():
			results[r.TransitionID] = r.Feedback
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for results")
		}
	}

	if results["tr-a"] != "a" {
		t.Errorf("worker-a result: expected feedback 'a', got %q", results["tr-a"])
	}
	if results["tr-b"] != "b" {
		t.Errorf("worker-b result: expected feedback 'b', got %q", results["tr-b"])
	}
}
