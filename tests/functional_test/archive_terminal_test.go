package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestArchiveTerminal_NoFurtherFiring verifies that after reviewer approval
// the token transitions to the complete (terminal) state and no further
// transitions fire, even after additional ticks.
func TestArchiveTerminal_NoFurtherFiring(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "settings page"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Approved. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Token should be in terminal state.
	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:failed")

	// Verify workers were called exactly once each (no extra firings).
	if provider.CallCount("swe") != 1 {
		t.Errorf("swe called unexpected number of times: expected 1, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 1 {
		t.Errorf("reviewer called unexpected number of times: expected 1, got %d", provider.CallCount("reviewer"))
	}
}

// TestArchiveTerminal_MultipleTokensAllTerminate verifies that when multiple
// work items are submitted, all reach terminal state and none fire further.
func TestArchiveTerminal_MultipleTokensAllTerminate(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "A"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "B"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Approved. COMPLETE"}, {Content: "Approved. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both tokens should be complete.
	h.Assert().
		PlaceTokenCount("code-change:complete", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")

	// Verify workers were called exactly twice each (once per token, no extra firings).
	if provider.CallCount("swe") != 2 {
		t.Errorf("swe called unexpected number of times: expected 2, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("reviewer called unexpected number of times: expected 2, got %d", provider.CallCount("reviewer"))
	}
}
