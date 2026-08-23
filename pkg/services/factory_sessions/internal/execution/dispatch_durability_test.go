package factorysessionexecution

import (
	"encoding/json"
	"testing"
)

func TestDispatchReadPreservesLatestLifecycleCursorAndDefaultsUnconfirmed(t *testing.T) {
	t.Parallel()

	input := []DispatchSummary{{
		ID: "dispatch-1", Status: DispatchStatusCompleted, DispatchKind: "JAVASCRIPT_AGENT",
		ConfirmationState: ConfirmationStateConfirmed,
	}}
	events := []json.RawMessage{
		json.RawMessage(`{"type":"DISPATCH_QUEUED","context":{"dispatchId":"dispatch-1","sequence":2}}`),
		json.RawMessage(`{"type":"DISPATCH_RECONCILED","context":{"dispatchId":"dispatch-1","sequence":7}}`),
		json.RawMessage(`{"type":"DISPATCH_INTERRUPTED","context":{"dispatchId":"other","sequence":9}}`),
	}

	result := dispatchesForRead(input, events)
	if len(result) != 1 {
		t.Fatalf("dispatchesForRead() = %#v, want one dispatch", result)
	}
	if result[0].ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("confirmation state = %q, want UNCONFIRMED", result[0].ConfirmationState)
	}
	if !result[0].StateSequenceKnown || result[0].StateSequence != 7 {
		t.Fatalf("state cursor = %#v, want known sequence 7", result[0])
	}
	if input[0].ConfirmationState != ConfirmationStateConfirmed || input[0].StateSequenceKnown {
		t.Fatalf("input dispatch mutated = %#v", input[0])
	}
}
