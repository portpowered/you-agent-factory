//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater rejection-to-acceptance sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "repeater test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	execMock := h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	if execMock.CallCount() != 3 {
		t.Errorf("expected exec-worker called 3 times, got %d", execMock.CallCount())
	}

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
}

func TestRepeater_GuardedLoopBreakerTerminatesRejectedRepeater(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater guarded loop-breaker sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "exhaustion test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
	)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "executor-loop-breaker", "task:failed")
}

func TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness(t *testing.T) {
	support.SkipLongFunctional(t, "slow repeater service-harness resource-release sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service resource repeater test"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Still working"},
		interfaces.InferenceResponse{Content: "Almost there"},
		interfaces.InferenceResponse{Content: "Done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Finalized. COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 15*time.Second)

	h.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if provider.CallCount() != 4 {
		t.Errorf("expected provider called 4 times, got %d", provider.CallCount())
	}
}
