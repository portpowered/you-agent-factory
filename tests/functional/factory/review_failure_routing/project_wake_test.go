package review_failure_routing

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const projectLeadWakeTransition = "project-lead-wake"

// projectWakeScenario seeds two Projects that differ only by their `project`
// tag, so a wake that ignored Project identity would bind either.
type projectWakeScenario struct {
	*reviewFailureScenario
	ownerName string
	ownerID   string
	peerName  string
	peerID    string
}

func openProjectWakeScenario(t *testing.T) *projectWakeScenario {
	t.Helper()
	return openProjectWakeScenarioWithProvider(t, func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
		return reviewFailureAccepted("lead reconciled the finished child"), nil
	})
}

func openProjectWakeScenarioWithProvider(t *testing.T, provider reviewFailureCommandResponder) *projectWakeScenario {
	t.Helper()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{provider: provider})
	wake := &projectWakeScenario{
		reviewFailureScenario: scenario,
		ownerName:             scenario.marker + "-zulu",
		ownerID:               scenario.marker + "-zulu-project",
		peerName:              scenario.marker + "-alpha",
		peerID:                scenario.marker + "-alpha-project",
	}
	// The peer is admitted first and sorts first, so an unguarded wake would
	// bind the peer instead of the owner.
	scenario.submit(t, scenario.marker+"-peer-project", reviewFailureSeed{
		Name: wake.peerName, WorkID: wake.peerID, WorkType: "project", State: "waiting",
		TraceID: scenario.marker + "-alpha-trace", Payload: "alpha project",
		Tags: map[string]string{"project": wake.peerName},
	})
	return wake
}

func (wake *projectWakeScenario) admitOwner(t *testing.T) {
	t.Helper()
	wake.submit(t, wake.marker+"-owner-project", reviewFailureSeed{
		Name: wake.ownerName, WorkID: wake.ownerID, WorkType: "project", State: "waiting",
		TraceID: wake.marker + "-zulu-trace", Payload: "zulu project",
		Tags: map[string]string{"project": wake.ownerName},
	})
}

// submitOwnedChild admits one owner-tagged idea at to-complete with a
// same-name task in taskState. Same-name Work needs separate requests.
func (wake *projectWakeScenario) submitOwnedChild(t *testing.T, slug, taskState string) (name, ideaID, taskID string) {
	t.Helper()
	name = wake.ownerName + "-" + slug
	ideaID = wake.marker + "-" + slug + "-idea"
	taskID = wake.marker + "-" + slug + "-task"
	traceID := wake.marker + "-" + slug + "-trace"
	wake.submit(t, taskID+"-request", reviewFailureSeed{
		Name: name, WorkID: taskID, WorkType: "task", State: taskState,
		TraceID: traceID, Payload: slug + " task",
	})
	wake.submit(t, ideaID+"-request", reviewFailureSeed{
		Name: name, WorkID: ideaID, WorkType: "idea", State: "to-complete",
		TraceID: traceID, Payload: slug + " idea", Tags: map[string]string{"project": wake.ownerName},
	})
	return name, ideaID, taskID
}

// TestProjectLeadWake_CompletedChildWakesOnlyItsOwnLeadOnce proves one child
// idea reaching complete wakes its own Project lead exactly once, tells the
// lead which child finished, and never wakes a peer Project.
func TestProjectLeadWake_CompletedChildWakesOnlyItsOwnLeadOnce(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	wake.admitOwner(t)
	stream := wake.eventStream(t)
	childName, ideaID, taskID := wake.submitOwnedChild(t, "slice", "to-complete")

	reportIDs := assertProjectLeadWakes(t, wake, stream, childName)
	assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{
		ideaID: "complete", taskID: "complete", reportIDs[0]: "delivered",
		wake.ownerID: "waiting", wake.peerID: "waiting",
	})
}

