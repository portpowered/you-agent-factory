//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestIdeaToPRD_CrossWorkTypeOutput verifies that a planner workstation
// consumes an idea token and produces a prd token as cross-work-type output.
func TestIdeaToPRD_CrossWorkTypeOutput(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd cross-work-type sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

	originTraceID := "trace-idea-to-prd-test"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "search bar on docs"}`),
		TraceID:    originTraceID,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"planner":       {{Content: "Done. COMPLETE"}},
		"prd-processor": {{Content: "Done. COMPLETE"}},
	})

	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)

	if provider.CallCount("planner") != 1 {
		t.Errorf("expected planner called 1 time, got %d", provider.CallCount("planner"))
	}

	assertWorkflowSessionPlaces(t, session, map[string]int{"idea:init": 0, "prd:complete": 1})
	assertListedWorkStateTrace(t, listedWork, "prd", "complete", originTraceID)
}

// TestIdeaToPRD_PlannerFailure verifies that when the planner fails, the idea
// token moves to failed state and no prd token is created.
func TestIdeaToPRD_PlannerFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd planner-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "broken idea"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("LLM timeout"),
		ExitCode: 1,
	})
	session := support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"idea:failed": 1, "prd:init": 0, "prd:complete": 0})
}

// TestIdeaToPRD_MultipleIdeas verifies that multiple idea tokens each produce
// their own prd token with independent lineage.
func TestIdeaToPRD_MultipleIdeas(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-to-prd multi-idea sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_to_prd"))

	trace1 := "trace-idea-multi-1"
	trace2 := "trace-idea-multi-2"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "idea one"}`),
		TraceID:    trace1,
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "idea two"}`),
		TraceID:    trace2,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"planner":       {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
		"prd-processor": {{Content: "Done. COMPLETE"}, {Content: "Done. COMPLETE"}},
	})

	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"idea:init": 0, "prd:complete": 2})
	assertCompletedWorkTraces(t, listedWork, "prd", "complete", []string{trace1, trace2})
}
