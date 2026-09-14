package review_failure_routing

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestReviewFailureRouting_PostStartupDispatchesEachExactLineagePairOnce
// proves F-01 through F-03 through the public Work and Factory Event surfaces.
// The request is admitted after the shared continuous host is already serving;
// old same-name records, terminal records, unmatched records, and a duplicate
// current-name record remain untouched while the exact current pair dispatches
// once.
func TestReviewFailureRouting_PostStartupDispatchesEachExactLineagePairOnce(t *testing.T) {
	t.Parallel()

	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("current review accepted"), nil
		},
	})
	stream := scenario.eventStream(t)
	currentTrace := scenario.marker + "-current-trace"
	oldTrace := scenario.marker + "-old-trace"
	terminalTrace := scenario.marker + "-terminal-trace"
	unmatchedTrace := scenario.marker + "-unmatched-trace"
	taskName := scenario.marker + "-task"
	reviewName := scenario.marker + "-review"

	// The history pair has the same customer-facing names as the current pair,
	// but a distinct lineage and terminal/failed states.
	scenario.submit(t, scenario.marker+"-history-task",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-old-task", WorkType: "task", State: "failed", TraceID: oldTrace, Payload: "old-task-payload"},
	)
	scenario.submit(t, scenario.marker+"-history-review",
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-old-review", WorkType: "review", State: "fin", TraceID: oldTrace, Payload: "old-review-payload"},
	)
	scenario.submit(t, scenario.marker+"-current",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-current-task", WorkType: "task", State: "in-review", TraceID: currentTrace, Payload: "current-task-payload"},
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-current-review", WorkType: "review", State: "init", TraceID: currentTrace, Payload: "current-review-payload"},
	)
	// This duplicate is admitted with its own Work ID after startup. It must
	// not be paired with the task already claimed by the first review dispatch.
	scenario.submit(t, scenario.marker+"-duplicate",
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-duplicate-review", WorkType: "review", State: "init", TraceID: currentTrace, Payload: "duplicate-review-payload"},
	)
	scenario.submit(t, scenario.marker+"-terminal-task",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-terminal-task", WorkType: "task", State: "complete", TraceID: terminalTrace, Payload: "terminal-task-payload"},
	)
	scenario.submit(t, scenario.marker+"-terminal-review",
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-terminal-review", WorkType: "review", State: "complete", TraceID: terminalTrace, Payload: "terminal-review-payload"},
	)
	scenario.submit(t, scenario.marker+"-unmatched",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-unmatched-task", WorkType: "task", State: "in-review", TraceID: unmatchedTrace, Payload: "unmatched-task-payload"},
	)
	responses := awaitReviewFailureDispatchResponses(t, stream, "review", 1)
	response := decodeReviewFailureDispatchResponse(t, responses[0])
	if response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("current review outcome = %q, want ACCEPTED", response.Outcome)
	}
	assertReviewFailureDispatchOutputStates(t, response, map[string]string{
		scenario.marker + "-current-task":   "to-complete",
		scenario.marker + "-current-review": "complete",
	})

	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 1 {
		t.Fatalf("public dispatch count = %d, want exactly one; dispatches = %#v", len(dispatches), dispatches)
	}
	assertExactReviewFailureInputIDs(t, dispatches[0], scenario.marker+"-current-task", scenario.marker+"-current-review")
	if dispatches[0].Request.CurrentChainingTraceId == nil || *dispatches[0].Request.CurrentChainingTraceId != currentTrace {
		t.Fatalf("dispatch current trace = %#v, want %q", dispatches[0].Request.CurrentChainingTraceId, currentTrace)
	}
	if dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("public review dispatch response = %#v, want one ACCEPTED response", dispatches[0].Response)
	}

	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		scenario.marker + "-current-task":     "to-complete",
		scenario.marker + "-current-review":   "complete",
		scenario.marker + "-duplicate-review": "init",
		scenario.marker + "-old-task":         "failed",
		scenario.marker + "-old-review":       "fin",
		scenario.marker + "-terminal-task":    "complete",
		scenario.marker + "-terminal-review":  "complete",
		scenario.marker + "-unmatched-task":   "in-review",
	})
	for _, request := range scenario.fixture.router.requestsFor(scenario.factoryDir) {
		if isReviewFailureProviderCommand(request.Command) &&
			!strings.Contains(providerCommandPrompt(request), scenario.marker+"-current") {
			t.Fatalf("provider prompt = %q, want current lineage marker", providerCommandPrompt(request))
		}
	}
}

