package customer_lifecycles_test

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// TestEscalatePlanFailure_ConsumesFailedPlanSoRestoredIdeaIsNotReescalated
// proves a failed plan is escalated exactly once. After the idea is reported
// failed, a later healthy idea of the same name reaching to-complete (the
// operator restoration path) must not be escalated against the stale plan.
func testFactoryreviewfailureroutingEscalatePlanFailure_ConsumesFailedPlanSoRestoredIdeaIsNotReescalated(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("unused"), nil
		},
	})
	stream := scenario.eventStream(t)
	name := scenario.marker + "-plan-escalation"
	traceID := scenario.marker + "-trace"
	firstIdea := scenario.marker + "-idea-first"
	restoredIdea := scenario.marker + "-idea-restored"

	scenario.submit(t, scenario.marker+"-failed-plan", reviewFailureSeed{Name: name, WorkID: scenario.marker + "-plan", WorkType: "plan", State: "failed", TraceID: traceID, Payload: "plan"})
	scenario.submit(t, scenario.marker+"-first-idea", reviewFailureSeed{Name: name, WorkID: firstIdea, WorkType: "idea", State: "to-complete", TraceID: traceID, Payload: "idea"})
	awaitReviewFailureDispatchResponses(t, stream, "report-idea-failure", 1)
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{firstIdea: "failed"})

	// Operator restoration: the idea re-plans and reaches to-complete again.
	scenario.submit(t, scenario.marker+"-restored-idea", reviewFailureSeed{Name: name, WorkID: restoredIdea, WorkType: "idea", State: "to-complete", TraceID: traceID, Payload: "idea"})
	awaitReviewFailureQuiescence(t, scenario)

	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{restoredIdea: "to-complete"})
	if reports := dispatchesWithTransition(reviewFailureDispatches(t, scenario), "report-idea-failure"); len(reports) != 1 {
		t.Fatalf("report-idea-failure dispatches = %d, want exactly 1: %#v", len(reports), reports)
	}
}
