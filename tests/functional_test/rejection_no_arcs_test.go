package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestRejection_NoRejectionArcs_FailsToken verifies that when an executor
// returns OutcomeRejected and the transition has no RejectionArcs configured,
// the token is routed to the work type's FAILED state place.
func TestRejection_NoRejectionArcs_FailsToken(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProvider(rejectedProviderResponse("not good enough"))
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")
}

// TestRejection_NoRejectionArcs_ReleasesResources verifies that when a
// rejection with no rejection arcs fails the token, consumed resource tokens
// are released back to their resource places so they can be reused.
func TestRejection_NoRejectionArcs_ReleasesResources(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_no_arcs_resources"))

	// Submit both items upfront — first will be rejected (releasing resource),
	// second will succeed using the released resource.
	testutil.WriteSeedFile(t, dir, "task", []byte("first item"))
	testutil.WriteSeedFile(t, dir, "task", []byte("second item"))

	provider := testutil.NewMockProvider(
		rejectedProviderResponse("not good enough"),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	// First item should be failed, second should be done.
	h.Assert().
		PlaceTokenCount("task:failed", 1).
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")
}

// TestRejection_WithRejectionArcs_RoutesViaArcs verifies that when rejection
// arcs ARE configured, the token routes via those arcs (existing behavior).
func TestRejection_WithRejectionArcs_RoutesViaArcs(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_with_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(
		rejectedProviderResponse("needs work"),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// Run to completion: rejection routes back to init, then retry accepts.
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

// TestRejection_NoRejectionArcs_FailureRecordSet verifies that when a token
// is failed via the rejection fallback path, its history contains a failure
// record with the rejection context.
func TestRejection_NoRejectionArcs_FailureRecordSet(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_no_arcs"))

	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(rejectedProviderResponse("missing tests"))
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 5*time.Second)

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:failed" {
			if len(tok.History.FailureLog) == 0 {
				t.Error("expected FailureLog to be populated on token failed via rejection fallback")
			}
			// TotalVisits should have an entry for the transition.
			if tok.History.TotalVisits["process"] == 0 {
				t.Error("expected TotalVisits[process] > 0")
			}
			return
		}
	}
	t.Error("no token found in task:failed")
}
