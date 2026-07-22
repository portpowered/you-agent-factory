package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestPoisonTokenMalformedSubmissions verifies that malformed work submissions
// (empty payload, missing WorkTypeID, invalid WorkTypeID, extremely large payload)
// don't crash the engine or corrupt the marking.
// portos:func-length-exception owner=agent-factory reason=legacy-poison-submission-fixture review=2026-07-19 removal=split-malformed-submission-cases-and-marking-assertions-before-next-poison-token-change
func TestPoisonTokenMalformedSubmissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	t.Run("empty_work_type_name", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		requireSubmitRejected(t, h, "", []byte(`{"test": "empty-type"}`), "missing workTypeName")
		assertNoWorkTokens(t, h, "empty WorkTypeID")
	})

	t.Run("invalid_work_type_name", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		requireSubmitRejected(t, h, "nonexistent-type", []byte(`{"test": "bad-type"}`), "unknown work type")
		assertNoWorkTokens(t, h, "invalid WorkTypeID")
	})

	t.Run("nil_payload", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		h.SubmitWork("task", nil)
		assertSingleCompletedSubmission(t, h, "nil payload")
	})

	t.Run("empty_payload", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		h.SubmitWork("task", []byte{})
		assertSingleCompletedSubmission(t, h, "empty payload")
	})

	t.Run("extremely_large_payload", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		largePayload := make([]byte, 1024*1024)
		for i := range largePayload {
			largePayload[i] = byte('A' + (i % 26))
		}
		h.SubmitWork("task", largePayload)
		assertSingleCompletedSubmission(t, h, "large payload")
	})

	t.Run("mixed_valid_and_invalid_submissions", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		requireSubmitRejected(t, h, "", []byte(`{"bad": 1}`), "missing workTypeName")
		requireSubmitRejected(t, h, "nonexistent", []byte(`{"bad": 2}`), "unknown work type")
		h.SubmitWork("task", []byte(`{"good": 1}`))
		requireSubmitRejected(t, h, "also-nonexistent", []byte(`{"bad": 3}`), "unknown work type")
		h.SubmitWork("task", []byte(`{"good": 2}`))

		h.WaitForTerminalCount(2, 10*time.Second)

		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 2 {
			t.Errorf("expected 2 valid tokens to complete, got %d", completeCount)
		}
	})

	t.Run("mixed_batch_rejects_without_partial_submit", func(t *testing.T) {
		h := newPoisonSubmitHarness(t)
		err := h.SubmitFullError(context.Background(), []work.SubmitRequest{
			{WorkTypeID: "task", Payload: []byte(`{"good": 1}`)},
			{WorkTypeID: "missing", Payload: []byte(`{"bad": 1}`)},
		})
		assertSubmitErrorContains(t, err, "unknown work type")

		snap := h.Marking()
		workTokens := countWorkTokens(snap)
		if workTokens != 0 {
			t.Errorf("expected rejected batch to create 0 work tokens, got %d", workTokens)
		}
	})
}

// TestPoisonTokenGeneratedBatchBoundaries verifies that invalid generated Work
// batches fail atomically while large valid batches complete through the
// customer-visible process boundary. Malformed internal WorkResult invariants
// are covered by the Factory Runtime subsystem that owns that contract.
func TestPoisonTokenGeneratedBatchBoundaries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	t.Run("result_with_massive_invalid_generated_work_batch", func(t *testing.T) {
		assertMassiveInvalidGeneratedBatchHandled(t)
	})

	t.Run("result_with_massive_valid_generated_work_batch", func(t *testing.T) {
		assertMassiveValidGeneratedBatchHandled(t)
	})
}

