//go:build functionallong

package repeater

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestRepeater_RefiresOnRejectedStopsOnAccepted proves that a repeater
// workstation refires Work while worker outputs omit the accept stop signal
// and stops with Work in a completed public state once an accepting output
// arrives, leaving no remaining init Work.
func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater rejection-to-acceptance sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "repeater test"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"exec-worker":   {{Content: "retry"}, {Content: "retry"}, {Content: "done COMPLETE"}},
		"finish-worker": {{Content: "done COMPLETE"}},
	})
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)

	if provider.CallCount("exec-worker") != 3 {
		t.Errorf("exec-worker call count = %d, want 3 reject-then-accept iterations", provider.CallCount("exec-worker"))
	}
	assertRepeaterWorkStates(t, listed, map[string]int{"task:complete": 1, "task:init": 0, "task:failed": 0})
}
