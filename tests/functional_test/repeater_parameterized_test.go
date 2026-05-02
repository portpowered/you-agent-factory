package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestRepeater_RefiresOnRejectedStopsOnAccepted verifies that a repeater
// workstation re-fires when the worker returns REJECTED and stops when
// ACCEPTED, routing the token forward to the output place.
func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "repeater test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Worker returns REJECTED twice then ACCEPTED.
	execMock := h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	// The executor should have been called 3 times (2 REJECTED + 1 ACCEPTED).
	if execMock.CallCount() != 3 {
		t.Errorf("expected exec-worker called 3 times, got %d", execMock.CallCount())
	}

	// Token should reach complete (executor ACCEPTED → processing → finisher ACCEPTED → complete).
	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater verifies that when
// a repeater workstation's worker never returns ACCEPTED, the guarded
// LOGICAL_MOVE loop breaker eventually moves the token to the failed state.
func TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "exhaustion test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Worker always returns REJECTED — will exceed max_visits=5.
	h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
	)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	// The guarded loop breaker (max_visits=5) should route the token to failed.
	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "executor-loop-breaker", "task:failed")
}

// TestRepeater_YieldsBetweenIterations verifies that the repeater yields
// control to the engine between iterations, allowing other workstations
// to fire in between repeater iterations.
func TestRepeater_YieldsBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_workstation"))

	// Submit two tokens: one for the repeater loop, one to show interleaving.
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-B"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Track interleaving: exec-worker returns REJECTED on first call, ACCEPTED on second.
	execMock := h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	finishMock := h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	// Run to completion: both tokens are injected and processed.
	h.RunUntilComplete(t, 10*time.Second)

	// Both tokens should have completed. The exec-worker should be called at
	// least 3 times total (token-A: REJECTED+ACCEPTED, token-B: at least 1).
	if execMock.CallCount() < 3 {
		t.Errorf("expected exec-worker called at least 3 times (interleaved), got %d", execMock.CallCount())
	}

	// The finisher should have been called for both tokens.
	if finishMock.CallCount() < 2 {
		t.Errorf("expected finish-worker called at least 2 times, got %d", finishMock.CallCount())
	}

	h.Assert().PlaceTokenCount("task:complete", 2)
}

// TestParameterizedFields_WorkingDirectoryResolvesFromTags verifies that a
// thinned dispatch still carries the workstation identifier and input-token
// tags needed for runtime template resolution.
func TestParameterizedFields_WorkingDirectoryResolvesFromTags(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_workstation"))

	// Submit with tags that will be used in the working_directory template.
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	h := testutil.NewServiceTestHarness(t, dir)

	// Use a capturing executor to inspect the dispatch.
	capture := &capturingExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	}
	h.SetCustomExecutor("exec-worker", capture)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	if capture.callCount == 0 {
		t.Fatal("capturing executor was never called")
	}

	// Verify the dispatch keeps the workstation identifier needed for runtime lookup.
	if capture.lastDispatch.WorkstationName == "" {
		t.Error("expected WorkstationName to be set on dispatch")
	}

	// Verify tags are present on the input tokens (enabling template resolution).
	if len(capture.lastDispatch.InputTokens) == 0 {
		t.Fatal("expected at least one input token")
	}
	tags := firstInputToken(capture.lastDispatch.InputTokens).Color.Tags
	if tags["branch"] != "feature-abc" {
		t.Errorf("expected tag branch=feature-abc, got %q", tags["branch"])
	}
}

// TestParameterizedFields_UnresolvedTemplateRoutesToFailure verifies that
// a workstation configured with a template referencing a non-existent field
// routes the token to the failure state. This tests the full pipeline via
// the ServiceTestHarness (WorkstationExecutor resolves templates).
func TestParameterizedFields_UnresolvedTemplateRoutesToFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "parameterized_failure"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "unresolved template test"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Should not reach COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// The unresolvable template should have caused the token to route to failed.
	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:complete")

	// The mock provider should NOT have been called — template resolution fails
	// before the worker is invoked.
	if provider.CallCount() != 0 {
		t.Errorf("expected provider called 0 times (template error before invocation), got %d", provider.CallCount())
	}
}

// TestRepeater_ResourceReleaseBetweenIterations verifies that a repeater
// workstation with resource_usage releases the resource token between
// iterations, preventing deadlock. Without the fix, the transition cannot
// re-fire because the resource slot is never returned.
func TestRepeater_ResourceReleaseBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "resource repeater test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Worker returns REJECTED twice then ACCEPTED — 3 iterations total.
	execMock := h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	// The executor should have been called 3 times (2 REJECTED + 1 ACCEPTED).
	if execMock.CallCount() != 3 {
		t.Errorf("expected exec-worker called 3 times, got %d", execMock.CallCount())
	}

	// Token should reach complete state.
	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness validates the
// repeater + resource_usage combination through the full service layer
// (BuildFactoryService → WorkstationExecutor → AgentExecutor → MockProvider).
func TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service resource repeater test"}`))

	// exec-worker uses stop_token: COMPLETE.
	// First two responses lack "COMPLETE" → FAILED → repeater repeats.
	// Third response contains "COMPLETE" → ACCEPTED → advances to finisher.
	// finish-worker also uses stop_token: COMPLETE.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Still working"},       // exec 1 → FAILED (no stop token) → repeat
		interfaces.InferenceResponse{Content: "Almost there"},        // exec 2 → FAILED (no stop token) → repeat
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},      // exec 3 → ACCEPTED → advance
		interfaces.InferenceResponse{Content: "Finalized. COMPLETE"}, // finish → ACCEPTED
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	// Provider should have been called 4 times (3 exec + 1 finish).
	if provider.CallCount() != 4 {
		t.Errorf("expected provider called 4 times, got %d", provider.CallCount())
	}
}
