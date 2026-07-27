package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// mockExecutor implements WorkerExecutor for testing.
type mockExecutor struct {
	fn func(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error)
}

func (m *mockExecutor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return m.fn(ctx, dispatch)
}

func TestWorkerPool_DispatchAndResult(t *testing.T) {
	pool := newWorkerPool(nil, testRuntimeClock{})

	executor := &mockExecutor{
		fn: func(ctx context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
			return workerexecution.WorkResult{
				TransitionID: d.TransitionID,
				Outcome:      workerexecution.OutcomeAccepted,
			}, nil
		},
	}

	pool.Register("test-worker", executor)
	pool.Start()
	defer pool.Stop()

	dispatch := work.WorkDispatch{
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
		if result.Outcome != workerexecution.OutcomeAccepted {
			t.Errorf("expected ACCEPTED, got %s", result.Outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result")
	}
}

func TestWorkerPool_DispatchPreservesExecutionMetadataForExecutor(t *testing.T) {
	pool := newWorkerPool(nil, testRuntimeClock{})
	seen := make(chan work.ExecutionMetadata, 1)

	executor := &mockExecutor{
		fn: func(ctx context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
			seen <- d.Execution
			return workerexecution.WorkResult{
				DispatchID:   d.DispatchID,
				TransitionID: d.TransitionID,
				Outcome:      workerexecution.OutcomeAccepted,
			}, nil
		},
	}

	pool.Register("test-worker", executor)
	pool.Start()
	defer pool.Stop()

	want := work.ExecutionMetadata{
		DispatchCreatedTick: 10,
		CurrentTick:         11,
		RequestID:           "request-1",
		TraceID:             "trace-1",
		WorkIDs:             []string{"work-1", "work-2"},
		ReplayKey:           "transition-1/trace-1/work-1/work-2",
	}
	if !pool.Dispatch("test-worker", work.WorkDispatch{
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
	pool := newWorkerPool(nil, testRuntimeClock{})

	ok := pool.Dispatch("nonexistent", work.WorkDispatch{TransitionID: "tr-1"})
	if ok {
		t.Fatal("expected dispatch to unknown worker type to return false")
	}
}

func TestWorkerRunner_ExecutorError(t *testing.T) {
	pool := newWorkerPool(nil, testRuntimeClock{})

	executor := &mockExecutor{
		fn: func(ctx context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
			return workerexecution.WorkResult{}, fmt.Errorf("connection refused")
		},
	}

	pool.Register("error-worker", executor)
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("error-worker", work.WorkDispatch{
		TransitionID: "tr-err",
	})

	select {
	case result := <-pool.ResultCh():
		if result.Outcome != workerexecution.OutcomeFailed {
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
	pool := newWorkerPool(nil, testRuntimeClock{})

	executor := &mockExecutor{
		fn: func(ctx context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
			panic("simulated panic")
		},
	}

	pool.Register("panic-worker", executor)
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("panic-worker", work.WorkDispatch{
		DispatchID:   "d-panic",
		TransitionID: "tr-panic",
	})

	select {
	case result := <-pool.ResultCh():
		if result.Outcome != workerexecution.OutcomeFailed {
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

func assertExecutionMetadataEqual(t *testing.T, want, got work.ExecutionMetadata) {
	t.Helper()
	if want.RequestID != got.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, want.RequestID)
	}
	if want.TraceID != got.TraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if len(want.WorkIDs) != len(got.WorkIDs) {
		t.Fatalf("WorkIDs length = %d, want %d", len(got.WorkIDs), len(want.WorkIDs))
	}
	for i := range want.WorkIDs {
		if want.WorkIDs[i] != got.WorkIDs[i] {
			t.Fatalf("WorkIDs[%d] = %q, want %q", i, got.WorkIDs[i], want.WorkIDs[i])
		}
	}
}

func TestWorkerPool_MultipleWorkerTypes(t *testing.T) {
	pool := newWorkerPool(nil, testRuntimeClock{})

	makeExecutor := func(suffix string) *mockExecutor {
		return &mockExecutor{
			fn: func(ctx context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
				return workerexecution.WorkResult{
					TransitionID: d.TransitionID,
					Outcome:      workerexecution.OutcomeAccepted,
					Feedback:     suffix,
				}, nil
			},
		}
	}

	pool.Register("worker-a", makeExecutor("a"))
	pool.Register("worker-b", makeExecutor("b"))
	pool.Start()
	defer pool.Stop()

	pool.Dispatch("worker-a", work.WorkDispatch{TransitionID: "tr-a"})
	pool.Dispatch("worker-b", work.WorkDispatch{TransitionID: "tr-b"})

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

func TestRuntimeWorkersBoundaryRouteNamesIncludeWorkerAndWorkstationKeys(t *testing.T) {
	net := &state.Net{
		Transitions: map[string]*petri.Transition{
			"tr-1": {ID: "tr-1", Name: "review", WorkerType: "swe"},
		},
	}
	names := runtimeWorkersBoundaryRouteNames(net, map[string]workers.WorkerExecutor{
		"swe": &mockExecutor{},
	})
	want := map[string]struct{}{"tr-1": {}, "review": {}, "swe": {}}
	if len(names) != len(want) {
		t.Fatalf("route names = %v, want %v", names, want)
	}
	for _, name := range names {
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected route name %q in %v", name, names)
		}
	}
}

func TestRuntimeWorkersBoundaryBindingsPreserveLegacyConcurrency(t *testing.T) {
	boundary := newRuntimeWorkersBoundary(
		nil,
		nil,
		map[string]workers.WorkerExecutor{"swe": &mockExecutor{}},
		true,
	)
	if len(boundary.bindings) != 1 {
		t.Fatalf("bindings = %d, want 1", len(boundary.bindings))
	}
	binding := boundary.bindings[0]
	if binding.Capacity != defaultRuntimeBufferSize || binding.QueueCapacity != defaultRuntimeBufferSize {
		t.Fatalf(
			"binding capacity = (%d, %d), want (%d, %d)",
			binding.Capacity,
			binding.QueueCapacity,
			defaultRuntimeBufferSize,
			defaultRuntimeBufferSize,
		)
	}
}
