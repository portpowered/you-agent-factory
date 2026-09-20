package review_failure_routing

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const reviewFailureReviewedTaskCompletionTransition = "complete-reviewed-task-after-failed-idea"

// TestReviewFailureRouting_ManualMigrationDispatchesExactReviewOnce proves the
// manual operator state where same-name historical Work shares the root trace,
// while the current task is in review and the current review is initial. The
// transition-generated test below owns the exact parent-child regression.
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
// transition-generated review dispatches, one rejection returns one correction
// to the same task Work, the accepted replacement leaves no matching task in
// review or review in init, and every observed dispatch has a response.
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
	works := scenario.listWorks(t)
	assertReviewFailureWorkStates(t, works, map[string]string{
		currentTaskID:       "to-complete",
		replacementReviewID: "complete",
	})
	assertNoReviewFailureStrands(t, works, name)
	assertNoIncompleteReviewFailureDispatches(t, dispatches)
}

// TestReviewFailureRouting_ReviewedTaskCompletesAfterFailedIdea proves an
// accepted task lifecycle can reach terminal completion while its failed idea
// and completed review remain readable through the same Factory Session.
func TestReviewFailureRouting_ReviewedTaskCompletesAfterFailedIdea(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("review accepted the exact merged candidate"), nil
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-shared-root-trace"
	name := scenario.marker + "-reviewed-task"
	failedIdeaID := scenario.marker + "-failed-idea"
	currentTaskID := scenario.marker + "-current-task"

	scenario.submit(t, scenario.marker+"-failed-idea-request", reviewFailureSeed{
		Name: name, WorkID: failedIdeaID, WorkType: "idea", State: "failed",
		TraceID: traceID, Payload: "retained failed idea",
	})
	scenario.submit(t, scenario.marker+"-current-task-request", reviewFailureSeed{
		Name: name, WorkID: currentTaskID, WorkType: "task", State: "init",
		TraceID: traceID, Payload: "reviewed task",
	})

	currentReviewID := acceptReviewFailureTaskThroughSession(t, scenario, stream, currentTaskID)
	completion := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(
		t, stream, reviewFailureReviewedTaskCompletionTransition, 1,
	)[0])
	if completion.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("reviewed-task completion outcome = %q, want ACCEPTED", completion.Outcome)
	}
	assertExactReviewFailureDispatchOutput(t, completion, map[string]reviewFailureOutputExpectation{
		failedIdeaID:    {workType: "idea", state: "failed"},
		currentTaskID:   {workType: "task", state: "complete"},
		currentReviewID: {workType: "review", state: "complete"},
	})

	dispatches := reviewFailureDispatches(t, scenario)
	completions := dispatchesWithTransition(dispatches, reviewFailureReviewedTaskCompletionTransition)
	if len(completions) != 1 {
		t.Fatalf("reviewed-task completion dispatches = %d, want one for exact Work IDs: %#v", len(completions), completions)
	}
	assertExactReviewFailureInputIDs(t, completions[0], failedIdeaID, currentTaskID, currentReviewID)
	assertNoIncompleteReviewFailureDispatches(t, dispatches)
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		failedIdeaID: "failed", currentTaskID: "complete", currentReviewID: "complete",
	})
}

// TestReviewFailureRouting_FailedIdeaRequiredBeforeReviewedTaskCompletion
// proves a completed review and task at to-complete do not complete without
// the matching failed idea input.
func TestReviewFailureRouting_FailedIdeaRequiredBeforeReviewedTaskCompletion(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("review accepted the candidate"), nil
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-shared-root-trace"
	name := scenario.marker + "-reviewed-task-without-idea"
	currentTaskID := scenario.marker + "-current-task"

	scenario.submit(t, scenario.marker+"-current-task-request", reviewFailureSeed{
		Name: name, WorkID: currentTaskID, WorkType: "task", State: "init",
		TraceID: traceID, Payload: "reviewed task without a failed idea",
	})
	currentReviewID := acceptReviewFailureTaskThroughSession(t, scenario, stream, currentTaskID)

	dispatches := reviewFailureDispatches(t, scenario)
	completions := dispatchesWithTransition(dispatches, reviewFailureReviewedTaskCompletionTransition)
	if len(completions) != 0 {
		t.Fatalf("reviewed-task completion dispatches without failed idea = %d, want none: %#v", len(completions), completions)
	}
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		currentTaskID: "to-complete", currentReviewID: "complete",
	})
}

