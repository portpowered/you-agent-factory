package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		Payload:    []byte(`{"title": "improve onboarding flow"}`),
		TraceID:    originTraceID,
	})

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"planner":  {{Content: "success<COMPLETE>"}},
		"executor": {{Content: "success<COMPLETE>"}},
		"reviewer": {{Content: "success<COMPLETE>"}},
		"archiver": {{Content: "success<COMPLETE>"}},
	})

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 30*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"code-change:archived": 1, "idea:init": 0, "idea:failed": 0, "prd:init": 0,
		"prd:failed": 0, "code-change:init": 0, "code-change:approved": 0, "code-change:failed": 0,
	})

	for _, workerType := range []string{"reviewer", "planner", "executor", "archiver"} {
		if len(provider.Calls(workerType)) != 1 {
			t.Errorf("expected %s called 1 time, got %d", workerType, len(provider.Calls(workerType)))
		}
	}

	assertListedWorkStateTrace(t, listedWork, "code-change", "archived", originTraceID)
}

// TestDispatcherLifecycle_PlannerFailure verifies that when the planner fails,
// the idea token moves to the failed state and no downstream tokens are created.
func TestDispatcherLifecycle_PlannerFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dispatcher_lifecycle_dir"))

	testutil.WriteSeedFile(t, dir, "idea", []byte(`{"title": "broken idea"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"planner": {{Content: "failed"}},
	})

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{ProviderOverride: provider}, 10*time.Second)
	assertWorkflowSessionPlaces(t, listed, map[string]int{
		"idea:failed": 1, "prd:init": 0, "code-change:init": 0, "code-change:archived": 0,
	})
}

func assertListedWorkStateTrace(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workType, state, traceID string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("%s:%s trace ID = %#v, want %q", workType, state, item.TraceId, traceID)
		}
		return
	}
	t.Errorf("listed Work missing %s:%s", workType, state)
}

func assertCompletedWorkTraces(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workType, state string,
	traceIDs []string,
) {
	t.Helper()
	wants := make(map[string]bool, len(traceIDs))
	for _, traceID := range traceIDs {
		wants[traceID] = true
	}
	found := map[string]bool{}
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != state {
			continue
		}
		if item.TraceId == nil || !wants[*item.TraceId] {
			t.Errorf("unexpected %s:%s trace ID %#v", workType, state, item.TraceId)
			continue
		}
		found[*item.TraceId] = true
	}
	for traceID := range wants {
		if !found[traceID] {
			t.Errorf("listed Work missing %s:%s trace %q", workType, state, traceID)
		}
	}
}
