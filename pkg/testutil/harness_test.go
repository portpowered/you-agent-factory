package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestMarkingAssert_PlaceTokenCount(t *testing.T) {
	// Build a config with two sequential stages: item:new → item:done.
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "item",
			States: []interfaces.StateConfig{
				{Name: "new", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "error", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "work",
			WorkerTypeName: "w",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "item", StateName: "new"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "item", StateName: "done"}},
		}},
	}
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	h := testutil.NewServiceTestHarness(t, dir)
	h.MockWorker("w", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	// Submit queues the token; RunUntilComplete processes it via the engine.
	h.SubmitWork("item", []byte("test"))
	h.RunUntilComplete(t, 5*time.Second)

	// Token should have moved to done after processing.
	h.Assert().
		HasTokenInPlace("item:done").
		HasNoTokenInPlace("item:new").
		PlaceTokenCount("item:done", 1).
		TokenCount(1)
}

// TestMockWorker_AsyncDispatch demonstrates that MockWorker works with async
// dispatch (WithRunAsync). The mock executor is registered after construction
// and executes asynchronously via the worker pool, producing results that flow
// through the full petri net.
func TestMockWorker_AsyncDispatch(t *testing.T) {
	// 2-stage pipeline: item:new → stage1 → item:done
	cfg := testutil.PipelineConfig(1, "processor")
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	// Register mock AFTER construction — delegating executor picks it up at runtime.
	mock := h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.SubmitWork("task", []byte(`{"title":"async mock test"}`))
	h.RunUntilComplete(t, 10*time.Second)

	// Token should have flowed through the full async pipeline.
	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		TokenCount(1)

	// Mock should have been invoked by the worker pool (2 transitions in pipeline).
	if mock.CallCount() != 2 {
		t.Errorf("expected mock called 2 times (step1 + finish), got %d", mock.CallCount())
	}
}

// TestSetCustomExecutor_AsyncDispatch demonstrates that SetCustomExecutor works
// with async dispatch. The custom executor runs in the worker pool and its
// results flow through the petri net asynchronously.
func TestSetCustomExecutor_AsyncDispatch(t *testing.T) {
	cfg := testutil.PipelineConfig(1, "processor")
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	// Register custom executor that tracks call count.
	tracker := &callTracker{}
	h.SetCustomExecutor("processor", tracker)

	h.SubmitWork("task", []byte(`{"title":"custom executor async"}`))
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		TokenCount(1)

	if tracker.count != 2 {
		t.Errorf("expected custom executor called 2 times, got %d", tracker.count)
	}
}

// callTracker is a simple WorkerExecutor that counts calls and always accepts.
type callTracker struct {
	count int
}

func (c *callTracker) Execute(_ context.Context, d interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	c.count++
	return interfaces.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

func TestMockExecutor_CallTracking(t *testing.T) {
	mock := testutil.NewMockExecutor(
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
	)

	if mock.CallCount() != 0 {
		t.Errorf("expected 0 calls, got %d", mock.CallCount())
	}

	dispatch := interfaces.WorkDispatch{TransitionID: "t1"}
	result, err := mock.Execute(t.Context(), dispatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Errorf("expected ACCEPTED, got %s", result.Outcome)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.CallCount())
	}

	result, err = mock.Execute(t.Context(), interfaces.WorkDispatch{TransitionID: "t2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeRejected {
		t.Errorf("expected REJECTED, got %s", result.Outcome)
	}

	// Third call should return default (ACCEPTED).
	result, err = mock.Execute(t.Context(), interfaces.WorkDispatch{TransitionID: "t3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Errorf("expected default ACCEPTED, got %s", result.Outcome)
	}

	if mock.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", mock.CallCount())
	}

	last := mock.LastCall()
	if last.TransitionID != "t3" {
		t.Errorf("expected last call transition t3, got %s", last.TransitionID)
	}
}