func TestReviewFailureRouting_StaleSameNameReviewCannotCompleteCurrentTask(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{})
	_ = scenario.eventStream(t)

	name := scenario.marker + "-reused-delivery"
	currentTraceID := scenario.marker + "-candidate-current"
	staleTraceID := scenario.marker + "-candidate-stale"
	failedIdeaID := scenario.marker + "-failed-idea"
	currentTaskID := scenario.marker + "-current-task"
	staleTaskID := scenario.marker + "-stale-task"
	staleReviewID := scenario.marker + "-stale-review"
	scenario.submit(t, scenario.marker+"-failed-idea", reviewFailureSeed{Name: name, WorkID: failedIdeaID, WorkType: "idea", State: "failed", TraceID: currentTraceID})
	scenario.submit(t, scenario.marker+"-stale-task", reviewFailureSeed{Name: name, WorkID: staleTaskID, WorkType: "task", State: "failed", TraceID: staleTraceID})
	scenario.submit(t, scenario.marker+"-stale-review", reviewFailureSeed{Name: name, WorkID: staleReviewID, WorkType: "review", State: "complete", TraceID: staleTraceID})
	scenario.submit(t, scenario.marker+"-current-task", reviewFailureSeed{Name: name, WorkID: currentTaskID, WorkType: "task", State: "to-complete", TraceID: currentTraceID})

	assertReviewFailureCompletionDoesNotFire(t, scenario, map[string]string{
		failedIdeaID:  "failed",
		currentTaskID: "to-complete",
		staleTaskID:   "failed",
		staleReviewID: "complete",
	})
}

func TestReviewFailureRouting_MismatchedCompletionIdentityOrNameDoesNotFire(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		ideaNameMismatch   bool
		ideaTraceMismatch  bool
		reviewNameMismatch bool
	}{
		{name: "stale failed idea identity", ideaTraceMismatch: true},
		{name: "failed idea name differs", ideaNameMismatch: true},
		{name: "completed review name differs", reviewNameMismatch: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{})
			_ = scenario.eventStream(t)

			name := scenario.marker + "-current-task"
			traceID := scenario.marker + "-current-candidate"
			ideaName := name
			ideaTraceID := traceID
			reviewName := name
			if tc.ideaNameMismatch {
				ideaName += "-stale"
			}
			if tc.ideaTraceMismatch {
				ideaTraceID += "-stale"
			}
			if tc.reviewNameMismatch {
				reviewName += "-stale"
			}
			failedIdeaID := scenario.marker + "-failed-idea"
			currentTaskID := scenario.marker + "-current-task"
			currentReviewID := scenario.marker + "-current-review"
			scenario.submit(t, scenario.marker+"-mismatched-idea", reviewFailureSeed{Name: ideaName, WorkID: failedIdeaID, WorkType: "idea", State: "failed", TraceID: ideaTraceID})
			scenario.submit(t, scenario.marker+"-mismatched-review", reviewFailureSeed{Name: reviewName, WorkID: currentReviewID, WorkType: "review", State: "complete", TraceID: traceID})
			scenario.submit(t, scenario.marker+"-mismatched-task", reviewFailureSeed{Name: name, WorkID: currentTaskID, WorkType: "task", State: "to-complete", TraceID: traceID})

			assertReviewFailureCompletionDoesNotFire(t, scenario, map[string]string{
				failedIdeaID:    "failed",
				currentTaskID:   "to-complete",
				currentReviewID: "complete",
			})
		})
	}
}

type reviewFailureAuthorizationCase struct {
	name     string
	evidence string
	feedback string
}

func TestReviewFailureRouting_UnauthorizedReviewEvidenceKeepsFeedbackActionable(t *testing.T) {
	// Keep the authorization matrix sequential: shared fixture routing matches
	// scenario markers by substring, so concurrent rf-1/rf-10 sessions can alias.
	cases := []reviewFailureAuthorizationCase{
		{
			name:     "missing approval",
			evidence: "candidate head 7d9c3a1245e6f7a8091b2c3d4e5f60718293a4b5; approval missing",
			feedback: "approval evidence is missing for candidate head 7d9c3a1245e6f7a8091b2c3d4e5f60718293a4b5",
		},
		{
			name:     "missing merge",
			evidence: "candidate head 8e0d4b2356f7081a92b3c4d5e6f708192a3b4c56; merge evidence missing",
			feedback: "candidate head 8e0d4b2356f7081a92b3c4d5e6f708192a3b4c56 has no merge evidence",
		},
		{
			name:     "merged head differs",
			evidence: "candidate head 9f1e5c346708192ab3c4d5e6f708192ab3c4d567; merged head 0123456789abcdef0123456789abcdef01234567",
			feedback: "merged head does not contain candidate head 9f1e5c346708192ab3c4d5e6f708192ab3c4d567",
		},
		{
			name:     "reviewed head differs",
			evidence: "candidate head a02f6d4578192ab3c4d5e6f708192ab3c4d5678; reviewed head fedcba9876543210fedcba9876543210fedcba98",
			feedback: "reviewed head fedcba9876543210fedcba9876543210fedcba98 differs from candidate head a02f6d4578192ab3c4d5e6f708192ab3c4d5678",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runReviewFailureAuthorizationCase(t, tc)
		})
	}
}

