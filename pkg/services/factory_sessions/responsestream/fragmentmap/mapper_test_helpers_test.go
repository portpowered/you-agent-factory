package fragmentmap_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
)

type mappedFragmentEnvelopeExpectation struct {
	sessionID          string
	runID              string
	sequence           int64
	recordedAt         time.Time
	dispatchID         string
	providerSessionRef string
	wantEmptyItemID    bool
	wantNonEmptyItemID bool
}

func assertMappedFragmentEnvelope(
	t *testing.T,
	event responseevents.FactoryResponseEvent,
	want mappedFragmentEnvelopeExpectation,
) {
	t.Helper()

	if event.FactorySessionID != want.sessionID || event.RunID != want.runID {
		t.Fatalf("session/run = %q/%q, want %q/%q", event.FactorySessionID, event.RunID, want.sessionID, want.runID)
	}
	if event.Sequence != want.sequence || !event.RecordedAt.Equal(want.recordedAt) {
		t.Fatalf("sequence/recordedAt = %d/%v, want %d/%v", event.Sequence, event.RecordedAt, want.sequence, want.recordedAt)
	}
	if event.DispatchID != want.dispatchID {
		t.Fatalf("dispatchId = %q, want %q", event.DispatchID, want.dispatchID)
	}
	if event.ProviderSessionRef != want.providerSessionRef {
		t.Fatalf("providerSessionRef = %q, want %q", event.ProviderSessionRef, want.providerSessionRef)
	}
	if want.wantNonEmptyItemID && event.ItemID == "" {
		t.Fatal("itemId must be synthesized for response fragments")
	}
	if want.wantEmptyItemID && event.ItemID != "" {
		t.Fatal("mapped fragment must not synthesize itemId")
	}
}

func assertMessageDeltaPayload(t *testing.T, payloadJSON json.RawMessage, wantTextDelta string) {
	t.Helper()

	var payload responseevents.MessageDeltaPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("unmarshal message delta payload: %v", err)
	}
	if payload.ContentBlockIndex != 0 || payload.ContentBlockKind != responseevents.ContentBlockText {
		t.Fatalf("delta block = index %d kind %q, want 0/TEXT", payload.ContentBlockIndex, payload.ContentBlockKind)
	}
	if payload.TextDelta != wantTextDelta {
		t.Fatalf("textDelta = %q, want %q", payload.TextDelta, wantTextDelta)
	}
}

func assertStreamGapPayload(
	t *testing.T,
	payloadJSON json.RawMessage,
	wantFromSequence int64,
	wantToSequence int64,
	wantFirstAvailableSequence int64,
	wantReason string,
) {
	t.Helper()

	var payload responseevents.StreamGapPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("unmarshal stream gap payload: %v", err)
	}
	if payload.FromSequence != wantFromSequence || payload.ToSequence != wantToSequence {
		t.Fatalf("gap bounds = %d/%d, want %d/%d", payload.FromSequence, payload.ToSequence, wantFromSequence, wantToSequence)
	}
	if payload.FirstAvailableSequence != wantFirstAvailableSequence {
		t.Fatalf("first available sequence = %d, want %d", payload.FirstAvailableSequence, wantFirstAvailableSequence)
	}
	if payload.Reason != wantReason {
		t.Fatalf("gap reason = %q, want %q", payload.Reason, wantReason)
	}
}