// TestReviewFailureRouting_ExactFailureOwners proves F-04 and F-06. A
// controlled CI failure routes only the current trace to its failed idea/task
// owner, while a reviewer execution failure routes only the current task and
// review to their failed/terminal states with the exact edge diagnostic.
func TestReviewFailureRouting_ExactFailureOwners(t *testing.T) {
	t.Parallel()
	const ciFeedback = "CI failed: exact current-head feedback 7f1c"
	const reviewFailure = "reviewer process failed: exact current owner 9b2e"

	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			if strings.Contains(providerCommandPrompt(request), "review-failure") {
				return reviewFailureFailed(reviewFailure)
			}
			return reviewFailureAccepted("provider success"), nil
		},
		script: func(_ context.Context, _ platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			if call == 1 {
				return reviewFailureScriptFailed(ciFeedback)
			}
			return platformprocess.CommandResult{Stdout: []byte("controlled-ci-success"), ExitCode: 0}, nil
		},
	})
	stream := scenario.eventStream(t)
	ciTrace := scenario.marker + "-ci-trace"
	oldTrace := scenario.marker + "-old-trace"
	scenario.submit(t, scenario.marker+"-ci-current",
		reviewFailureSeed{Name: scenario.marker + "-ci-idea", WorkID: scenario.marker + "-ci-idea-current", WorkType: "idea", State: "to-complete", TraceID: ciTrace, Payload: "ci-idea-current"},
		reviewFailureSeed{Name: scenario.marker + "-ci-task", WorkID: scenario.marker + "-ci-task-current", WorkType: "task", State: "awaiting-ci", TraceID: ciTrace, Payload: "ci-task-current"},
	)
	scenario.submit(t, scenario.marker+"-ci-old-idea",
		reviewFailureSeed{Name: scenario.marker + "-ci-idea", WorkID: scenario.marker + "-ci-idea-old", WorkType: "idea", State: "failed", TraceID: oldTrace, Payload: "ci-idea-old"},
	)
	scenario.submit(t, scenario.marker+"-ci-old-task",
		reviewFailureSeed{Name: scenario.marker + "-ci-task", WorkID: scenario.marker + "-ci-task-old", WorkType: "task", State: "failed", TraceID: oldTrace, Payload: "ci-task-old"},
	)

	ciResponses := awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)
	ciResponse := decodeReviewFailureDispatchResponse(t, ciResponses[0])
	if ciResponse.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("CI dispatch outcome = %q, want FAILED", ciResponse.Outcome)
	}
	assertExactReviewFailureDiagnostic(t, ciResponse, ciFeedback)
	assertReviewFailureDispatchOutputStates(t, ciResponse, map[string]string{
		scenario.marker + "-ci-task-current": "failed",
	})

	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		scenario.marker + "-ci-idea-current": "failed",
		scenario.marker + "-ci-task-current": "failed",
		scenario.marker + "-ci-idea-old":     "failed",
		scenario.marker + "-ci-task-old":     "failed",
	})
	ciDispatches := reviewFailureDispatches(t, scenario)
	if len(ciDispatches) != 2 {
		t.Fatalf("CI failure-owner dispatch count = %d, want ci-wait plus one escalation; dispatches = %#v", len(ciDispatches), ciDispatches)
	}
	for _, dispatch := range ciDispatches {
		switch dispatch.Request.TransitionId {
		case "ci-wait":
			assertExactReviewFailureInputIDs(t, dispatch, scenario.marker+"-ci-task-current")
		case "escalate-task-failure":
			assertExactReviewFailureInputIDs(t, dispatch, scenario.marker+"-ci-idea-current", scenario.marker+"-ci-task-current")
		default:
			t.Fatalf("unexpected CI failure-owner transition %q; dispatches = %#v", dispatch.Request.TransitionId, ciDispatches)
		}
	}

	reviewTrace := scenario.marker + "-review-trace"
	scenario.submit(t, scenario.marker+"-review-failure",
		reviewFailureSeed{Name: scenario.marker + "-review-failure-task", WorkID: scenario.marker + "-review-failure-task-current", WorkType: "task", State: "in-review", TraceID: reviewTrace, Payload: "review-failure-task-current"},
		reviewFailureSeed{Name: scenario.marker + "-review-failure-review", WorkID: scenario.marker + "-review-failure-review-current", WorkType: "review", State: "init", TraceID: reviewTrace, Payload: "review-failure-review-current"},
	)
	reviewResponses := awaitReviewFailureDispatchResponses(t, stream, "review", 1)
	reviewResponse := decodeReviewFailureDispatchResponse(t, reviewResponses[0])
	if reviewResponse.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("reviewer dispatch outcome = %q, want FAILED", reviewResponse.Outcome)
	}
	assertExactReviewFailureDiagnostic(t, reviewResponse, reviewFailure)
	assertReviewFailureDispatchOutputStates(t, reviewResponse, map[string]string{
		scenario.marker + "-review-failure-task-current":   "failed",
		scenario.marker + "-review-failure-review-current": "fin",
	})
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		scenario.marker + "-review-failure-task-current":   "failed",
		scenario.marker + "-review-failure-review-current": "fin",
	})

	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 3 {
		t.Fatalf("failure-owner dispatch count = %d, want CI, current escalation, and reviewer only; dispatches = %#v", len(dispatches), dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == "review" {
			assertExactReviewFailureInputIDs(t, dispatch, scenario.marker+"-review-failure-task-current", scenario.marker+"-review-failure-review-current")
		}
	}
}

