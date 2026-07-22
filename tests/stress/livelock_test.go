package stress_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "worker-a", StopToken: "COMPLETE"}, {Name: "worker-b", StopToken: "COMPLETE"}},
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
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
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"worker-a": repeatedInferenceResponses("COMPLETE", 20),
		"worker-b": repeatedInferenceResponses("loop back", 20),
	})
	h := startStressProcess(t, dir, provider)

	h.SubmitWork("task", []byte(`{"task": "infinite loop test"}`))

	h.WaitForTerminalCount(1, 10*time.Second)

	// Assert: token ends in task:failed (guarded loop-breaker route).
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)

	// Assert: step-a called exactly 10 times (loop breaker fires on 11th arrival at init).
	if provider.CallCount("worker-a") != 10 {
		t.Errorf("expected worker-a called 10 times, got %d", provider.CallCount("worker-a"))
	}

	// Assert: step-b called exactly 10 times (each cycle: a→b→reject→a).
	if provider.CallCount("worker-b") != 10 {
		t.Errorf("expected worker-b called 10 times, got %d", provider.CallCount("worker-b"))
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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "worker-1", StopToken: "COMPLETE"}, {Name: "worker-2", StopToken: "COMPLETE"}, {Name: "worker-3", StopToken: "COMPLETE"}},
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
				OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				OnFailure:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
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
	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"worker-1": repeatedInferenceResponses("COMPLETE", 20),
		"worker-2": repeatedInferenceResponses("COMPLETE", 20),
		"worker-3": repeatedInferenceResponses("cycle back", 20),
	})
	h := startStressProcess(t, dir, provider)

	h.SubmitWork("task", []byte(`{"task": "triangle livelock test"}`))

	h.WaitForTerminalCount(1, 10*time.Second)

	// Assert: token ends in task:failed.
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:stage-a").
		HasNoTokenInPlace("task:stage-b").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)

	// Assert: each worker called exactly 5 times (5 full cycles).
	if provider.CallCount("worker-1") != 5 {
		t.Errorf("expected worker-1 called 5 times, got %d", provider.CallCount("worker-1"))
	}
	if provider.CallCount("worker-2") != 5 {
		t.Errorf("expected worker-2 called 5 times, got %d", provider.CallCount("worker-2"))
	}
	if provider.CallCount("worker-3") != 5 {
		t.Errorf("expected worker-3 called 5 times, got %d", provider.CallCount("worker-3"))
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

	t.Run("InfiniteLoop", func(t *testing.T) {
		h := newInfiniteLoopTimeoutHarness(t)
		h.SubmitWork("task", []byte(`{}`))
		h.WaitForTerminalCount(1, 10*time.Second)
		h.Assert().HasTokenInPlace("task:failed").TokenCount(1)
	})

	t.Run("TriangleLoop", func(t *testing.T) {
		h := newTriangleLoopTimeoutHarness(t)
		h.SubmitWork("task", []byte(`{}`))
		h.WaitForTerminalCount(1, 10*time.Second)
		h.Assert().HasTokenInPlace("task:failed").TokenCount(1)
	})

	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Fatalf("all livelock tests took %v, expected < 5s", elapsed)
	}
	t.Logf("all livelock variants completed in %v", elapsed)
}

func newInfiniteLoopTimeoutHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "wa", StopToken: "COMPLETE"}, {Name: "wb", StopToken: "COMPLETE"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "step-a", WorkerTypeName: "wa", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}}},
			{Name: "step-b", WorkerTypeName: "wb", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}, OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
			guardedLoopBreakerWorkstation("exhausted", "step-a", 10, interfaces.IOConfig{WorkTypeName: "task", StateName: "init"}, interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"}),
		},
	})
	return startStressProcess(t, dir, testutil.NewMockWorkerMapProvider(
		map[string][]workerexecution.InferenceResponse{
			"wa": repeatedInferenceResponses("COMPLETE", 20),
			"wb": repeatedInferenceResponses("loop back", 20),
		},
	))
}

func newTriangleLoopTimeoutHarness(t *testing.T) *stressProcessHarness {
	t.Helper()

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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "w1", StopToken: "COMPLETE"}, {Name: "w2", StopToken: "COMPLETE"}, {Name: "w3", StopToken: "COMPLETE"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "s1", WorkerTypeName: "w1", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "mid"}}},
			{Name: "s2", WorkerTypeName: "w2", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "mid"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "end"}}},
			{Name: "s3", WorkerTypeName: "w3", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "end"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}, OnRejection: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
			guardedLoopBreakerWorkstation("ex", "s1", 5, interfaces.IOConfig{WorkTypeName: "task", StateName: "init"}, interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"}),
		},
	})
	return startStressProcess(t, dir, testutil.NewMockWorkerMapProvider(
		map[string][]workerexecution.InferenceResponse{
			"w1": repeatedInferenceResponses("COMPLETE", 20),
			"w2": repeatedInferenceResponses("COMPLETE", 20),
			"w3": repeatedInferenceResponses("loop back", 20),
		},
	))
}

func repeatedInferenceResponses(content string, count int) []workerexecution.InferenceResponse {
	responses := make([]workerexecution.InferenceResponse, count)
	for i := range responses {
		responses[i] = workerexecution.InferenceResponse{Content: content}
	}
	return responses
}
