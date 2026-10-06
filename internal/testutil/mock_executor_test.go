package testutil_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProviderDoublesUseScenarioContinuationCapability(t *testing.T) {
	t.Parallel()
	for _, supported := range []bool{false, true} {
		support := &scenarioContinuationCapability{supported: supported}
		doubles := []providers.Service{
			testutil.NativeProvider{ContinuationSupport: support},
			testutil.ProviderServiceAdapter{ContinuationSupport: support},
		}
		for _, double := range doubles {
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "captured-session"}
			got, err := double.SupportsContinuation(t.Context(), reference)
			if err != nil || got != supported || support.reference != reference {
				t.Fatalf("capability = %v, %v, reference = %#v; want %v, %#v", got, err, support.reference, supported, reference)
			}
			if _, err := double.SupportsContinuation(t.Context(), providers.SessionRef{}); err == nil {
				t.Fatal("invalid reference accepted")
			}
		}
	}
}

type scenarioContinuationCapability struct {
	supported bool
	reference providers.SessionRef
}

func (support *scenarioContinuationCapability) SupportsContinuation(_ context.Context, reference providers.SessionRef) (bool, error) {
	support.reference = reference
	return support.supported, reference.Validate()
}

func TestMockExecutorTracksCallsAndFallsBackToAccepted(t *testing.T) {
	t.Parallel()

	mock := testutil.NewMockExecutor(
		workers.WorkResult{Outcome: workers.OutcomeAccepted},
		workers.WorkResult{Outcome: workers.OutcomeRejected},
	)
	for index, dispatch := range []work.WorkDispatch{
		{DispatchID: "dispatch-1", TransitionID: "transition-1"},
		{DispatchID: "dispatch-2", TransitionID: "transition-2"},
		{DispatchID: "dispatch-3", TransitionID: "transition-3"},
	} {
		result, err := mock.Execute(t.Context(), dispatch)
		if err != nil {
			t.Fatalf("Execute(%d) error = %v", index, err)
		}
		want := workers.OutcomeAccepted
		if index == 1 {
			want = workers.OutcomeRejected
		}
		if result.Outcome != want {
			t.Errorf("Execute(%d) outcome = %s, want %s", index, result.Outcome, want)
		}
		if result.DispatchID != dispatch.DispatchID || result.TransitionID != dispatch.TransitionID {
			t.Errorf("Execute(%d) correlation = (%q, %q), want (%q, %q)",
				index,
				result.DispatchID,
				result.TransitionID,
				dispatch.DispatchID,
				dispatch.TransitionID,
			)
		}
	}

	if got := mock.CallCount(); got != 3 {
		t.Fatalf("CallCount() = %d, want 3", got)
	}
	if got := mock.LastCall().DispatchID; got != "dispatch-3" {
		t.Fatalf("LastCall().DispatchID = %q, want dispatch-3", got)
	}
	if calls := mock.Calls(); len(calls) != 3 || calls[0].DispatchID != "dispatch-1" {
		t.Fatalf("Calls() = %#v, want detached ordered calls", calls)
	}
}
