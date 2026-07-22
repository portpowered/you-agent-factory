//go:build functionallong

package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The logic of the flow goes (take an input idea, convert it to a plan, run a script, convert it to a task, execute teht ask, review the task):
// It has 5 resources

// Here we create an idea, we expect it to fail and have 5 resources in place of the resource positions.
func TestIdeaPlanExecuteReviewWithLimitsFailsOnScriptExecution(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-plan-review-execute script-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))

	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	work := map[string][]testutil.WorkResponse{
		"planner": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session := runIdeaPlanReviewWithLimits(t, dir, provider)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"idea:complete": 1, "plan:complete": 1, "task:failed": 1,
	})

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount("planner") != 1 {
		t.Errorf("expected provider called 1 times, got %d", provider.CallCount("planner"))
	}
}

// Here we create an idea, we expect it to fail and have 5 resources in place of the resource positions.
func TestIdeaPlanExecuteReviewWithLimitsFailsOnIdeation(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-plan-review-execute ideation-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))

	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	work := map[string][]testutil.WorkResponse{
		"planner": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session := runIdeaPlanReviewWithLimits(t, dir, provider)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"idea:complete": 1, "plan:complete": 1, "task:failed": 1,
	})

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount("planner") != 1 {
		t.Errorf("expected provider called 1 times, got %d", provider.CallCount("planner"))
	}
}

// Here we create an idea, we expect it to fail and have 5 resources in place of the resource positions.
func TestIdeaPlanExecuteReviewWithLimitsFailsOnExecutorDueToRepeatingTooMuch(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-plan-review-execute repeat-failure sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))

	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	work := map[string][]testutil.WorkResponse{
		"planner": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"processor": {
			{Content: "Task execution failed.<FAILED>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session := runIdeaPlanReviewWithLimits(t, dir, provider)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"idea:complete": 1, "plan:complete": 1, "task:failed": 1,
		"executor-slot:available": 5,
	})

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount("planner") != 1 {
		t.Errorf("expected provider called 1 times, got %d", provider.CallCount("planner"))
	}
}

func TestIdeaPlanExecuteReviewWithLimitsFailsOnExecutorFullPass(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-plan-review-execute full-pass sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))

	testutil.WriteSeedMarkdownFile(t, dir, "idea", "architecture-review",
		[]byte("# Architecture Review\n\nPlease review the system architecture."))

	work := map[string][]testutil.WorkResponse{
		"planner": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"processor": {
			{Content: "Task execution failed.<COMPLETE>"},
		},
		"reviewer": {
			{Content: "Task execution failed.<COMPLETE>"},
		},
	}
	provider := testutil.NewMockWorkerMapProviderWithDefault(work)

	session := runIdeaPlanReviewWithLimits(t, dir, provider)
	assertWorkflowSessionPlaces(t, session, map[string]int{
		"idea:complete": 1, "plan:complete": 1, "task:complete": 1,
	})

	// Verify the provider was called twice (once per worker in the pipeline).
	if provider.CallCount("planner") != 1 {
		t.Errorf("expected provider called 1 times, got %d", provider.CallCount("planner"))
	}
}

// TestIdeaPlanExecuteReviewWithLimits_TraceLineageAndOutcomes verifies that a
// single seed trace survives the full worker-pool/file-watcher path and can be
// reconstructed from the resulting terminal tokens alone.
func TestIdeaPlanExecuteReviewWithLimits_TraceLineageAndOutcomes(t *testing.T) {
	support.SkipLongFunctional(t, "slow idea-plan-execute-review lineage sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "idea_plan_execute_review_with_limits"))

	originTraceID := "trace-idea-plan-review-limits-001"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "idea",
		TraceID:    originTraceID,
		Payload:    []byte(`{"title": "trace lineage test"}`),
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"planner": {
			{Content: "Task processed successfully.<COMPLETE>"},
		},
		"processor": {
			{Content: "Task execution failed.<FAILED>"},
		},
	})

	session := runIdeaPlanReviewWithLimits(t, dir, provider)
	tracePlaces := make(map[string]int)
	if session.Runtime.Petri != nil {
		for _, token := range session.Runtime.Petri.Marking {
			if token.TraceId == originTraceID {
				tracePlaces[token.PlaceId]++
			}
		}
	}

	if len(tracePlaces) != 3 {
		t.Fatalf("expected 3 trace-backed tokens, got %d: %#v", len(tracePlaces), tracePlaces)
	}

	for _, placeID := range []string{"idea:complete", "plan:complete", "task:failed"} {
		if tracePlaces[placeID] != 1 {
			t.Errorf("expected trace %q to appear once in %q, got %d", originTraceID, placeID, tracePlaces[placeID])
		}
	}

	assertWorkflowSessionPlaces(t, session, map[string]int{
		"idea:complete": 1, "plan:complete": 1, "task:failed": 1,
		"idea:init": 0, "plan:init": 0, "task:init": 0,
		"task:in-review": 0, "task:complete": 0,
	})

	if provider.CallCount("planner") != 1 {
		t.Errorf("expected planner called 1 time, got %d", provider.CallCount("planner"))
	}
	if provider.CallCount("processor") != 5 {
		t.Errorf("expected processor called 5 times before exhaustion, got %d", provider.CallCount("processor"))
	}
	if provider.CallCount("reviewer") != 0 {
		t.Errorf("expected reviewer not to be called after processor failure, got %d", provider.CallCount("reviewer"))
	}
}

func runIdeaPlanReviewWithLimits(
	t *testing.T,
	dir string,
	provider *testutil.MockWorkerMapProvider,
) factoryapi.FactorySession {
	t.Helper()
	return support.RunFactoryToCompletionWithEdges(t, dir, serviceedges.Edges{
		ProviderOverride:    provider,
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("script-output-ok"),
	}, 10*time.Second)
}
