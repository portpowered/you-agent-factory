package workflow

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

// TestDispatcherLifecycle_IdeaToArchive exercises the full dispatcher lifecycle:
//
//	idea -> plan (produces prd) -> execute (produces code-change) -> review -> archive-gate -> archived
//
// This verifies cross-work-type token production at the plan and execute stages,
// and confirms that the archived code-change token traces back to the original idea
// via a shared TraceID.
func TestDispatcherLifecycle_IdeaToArchive(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))

	originTraceID := "trace-idea-lifecycle-test"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "improve onboarding flow"}`),
		TraceID:    originTraceID,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"planner":  {{Content: "success<COMPLETE>"}},
		"executor": {{Content: "success<COMPLETE>"}},
		"reviewer": {{Content: "success<COMPLETE>"}},
		"archiver": {{Content: "success<COMPLETE>"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 1000*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:archived").
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:failed").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:approved").
		HasNoTokenInPlace("code-change:failed")

	for _, workerType := range []string{"reviewer", "planner", "executor", "archiver"} {
		if len(provider.Calls(workerType)) != 1 {
			t.Errorf("expected %s called 1 time, got %d", workerType, len(provider.Calls(workerType)))
		}
	}

	h.Assert().TokenHasTraceID("code-change:archived", originTraceID)
}

// TestDispatcherLifecycle_PlannerFailure verifies that when the planner fails,
// the idea token moves to the failed state and no downstream tokens are created.
func TestDispatcherLifecycle_PlannerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "broken idea"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"planner": {{Content: "failed"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap())

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("code-change:init").
		HasNoTokenInPlace("code-change:archived")
}

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
