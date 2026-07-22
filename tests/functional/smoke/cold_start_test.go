package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestColdStart_PreSeededTokensProcessed(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "auth"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "logging"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "metrics"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
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

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 3 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want three terminal work items", status.Categories)
	}

	if provider.CallCount("swe") != 3 {
		t.Errorf("expected swe called 3 times, got %d", provider.CallCount("swe"))
	}
}

func TestColdStart_MixedPreSeededAndLateSubmit(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "pre-existing"}`))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "new-arrival"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 2 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want two terminal work items", status.Categories)
	}
	if provider.CallCount("swe") != 2 {
		t.Errorf("expected swe called 2 times, got %d", provider.CallCount("swe"))
	}
}
