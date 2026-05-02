package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestColdStart_PreSeededTokensProcessed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

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
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("code-change:complete", 3).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")

	if provider.CallCount("swe") != 3 {
		t.Errorf("expected swe called 3 times, got %d", provider.CallCount("swe"))
	}
}

func TestColdStart_MixedPreSeededAndLateSubmit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "pre-existing"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "new-arrival"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("code-change:complete", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")
}
