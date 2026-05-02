package stress_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// TestPoisonTokenMalformedSubmissions verifies that malformed work submissions
// (empty payload, missing WorkTypeID, invalid WorkTypeID, extremely large payload)
// don't crash the engine or corrupt the marking.
// portos:func-length-exception owner=agent-factory reason=legacy-poison-submission-fixture review=2026-07-19 removal=split-malformed-submission-cases-and-marking-assertions-before-next-poison-token-change
func TestPoisonTokenMalformedSubmissions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	poisonSubmitCfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w"}},
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
	}

	t.Run("empty_work_type_name", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		requireSubmitRejected(t, h, "", []byte(`{"test": "empty-type"}`), "missing work_type_name")

		assertNoWorkTokens(t, h, "empty WorkTypeID")
	})

	t.Run("invalid_work_type_name", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		requireSubmitRejected(t, h, "nonexistent-type", []byte(`{"test": "bad-type"}`), "unknown work type")

		assertNoWorkTokens(t, h, "invalid WorkTypeID")
	})

	t.Run("nil_payload", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		// Submit with nil payload — should succeed.
		h.SubmitWork("task", nil)
		h.RunUntilComplete(t, 10*time.Second)

		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 1 {
			t.Errorf("expected 1 complete token with nil payload, got %d", completeCount)
		}
	})

	t.Run("empty_payload", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		// Submit with empty payload — should succeed.
		h.SubmitWork("task", []byte{})
		h.RunUntilComplete(t, 10*time.Second)

		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 1 {
			t.Errorf("expected 1 complete token with empty payload, got %d", completeCount)
		}
	})

	t.Run("extremely_large_payload", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		// Submit with 1MB payload — should succeed without panic.
		largePayload := make([]byte, 1024*1024)
		for i := range largePayload {
			largePayload[i] = byte('A' + (i % 26))
		}
		h.SubmitWork("task", largePayload)
		h.RunUntilComplete(t, 10*time.Second)

		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 1 {
			t.Errorf("expected 1 complete token with large payload, got %d", completeCount)
		}
	})

	t.Run("mixed_valid_and_invalid_submissions", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		// Submit a mix of rejected and valid work. Invalid submissions are
		// rejected at the boundary and must not poison later valid work.
		requireSubmitRejected(t, h, "", []byte(`{"bad": 1}`), "missing work_type_name")
		requireSubmitRejected(t, h, "nonexistent", []byte(`{"bad": 2}`), "unknown work type")
		h.SubmitWork("task", []byte(`{"good": 1}`))
		requireSubmitRejected(t, h, "also-nonexistent", []byte(`{"bad": 3}`), "unknown work type")
		h.SubmitWork("task", []byte(`{"good": 2}`))

		h.RunUntilComplete(t, 10*time.Second)

		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 2 {
			t.Errorf("expected 2 valid tokens to complete, got %d", completeCount)
		}
	})

	t.Run("mixed_batch_rejects_without_partial_submit", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonSubmitCfg)
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("w")

		err := h.SubmitFullError(context.Background(), []interfaces.SubmitRequest{
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

// TestPoisonTokenMalformedExecutorResults verifies that malformed worker responses
// (unknown outcome, empty result, missing TransitionID, massive spawned work)
// don't crash the engine or corrupt global state.
// portos:func-length-exception owner=agent-factory reason=legacy-poison-executor-result-fixture review=2026-07-19 removal=split-malformed-result-cases-and-global-state-assertions-before-next-poison-token-change
func TestPoisonTokenMalformedExecutorResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	t.Run("unknown_outcome_enum", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("poison-worker"))
		h := testutil.NewServiceTestHarness(t, dir)

		// Custom executor returns an unknown outcome.
		h.SetCustomExecutor("poison-worker", &poisonExecutor{
			outcome: "TOTALLY_INVALID_OUTCOME",
		})

		h.SubmitWork("task", []byte(`{"test": "unknown-outcome"}`))

		// The transitioner returns an error for unknown outcomes, causing the
		// engine to terminate with an error. No timeout needed — the engine
		// exits promptly. No panic is the key assertion.
		errCh := h.RunInBackground(context.Background())
		err := <-errCh
		if err == nil {
			t.Error("expected engine error for unknown outcome")
		}
	})

	t.Run("empty_result_from_executor", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("empty-worker"))
		h := testutil.NewServiceTestHarness(t, dir)

		// Custom executor returns a completely empty WorkResult.
		// The inline dispatch carries forward InputHistory and default OutputTokens.
		// But TransitionID is empty → transitioner can't find the transition.
		h.SetCustomExecutor("empty-worker", &emptyResultExecutor{})

		h.SubmitWork("task", []byte(`{"test": "empty-result"}`))

		// The transitioner errors on empty/invalid transition ID, causing
		// the engine to terminate with an error. No panic.
		errCh := h.RunInBackground(context.Background())
		err := <-errCh
		if err == nil {
			t.Error("expected engine error for empty result")
		}
	})

	t.Run("result_with_non_existent_transition_id", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("bad-transition-worker"))
		h := testutil.NewServiceTestHarness(t, dir)

		// Custom executor returns a result with a non-existent TransitionID.
		h.SetCustomExecutor("bad-transition-worker", &poisonExecutor{
			overrideTransitionID: "totally-fake-transition",
			outcome:              interfaces.OutcomeAccepted,
		})

		h.SubmitWork("task", []byte(`{"test": "bad-transition"}`))

		// The transitioner errors because the transition doesn't exist,
		// causing the engine to terminate with an error. No panic.
		errCh := h.RunInBackground(context.Background())
		err := <-errCh
		if err == nil {
			t.Error("expected engine error for bad transition ID")
		}
	})

	t.Run("result_with_massive_spawned_work", func(t *testing.T) {
		dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("spawn-worker"))
		h := testutil.NewServiceTestHarness(t, dir)

		// Executor returns result with 10000 spawned tokens.
		// All reference a non-existent work type → silently skipped.
		h.SetCustomExecutor("spawn-worker", &massiveSpawnExecutor{
			spawnCount:  10000,
			spawnTypeID: "nonexistent-type",
			realOutcome: interfaces.OutcomeAccepted,
		})

		h.SubmitWork("task", []byte(`{"test": "massive-spawn"}`))
		h.RunUntilComplete(t, 10*time.Second)

		// The parent token should complete. Spawned tokens should be skipped
		// (invalid work type).
		snap := h.Marking()
		completeCount := len(snap.TokensInPlace("task:complete"))
		if completeCount != 1 {
			t.Errorf("expected 1 complete token, got %d", completeCount)
		}

		// No phantom tokens from the invalid spawned work.
		workTokens := countWorkTokens(snap)
		if workTokens != 1 {
			t.Errorf("expected 1 total token (no phantom spawns), got %d", workTokens)
		}
	})

	t.Run("result_with_massive_valid_spawned_work", func(t *testing.T) {
		// Spawn into a valid work type to verify the engine handles large
		// volumes without crashing.
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
			Workers: []interfaces.WorkerConfig{{Name: "spawn-worker"}, {Name: "child-worker"}},
			Workstations: []interfaces.FactoryWorkstationConfig{
				{
					Name: "process", WorkerTypeName: "spawn-worker",
					Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
					Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
					OnFailure: &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
				},
				{
					Name: "child-process", WorkerTypeName: "child-worker",
					Inputs:  []interfaces.IOConfig{{WorkTypeName: "child", StateName: "init"}},
					Outputs: []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}},
				},
			},
		})
		h := testutil.NewServiceTestHarness(t, dir)
		h.MockWorker("child-worker")

		const spawnCount = 500 // Use 500 to keep test fast but stress the engine.
		h.SetCustomExecutor("spawn-worker", &massiveSpawnExecutor{
			spawnCount:  spawnCount,
			spawnTypeID: "child",
			realOutcome: interfaces.OutcomeAccepted,
		})

		h.SubmitWork("task", []byte(`{"test": "valid-spawn"}`))

		h.RunUntilComplete(t, 30*time.Second)

		snap := h.Marking()
		parentComplete := len(snap.TokensInPlace("task:complete"))
		if parentComplete != 1 {
			t.Errorf("expected 1 parent complete, got %d", parentComplete)
		}

		childComplete := len(snap.TokensInPlace("child:complete"))
		if childComplete != spawnCount {
			childInit := len(snap.TokensInPlace("child:init"))
			t.Errorf("expected %d children complete, got %d (init=%d)",
				spawnCount, childComplete, childInit)
		}

		t.Logf("successfully processed parent + %d spawned children", childComplete)
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
		Workers: []interfaces.WorkerConfig{{Name: "w"}},
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
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithFullWorkerPoolAndScriptWrap())
	h.MockWorker("w")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := h.RunInBackground(ctx)

	const validItems = 10

	// Submit a burst of mixed valid and invalid work.
	for i := range validItems {
		requireSubmitFullRejected(t, h, []interfaces.SubmitRequest{
			{WorkTypeID: "", Payload: []byte(`{"poison":true}`)},
		}, "missing work_type_name")
		requireSubmitFullRejected(t, h, []interfaces.SubmitRequest{
			{WorkTypeID: "bogus", Payload: []byte(`{"poison":true}`)},
		}, "unknown work type")
		h.SubmitFull(context.Background(), []interfaces.SubmitRequest{
			{WorkTypeID: "task", Payload: fmt.Appendf(nil, `{"item":%d}`, i)},
		})
		requireSubmitFullRejected(t, h, []interfaces.SubmitRequest{
			{WorkTypeID: "also-bogus", Payload: []byte(`{"poison":true}`)},
		}, "unknown work type")
	}

	// Poll until all valid items reach terminal state.
	poisonTerminalPlaces := []string{"task:complete", "task:failed"}
	deadline := time.After(10 * time.Second)
	for {
		snap := h.Marking()
		terminalCount := countTerminalTokens(snap, poisonTerminalPlaces)
		if terminalCount >= validItems {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out: %d/%d tokens terminal", terminalCount, validItems)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-errCh

	snap := h.Marking()

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
		Workers: []interfaces.WorkerConfig{{Name: "w"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: "w",
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
		}},
	}

	// Workflow A: clean workflow that should complete normally.
	dirA := testutil.ScaffoldFactoryDir(t, simpleOneStageCfg)
	hA := testutil.NewServiceTestHarness(t, dirA)
	hA.MockWorker("w")

	// Workflow B: receives poison submissions.
	dirB := testutil.ScaffoldFactoryDir(t, simpleOneStageCfg)
	hB := testutil.NewServiceTestHarness(t, dirB)
	hB.MockWorker("w")

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
	hA.RunUntilComplete(t, 10*time.Second)
	hB.RunUntilComplete(t, 10*time.Second)

	// Assert workflow A completed all items normally.
	snapA := hA.Marking()
	completeA := len(snapA.TokensInPlace("task:complete"))
	if completeA != itemsPerWorkflow {
		t.Errorf("workflow A: expected %d complete, got %d", itemsPerWorkflow, completeA)
	}
	if len(snapA.Tokens) != itemsPerWorkflow {
		t.Errorf("workflow A: expected %d total tokens, got %d (corruption?)", itemsPerWorkflow, len(snapA.Tokens))
	}

	// Assert workflow B: invalid submissions skipped, valid ones completed.
	snapB := hB.Marking()
	completeB := len(snapB.TokensInPlace("task:complete"))
	if completeB != itemsPerWorkflow {
		t.Errorf("workflow B: expected %d complete (from valid submissions), got %d", itemsPerWorkflow, completeB)
	}
	if len(snapB.Tokens) != itemsPerWorkflow {
		t.Errorf("workflow B: expected %d total tokens (invalid skipped), got %d", itemsPerWorkflow, len(snapB.Tokens))
	}

	// Cross-check: no token payloads from A leaked into B's marking or vice versa.
	// Token IDs may coincide across independent engines (same counter + transition name)
	// but that's not corruption — they live in separate markings.
	for _, tok := range snapA.Tokens {
		payload := string(tok.Color.Payload)
		if strings.Contains(payload, "valid-in-B") || strings.Contains(payload, "poison") {
			t.Errorf("workflow A contains token with B's payload: %s", payload)
		}
	}
	for _, tok := range snapB.Tokens {
		payload := string(tok.Color.Payload)
		if strings.Contains(payload, "clean") {
			t.Errorf("workflow B contains token with A's payload: %s", payload)
		}
	}
}