// TestProjectLeadWake_FailedChildWakesOnlyItsOwnLeadOnce proves a child idea
// failing through task escalation also wakes its own lead exactly once.
func TestProjectLeadWake_FailedChildWakesOnlyItsOwnLeadOnce(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	wake.admitOwner(t)
	stream := wake.eventStream(t)
	childName, ideaID, taskID := wake.submitOwnedChild(t, "broken-slice", "failed")

	reportIDs := assertProjectLeadWakes(t, wake, stream, childName)
	assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{
		ideaID: "failed", taskID: "escalated", reportIDs[0]: "delivered",
		wake.ownerID: "waiting", wake.peerID: "waiting",
	})
}

// TestProjectLeadWake_AgentDecisionFailureWakesOnlyItsOwnLeadOnce proves a
// tagged idea or validation whose agent returns a non-accepting decision
// envelope (the provider command itself succeeds) passes through
// reporting-failed and wakes its own lead exactly once. A FAILED decision is a
// terminal Work failure, which the runtime routes to the work type's first
// FAILED state rather than the authored onFailure route, so it must not land
// directly in failed and skip the report.
func TestProjectLeadWake_AgentDecisionFailureWakesOnlyItsOwnLeadOnce(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		slug     string
		workType string
		report   string
		decision string
	}{
		{name: "plan failed decision", slug: "plan-failed", workType: "idea", report: "report-idea-failure", decision: `{"decision":"FAILED","feedback":"planner cannot plan this idea"}`},
		{name: "plan rejected decision", slug: "plan-rejected", workType: "idea", report: "report-idea-failure", decision: `{"decision":"REJECTED","feedback":"planner rejected this idea"}`},
		{name: "validate failed decision", slug: "validate-failed", workType: "validation", report: "report-validation-failure", decision: `{"decision":"FAILED","feedback":"validator cannot validate this"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wake := openProjectWakeScenarioWithProvider(t, func(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
				if strings.Contains(providerCommandPrompt(request), "Project Lead wake") {
					return reviewFailureAccepted("lead reconciled the finished child"), nil
				}
				return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(tc.decision)}, nil
			})
			wake.admitOwner(t)
			stream := wake.eventStream(t)
			name := wake.ownerName + "-" + tc.slug
			childID := wake.marker + "-" + tc.slug + "-" + tc.workType
			wake.submit(t, childID+"-request", reviewFailureSeed{
				Name: name, WorkID: childID, WorkType: tc.workType, State: "init",
				TraceID: wake.marker + "-" + tc.slug + "-trace", Payload: tc.slug + " " + tc.workType,
				Tags: map[string]string{"project": wake.ownerName},
			})

			reportIDs := assertProjectLeadWakes(t, wake, stream, name)
			assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{
				childID: "failed", reportIDs[0]: "delivered",
				wake.ownerID: "waiting", wake.peerID: "waiting",
			})
			dispatches := reviewFailureDispatches(t, wake.reviewFailureScenario)
			if reports := dispatchesWithTransition(dispatches, tc.report); len(reports) != 1 {
				t.Fatalf("%s dispatches = %d, want exactly 1: %#v", tc.report, len(reports), reports)
			}
		})
	}
}

// TestProjectLeadWake_ChildrenFinishingWhileLeadIsBusyQueueOneWakeEach proves
// reports for children that finish while the lead is not waiting are kept,
// then wake the lead once per child when it returns to waiting.
func TestProjectLeadWake_ChildrenFinishingWhileLeadIsBusyQueueOneWakeEach(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	stream := wake.eventStream(t)
	firstName, firstIdeaID, _ := wake.submitOwnedChild(t, "first", "to-complete")
	secondName, secondIdeaID, _ := wake.submitOwnedChild(t, "second", "failed")
	awaitReviewFailureDispatchResponses(t, stream, "report-idea-complete", 1)
	awaitReviewFailureDispatchResponses(t, stream, "report-idea-failure", 1)
	assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{
		firstIdeaID: "complete", secondIdeaID: "failed", wake.peerID: "waiting",
	})

	wake.admitOwner(t)
	reportIDs := assertProjectLeadWakes(t, wake, stream, firstName, secondName)
	assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{
		reportIDs[0]: "delivered", reportIDs[1]: "delivered",
		wake.ownerID: "waiting", wake.peerID: "waiting",
	})
}

// assertProjectLeadWakes waits for one owner wake per named child and returns
// the delivered report Work IDs. It proves exactly one wake per child, none
// for the peer, and lead prompts that name each finished child.
func assertProjectLeadWakes(
	t *testing.T,
	wake *projectWakeScenario,
	stream *support.FactoryEventStream,
	childNames ...string,
) []string {
	t.Helper()
	pending := make(map[string]bool, len(childNames))
	for _, name := range childNames {
		pending[name] = true
	}
	reportIDs := make([]string, 0, len(childNames))
	for _, event := range awaitReviewFailureDispatchResponses(t, stream, projectLeadWakeTransition, len(childNames)) {
		response := decodeReviewFailureDispatchResponse(t, event)
		if response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("project lead wake outcome = %q, want ACCEPTED", response.Outcome)
		}
		name, reportID := deliveredProjectReport(t, response, wake.ownerName)
		if !pending[name] {
			t.Fatalf("project lead wake delivered report for %q, want one of the remaining children %v", name, pending)
		}
		delete(pending, name)
		reportIDs = append(reportIDs, reportID)
	}

	dispatches := reviewFailureDispatches(t, wake.reviewFailureScenario)
	wakes := dispatchesWithTransition(dispatches, projectLeadWakeTransition)
	if len(wakes) != len(childNames) {
		t.Fatalf("project lead wake dispatches = %d, want exactly %d: %#v", len(wakes), len(childNames), wakes)
	}
	for index, dispatch := range wakes {
		assertExactReviewFailureInputIDs(t, dispatch, wake.ownerID, reportIDs[index])
	}
	for _, dispatch := range dispatches {
		for _, workID := range dispatchInputIDs(dispatch) {
			if workID == wake.peerID && dispatch.Request.TransitionId != "project-lead-checkin" {
				t.Fatalf("peer Project %q was dispatched by %q: %#v", wake.peerID, dispatch.Request.TransitionId, dispatch)
			}
		}
	}
	assertProjectLeadWakePrompts(t, wake, childNames)
	return reportIDs
}

func assertProjectLeadWakePrompts(t *testing.T, wake *projectWakeScenario, childNames []string) {
	t.Helper()
	unseen := make(map[string]bool, len(childNames))
	for _, name := range childNames {
		unseen[name] = true
	}
	prompts := 0
	for _, request := range reviewFailureProviderRequests(wake.fixture.router.requestsFor(wake.factoryDir)) {
		prompt := providerCommandPrompt(request)
		if !strings.Contains(prompt, "Project Lead wake") {
			continue
		}
		prompts++
		if !strings.Contains(prompt, wake.ownerID) || strings.Contains(prompt, wake.peerID) {
			t.Fatalf("project lead wake prompt must bind owner %q and not peer %q:\n%s", wake.ownerID, wake.peerID, prompt)
		}
		for name := range unseen {
			if strings.Contains(prompt, "name `"+name+"`") {
				delete(unseen, name)
			}
		}
	}
	if prompts != len(childNames) || len(unseen) != 0 {
		t.Fatalf("project lead wake prompts = %d naming children %v; missing %v", prompts, childNames, unseen)
	}
}

func deliveredProjectReport(
	t *testing.T,
	response factoryapi.DispatchResponseEventPayload,
	projectTag string,
) (name, workID string) {
	t.Helper()
	if response.OutputWork == nil {
		t.Fatal("project lead wake outputWork = nil, want project and delivered report")
	}
	for _, work := range *response.OutputWork {
		if work.WorkTypeName == nil || *work.WorkTypeName != "project-report" {
			continue
		}
		if work.State == nil || work.State.Name != "delivered" || work.WorkId == nil {
			t.Fatalf("project lead wake report = %#v, want delivered report", work)
		}
		if work.Tags == nil || (*work.Tags)["project"] != projectTag {
			t.Fatalf("project lead wake report tags = %#v, want project %q", work.Tags, projectTag)
		}
		return work.Name, *work.WorkId
	}
	t.Fatalf("project lead wake outputWork = %#v, want one project-report", *response.OutputWork)
	return "", ""
}