// TestPoisonTokenValidWorkUnaffected verifies that valid work submitted
// alongside poison tokens still completes successfully.
// portos:func-length-exception owner=agent-factory reason=legacy-poison-valid-work-fixture review=2026-07-19 removal=split-valid-work-setup-poison-input-and-completion-assertions-before-next-poison-token-change
func TestPoisonTokenValidWorkUnaffected(t *testing.T) {
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
		Workers: []interfaces.FactoryWorkerConfig{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "process", WorkerTypeName: "w",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
			},
			{
				Name: "finish", WorkerTypeName: "w",
				Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "processing"}},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			},
		},
	})
	h := startStressProcess(t, dir, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})

	const validItems = 10

	// Submit a burst of mixed valid and invalid work.
	for i := range validItems {
		requireSubmitFullRejected(t, h, []work.SubmitRequest{
			{WorkTypeID: "", Payload: []byte(`{"poison":true}`)},
		}, "missing workTypeName")
		requireSubmitFullRejected(t, h, []work.SubmitRequest{
			{WorkTypeID: "bogus", Payload: []byte(`{"poison":true}`)},
		}, "unknown work type")
		h.SubmitFull(context.Background(), []work.SubmitRequest{
			{WorkTypeID: "task", Payload: fmt.Appendf(nil, `{"item":%d}`, i)},
		})
		requireSubmitFullRejected(t, h, []work.SubmitRequest{
			{WorkTypeID: "also-bogus", Payload: []byte(`{"poison":true}`)},
		}, "unknown work type")
	}

	// Poll until all valid items reach terminal state.
	poisonTerminalPlaces := []string{"task:complete", "task:failed"}
	h.WaitForTerminalCount(validItems, 10*time.Second)
	snap := h.Marking()
	h.Stop()

	// All valid items should complete.
	terminalCount := countTerminalTokens(snap, poisonTerminalPlaces)
	if terminalCount != validItems {
		t.Errorf("expected %d terminal tokens, got %d", validItems, terminalCount)
	}

	// Total tokens should exactly equal valid items (no phantom tokens from invalid submissions).
	if len(snap.Tokens) != validItems {
		t.Errorf("expected %d total tokens, got %d (phantom tokens from invalid submissions?)", validItems, len(snap.Tokens))
	}

	t.Logf("all %d valid items completed, %d total tokens", terminalCount, len(snap.Tokens))
}

// TestPoisonTokenNoGlobalStateCorruption verifies that malformed work in one
// workflow engine does not corrupt the state of a separate workflow engine
// running concurrently.
// portos:func-length-exception owner=agent-factory reason=legacy-poison-isolation-fixture review=2026-07-19 removal=split-concurrent-engine-setup-poison-run-and-isolation-assertions-before-next-poison-token-change
func TestPoisonTokenNoGlobalStateCorruption(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Config for simple single-step workflows.
	simpleOneStageCfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: "w",
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
		}},
	}

	// Workflow A: clean workflow that should complete normally.
	dirA := testutil.ScaffoldFactoryDir(t, simpleOneStageCfg)
	hA := startStressProcess(t, dirA, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})

	// Workflow B: receives poison submissions.
	dirB := testutil.ScaffoldFactoryDir(t, simpleOneStageCfg)
	hB := startStressProcess(t, dirB, workerExecutorProvider{
		executor: testutil.NewMockExecutor(),
	})

	const itemsPerWorkflow = 5

	// Submit valid work to A.
	for i := range itemsPerWorkflow {
		hA.SubmitWork("task", fmt.Appendf(nil, `{"clean": %d}`, i))
	}

	// Submit a mix of valid and invalid to B. Invalid types are rejected at the
	// boundary and must not corrupt the clean workflow or later valid work.
	for i := range itemsPerWorkflow {
		requireSubmitRejected(t, hB, "nonexistent-type", fmt.Appendf(nil, `{"poison": %d}`, i), "unknown work type")
	}
	// Also submit some valid work to B.
	for i := range itemsPerWorkflow {
		hB.SubmitWork("task", fmt.Appendf(nil, `{"valid-in-B": %d}`, i))
	}

	// Run both. Poison submissions in B should not affect A.
	hA.WaitForTerminalCount(itemsPerWorkflow, 10*time.Second)
	hB.WaitForTerminalCount(itemsPerWorkflow, 10*time.Second)

	snapA := hA.Marking()
	assertPoisonIsolationMarking(t, snapA, itemsPerWorkflow, "workflow A")

	snapB := hB.Marking()
	assertPoisonIsolationMarking(t, snapB, itemsPerWorkflow, "workflow B")
	hA.Stop()
	hB.Stop()
}

func assertMassiveInvalidGeneratedBatchHandled(t *testing.T) {
	t.Helper()

	executor := &massiveGeneratedBatchExecutor{
		spawnCount:  10000,
		spawnTypeID: "nonexistent-type",
		realOutcome: workerexecution.OutcomeAccepted,
	}
	dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("spawn-worker"))
	h := startStressProcess(t, dir, workerExecutorProvider{executor: executor})
	t.Cleanup(h.Stop)
	h.SubmitWork("task", []byte(`{"test": "massive-spawn"}`))
	h.WaitForTerminalCount(1, 10*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "task:failed", 1)
	if workItems := session.Runtime.Progress.TotalTokens; workItems != 1 {
		t.Errorf("expected 1 public Work item (no partial generated submissions), got %d", workItems)
	}
}