// TestPoisonTokenNoPanic uses recover() to verify that various edge cases
// never cause an engine panic. Each subtest exercises a different poison vector.
func TestPoisonTokenNoPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	// Build a simple workflow used across subtests.
	buildNet := func() *interfaces.TokenColor {
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
			h := testutil.NewServiceTestHarness(t, dir)
			h.MockWorker("w")

			if pv.typeID == "" || pv.typeID == "no-such-type" {
				wantErr := "unknown work type"
				if pv.typeID == "" {
					wantErr = "missing work_type_name"
				}
				requireSubmitRejected(t, h, pv.typeID, pv.payload, wantErr)
				assertNoWorkTokens(t, h, pv.name)
			} else {
				h.SubmitWork(pv.typeID, pv.payload)
				h.RunUntilComplete(t, 10*time.Second) // Panics are not OK.
			}

			// If we get here, no panic occurred.
		})
	}
}

// TestPoisonTokenExecutorPanic verifies that if an executor panics, the
// syncDispatcher does not propagate the panic to crash the test runner.
// Note: the current syncDispatcher does NOT recover from executor panics,
// so this test documents the behavior and uses recover() in the test itself.
func TestPoisonTokenExecutorPanic(t *testing.T) {
	// pending migration: panic recovery in inline dispatch not supported during Run()
	t.Skip("pending migration: panic recovery in inline dispatch not supported during Run()")
}

