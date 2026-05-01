package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries verifies that the
// review -> reject -> re-execute loop terminates after max_visits=3 rejections
// via the guarded LOGICAL_MOVE loop breaker, and the token ends up in failed state.
func TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		rejectedProviderResponse("missing tests"),
		acceptedProviderResponse(),
		rejectedProviderResponse("still no tests"),
		acceptedProviderResponse(),
		rejectedProviderResponse("tests still missing"),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Verify the loop ran exactly 3 times.
	if got := len(providerCallsForWorker(provider, "swe")); got != 3 {
		t.Errorf("expected swe called 3 times, got %d", got)
	}
	if got := len(providerCallsForWorker(provider, "reviewer")); got != 3 {
		t.Errorf("expected reviewer called 3 times, got %d", got)
	}

	// Token should be in failed state after the guarded loop breaker fires.
	h.Assert().
		HasTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-exhaustion", "code-change:failed")
}

// TestReviewRetryLoopBreaker_FeedbackPropagated verifies that rejection feedback
// from the reviewer is propagated to the executor on each retry iteration via
// the _rejection_feedback tag on the input token.
func TestReviewRetryLoopBreaker_FeedbackPropagated(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	h := testutil.NewServiceTestHarness(t, dir)

	sweMock := h.MockWorker("swe",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.MockWorker("reviewer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "add unit tests"},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "tests incomplete"},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "coverage too low"},
	)

	h.RunUntilComplete(t, 10*time.Second)

	calls := sweMock.Calls()
	if len(calls) != 3 {
		t.Fatalf("expected 3 swe calls, got %d", len(calls))
	}

	// First dispatch should have no rejection feedback.
	firstColor := firstInputToken(calls[0].InputTokens).Color
	if _, ok := firstColor.Tags["_rejection_feedback"]; ok {
		t.Error("first swe dispatch should not have _rejection_feedback tag")
	}

	// Second dispatch should carry first rejection feedback.
	secondColor := firstInputToken(calls[1].InputTokens).Color
	if fb := secondColor.Tags["_rejection_feedback"]; fb != "add unit tests" {
		t.Errorf("second dispatch: expected feedback %q, got %q", "add unit tests", fb)
	}

	// Third dispatch should carry second rejection feedback.
	thirdColor := firstInputToken(calls[2].InputTokens).Color
	if fb := thirdColor.Tags["_rejection_feedback"]; fb != "tests incomplete" {
		t.Errorf("third dispatch: expected feedback %q, got %q", "tests incomplete", fb)
	}
}

// TestReviewRetryLoopBreaker_SucceedsBeforeLimit verifies that if the reviewer
// approves before the guarded loop-breaker limit, the token completes normally
// and does not reach failed state.
func TestReviewRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "login"}`))
	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		rejectedProviderResponse("needs work"),
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// SWE called twice (initial + after rejection), reviewer called twice.
	if got := len(providerCallsForWorker(provider, "swe")); got != 2 {
		t.Errorf("expected swe called 2 times, got %d", got)
	}
	if got := len(providerCallsForWorker(provider, "reviewer")); got != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", got)
	}

	// Token should be in complete state.
	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:init")
}
