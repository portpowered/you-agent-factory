package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestReviewRetryLoopBreaker_TerminatesAfterMaxRetries(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("missing tests"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("still no tests"),
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("tests still missing"),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 3 {
		t.Errorf("expected swe called 3 times, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 3 {
		t.Errorf("expected reviewer called 3 times, got %d", got)
	}

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

func TestReviewRetryLoopBreaker_FeedbackPropagated(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
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

	firstColor := support.FirstInputToken(calls[0].InputTokens).Color
	if _, ok := firstColor.Tags["_rejection_feedback"]; ok {
		t.Error("first swe dispatch should not have _rejection_feedback tag")
	}

	secondColor := support.FirstInputToken(calls[1].InputTokens).Color
	if fb := secondColor.Tags["_rejection_feedback"]; fb != "add unit tests" {
		t.Errorf("second dispatch: expected feedback %q, got %q", "add unit tests", fb)
	}

	thirdColor := support.FirstInputToken(calls[2].InputTokens).Color
	if fb := thirdColor.Tags["_rejection_feedback"]; fb != "tests incomplete" {
		t.Errorf("third dispatch: expected feedback %q, got %q", "tests incomplete", fb)
	}
}

func TestReviewRetryLoopBreaker_SucceedsBeforeLimit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "review_retry_exhaustion"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "login"}`))
	provider := testutil.NewMockProvider(
		support.AcceptedProviderResponse(),
		support.RejectedProviderResponse("needs work"),
		support.AcceptedProviderResponse(),
		support.AcceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if got := len(support.ProviderCallsForWorker(provider, "swe")); got != 2 {
		t.Errorf("expected swe called 2 times, got %d", got)
	}
	if got := len(support.ProviderCallsForWorker(provider, "reviewer")); got != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", got)
	}

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:failed").
		HasNoTokenInPlace("code-change:init")
}
