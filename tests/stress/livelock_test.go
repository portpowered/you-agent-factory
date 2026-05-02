package stress_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestLivelockInfiniteLoop validates that a simple infinite loop
// (A → B → A → B → ...) is terminated by a guarded loop-breaker workstation
// after the specified number of visits.
//
// Workflow: task:init → step-a (worker) → task:processing → step-b (worker, rejects → init) → task:complete
// Loop breaker: VisitCountGuard on "step-a" at TotalVisits >= 10
//
// Since step-b always rejects, the token loops init → processing → init endlessly.
// The loop breaker fires after 10 visits to step-a, routing the token to failed.
func TestLivelockInfiniteLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}, {Name: "worker-b"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "step-a", WorkerTypeName: "worker-a",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
			},
			{
				Name: "step-b", WorkerTypeName: "worker-b",
				Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
				OnFailure:   &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
			},
			guardedLoopBreakerWorkstation(
				"loop-exhausted",
				"step-a",
				10,
				interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
				interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
			),
		},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	// worker-a always accepts (moves init → processing).
	workerAResults := make([]interfaces.WorkResult, 20)
	for i := range workerAResults {
		workerAResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	workerA := h.MockWorker("worker-a", workerAResults...)

	// worker-b always rejects (moves processing → init, creating the loop).
	workerBResults := make([]interfaces.WorkResult, 20)
	for i := range workerBResults {
		workerBResults[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "loop back"}
	}
	workerB := h.MockWorker("worker-b", workerBResults...)

	h.SubmitWork("task", []byte(`{"task": "infinite loop test"}`))

	h.RunUntilComplete(t, 10*time.Second)

	// Assert: token ends in task:failed (guarded loop-breaker route).
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)

	// Assert: step-a called exactly 10 times (loop breaker fires on 11th arrival at init).
	if workerA.CallCount() != 10 {
		t.Errorf("expected worker-a called 10 times, got %d", workerA.CallCount())
	}

	// Assert: step-b called exactly 10 times (each cycle: a→b→reject→a).
	if workerB.CallCount() != 10 {
		t.Errorf("expected worker-b called 10 times, got %d", workerB.CallCount())
	}
}

