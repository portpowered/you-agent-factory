//go:build functionallong

package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
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

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 1000*time.Second)

	h.Assert().
		HasNoTokenInPlace("idea:init").
		HasTokenInPlace("prd:failed").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:archived")
}
