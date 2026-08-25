package factorysessionexecution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
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

func TestLiveDispatchListAndDetailConfirmAfterCompletedFlush(t *testing.T) {
	const (
		sessionID    = "dur-sess-dispatch-watermark"
		generationID = "generation-dispatch-watermark"
		dispatchID   = "dispatch-watermark"
	)
	reader := &dispatchDurabilityReader{}
	service := &JavaScriptRuntimeService{
		sessions: map[string]*runtimeSessionState{
			sessionID: {
				session: SessionReadResult{
					SessionID:        sessionID,
					OrchestratorKind: "JAVASCRIPT",
				},
				dispatches: []DispatchSummary{{
					ID: dispatchID, Status: DispatchStatusCompleted, DispatchKind: "AGENT",
				}},
				events: []json.RawMessage{
					json.RawMessage(`{"type":"DISPATCH_RECONCILED","context":{"dispatchId":"dispatch-watermark","sequence":7}}`),
				},
			},
		},
	}
	service.SetDispatchDurability(reader, generationID)

	list, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches before flush: %v", err)
	}
	if len(list.Dispatches) != 1 || list.Dispatches[0].ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("ListDispatches before flush = %#v, want UNCONFIRMED", list.Dispatches)
	}
	if reader.calls != 1 {
		t.Fatalf("ListDispatches watermark calls = %d, want one sample", reader.calls)
	}

	detail, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch before flush: %v", err)
	}
	if detail.ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("GetDispatch before flush = %#v, want UNCONFIRMED", detail)
	}

	reader.available = true
	reader.cursor = recordings.CanonicalEventCursor{StreamGenerationID: generationID, Sequence: 7}
	list, err = service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after flush: %v", err)
	}
	if list.Dispatches[0].ConfirmationState != ConfirmationStateConfirmed {
		t.Fatalf("ListDispatches after flush = %#v, want CONFIRMED", list.Dispatches)
	}

	detail, err = service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after flush: %v", err)
	}
	if detail.ConfirmationState != ConfirmationStateConfirmed {
		t.Fatalf("GetDispatch after flush = %#v, want CONFIRMED", detail)
	}
	if reader.calls != 4 {
		t.Fatalf("dispatch read watermark calls = %d, want one sample per response", reader.calls)
	}
}

type dispatchDurabilityReader struct {
	cursor    recordings.CanonicalEventCursor
	available bool
	calls     int
}

func (reader *dispatchDurabilityReader) CompletedFlushWatermark(
	string,
) (recordings.CanonicalEventCursor, bool) {
	reader.calls++
	return reader.cursor, reader.available
}
