package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// TestIdeaToPRD_CrossWorkTypeOutput verifies that a planner workstation
// consumes an idea token and produces a prd token as cross-work-type output.
// This is the isolated version of the cross-work-type step from DP-001.
func TestIdeaToPRD_CrossWorkTypeOutput(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "idea_to_prd"))

	// Write seed file with a known TraceID for lineage verification.
	originTraceID := "trace-idea-to-prd-test"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "search bar on docs"}`),
		TraceID:    originTraceID,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"planner":       {{Content: "Done. COMPLETE"}},
		"prd-processor": {{Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	// Run to completion so the planner fires, produces the prd token,
	// and prd-processor advances it to prd:complete.
	h.RunUntilComplete(t, 10*time.Second)

	// The planner should have fired exactly once.
	if provider.CallCount("planner") != 1 {
		t.Errorf("expected planner called 1 time, got %d", provider.CallCount("planner"))
	}

	// Idea token must be consumed (no longer in init).
	h.Assert().HasNoTokenInPlace("idea:init")

	// PRD token must have reached terminal state.
	h.Assert().HasTokenInPlace("prd:complete")

	// PRD token carries lineage: same TraceID as the original idea.
	h.Assert().TokenHasTraceID("prd:complete", originTraceID)

	// Cross-work-type output tokens get WorkTypeID from the target place, not the input.
	h.Assert().TokenHasWorkTypeID("prd:complete", "prd")
}

// TestIdeaToPRD_PlannerFailure verifies that when the planner fails, the idea
// token moves to failed state and no prd token is created.
func TestIdeaToPRD_PlannerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "idea_to_prd"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "broken idea"}`))

	runner := testutil.NewProviderCommandRunner(workers.CommandResult{
		Stderr:   []byte("LLM timeout"),
		ExitCode: 1,
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Idea moved to failed, no prd created.
	h.Assert().
		HasTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:complete")
}

// TestIdeaToPRD_MultipleIdeas verifies that multiple idea tokens each produce
// their own prd token with independent lineage.
func TestIdeaToPRD_MultipleIdeas(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "idea_to_prd"))

	// Write two seed files with known TraceIDs for lineage verification.
	trace1 := "trace-idea-multi-1"
	trace2 := "trace-idea-multi-2"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "idea one"}`),
		TraceID:    trace1,
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "idea two"}`),
		TraceID:    trace2,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"planner":       {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"prd-processor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both ideas consumed.
	h.Assert().HasNoTokenInPlace("idea:init")

	// Two prd tokens completed.
	h.Assert().PlaceTokenCount("prd:complete", 2)

	// Each prd carries the correct lineage trace.
	h.Assert().
		TokenHasTraceID("prd:complete", trace1).
		TokenHasTraceID("prd:complete", trace2)
}
