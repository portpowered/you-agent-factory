package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRepeater_RefiresOnRejectedStopsOnAccepted(t *testing.T) {
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

func TestRepeater_YieldsBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-A"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "token-B"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	execMock := h.MockWorker("exec-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	finishMock := h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	if execMock.CallCount() < 3 {
		t.Errorf("expected exec-worker called at least 3 times (interleaved), got %d", execMock.CallCount())
	}
	if finishMock.CallCount() < 2 {
		t.Errorf("expected finish-worker called at least 2 times, got %d", finishMock.CallCount())
	}

	h.Assert().PlaceTokenCount("task:complete", 2)
}

func TestParameterizedFields_WorkingDirectoryResolvesFromTags(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_workstation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte(`{}`),
		Tags:       map[string]string{"branch": "feature-abc"},
	})

	h := testutil.NewServiceTestHarness(t, dir)

	capture := &capturingExecutor{
		result: interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	}
	h.SetCustomExecutor("exec-worker", capture)
	h.MockWorker("finish-worker",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	if capture.callCount == 0 {
		t.Fatal("capturing executor was never called")
	}
	if capture.lastDispatch.WorkstationName == "" {
		t.Error("expected WorkstationName to be set on dispatch")
	}
	if len(capture.lastDispatch.InputTokens) == 0 {
		t.Fatal("expected at least one input token")
	}
	tags := firstInputToken(capture.lastDispatch.InputTokens).Color.Tags
	if tags["branch"] != "feature-abc" {
		t.Errorf("expected tag branch=feature-abc, got %q", tags["branch"])
	}
}

func TestParameterizedFields_UnresolvedTemplateRoutesToFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "parameterized_failure"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "unresolved template test"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Should not reach COMPLETE"},
	)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:complete")

	if provider.CallCount() != 0 {
		t.Errorf("expected provider called 0 times (template error before invocation), got %d", provider.CallCount())
	}
}

func TestRepeater_ResourceReleaseBetweenIterations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "repeater_resource"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "resource repeater test"}`))

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

func TestRepeater_ResourceReleaseBetweenIterations_ServiceHarness(t *testing.T) {
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