func runReviewFailureAuthorizationCase(t *testing.T, tc reviewFailureAuthorizationCase) {
	t.Helper()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, call int) (platformprocess.CommandResult, error) {
			switch call {
			case 2:
				return reviewFailureRejected(tc.feedback), nil
			case 4:
				return platformprocess.CommandResult{}, errors.New("controlled reviewer failure after rejected evidence")
			default:
				return reviewFailureAccepted("processor completed the correction attempt"), nil
			}
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-candidate"
	name := scenario.marker + "-reviewed-task"
	taskID := scenario.marker + "-current-task"
	ideaID := scenario.marker + "-failed-idea"
	scenario.submit(t, scenario.marker+"-authorization-idea", reviewFailureSeed{Name: name, WorkID: ideaID, WorkType: "idea", State: "failed", TraceID: traceID, Payload: tc.evidence})
	scenario.submit(t, scenario.marker+"-authorization-task", reviewFailureSeed{Name: name, WorkID: taskID, WorkType: "task", State: "init", TraceID: traceID, Payload: tc.evidence})

	firstProcess := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "process", 1)[0])
	assertReviewFailureDispatchOutputStates(t, firstProcess, map[string]string{taskID: "awaiting-ci"})
	_ = awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)
	firstReview := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	if firstReview.Outcome != factoryapi.WorkOutcomeRejected || firstReview.Feedback == nil || *firstReview.Feedback != tc.feedback {
		t.Fatalf("review rejection = outcome %q feedback %#v, want REJECTED with exact feedback %q", firstReview.Outcome, firstReview.Feedback, tc.feedback)
	}
	assertNoReviewFailureCompletionDispatch(t, scenario)

	secondProcess := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "process", 1)[0])
	assertReviewFailureDispatchOutputStates(t, secondProcess, map[string]string{taskID: "awaiting-ci"})
	_ = awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)
	failedReview := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	if failedReview.Outcome != factoryapi.WorkOutcomeFailed || failedReview.Error == nil || strings.TrimSpace(*failedReview.Error) == "" {
		var failure string
		if failedReview.Error != nil {
			failure = *failedReview.Error
		}
		t.Fatalf("review failure = outcome %q error %q, want FAILED with an observable error", failedReview.Outcome, failure)
	}
	assertReviewFailureDispatchOutputStates(t, failedReview, map[string]string{taskID: "failed"})
	assertNoReviewFailureCompletionDispatch(t, scenario)
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{ideaID: "failed", taskID: "failed"})
}

func TestReviewFailureRouting_ReplayedCompletionRequestIsExactlyOnce(t *testing.T) {
	t.Parallel()
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{
		provider: func(_ context.Context, _ platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
			return reviewFailureAccepted("review accepted the exact merged candidate"), nil
		},
	})
	stream := scenario.eventStream(t)
	traceID := scenario.marker + "-shared-root-trace"
	name := scenario.marker + "-replayed-completion"
	ideaRequestID := scenario.marker + "-idea-request"
	taskRequestID := scenario.marker + "-task-request"
	failedIdea := reviewFailureSeed{Name: name, WorkID: scenario.marker + "-failed-idea", WorkType: "idea", State: "failed", TraceID: traceID, Payload: "retained failed idea"}
	currentTask := reviewFailureSeed{Name: name, WorkID: scenario.marker + "-current-task", WorkType: "task", State: "init", TraceID: traceID, Payload: "reviewed task"}
	scenario.submit(t, ideaRequestID, failedIdea)
	scenario.submit(t, taskRequestID, currentTask)
	currentReviewID := acceptReviewFailureTaskThroughSession(t, scenario, stream, currentTask.WorkID)
	completion := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(
		t, stream, reviewFailureReviewedTaskCompletionTransition, 1,
	)[0])
	if completion.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("reviewed-task completion outcome = %q, want ACCEPTED", completion.Outcome)
	}
	assertExactReviewFailureDispatchOutput(t, completion, map[string]reviewFailureOutputExpectation{
		failedIdea.WorkID:  {workType: "idea", state: "failed"},
		currentTask.WorkID: {workType: "task", state: "complete"},
		currentReviewID:    {workType: "review", state: "complete"},
	})
	assertReviewFailureCompletionDispatch(t, scenario, failedIdea.WorkID, currentTask.WorkID, currentReviewID)

	scenario.submit(t, taskRequestID, currentTask)

	assertReviewFailureCompletionDispatch(t, scenario, failedIdea.WorkID, currentTask.WorkID, currentReviewID)
	assertReviewFailureWorkStates(t, scenario.listWorks(t), map[string]string{
		failedIdea.WorkID:  "failed",
		currentTask.WorkID: "complete",
		currentReviewID:    "complete",
	})
}

