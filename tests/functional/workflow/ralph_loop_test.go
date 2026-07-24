package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestRalphLoop_ConvergesOnReviewerAccept(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "implement feature"}`))

	work := map[string][]workerexecution.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if provider.CallCount("executor-worker") != 1 {
		t.Errorf("expected executor called 1 time, got %d", provider.CallCount("executor-worker"))
	}
	if provider.CallCount("reviewer-worker") != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", provider.CallCount("reviewer-worker"))
	}

	assertWorkflowSessionPlaces(t, listed, map[string]int{"story:complete": 1, "story:init": 0, "story:failed": 0})
}
