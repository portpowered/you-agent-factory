package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestArchiveTerminal_NoFurtherFiring(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
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

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review").
		HasNoTokenInPlace("code-change:failed")

	if provider.CallCount("swe") != 1 {
		t.Errorf("swe called unexpected number of times: expected 1, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 1 {
		t.Errorf("reviewer called unexpected number of times: expected 1, got %d", provider.CallCount("reviewer"))
	}
}

func TestArchiveTerminal_MultipleTokensAllTerminate(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
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

	h.Assert().
		PlaceTokenCount("code-change:complete", 2).
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:in-review")

	if provider.CallCount("swe") != 2 {
		t.Errorf("swe called unexpected number of times: expected 2, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("reviewer called unexpected number of times: expected 2, got %d", provider.CallCount("reviewer"))
	}
}