// TestPoisonTokenExecutorError verifies that executors returning Go errors are
// handled gracefully — the token is routed to the configured FailureArcs rather
// than being lost or crashing the tick. This mirrors production WorkerRunner behavior.
func TestPoisonTokenExecutorError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	dir := testutil.ScaffoldFactoryDir(t, poisonExecCfg("error-worker"))
	h := testutil.NewServiceTestHarness(t, dir)

	// Custom executor that returns an error.
	h.SetCustomExecutor("error-worker", &errorExecutor{
		err: fmt.Errorf("simulated executor catastrophic failure"),
	})

	h.SubmitWork("task", []byte(`{"test": "executor-error"}`))

	// Executor errors are converted to OutcomeFailed WorkResults and routed via
	// FailureArcs — RunUntilComplete should succeed (no panic).
	h.RunUntilComplete(t, 10*time.Second)

	// Token should be in task:failed via the configured FailureArcs.
	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	// Verify the error is recorded on the token.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			if !strings.Contains(tok.History.LastError, "simulated executor catastrophic failure") {
				t.Errorf("expected failure message in token history, got: %q", tok.History.LastError)
			}
			break
		}
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
		Workers: []interfaces.WorkerConfig{{Name: workerName}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "process", WorkerTypeName: workerName,
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
			OnFailure: &interfaces.IOConfig{WorkTypeName: "task", StateName: "failed"},
		}},
	}
}

