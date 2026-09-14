package review_failure_routing

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestReviewFailureRouting_ManualMigrationDispatchesExactReviewOnce proves the
// preserved operator state: same-name historical Work shares the root trace,
// while the current task is in review and its matching review is initial.
// The public batch boundary represents the manual-migration/recovery path.
func TestReviewFailureRouting_ManualMigrationDispatchesExactReviewOnce(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("current review accepted"), nil
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-shared-root-trace"
	name := scenario.marker + "-repeated-delivery"
	oldTaskID := scenario.marker + "-old-task"
	currentTaskID := scenario.marker + "-current-task"
	currentReviewID := scenario.marker + "-current-review"

	scenario.submit(t, scenario.marker+"-history-task", reviewFailureSeed{Name: name, WorkID: oldTaskID, WorkType: "task", State: "failed", TraceID: traceID, Payload: "old-task"})
	scenario.submit(t, scenario.marker+"-history-review", reviewFailureSeed{Name: name, WorkID: scenario.marker + "-old-review", WorkType: "review", State: "fin", TraceID: traceID, Payload: "old-review"})
	scenario.submit(t, scenario.marker+"-manual-task", reviewFailureSeed{Name: name, WorkID: currentTaskID, WorkType: "task", State: "in-review", TraceID: traceID, Payload: "current-task"})
	scenario.submit(t, scenario.marker+"-manual-review", reviewFailureSeed{Name: name, WorkID: currentReviewID, WorkType: "review", State: "init", TraceID: traceID, Payload: "current-review"})

	response := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	if response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("restored review outcome = %q, want ACCEPTED", response.Outcome)
	}
	assertReviewFailureDispatchOutputStates(t, response, map[string]string{currentTaskID: "to-complete", currentReviewID: "complete"})
	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 1 {
		t.Fatalf("manual-migration dispatch count = %d, want exactly one: %#v", len(dispatches), dispatches)
	}
	assertExactReviewFailureInputIDs(t, dispatches[0], currentTaskID, currentReviewID)
}

// TestReviewFailureRouting_RejectionReturnsOneCorrectionToSameOwner proves a
// terminal red review emits one correction packet for the same task owner,
// carries the exact feedback into that processor prompt, and retries review
// with only the replacement child.
func TestReviewFailureRouting_RejectionReturnsOneCorrectionToSameOwner(t *testing.T) {
	t.Parallel()
	const feedback = "review feedback: add the exact failure-routing evidence 4a6d"
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			if call == 2 {
				return reviewFailureRejected(feedback), nil
			}
			return reviewFailureAccepted("corrected current task"), nil
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-shared-root-trace"
	name := scenario.marker + "-repeated-delivery"
	oldTaskID := scenario.marker + "-old-task"
	currentTaskID := scenario.marker + "-current-task"

	scenario.submit(t, scenario.marker+"-history-task", reviewFailureSeed{Name: name, WorkID: oldTaskID, WorkType: "task", State: "failed", TraceID: traceID, Payload: "old-task"})
	scenario.submit(t, scenario.marker+"-history-review", reviewFailureSeed{Name: name, WorkID: scenario.marker + "-old-review", WorkType: "review", State: "fin", TraceID: traceID, Payload: "old-review"})
	scenario.submit(t, scenario.marker+"-current-task", reviewFailureSeed{Name: name, WorkID: currentTaskID, WorkType: "task", State: "init", TraceID: traceID, Payload: "current-task"})

	firstProcess := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "process", 1)[0])
	currentReviewID := outputReviewWorkID(t, firstProcess, "init")
	_ = awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)
	first := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	_ = awaitReviewFailureDispatchResponses(t, stream, "process", 1)
	_ = awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)
	second := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	if first.Outcome != factoryapi.WorkOutcomeRejected || second.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("review outcomes = %q then %q, want REJECTED then ACCEPTED", first.Outcome, second.Outcome)
	}
	if first.Feedback == nil || *first.Feedback != feedback {
		t.Fatalf("first review feedback = %#v, want exact %q", first.Feedback, feedback)
	}
	replacementReviewID := replacementReviewWorkID(t, second)
	providerRequests := reviewFailureProviderRequests(scenario.fixture.router.requestsFor(scenario.factoryDir))
	if len(providerRequests) != 4 {
		t.Fatalf("provider request count = %d, want processor/reviewer/processor/reviewer", len(providerRequests))
	}
	if !strings.Contains(providerCommandPrompt(providerRequests[2]), feedback) {
		t.Fatalf("correction prompt did not carry exact feedback %q", feedback)
	}

	dispatches := reviewFailureDispatches(t, scenario)
	if len(dispatches) != 6 {
		t.Fatalf("dispatch count = %d, want two process/ci-wait/review rounds: %#v", len(dispatches), dispatches)
	}
	process := dispatchesWithTransition(dispatches, "process")
	if len(process) != 2 {
		t.Fatalf("processor dispatches = %d, want initial plus one correction: %#v", len(process), dispatches)
	}
	assertExactReviewFailureInputIDs(t, process[1], currentTaskID)
	assertRejectionReviewDispatches(t, dispatches, currentTaskID, currentReviewID, replacementReviewID)
}

func replacementReviewWorkID(t *testing.T, response factoryapi.DispatchResponseEventPayload) string {
	return outputReviewWorkID(t, response, "complete")
}

func outputReviewWorkID(t *testing.T, response factoryapi.DispatchResponseEventPayload, state string) string {
	t.Helper()
	if response.OutputWork != nil {
		for _, item := range *response.OutputWork {
			if item.WorkId != nil && item.WorkTypeName != nil && *item.WorkTypeName == "review" && item.State != nil && item.State.Name == state {
				return *item.WorkId
			}
		}
	}
	t.Fatalf("dispatch output has no review in state %q: %#v", state, response)
	return ""
}

func reviewFailureProviderRequests(requests []platformprocess.CommandRequest) []platformprocess.CommandRequest {
	var providerRequests []platformprocess.CommandRequest
	for _, request := range requests {
		if isReviewFailureProviderCommand(request.Command) {
			providerRequests = append(providerRequests, request)
		}
	}
	return providerRequests
}

func dispatchesWithTransition(dispatches []support.DispatchEventObservation, transition string) []support.DispatchEventObservation {
	var matched []support.DispatchEventObservation
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == transition {
			matched = append(matched, dispatch)
		}
	}
	return matched
}

func assertRejectionReviewDispatches(t *testing.T, dispatches []support.DispatchEventObservation, taskID, originalReviewID, replacementReviewID string) {
	t.Helper()
	reviews := dispatchesWithTransition(dispatches, "review")
	if len(reviews) != 2 {
		t.Fatalf("review dispatches = %d, want two exact attempts: %#v", len(reviews), reviews)
	}
	assertExactReviewFailureInputIDs(t, reviews[0], taskID, originalReviewID)
	assertExactReviewFailureInputIDs(t, reviews[1], taskID, replacementReviewID)
}