func assertMassiveValidGeneratedBatchHandled(t *testing.T) {
	t.Helper()

	const spawnCount = 500
	dir := testutil.ScaffoldFactoryDir(t, &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "task", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
			{Name: "child", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			}},
		},
		Workers: []interfaces.FactoryWorkerConfig{{Name: "spawn-worker"}, {Name: "child-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "process", WorkerTypeName: "spawn-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
			{Name: "child-process", WorkerTypeName: "child-worker", Inputs: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "init"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}}},
		},
	})
	h := startStressProcess(t, dir, workerExecutorProvider{executor: workerMuxExecutor{
		"spawn-worker": &massiveGeneratedBatchExecutor{
			spawnCount:  spawnCount,
			spawnTypeID: "child",
			realOutcome: workerexecution.OutcomeAccepted,
		},
		"child-worker": &acceptedCountingExecutor{},
	}})
	t.Cleanup(h.Stop)
	h.SubmitWork("task", []byte(`{"test": "valid-spawn"}`))
	h.WaitForTerminalCount(spawnCount+1, 30*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "task:complete", 1)
	childComplete := sessionPlaceCount(session, "child:complete")
	if childComplete != spawnCount {
		childInit := sessionPlaceCount(session, "child:init")
		t.Errorf("expected %d children complete, got %d (init=%d)", spawnCount, childComplete, childInit)
	}
	t.Logf("successfully processed parent + %d generated children", childComplete)
}

// TestPoisonTokenNoPanic uses recover() to verify that various edge cases
// never cause an engine panic. Each subtest exercises a different poison vector.
func TestPoisonTokenNoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Build a simple workflow used across subtests.
	buildNet := func() *factoryruntime.RuntimeTokenColor {
		return nil // placeholder, not used
	}
	_ = buildNet

	poisonVectors := []struct {
		name    string
		payload []byte
		typeID  string
	}{
		{"empty_payload", []byte{}, "task"},
		{"nil_payload", nil, "task"},
		{"empty_type", []byte(`{}`), ""},
		{"missing_type", []byte(`{}`), "no-such-type"},
		{"1mb_payload", make([]byte, 1024*1024), "task"},
		{"json_null", []byte("null"), "task"},
		{"json_number", []byte("42"), "task"},
		{"json_string", []byte(`"just a string"`), "task"},
		{"json_array", []byte(`[1,2,3]`), "task"},
		{"binary_garbage", []byte{0xFF, 0xFE, 0x00, 0x01, 0x80}, "task"},
	}

	for _, pv := range poisonVectors {
		t.Run(pv.name, func(t *testing.T) {
			// Use recover to catch any panics.
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC on poison vector %q: %v", pv.name, r)
				}
			}()

			dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("w"))
			h := startStressProcess(t, dir, workerExecutorProvider{
				executor: &acceptedCountingExecutor{},
			})
			t.Cleanup(h.Stop)

			if pv.typeID == "" || pv.typeID == "no-such-type" {
				wantErr := "unknown work type"
				if pv.typeID == "" {
					wantErr = "missing workTypeName"
				}
				requireSubmitRejected(t, h, pv.typeID, pv.payload, wantErr)
				assertNoWorkTokens(t, h, pv.name)
			} else {
				h.SubmitWork(pv.typeID, pv.payload)
				h.WaitForTerminalCount(1, 10*time.Second)
			}

			// If we get here, no panic occurred.
		})
	}
}

// TestPoisonTokenExecutorPanic verifies that if an executor panics, the
// Factory runtime recovers the panic and routes the Work through failed-Work
// semantics without crashing the test process.
func TestPoisonTokenExecutorPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("panic-worker"))
	h := startStressProcess(t, dir, workerExecutorProvider{executor: &panicWorkerExecutor{
		message: "simulated executor catastrophic panic",
	}})
	t.Cleanup(h.Stop)

	h.SubmitWork("task", []byte(`{"test": "executor-panic"}`))

	// Executor panics are converted to OutcomeFailed WorkResults and routed via
	// FailureArcs — RunUntilComplete should succeed (no panic escaping Run()).
	h.WaitForTerminalCount(1, 10*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "task:failed", 1)
	assertSessionPlaceCount(t, session, "task:init", 0)
	assertSessionPlaceCount(t, session, "task:complete", 0)
	lastError := sessionTokenLastError(session, "task:failed")
	if !strings.Contains(lastError, "executor panic:") {
		t.Errorf("expected panic failure message in token history, got: %q", lastError)
	}
	if !strings.Contains(lastError, "simulated executor catastrophic panic") {
		t.Errorf("expected panic value in token history, got: %q", lastError)
	}
}

