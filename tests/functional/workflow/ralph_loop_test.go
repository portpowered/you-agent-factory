package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRalphLoop_ConvergesOnReviewerAccept(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "ralph_loop"))

	testutil.WriteSeedFile(t, dir, "story", []byte(`{"title": "implement feature"}`))

	work := map[string][]interfaces.InferenceResponse{
		"executor-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
		},
		"reviewer-worker": {
			{Content: "code with missing error handling <COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithProvider(provider),
	)

	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("executor-worker") != 1 {
		t.Errorf("expected executor called 1 time, got %d", provider.CallCount("executor-worker"))
	}
	if provider.CallCount("reviewer-worker") != 1 {
		t.Errorf("expected reviewer called 1 time, got %d", provider.CallCount("reviewer-worker"))
	}

	h.Assert().
		PlaceTokenCount("story:complete", 1).
		HasNoTokenInPlace("story:init").
		HasNoTokenInPlace("story:failed")
}
