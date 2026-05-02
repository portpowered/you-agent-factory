//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestIdeaToPRD_CrossWorkTypeOutput verifies that a planner workstation
// consumes an idea token and produces a prd token as cross-work-type output.
func TestIdeaToPRD_CrossWorkTypeOutput(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd cross-work-type sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

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
	h.RunUntilComplete(t, 10*time.Second)

	if provider.CallCount("planner") != 1 {
		t.Errorf("expected planner called 1 time, got %d", provider.CallCount("planner"))
	}

	h.Assert().HasNoTokenInPlace("idea:init")
	h.Assert().HasTokenInPlace("prd:complete")
	h.Assert().TokenHasTraceID("prd:complete", originTraceID)
	h.Assert().TokenHasWorkTypeID("prd:complete", "prd")
}

// TestIdeaToPRD_PlannerFailure verifies that when the planner fails, the idea
// token moves to failed state and no prd token is created.
func TestIdeaToPRD_PlannerFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd planner-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

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

	h.Assert().
		HasTokenInPlace("idea:failed").
		HasNoTokenInPlace("prd:init").
		HasNoTokenInPlace("prd:complete")
}

// TestIdeaToPRD_MultipleIdeas verifies that multiple idea tokens each produce
// their own prd token with independent lineage.
func TestIdeaToPRD_MultipleIdeas(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd multi-idea sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

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

	h.Assert().HasNoTokenInPlace("idea:init")
	h.Assert().PlaceTokenCount("prd:complete", 2)
	h.Assert().
		TokenHasTraceID("prd:complete", trace1).
		TokenHasTraceID("prd:complete", trace2)
}