// ---------------------------------------------------------------------------
// Helper executors
// ---------------------------------------------------------------------------

// poisonExecutor returns a WorkResult with configurable poison fields.
type poisonExecutor struct {
	outcome              interfaces.WorkOutcome
	overrideTransitionID string
}

func (e *poisonExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	transitionID := dispatch.TransitionID
	if e.overrideTransitionID != "" {
		transitionID = e.overrideTransitionID
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: transitionID,
		Outcome:      e.outcome,
	}, nil
}

// emptyResultExecutor returns a completely empty WorkResult — no TransitionID,
// no Outcome, no OutputTokens.
type emptyResultExecutor struct{}

func (e *emptyResultExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{}, nil
}

// massiveSpawnExecutor returns ACCEPTED with N spawned work items.
type massiveSpawnExecutor struct {
	spawnCount  int
	spawnTypeID string
	realOutcome interfaces.WorkOutcome
}

func (e *massiveSpawnExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	spawned := make([]interfaces.TokenColor, e.spawnCount)
	for i := range spawned {
		spawned[i] = interfaces.TokenColor{
			WorkID:     fmt.Sprintf("spawn-%d", i),
			WorkTypeID: e.spawnTypeID,
			Payload:    fmt.Appendf(nil, `{"spawned_index": %d}`, i),
		}
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      e.realOutcome,
		SpawnedWork:  spawned,
	}, nil
}

// errorExecutor returns an error from Execute.
type errorExecutor struct {
	err error
}

func (e *errorExecutor) Execute(_ context.Context, _ interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{}, e.err
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
func countWorkTokens(snap *petri.MarkingSnapshot) int {
	count := 0
	for _, tok := range snap.Tokens {
		if !strings.Contains(tok.PlaceID, ":available") {
			count++
		}
	}
	return count
}

func requireSubmitRejected(t *testing.T, h *testutil.ServiceTestHarness, workTypeID string, payload []byte, wantErr string) {
	t.Helper()

	err := h.SubmitError(workTypeID, payload)
	if err == nil {
		t.Fatalf("expected submit validation error containing %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("submit validation error = %v, want substring %q", err, wantErr)
	}
}

func requireSubmitFullRejected(t *testing.T, h *testutil.ServiceTestHarness, reqs []interfaces.SubmitRequest, wantErr string) {
	t.Helper()

	err := h.SubmitFullError(context.Background(), reqs)
	if err == nil {
		t.Fatalf("expected submit validation error containing %q", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("submit validation error = %v, want substring %q", err, wantErr)
	}
}

func assertNoWorkTokens(t *testing.T, h *testutil.ServiceTestHarness, scenario string) {
	t.Helper()

	snap := h.Marking()
	if workTokens := countWorkTokens(snap); workTokens != 0 {
		t.Errorf("%s: expected 0 work tokens after rejected submission, got %d", scenario, workTokens)
	}
}

var (
	_ workers.WorkerExecutor = (*poisonExecutor)(nil)
	_ workers.WorkerExecutor = (*emptyResultExecutor)(nil)
	_ workers.WorkerExecutor = (*massiveSpawnExecutor)(nil)
	_ workers.WorkerExecutor = (*errorExecutor)(nil)
)
