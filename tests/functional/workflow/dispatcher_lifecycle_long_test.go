//go:build functionallong

package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

// TestDispatcherLifecycle_ExecutorFailure verifies that when the executor fails,
// the prd token moves to failed state and no code-change tokens are created.
func TestDispatcherLifecycle_ExecutorFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow dispatcher lifecycle failure smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "failing executor"}`))

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"planner":  {{Content: "success<COMPLETE>"}},
		"executor": {{Content: "failed", Error: errors.New("failed executors")}},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 30*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"idea:init": 0, "prd:failed": 1, "code-change:init": 0, "code-change:archived": 0,
	})
}
