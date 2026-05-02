package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestColdStart_PreSeededTokensProcessed verifies that tokens pre-seeded via
// seed files are picked up and processed on startup. This confirms cold-start
// work discovery parity with the legacy dispatcher.
func TestColdStart_PreSeededTokensProcessed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	// Pre-seed 3 tokens as seed files -- simulates cold start with
	// existing work items discovered on startup.
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "auth"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "logging"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "metrics"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// RunUntilComplete injects and processes the pre-seeded tokens.
	h.RunUntilComplete(t, 10*time.Second)

	// All 3 should reach terminal state.
	h.Assert().
		PlaceTokenCount("code-change:complete", 3).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")

	if provider.CallCount("swe") != 3 {
		t.Errorf("expected swe called 3 times, got %d", provider.CallCount("swe"))
	}
}

// TestColdStart_SingleTokenReachesTerminal verifies a single pre-seeded
// token completes the full pipeline on cold start.
func TestColdStart_SingleTokenReachesTerminal(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "fix-bug"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe": {
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// RunUntilComplete injects and processes the pre-seeded token.
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		TokenCount(1)

	if provider.CallCount("swe") != 1 {
		t.Errorf("expected swe called once, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called once, got %d", provider.CallCount("reviewer"))
	}
}

// TestColdStart_MixedPreSeededAndLateSubmit verifies that multiple pre-seeded
// tokens all process correctly on cold start.
func TestColdStart_MixedPreSeededAndLateSubmit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	// Pre-seed two tokens (both discovered on startup).
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "pre-existing"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "new-arrival"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
		"reviewer": {
			{Content: "Done. COMPLETE"},
			{Content: "Done. COMPLETE"},
		},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	// Both should complete.
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("code-change:complete", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")
}