func assertReviewFailureCompletionDoesNotFire(t *testing.T, scenario *reviewFailureScenario, states map[string]string) {
	t.Helper()
	assertReviewFailureWorkStates(t, scenario.listWorks(t), states)
	assertNoReviewFailureCompletionDispatch(t, scenario)
}

func assertNoReviewFailureCompletionDispatch(t *testing.T, scenario *reviewFailureScenario) {
	t.Helper()
	if dispatches := dispatchesWithTransition(reviewFailureDispatches(t, scenario), reviewFailureReviewedTaskCompletionTransition); len(dispatches) != 0 {
		t.Fatalf("reviewed-task completion dispatches = %d, want none: %#v", len(dispatches), dispatches)
	}
}

func assertReviewFailureCompletionDispatch(t *testing.T, scenario *reviewFailureScenario, failedIdeaID, taskID, reviewID string) {
	t.Helper()
	dispatches := dispatchesWithTransition(reviewFailureDispatches(t, scenario), reviewFailureReviewedTaskCompletionTransition)
	if len(dispatches) != 1 {
		t.Fatalf("reviewed-task completion dispatches = %d, want exactly one: %#v", len(dispatches), dispatches)
	}
	assertExactReviewFailureInputIDs(t, dispatches[0], failedIdeaID, taskID, reviewID)
	if dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("reviewed-task completion response = %#v, want one accepted terminal output", dispatches[0].Response)
	}
}

func acceptReviewFailureTaskThroughSession(
	t *testing.T,
	scenario *reviewFailureScenario,
	stream *support.FactoryEventStream,
	taskID string,
) string {
	t.Helper()
	process := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "process", 1)[0])
	if process.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("task process outcome = %q, want ACCEPTED", process.Outcome)
	}
	assertReviewFailureDispatchOutputStates(t, process, map[string]string{taskID: "awaiting-ci"})

	ciWait := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "ci-wait", 1)[0])
	if ciWait.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("task CI outcome = %q, want ACCEPTED", ciWait.Outcome)
	}
	assertReviewFailureDispatchOutputStates(t, ciWait, map[string]string{taskID: "in-review"})

	review := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "review", 1)[0])
	if review.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("task review outcome = %q, want ACCEPTED", review.Outcome)
	}
	reviewID := outputReviewWorkID(t, review, "complete")
	assertReviewFailureDispatchOutputStates(t, review, map[string]string{
		taskID: "to-complete", reviewID: "complete",
	})
	return reviewID
}

type reviewFailureOutputExpectation struct {
	workType string
	state    string
}

func assertExactReviewFailureDispatchOutput(
	t *testing.T,
	payload factoryapi.DispatchResponseEventPayload,
	want map[string]reviewFailureOutputExpectation,
) {
	t.Helper()
	if payload.OutputWork == nil {
		t.Fatalf("dispatch outputWork = nil, want exact Work IDs %v", want)
	}
	seen := make(map[string]struct{}, len(*payload.OutputWork))
	for _, work := range *payload.OutputWork {
		if work.WorkId == nil {
			t.Fatalf("dispatch outputWork contains Work without an ID: %#v", work)
		}
		workID := *work.WorkId
		expected, ok := want[workID]
		if !ok {
			t.Fatalf("dispatch outputWork contains unexpected Work ID %q; want %v", workID, want)
		}
		if _, duplicate := seen[workID]; duplicate {
			t.Fatalf("dispatch outputWork contains duplicate Work ID %q", workID)
		}
		seen[workID] = struct{}{}
		if work.WorkTypeName == nil || *work.WorkTypeName != expected.workType {
			t.Fatalf("dispatch outputWork %q type = %#v, want %q", workID, work.WorkTypeName, expected.workType)
		}
		if work.State == nil || work.State.Name != expected.state {
			t.Fatalf("dispatch outputWork %q state = %#v, want %q", workID, work.State, expected.state)
		}
	}
	for workID := range want {
		if _, ok := seen[workID]; !ok {
			t.Fatalf("dispatch outputWork is missing exact Work ID %q; got %v", workID, seen)
		}
	}
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