func assertExactReviewFailureDiagnostic(t *testing.T, payload factoryapi.DispatchResponseEventPayload, want string) {
	t.Helper()
	if payload.Error != nil && *payload.Error == want {
		return
	}
	if payload.Feedback != nil && *payload.Feedback == want {
		return
	}
	if payload.FailureDetail != nil && payload.FailureDetail.Message == want {
		return
	}
	t.Fatalf("dispatch failure diagnostic = %#v, want exact %q", payload, want)
}

// TestReviewFailureRouting_RejectionCarriesFeedbackToTheCurrentCorrection
// proves F-05 through two review attempts. The first rejection carries an
// exact feedback string into the next processor prompt and only the current
// task/review pair reaches completion.
func TestReviewFailureRouting_RejectionCarriesFeedbackToTheCurrentCorrection(t *testing.T) {
	t.Parallel()
	const feedback = "review feedback: add the exact failure-routing evidence 4a6d"

	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			switch call {
			case 1:
				return reviewFailureRejected(feedback), nil
			case 2:
				return reviewFailureAccepted("revised current task"), nil
			case 3:
				return reviewFailureAccepted("current review accepted"), nil
			default:
				return reviewFailureAccepted("unexpected extra provider call"), nil
			}
		},
	})
	stream := scenario.eventStream(t)
	trace := scenario.marker + "-current-trace"
	oldTrace := scenario.marker + "-old-trace"
	taskName := scenario.marker + "-rejection-task"
	reviewName := scenario.marker + "-rejection-review"
	scenario.submit(t, scenario.marker+"-rejection-current",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-rejection-task-current", WorkType: "task", State: "in-review", TraceID: trace, Payload: "current-task-input"},
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-rejection-review-current", WorkType: "review", State: "init", TraceID: trace, Payload: "current-review-input"},
	)
	scenario.submit(t, scenario.marker+"-rejection-old-task",
		reviewFailureSeed{Name: taskName, WorkID: scenario.marker + "-rejection-task-old", WorkType: "task", State: "failed", TraceID: oldTrace, Payload: "old-task-input"},
	)
	scenario.submit(t, scenario.marker+"-rejection-old-review",
		reviewFailureSeed{Name: reviewName, WorkID: scenario.marker + "-rejection-review-old", WorkType: "review", State: "fin", TraceID: oldTrace, Payload: "old-review-input"},
	)

	responses := awaitReviewFailureDispatchResponses(t, stream, "review", 2)
	first := decodeReviewFailureDispatchResponse(t, responses[0])
	second := decodeReviewFailureDispatchResponse(t, responses[1])
	if first.Outcome != factoryapi.WorkOutcomeRejected || second.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("review outcomes = %q then %q, want REJECTED then ACCEPTED", first.Outcome, second.Outcome)
	}
	if first.Feedback == nil || *first.Feedback != feedback {
		t.Fatalf("first review feedback = %#v, want exact %q", first.Feedback, feedback)
	}
	replacementReviewID := replacementReviewWorkID(t, second)
	assertReviewFailureDispatchOutputStates(t, second, map[string]string{
		scenario.marker + "-rejection-task-current": "to-complete",
		replacementReviewID:                         "complete",
	})

	requests := scenario.fixture.router.requestsFor(scenario.factoryDir)
	providerRequests := reviewFailureProviderRequests(requests)
	if len(providerRequests) != 3 {
		t.Fatalf("provider request count = %d, want reviewer/processor/reviewer sequence; requests = %#v", len(providerRequests), requests)
	}
	if !hasReviewFailureFeedbackPrompt(providerRequests[1:], feedback) {
		t.Fatalf("no current correction provider prompt carried exact feedback %q", feedback)
	}

	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 4 {
		t.Fatalf("rejection dispatch count = %d, want review/process/ci-wait/review; dispatches = %#v", len(dispatches), dispatches)
	}
	assertRejectionReviewDispatches(t, dispatches, scenario.marker+"-rejection-task-current", scenario.marker+"-rejection-review-current", replacementReviewID)
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		scenario.marker + "-rejection-task-current": "to-complete",
		replacementReviewID:                         "complete",
		scenario.marker + "-rejection-task-old":     "failed",
		scenario.marker + "-rejection-review-old":   "fin",
	})
}

