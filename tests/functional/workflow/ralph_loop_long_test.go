//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRalphLoop_IteratesOnRejectionThenConverges(t *testing.T) {
	support.SkipLongFunctional(t, "slow ralph rejection-iteration convergence sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "iterate and converge"}`))

	work := map[string][]interfaces.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "missing error handling"},
			{Content: "missing error handling"},
			{Content: "code with missing error handling <COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("executor-worker") != 3 {
		t.Errorf("expected executor called 3 times, got %d", provider.CallCount("executor-worker"))
	}
	if provider.CallCount("reviewer-worker") != 3 {
		t.Errorf("expected reviewer called 3 times, got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:complete", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:failed")
}

func TestRalphLoop_GuardedReviewLoopBreakerTerminatesInfiniteLoop(t *testing.T) {
	support.SkipLongFunctional(t, "slow ralph guarded review loop-breaker sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "infinite loop test"}`))

	work := map[string][]interfaces.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "missing error handling"},
			{Content: "missing error handling"},
			{Content: "missing error handling"},
			{Content: "missing error handling"},
			{Content: "missing error handling"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("reviewer-worker") != 3 {
		t.Errorf("expected reviewer called exactly 3 times (max_visits), got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:failed", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "reviewer-loop-breaker", "story:failed")
}
