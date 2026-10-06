package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCanonicalCanceledDispatchDropsRoutingAndWorkOutput(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"terminal", "cancellation", "outcome"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			dispatch := workers.WorkstationDispatchResult{Result: canceledRoutingTestResult()}
			switch source {
			case "terminal":
				dispatch.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeCanceled
			case "cancellation":
				dispatch.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
			case "outcome":
				dispatch.Result.Outcome = workers.OutcomeCanceled
			}
			result := canonicalWorkResult(workers.WorkstationDispatchRequest{}, dispatch, errors.New("signal exit"))
			assertCanceledRoutingResult(t, result)
			if result.Error != "native diagnostic" || result.Metrics.RetryCount != 2 {
				t.Fatalf("cancellation lost diagnostics or execution metrics: %#v", result)
			}
		})
	}
}

func TestReplayPlannedCompletionPreservesCancellationAndLiveFailures(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		planned    workers.WorkOutcome
		usePlanned bool
	}{
		{"recorded cancellation", workers.OutcomeCanceled, true},
		{"recorded failure", workers.OutcomeFailed, true},
		{"recorded success cannot hide failure", workers.OutcomeAccepted, false},
		{"recorded continuation cannot hide failure", workers.OutcomeContinue, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			live := workers.WorkResult{Outcome: workers.OutcomeFailed, Error: "native cancellation error"}
			planned := workers.WorkResult{Outcome: test.planned}
			result, applied, err := choosePlannedWorkersResult(workers.WorkstationDispatchRequest{}, live, planned)
			want := live
			if test.usePlanned {
				want = planned
			}
			if err != nil || applied != test.usePlanned || !reflect.DeepEqual(result, want) {
				t.Fatalf("completion = %#v, applied=%t, err=%v; want %#v, applied=%t", result, applied, err, want, test.usePlanned)
			}
		})
	}
}

func TestCanceledMaterializationDropsRoutingWithoutCreatingWork(t *testing.T) {
	t.Parallel()
	service := &countingMaterializationWorkService{}
	result := canceledRoutingTestResult()
	result.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
	result = applyMaterializedWorkerOutput(t.Context(), service, nil, nil, work.WorkDispatch{}, result,
		workers.ProposedOutput{Classification: "done", Feedback: "partial feedback",
			ProposedWork: []workers.ProposedWork{{WorkTypeID: "task", Name: "partial"}}}, true)
	assertCanceledRoutingResult(t, result)
	if service.calls.Load() != 0 {
		t.Fatal("canceled execution materialized proposed Work")
	}
}

func TestCanceledDispatchProjectionDropsClassifierFeedback(t *testing.T) {
	t.Parallel()
	result, err := workstationDispatchResultFromExecute(workers.WorkstationDispatchRequest{}, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeCanceled, ProposedOutputPresent: true,
		Output: workers.ProposedOutput{Classification: "done", Feedback: "partial feedback",
			Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial"}}},
	}, context.Canceled)
	if !errors.Is(err, context.Canceled) || result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("dispatch = %#v, %v; want cancellation", result, err)
	}
	assertCanceledRoutingResult(t, result.Result)
	if result.ProposedOutput == nil || !reflect.DeepEqual(*result.ProposedOutput, workers.ProposedOutput{}) {
		t.Fatalf("canceled proposal = %#v", result.ProposedOutput)
	}
}

func canceledRoutingTestResult() workers.WorkResult {
	return workers.WorkResult{
		Outcome: workers.OutcomeAccepted, Output: "partial", Error: "native diagnostic",
		OutputContent:      []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial"}},
		RecordedOutputWork: []work.FactoryWorkItem{{ID: "partial-work"}},
		StructuredResult:   map[string]any{"partial": true}, StructuredResultPresent: true,
		Feedback: "partial feedback", SelectedClassificationLabel: "done",
		Continuation:    &workers.ProviderContinuationRef{ProviderSessionID: "partial"},
		FailureDetail:   &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown},
		FailureMetadata: &workers.WorkFailureMetadata{Family: workers.WorkFailureFamilyRetryable},
		Metrics:         workers.WorkMetrics{RetryCount: 2},
	}
}

