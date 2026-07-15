package testutil_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
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
		Workers: []workerconfig.Config{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "work",
			WorkerTypeName: "w",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "item", StateName: "new"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "item", StateName: "done"}},
		}},
	}
	dir := testutil.ScaffoldFactoryDir(t, cfg)

	h := testutil.NewServiceTestHarness(t, dir)
	h.MockWorker("w", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

	// Submit queues the token; RunUntilComplete processes it via the engine.
	if err := h.SubmitWork("item", []byte("test")); err != nil {
		t.Fatalf("submit work: %v", err)
	}
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
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
	)

	if err := h.SubmitWork("task", []byte(`{"title":"async mock test"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
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

func TestMockWorker_AllowsBuiltInRunnerConfigWithoutLocalBinary(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []workerconfig.Config{{Name: "worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}
	dir := testutil.ScaffoldFactoryDir(t, cfg)
	workerDir := filepath.Join(dir, interfaces.WorkersDir, "worker")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, interfaces.FactoryAgentsFileName), []byte(`---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: codex
stopToken: COMPLETE
---
Process the task.
`), 0o644); err != nil {
		t.Fatalf("WriteFile AGENTS.md: %v", err)
	}

	h := testutil.NewServiceTestHarness(t, dir)
	mock := h.MockWorker("worker", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

	if err := h.SubmitWork("task", []byte(`{"title":"uses mocked runner"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		HasTokenInPlace("task:done").
		HasNoTokenInPlace("task:init")

	if mock.CallCount() != 1 {
		t.Fatalf("expected mocked worker call count 1, got %d", mock.CallCount())
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

	if err := h.SubmitWork("task", []byte(`{"title":"custom executor async"}`)); err != nil {
		t.Fatalf("submit work: %v", err)
	}
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

func (c *callTracker) Execute(_ context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
	c.count++
	return workerexecution.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

func TestMockExecutor_CallTracking(t *testing.T) {
	mock := testutil.NewMockExecutor(
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeRejected},
	)

	if mock.CallCount() != 0 {
		t.Errorf("expected 0 calls, got %d", mock.CallCount())
	}

	dispatch := work.WorkDispatch{TransitionID: "t1"}
	result, err := mock.Execute(t.Context(), dispatch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Errorf("expected ACCEPTED, got %s", result.Outcome)
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.CallCount())
	}

	result, err = mock.Execute(t.Context(), work.WorkDispatch{TransitionID: "t2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeRejected {
		t.Errorf("expected REJECTED, got %s", result.Outcome)
	}

	// Third call should return default (ACCEPTED).
	result, err = mock.Execute(t.Context(), work.WorkDispatch{TransitionID: "t3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
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