func replacementReviewWorkID(t *testing.T, response factoryapi.DispatchResponseEventPayload) string {
	t.Helper()
	if response.OutputWork != nil {
		for _, work := range *response.OutputWork {
			if work.WorkId != nil && work.WorkTypeName != nil && *work.WorkTypeName == "review" &&
				work.State != nil && work.State.Name == "complete" {
				return *work.WorkId
			}
		}
	}
	t.Fatalf("accepted review output has no current completed review Work: %#v", response.OutputWork)
	return ""
}

func reviewFailureProviderRequests(requests []platformprocess.CommandRequest) []platformprocess.CommandRequest {
	providerRequests := make([]platformprocess.CommandRequest, 0, len(requests))
	for _, request := range requests {
		if isReviewFailureProviderCommand(request.Command) {
			providerRequests = append(providerRequests, request)
		}
	}
	return providerRequests
}

func hasReviewFailureFeedbackPrompt(requests []platformprocess.CommandRequest, feedback string) bool {
	for _, request := range requests {
		if strings.Contains(providerCommandPrompt(request), feedback) {
			return true
		}
	}
	return false
}

func assertRejectionReviewDispatches(t *testing.T, dispatches []support.DispatchEventObservation, taskID, originalReviewID, replacementReviewID string) {
	t.Helper()
	reviewCount := 0
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "review" {
			continue
		}
		reviewCount++
		switch reviewCount {
		case 1:
			assertExactReviewFailureInputIDs(t, dispatch, taskID, originalReviewID)
		case 2:
			assertExactReviewFailureInputIDs(t, dispatch, taskID, replacementReviewID)
		default:
			t.Fatalf("review dispatch count = %d, want no unchanged third attempt: %#v", reviewCount, dispatch)
		}
	}
	if reviewCount != 2 {
		t.Fatalf("review dispatches = %d, want two exact current attempts", reviewCount)
	}
}

// TestReviewFailureRouting_CompletesCurrentTaskAfterFailedIdea proves F-07.
// The three current records form the exact failed-idea completion route while
// an old same-name failed idea remains a separate historical record.
func TestReviewFailureRouting_CompletesCurrentTaskAfterFailedIdea(t *testing.T) {
	t.Parallel()

	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{})
	stream := scenario.eventStream(t)
	currentTrace := scenario.marker + "-current-trace"
	oldTrace := scenario.marker + "-old-trace"
	ideaName := scenario.marker + "-failed-idea"
	scenario.submit(t, scenario.marker+"-failed-idea-completion-current",
		reviewFailureSeed{Name: ideaName, WorkID: scenario.marker + "-idea-current", WorkType: "idea", State: "failed", TraceID: currentTrace, Payload: "current-failed-idea"},
		reviewFailureSeed{Name: scenario.marker + "-completion-task", WorkID: scenario.marker + "-task-current", WorkType: "task", State: "to-complete", TraceID: currentTrace, Payload: "current-to-complete-task"},
		reviewFailureSeed{Name: scenario.marker + "-completion-review", WorkID: scenario.marker + "-review-current", WorkType: "review", State: "complete", TraceID: currentTrace, Payload: "current-complete-review"},
	)
	scenario.submit(t, scenario.marker+"-failed-idea-completion-old",
		reviewFailureSeed{Name: ideaName, WorkID: scenario.marker + "-idea-old", WorkType: "idea", State: "failed", TraceID: oldTrace, Payload: "old-failed-idea"},
	)
	awaitReviewFailureEvents(t, stream, func(event factoryapi.FactoryEvent) bool {
		return event.Type == factoryapi.FactoryEventTypeWorkRequest &&
			event.Id == "factory-event/work-request/"+scenario.marker+"-failed-idea-completion-old"
	})

	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 1 {
		t.Fatalf("failed-idea completion dispatch count = %d, want one exact current logical completion; dispatches = %#v", len(dispatches), dispatches)
	}
	if dispatches[0].Request.TransitionId != "complete-reviewed-task-after-failed-idea" {
		t.Fatalf("failed-idea completion transition = %q, want exact current completion", dispatches[0].Request.TransitionId)
	}
	assertExactReviewFailureInputIDs(t, dispatches[0],
		scenario.marker+"-idea-current",
		scenario.marker+"-task-current",
		scenario.marker+"-review-current",
	)
	for _, request := range scenario.fixture.router.requestsFor(scenario.factoryDir) {
		if isReviewFailureProviderCommand(request.Command) || strings.EqualFold(request.Command, "python") {
			t.Fatalf("failed-idea completion invoked worker command %q, want logical completion only", request.Command)
		}
	}
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		scenario.marker + "-idea-current":   "failed",
		scenario.marker + "-task-current":   "complete",
		scenario.marker + "-review-current": "complete",
		scenario.marker + "-idea-old":       "failed",
	})
}