// TestPoisonTokenExecutorError verifies that executors returning Go errors are
// handled gracefully — the token is routed to the configured FailureArcs rather
// than being lost or crashing the tick. This mirrors production WorkerRunner behavior.
func TestPoisonTokenExecutorError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("error-worker"))
	h := startStressProcess(t, dir, workerExecutorProvider{executor: &errorExecutor{
		err: fmt.Errorf("simulated executor catastrophic failure"),
	}})
	t.Cleanup(h.Stop)

	h.SubmitWork("task", []byte(`{"test": "executor-error"}`))

	// Executor errors are converted to OutcomeFailed WorkResults and routed via
	// FailureArcs — RunUntilComplete should succeed (no panic).
	h.WaitForTerminalCount(1, 10*time.Second)

	session := h.Session()
	assertSessionPlaceCount(t, session, "task:failed", 1)
	assertSessionPlaceCount(t, session, "task:init", 0)
	assertSessionPlaceCount(t, session, "task:complete", 0)
	lastError := sessionTokenLastError(session, "task:failed")
	if !strings.Contains(lastError, "simulated executor catastrophic failure") {
		t.Errorf("expected failure message in token history, got: %q", lastError)
	}
}

// ---------------------------------------------------------------------------
// Helper configs
// ---------------------------------------------------------------------------

// poisonExecCfg returns a config for a single-step task workflow with failure path.
func poisonExecCfg(workerName string) *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: workerName,
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Helper executors
// ---------------------------------------------------------------------------

// massiveGeneratedBatchExecutor returns ACCEPTED with a canonical Work Request
// containing N generated Work items.
type massiveGeneratedBatchExecutor struct {
	spawnCount  int
	spawnTypeID string
	realOutcome workerexecution.WorkOutcome
}

func (e *massiveGeneratedBatchExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	generated := make([]work.Work, e.spawnCount)
	for i := range generated {
		generated[i] = work.Work{
			Name:       fmt.Sprintf("generated-%d", i),
			WorkID:     fmt.Sprintf("generated-%d", i),
			WorkTypeID: e.spawnTypeID,
			Payload:    map[string]int{"generated_index": i},
		}
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      e.realOutcome,
		Output:       workerGeneratedBatchOutput(generated),
	}, nil
}

// errorExecutor returns an error from Execute.
type errorExecutor struct {
	err error
}

func (e *errorExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{}, e.err
}

// panicWorkerExecutor panics from Execute to exercise executor panic recovery.
type panicWorkerExecutor struct {
	message string
}

func (e *panicWorkerExecutor) Execute(_ context.Context, _ work.WorkDispatch) (workerexecution.WorkResult, error) {
	panic(e.message)
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func assertSubmitErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected submit error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("submit error = %q, want substring %q", err.Error(), want)
	}
}

// countWorkTokens counts all tokens in the marking (excluding resource tokens).
func countWorkTokens(snap *factoryruntime.PetriMarkingSnapshot) int {
	count := 0
	for _, tok := range snap.Tokens {
		if !strings.Contains(tok.PlaceID, ":available") {
			count++
		}
	}
	return count
}

type poisonSubmissionHarness interface {
	SubmitError(string, []byte) error
	SubmitFullError(context.Context, []work.SubmitRequest) error
	Marking() *factoryruntime.PetriMarkingSnapshot
}

func requireSubmitRejected(t *testing.T, h poisonSubmissionHarness, workTypeID string, payload []byte, wantErr string) {
	t.Helper()

	err := h.SubmitError(workTypeID, payload)
	if err == nil {
		t.Fatalf("expected submit validation error containing %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("submit validation error = %v, want substring %q", err, wantErr)
	}
}

func requireSubmitFullRejected(t *testing.T, h poisonSubmissionHarness, reqs []work.SubmitRequest, wantErr string) {
	t.Helper()

	err := h.SubmitFullError(context.Background(), reqs)
	if err == nil {
		t.Fatalf("expected submit validation error containing %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("submit validation error = %v, want substring %q", err, wantErr)
	}
}

func assertNoWorkTokens(t *testing.T, h poisonSubmissionHarness, scenario string) {
	t.Helper()

	snap := h.Marking()
	if workTokens := countWorkTokens(snap); workTokens != 0 {
		t.Errorf("%s: expected 0 work tokens after rejected submission, got %d", scenario, workTokens)
	}
}

var (
	_ workers.WorkerExecutor = (*massiveGeneratedBatchExecutor)(nil)
	_ workers.WorkerExecutor = (*errorExecutor)(nil)
	_ workers.WorkerExecutor = (*panicWorkerExecutor)(nil)
)
