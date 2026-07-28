//go:build functionallong

package review

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCodeReviewLoop proves the code_review factory workflow completes after a
// reject-then-accept review loop, propagates reviewer feedback into the second
// coding dispatch, and leaves exactly one successful terminal code-change Work.
func TestCodeReviewLoop(t *testing.T) {
	support.SkipLongFunctional(t, "slow code-review retry loop smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte("implement feature X"))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with proper error handling <COMPLETE>"},
		},
		"reviewer": {
			{Content: "missing error handling"},
			{Content: "looks good<COMPLETE>"},
		},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)

	terminal := support.WorkCustomerLocation("code-change", "complete")
	assertCodeReviewLoopWorkAtCustomerStates(t, listed, map[string]int{
		terminal: 1,
		support.WorkCustomerLocation("code-change", "init"):      0,
		support.WorkCustomerLocation("code-change", "in-review"): 0,
		support.WorkCustomerLocation("code-change", "failed"):    0,
	})

	if provider.CallCount("swe") != 2 {
		t.Errorf("swe call count = %d, want 2", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("reviewer call count = %d, want 2", provider.CallCount("reviewer"))
	}

	sweCalls := provider.Calls("swe")
	if len(sweCalls) < 2 {
		t.Fatalf("swe calls = %d, want at least 2", len(sweCalls))
	}
	if len(sweCalls[1].UserMessage) == 0 {
		t.Fatal("second coding dispatch has no input tokens")
	}
}

func assertCodeReviewLoopWorkAtCustomerStates(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Errorf("%s Work count = %d, want %d", location, got, want)
		}
	}
}