func assertCanceledRoutingResult(t *testing.T, result workers.WorkResult) {
	t.Helper()
	if result.Outcome != workers.OutcomeCanceled || result.Cancellation == nil ||
		result.Output != "" || len(result.OutputContent) != 0 || len(result.RecordedOutputWork) != 0 ||
		result.StructuredResult != nil || result.StructuredResultPresent ||
		result.Feedback != "" || result.SelectedClassificationLabel != "" || result.Continuation != nil ||
		result.FailureDetail != nil || result.FailureMetadata != nil {
		t.Fatalf("canceled result retained routing, output or retry data: %#v", result)
	}
}

func TestBeginWorkerAttemptResolvesForceAndRecoversDetachedResult(t *testing.T) {
	t.Parallel()
	nativeErr := errors.New("signal exit")
	journalErr := errors.New("force acknowledgement unavailable")
	for _, test := range []struct {
		name          string
		forced        bool
		completionErr error
	}{
		{name: "confirmed force", forced: true},
		{name: "unconfirmed force", completionErr: nativeErr},
		{name: "confirmed with persistence loss", forced: true, completionErr: journalErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sessions := &beginRuntimeAttemptService{
				Service: &fakeWorkerSessionsService{},
				resolveResult: func(result workers.WorkstationDispatchResult, _ error) (workers.WorkstationDispatchResult, bool, error) {
					return result, test.forced, test.completionErr
				},
			}
			f := &factoryImpl{
				cfg:          &runtimeConfig{workerSessions: sessions, workerAttempts: sessions, clock: platformclock.Real{}},
				eventHistory: &recordingfixtures.ScriptedRuntimeLedger{},
			}
			request := detachedTargetRequest()
			complete, err := f.BeginWorkerAttempt(t.Context(), &request)
			if err != nil {
				t.Fatal(err)
			}
			original := workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeFailed,
				Output: workers.ProposedOutput{
					Primary:        []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial"}},
					Classification: "done", Feedback: "partial feedback",
					ProposedWork: []workers.ProposedWork{{WorkTypeID: "task", Name: "partial"}},
				},
				StructuredResult: map[string]any{"partial": true}, StructuredResultPresent: true,
				Continuation: &workers.ProviderContinuationRef{ProviderSessionID: "partial"},
				Failure:      &workers.ExecutionFailure{Family: workers.WorkFailureFamilyRetryable, RetryHint: true, Message: "signal exit"},
			}
			resolved, err := complete(t.Context(), original, nativeErr)
			if !errors.Is(err, test.completionErr) {
				t.Fatalf("completion error = %v, want %v", err, test.completionErr)
			}
			assertDirectAttemptForceResult(t, resolved, test.forced)
			saved := resolved.Clone()
			original.Failure.Message = "producer mutation"
			original.Output.Primary[0].Text = "producer mutation"
			resolved.Failure.Message = "caller mutation"
			if len(resolved.Output.Primary) > 0 {
				resolved.Output.Primary[0].Text = "caller mutation"
			}
			recovered, err := complete(t.Context(), workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, nil)
			if !errors.Is(err, test.completionErr) || !reflect.DeepEqual(recovered, saved) || sessions.completeCalls != 1 {
				t.Fatalf("duplicate = %#v, %v, calls = %d; want detached original %#v", recovered, err, sessions.completeCalls, saved)
			}
		})
	}
}

func assertDirectAttemptForceResult(t *testing.T, resolved workers.ExecuteResult, forced bool) {
	t.Helper()
	if !forced {
		if resolved.Outcome != workers.ExecutionOutcomeFailed || len(resolved.Output.Primary) != 1 || resolved.Failure == nil || !resolved.Failure.RetryHint {
			t.Fatalf("unconfirmed force lost native failure: %#v", resolved)
		}
		return
	}
	if resolved.Outcome != workers.ExecutionOutcomeCanceled || resolved.Cancellation == nil || !reflect.DeepEqual(resolved.Output, workers.ProposedOutput{}) || resolved.StructuredResultPresent || resolved.StructuredResult != nil || resolved.Continuation != nil || resolved.Failure == nil || resolved.Failure.RetryHint {
		t.Fatalf("confirmed force retained retry/output: %#v", resolved)
	}
}
