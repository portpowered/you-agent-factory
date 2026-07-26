//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestCodeReviewLoop(t *testing.T) {
	support.SkipLongFunctional(t, "slow code-review retry loop smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte("implement feature X"))

	work := map[string][]workerexecution.InferenceResponse{
		"swe": {
			{Content: "code with missing error handling <COMPLETE>"},
			{Content: "code with proper error handling <COMPLETE>"},
		},
		"reviewer": {
			{Content: "missing error handling"},
			{Content: "looks good<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"code-change:complete": 1, "code-change:init": 0, "code-change:in-review": 0, "code-change:failed": 0,
	})

	if provider.CallCount("swe") != 2 {
		t.Errorf("expected swe called 2 times, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 2 {
		t.Errorf("expected reviewer called 2 times, got %d", provider.CallCount("reviewer"))
	}

	sweCalls := provider.Calls("swe")
	if len(sweCalls) < 2 {
		t.Fatalf("expected at least 2 swe calls, got %d", len(sweCalls))
	}
	secondDispatch := sweCalls[1]
	if len(secondDispatch.UserMessage) == 0 {
		t.Fatal("second coding dispatch has no input tokens")
	}
}
