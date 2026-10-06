package runtime

import (
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
				Outcome:          workers.ExecutionOutcomeFailed,
				Output:           workers.ProposedOutput{Primary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial"}}},
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
	if resolved.Outcome != workers.ExecutionOutcomeCanceled || resolved.Cancellation == nil || len(resolved.Output.Primary) != 0 || resolved.StructuredResultPresent || resolved.StructuredResult != nil || resolved.Continuation != nil || resolved.Failure == nil || resolved.Failure.RetryHint {
		t.Fatalf("confirmed force retained retry/output: %#v", resolved)
	}
}
