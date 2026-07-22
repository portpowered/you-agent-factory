package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestArchiveTerminal_NoFurtherFiring(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "settings page"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Approved. COMPLETE"}},
	})

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 1 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want one terminal work", status.Categories)
	}

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

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Approved. COMPLETE"}, {Content: "Approved. COMPLETE"}},
	})

	status := runFactoryThroughCustomerProcess(t, dir, provider)
	if status.Categories.Terminal != 2 || status.Categories.Failed != 0 {
		t.Fatalf("status categories = %+v, want two terminal work items", status.Categories)
	}

	if provider.CallCount("swe") != 2 {
		t.Errorf("swe called unexpected number of times: expected 2, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("reviewer called unexpected number of times: expected 2, got %d", provider.CallCount("reviewer"))
	}
}
