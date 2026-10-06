package customer_lifecycles_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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
func testFactoryreviewfailureroutingProjectLeadWake_CompletedChildWakesOnlyItsOwnLeadOnce(t *testing.T) {
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
func testFactoryreviewfailureroutingProjectLeadWake_FailedChildWakesOnlyItsOwnLeadOnce(t *testing.T) {
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
func testFactoryreviewfailureroutingProjectLeadWake_AgentDecisionFailureWakesOnlyItsOwnLeadOnce(t *testing.T) {
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
func testFactoryreviewfailureroutingProjectLeadWake_ChildrenFinishingWhileLeadIsBusyQueueOneWakeEach(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	stream := wake.eventStream(t)
	firstName, firstIdeaID, _ := wake.submitOwnedChild(t, "first", "to-complete")
	secondName, secondIdeaID, _ := wake.submitOwnedChild(t, "second", "failed")
	awaitReviewFailureDispatchResponses(t, stream, "report-idea-complete", 1)
	awaitReviewFailureDispatchResponses(t, stream, "report-idea-failure", 1)
	awaitReviewFailureWorkStates(t, wake.reviewFailureScenario, map[string]string{
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

// Controlled provider outcomes prove reporting through public Work/Events;
// actual draft writing and agent admission remain VAL-LOOPBACK obligations.
func testProjectLoopbackProposalOutcomes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, decision, state  string
		commandError, untagged bool
	}{
		{name: "F1 accepted proposal", decision: "ACCEPTED", state: "complete"},
		{name: "F2 failed proposal", decision: "FAILED", state: "failed"},
		{name: "F3 command failure with saved draft", state: "failed", commandError: true},
		{name: "F6 untagged accepted hold", decision: "ACCEPTED", state: "complete", untagged: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proposal := "docs/temp/projects/fixture/proposals/loopback.json"
			wake := openProjectWakeScenarioWithProvider(t, func(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
				if strings.Contains(providerCommandPrompt(request), "Project Lead wake") {
					return reviewFailureAccepted("reconciled"), nil
				}
				if tc.commandError {
					return platformprocess.CommandResult{}, errors.New("controlled loopback provider failure")
				}
				output := proposal
				if tc.untagged {
					output = "hold"
				}
				envelope, _ := json.Marshal(map[string]string{"decision": tc.decision, "feedback": "original gap evidence", "output": output})
				return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(string(envelope))}, nil
			})
			wake.admitOwner(t)
			stream := wake.eventStream(t)
			childID, name := wake.marker+"-thoughts", wake.ownerName+"-loopback"
			tags := map[string]string{"project": wake.ownerName}
			if tc.untagged {
				tags = nil
			}
			wake.submit(t, childID+"-request", reviewFailureSeed{Name: name, WorkID: childID, WorkType: "thoughts", State: "init", TraceID: childID + "-trace", Payload: "original gap evidence; saved proposal " + proposal, Tags: tags})
			route := "report-thoughts-complete"
			if tc.state == "failed" {
				route = "report-thoughts-failure"
			}
			if tc.untagged {
				awaitReviewFailureDispatchResponses(t, stream, route, 1)
				awaitReviewFailureWorkStates(t, wake.reviewFailureScenario, map[string]string{childID: tc.state})
			} else {
				assertProjectLeadWakes(t, wake, stream, name)
			}
			works := wake.listWorks(t)
			original := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(wake.fixture.baseURL, wake.sessionID, "/work/"+childID))
			assertPayloadHasSentinel(t, "original saved reference", original.Payload, proposal)
			assertReviewFailureWorkStates(t, works, map[string]string{childID: tc.state, wake.ownerID: "waiting", wake.peerID: "waiting"})
			assertLoopbackReportOrigin(t, works, childID, tc.untagged)
			assertLoopbackReadableEvidence(t, wake, childID, tc.commandError, tc.untagged)
			dispatches := reviewFailureDispatches(t, wake.reviewFailureScenario)
			if len(dispatchesWithTransition(dispatches, route)) != 1 {
				t.Fatalf("report dispatched more than once: %#v", dispatches)
			}
			if tc.untagged && len(dispatchesWithTransition(dispatches, projectLeadWakeTransition)) != 0 {
				t.Fatal("untagged loopback woke a tagged Project")
			}
			if !tc.commandError && !tc.untagged {
				for _, work := range works {
					if work.WorkId != nil && *work.WorkId == childID {
						assertPayloadHasSentinel(t, "original failure evidence", work.Payload, "original gap evidence")
						assertPayloadHasSentinel(t, "last output", work.Tags, proposal)
					}
				}
			}
		})
	}
}

func testProjectLoopbackSequentialOrigins(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	wake.admitOwner(t)
	stream := wake.eventStream(t)
	name := wake.ownerName + "-loopback"
	origins := map[string]bool{}
	for _, suffix := range []string{"first", "second"} {
		id := wake.marker + "-" + suffix + "-thoughts"
		wake.submit(t, id+"-request", reviewFailureSeed{Name: name, WorkID: id, WorkType: "thoughts", State: "reporting-complete", TraceID: id + "-trace", Payload: "docs/temp/projects/fixture/proposals/loopback.json", Tags: map[string]string{"project": wake.ownerName}})
		awaitReviewFailureDispatchResponses(t, stream, projectLeadWakeTransition, 1)
		origins[id] = true
	}
	reports := 0
	for _, work := range wake.listWorks(t) {
		if work.WorkTypeName == nil || *work.WorkTypeName != "project-report" {
			continue
		}
		reports++
		origin := reportOriginWorkID(work)
		if !origins[origin] {
			t.Fatalf("duplicate or incorrect origin %q", origin)
		}
		delete(origins, origin)
		if work.State.Name != "delivered" {
			t.Fatalf("report not delivered: %#v", work)
		}
	}
	if reports != 2 || len(origins) != 0 {
		t.Fatalf("reports=%d missing=%v", reports, origins)
	}
	dispatches := dispatchesWithTransition(reviewFailureDispatches(t, wake.reviewFailureScenario), projectLeadWakeTransition)
	if len(dispatches) != 2 {
		t.Fatalf("wakes=%d want 2", len(dispatches))
	}
	for _, dispatch := range dispatches {
		ids := dispatchInputIDs(dispatch)
		for _, id := range ids {
			if id == wake.peerID {
				t.Fatal("peer woke")
			}
		}
	}
}

// F4 holds a real lead dispatch at the external provider boundary while a
// second thoughts completes; normal dispatch completion releases the lead.
func testProjectLoopbackBusyLead(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	var first, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	wake := openProjectWakeScenarioWithProvider(t, func(ctx context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
		if strings.Contains(providerCommandPrompt(request), "Project Lead wake") {
			first.Do(func() {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
		}
		return reviewFailureAccepted("lead reconciled"), nil
	})
	wake.admitOwner(t)
	stream := wake.eventStream(t)
	submit := func(slug string) {
		id := wake.marker + "-" + slug + "-thoughts"
		wake.submit(t, id+"-request", reviewFailureSeed{Name: wake.ownerName + "-" + slug, WorkID: id, WorkType: "thoughts", State: "reporting-complete", TraceID: id + "-trace", Payload: "saved proposal", Tags: map[string]string{"project": wake.ownerName}})
	}
	submit("first")
	// Signal is emitted only when the first lead provider dispatch is active.
	ctx, cancel := context.WithTimeout(context.Background(), reviewFailureEventTimeout)
	defer cancel()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("lead dispatch did not start")
	}
	submit("second")
	awaitReviewFailureDispatchResponses(t, stream, "report-thoughts-complete", 2)
	pending := 0
	for _, work := range wake.listWorks(t) {
		if work.WorkTypeName != nil && *work.WorkTypeName == "project-report" && work.State.Name == "pending" {
			pending++
		}
	}
	if pending != 2 {
		unblock()
		t.Fatalf("pending reports while lead busy=%d want 2 (in-flight and queued)", pending)
	}
	unblock()
	assertProjectLeadWakes(t, wake, stream, wake.ownerName+"-first", wake.ownerName+"-second")
}

func assertLoopbackReportOrigin(t *testing.T, works []factoryapi.Work, childID string, untagged bool) {
	t.Helper()
	reports := 0
	for _, work := range works {
		if work.WorkTypeName == nil || *work.WorkTypeName != "project-report" {
			continue
		}
		reports++
		if reportOriginWorkID(work) != childID {
			t.Fatalf("report origin = %q, want %q", reportOriginWorkID(work), childID)
		}
		// Reports retain the exact origin; the proposal stays readable on that Work.
		if untagged && (work.State == nil || work.State.Name != "pending") {
			t.Fatalf("untagged report delivered: %#v", work)
		}
	}
	if reports != 1 {
		t.Fatalf("reports = %d, want 1", reports)
	}
}

func assertLoopbackReadableEvidence(t *testing.T, wake *projectWakeScenario, childID string, commandError, untagged bool) {
	t.Helper()
	if commandError {
		events := support.GetFactoryEventsForSessionAt(t, wake.fixture.baseURL, wake.sessionID)
		foundFailure := false
		for _, event := range events {
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			response := decodeReviewFailureDispatchResponse(t, event)
			if response.TransitionId == "ideafy" && response.Outcome == factoryapi.WorkOutcomeFailed {
				foundFailure = true
			}
		}
		if !foundFailure {
			t.Fatal("provider failure has no retained FAILED ideafy dispatch Event")
		}
	}
	if untagged {
		return
	}
	found := false
	for _, request := range reviewFailureProviderRequests(wake.fixture.router.requestsFor(wake.factoryDir)) {
		prompt := providerCommandPrompt(request)
		if strings.Contains(prompt, "Project Lead wake") && strings.Contains(prompt, childID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("lead prompt did not identify origin %q", childID)
	}
}
