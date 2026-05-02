package functional_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
)

// TestConfigDriven_HappyPath validates a two-stage pipeline through the full
// service layer: BuildFactoryService → WorkstationExecutor → AgentExecutor →
// mock Provider. Work enters via a seed file picked up by preseed.
func TestConfigDriven_HappyPath(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Config-driven happy path"}`))

	// Both workers use stop_token: COMPLETE — include it in response Content.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Step one done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Step two done. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:failed").
		TokenCount(1)

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}

// TestConfigDriven_HappyPath_FailureRouting verifies that a provider error
// routes the token to the failed state via the config-driven on_failure field.
func TestConfigDriven_HappyPath_FailureRouting(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "happy_path"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will fail"}`))

	// Provider returns an error on the first call → AgentExecutor maps to OutcomeFailed.
	provider := testutil.NewMockProviderWithErrors(
		[]interfaces.InferenceResponse{{Content: ""}},
		[]error{fmt.Errorf("something went wrong")},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete").
		TokenCount(1)
}

// TestConfigDriven_RetryLoopBreaker validates a rejection loop with a guarded
// loop breaker. Reviewer responses omit "ACCEPTED" -> REJECTED, triggering
// on_rejection back to init. After 3 reviewer rejections the loop breaker
// fires -> task:failed.
func TestConfigDriven_RetryLoopBreaker(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will exhaust retries"}`))

	// Interleaved responses: processor (COMPLETE→accept), reviewer (no ACCEPTED→reject), ...
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 1 → ACCEPTED
		interfaces.InferenceResponse{Content: "Needs work"},          // reviewer 1 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 2 → ACCEPTED
		interfaces.InferenceResponse{Content: "Still needs work"},    // reviewer 2 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"}, // processor 3 -> ACCEPTED
		interfaces.InferenceResponse{Content: "Not good enough"},     // reviewer 3 -> REJECTED -> loop breaker
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	// Token should be in failed state due to the guarded loop breaker.
	h.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	// 6 provider calls total: 3 processor + 3 reviewer, interleaved.
	if provider.CallCount() != 6 {
		t.Errorf("expected provider called 6 times, got %d", provider.CallCount())
	}

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-exhaustion", "task:failed")
}

// TestConfigDriven_RetryLoopBreaker_SucceedsBeforeLimit verifies that if the
// reviewer accepts before the loop-breaker limit, the token completes normally.
func TestConfigDriven_RetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "retry_exhaustion"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "Will succeed on second try"}`))

	// Interleaved: processor accept, reviewer reject, processor accept, reviewer accept.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},  // processor 1 → ACCEPTED
		interfaces.InferenceResponse{Content: "Needs work"},           // reviewer 1 → REJECTED
		interfaces.InferenceResponse{Content: "Processed. COMPLETE"},  // processor 2 → ACCEPTED
		interfaces.InferenceResponse{Content: "Looks good. ACCEPTED"}, // reviewer 2 → ACCEPTED
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestConfigDriven_AddWorkType validates that multiple independent work types
// process correctly. Work enters via seed files for each type.
func TestConfigDriven_AddWorkType(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_work_type"))

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	// Both workers use stop_token: COMPLETE — provider responses include it.
	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Request handled. COMPLETE"},
		interfaces.InferenceResponse{Content: "Review handled. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("request:complete").
		HasTokenInPlace("review:complete").
		HasNoTokenInPlace("request:init").
		HasNoTokenInPlace("review:init").
		PlaceTokenCount("request:complete", 1).
		PlaceTokenCount("review:complete", 1)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}
}