// TestLivelockTriangleLoop validates that a 3-node cycle (A → B → C → A → ...)
// where each transition succeeds but routes back to the start is terminated by
// a GlobalLimits MaxTotalVisits constraint.
//
// Workflow: task:init → step-1 → task:stage-a → step-2 → task:stage-b → step-3 (rejects → init)
// Loop breaker: VisitCountGuard on "step-1" at TotalVisits >= 5
//
// Each full cycle visits step-1, step-2, step-3. After 5 cycles through step-1
// (= 15 total transition visits across all three), the loop breaker fires.
// portos:func-length-exception owner=agent-factory reason=legacy-livelock-triangle-fixture review=2026-07-19 removal=split-cycle-fixture-run-and-guard-assertions-before-next-livelock-change
func TestLivelockTriangleLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "stage-a", Type: interfaces.StateTypeProcessing},
				{Name: "stage-b", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-1"}, {Name: "worker-2"}, {Name: "worker-3"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "step-1", WorkerTypeName: "worker-1",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage-a"}},
			},
			{
				Name: "step-2", WorkerTypeName: "worker-2",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage-a"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage-b"}},
			},
			{
				Name: "step-3", WorkerTypeName: "worker-3",
				Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "stage-b"}},
				Outputs:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
				OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
				OnFailure:   &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
			},
			guardedLoopBreakerWorkstation(
				"triangle-exhausted",
				"step-1",
				5,
				interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
				interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
			),
		},
	})
	h := testutil.NewServiceTestHarness(t, dir)

	// All workers accept, but step-3 always rejects → loops back to init.
	w1Results := make([]interfaces.WorkResult, 20)
	for i := range w1Results {
		w1Results[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	w1 := h.MockWorker("worker-1", w1Results...)

	w2Results := make([]interfaces.WorkResult, 20)
	for i := range w2Results {
		w2Results[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
	}
	w2 := h.MockWorker("worker-2", w2Results...)

	w3Results := make([]interfaces.WorkResult, 20)
	for i := range w3Results {
		w3Results[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "cycle back"}
	}
	w3 := h.MockWorker("worker-3", w3Results...)

	h.SubmitWork("task", []byte(`{"task": "triangle livelock test"}`))

	h.RunUntilComplete(t, 10*time.Second)

	// Assert: token ends in task:failed.
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:stage-a").
		HasNoTokenInPlace("task:stage-b").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)

	// Assert: each worker called exactly 5 times (5 full cycles).
	if w1.CallCount() != 5 {
		t.Errorf("expected worker-1 called 5 times, got %d", w1.CallCount())
	}
	if w2.CallCount() != 5 {
		t.Errorf("expected worker-2 called 5 times, got %d", w2.CallCount())
	}
	if w3.CallCount() != 5 {
		t.Errorf("expected worker-3 called 5 times, got %d", w3.CallCount())
	}
}

// TestLivelockExecutionTimeout verifies livelock variants complete
// within a 5s time bound, proving no actual infinite loops occur.
// portos:func-length-exception owner=agent-factory reason=legacy-livelock-timeout-table review=2026-07-19 removal=split-timeout-scenario-fixtures-and-duration-assertions-before-next-livelock-change
func TestLivelockExecutionTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	start := time.Now()

	// Run all three variants sequentially within the timeout.
	t.Run("InfiniteLoop", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			}},
			Workers: []interfaces.WorkerConfig{{Name: "wa"}, {Name: "wb"}},
			Workstations: []interfaces.FactoryWorkstationConfig{
				{
					Name: "step-a", WorkerTypeName: "wa",
					Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
					Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				},
				{
					Name: "step-b", WorkerTypeName: "wb",
					Inputs:      []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
					Outputs:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
					OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
					OnFailure:   &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
				},
				guardedLoopBreakerWorkstation(
					"exhausted",
					"step-a",
					10,
					interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
					interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
				),
			},
		})
		h := testutil.NewServiceTestHarness(t, dir)
		aRes := make([]interfaces.WorkResult, 20)
		bRes := make([]interfaces.WorkResult, 20)
		for i := range 20 {
			aRes[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
			bRes[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeRejected}
		}
		h.MockWorker("wa", aRes...)
		h.MockWorker("wb", bRes...)
		h.SubmitWork("task", []byte(`{}`))
		h.RunUntilComplete(t, 10*time.Second)
		h.Assert().HasTokenInPlace("task:failed").TokenCount(1)
	})

	t.Run("TriangleLoop", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
			WorkTypes: []interfaces.WorkTypeConfig{{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "mid", Type: interfaces.StateTypeProcessing},
					{Name: "end", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			}},
			Workers: []interfaces.WorkerConfig{{Name: "w1"}, {Name: "w2"}, {Name: "w3"}},
			Workstations: []interfaces.FactoryWorkstationConfig{
				{Name: "s1", WorkerTypeName: "w1", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "mid"}}},
				{Name: "s2", WorkerTypeName: "w2", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "mid"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "end"}}},
				{Name: "s3", WorkerTypeName: "w3", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "end"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}, OnRejection: &interfaces.IOConfig{WorkTypeName: "task", StateName: "init"}, OnFailure: &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"}},
				guardedLoopBreakerWorkstation(
					"ex",
					"s1",
					5,
					interfaces.IOConfig{WorkTypeName: "task", StateName: "init"},
					interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
				),
			},
		})
		h := testutil.NewServiceTestHarness(t, dir)
		r := make([]interfaces.WorkResult, 20)
		rr := make([]interfaces.WorkResult, 20)
		for i := range 20 {
			r[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted}
			rr[i] = interfaces.WorkResult{Outcome: interfaces.OutcomeRejected}
		}
		h.MockWorker("w1", r...)
		h.MockWorker("w2", r...)
		h.MockWorker("w3", rr...)
		h.SubmitWork("task", []byte(`{}`))
		h.RunUntilComplete(t, 10*time.Second)
		h.Assert().HasTokenInPlace("task:failed").TokenCount(1)
	})

	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("all livelock tests took %v, expected < 5s", elapsed)
	}
	t.Logf("all livelock variants completed in %v", elapsed)
}
