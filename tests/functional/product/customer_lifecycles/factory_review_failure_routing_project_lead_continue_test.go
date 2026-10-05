package customer_lifecycles_test

import (
	"context"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProjectLead_ContinueDecisionKeepsProjectWaitingWithoutEscalation proves a
// project lead returning CONTINUE ("more to do, carry on") is a healthy
// outcome: the Project returns to waiting, escalate-project never dispatches,
// and no thoughts Work is created for the meta-planner.
func testFactoryreviewfailureroutingProjectLead_ContinueDecisionKeepsProjectWaitingWithoutEscalation(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(
				`{"decision":"CONTINUE","feedback":"more to do, carry on"}`,
			)}, nil
		},
	})
	stream := scenario.eventStream(t)
	projectName := scenario.marker + "-continuing-project"
	projectID := scenario.marker + "-continuing-project-id"
	scenario.submit(t, scenario.marker+"-continuing-project-request", reviewFailureSeed{
		Name: projectName, WorkID: projectID, WorkType: "project", State: "init",
		TraceID: scenario.marker + "-continuing-trace", Payload: "continue",
		Tags: map[string]string{"project": projectName},
	})

	response := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(
		t, stream, reviewFailureProjectLeadTransition, 1,
	)[0])
	if response.Outcome != factoryapi.WorkOutcomeContinue {
		t.Fatalf("project lead outcome = %q, want CONTINUE", response.Outcome)
	}
	awaitReviewFailureQuiescence(t, scenario)

	works := scenario.listWorks(t)
	assertReviewFailureWorkStates(t, works, map[string]string{projectID: "waiting"})
	for _, work := range works {
		if work.WorkTypeName != nil && *work.WorkTypeName == "thoughts" {
			t.Fatalf("CONTINUE created a thoughts Work %#v, want none", work)
		}
	}
	dispatches := reviewFailureDispatches(t, scenario)
	if escalations := dispatchesWithTransition(dispatches, "escalate-project"); len(escalations) != 0 {
		t.Fatalf("escalate-project dispatches = %d, want 0: %#v", len(escalations), escalations)
	}
	assertNoIncompleteReviewFailureDispatches(t, dispatches)
}
