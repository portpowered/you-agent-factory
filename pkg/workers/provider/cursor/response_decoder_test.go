package cursor

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

func TestResponseEventDecoder_MapsCursorInitializationAndAssistantFixture(t *testing.T) {
	raw := readCursorStreamFixture(t, "session_assistant.ndjson")
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if len(result.Drafts) != 3 {
		t.Fatalf("draft count = %d, want 3: %#v", len(result.Drafts), result.Drafts)
	}
	assertCursorSessionDraft(t, result.Drafts[0])
	assertCursorMessageDelta(t, result.Drafts[1], "Plan ")
	assertCursorMessageDelta(t, result.Drafts[2], "done")
	for index, draft := range result.Drafts {
		if draft.Kind == responseevents.KindReasoning {
			t.Fatalf("draft[%d] inferred reasoning: %#v", index, draft)
		}
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
	}
}

func TestResponseEventDecoder_ChunkingAndFinalRecordTerminationDoNotChangeSemantics(t *testing.T) {
	raw := []byte(strings.TrimSuffix(string(readCursorStreamFixture(t, "unterminated_assistant.ndjson")), "\n"))
	whole := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	splitAt := len(raw) / 2
	chunked := decodeCursorObservations(t, []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: raw[:splitAt]},
		{Stream: adapter.OutputStreamStdout, Chunk: raw[splitAt:]},
	})
	terminated := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: append(append([]byte(nil), raw...), '\n')}})

	if !reflect.DeepEqual(whole, chunked) || !reflect.DeepEqual(whole, terminated) {
		t.Fatalf("semantic results differ by delivery: whole=%#v chunked=%#v terminated=%#v", whole, chunked, terminated)
	}
	if len(whole.Drafts) != 2 {
		t.Fatalf("draft count = %d, want session and message", len(whole.Drafts))
	}
}

func TestResponseEventDecoder_BoundsDiagnosticsAndRecoversAfterMalformedAndUnknownRecords(t *testing.T) {
	privatePayload := strings.Repeat("private-prompt-", 30)
	raw := []byte("{not-json \"" + privatePayload + "\"}\n" +
		"{\"type\":\"future_cursor_shape\",\"prompt\":\"" + privatePayload + "\"}\n" +
		"{\"type\":\"assistant\",\"timestamp_ms\":9,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"recovered\"}]},\"session_id\":\"cursor-session-safe\"}\n")
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if len(result.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want malformed and unknown diagnostics", result.Diagnostics)
	}
	for _, diagnostic := range result.Diagnostics {
		if len(diagnostic.Message) > 160 || strings.Contains(diagnostic.Message, privatePayload) {
			t.Fatalf("unsafe diagnostic = %#v", diagnostic)
		}
	}
	if len(result.Drafts) != 1 {
		t.Fatalf("drafts = %#v, want later valid assistant draft", result.Drafts)
	}
	assertCursorMessageDelta(t, result.Drafts[0], "recovered")
}

func decodeCursorObservations(t *testing.T, observations []adapter.Observation) adapter.DecodeResult {
	t.Helper()
	decoder := NewResponseEventDecoder(adapter.DecoderContext{RunID: "run-cursor-1", DispatchID: "dispatch-cursor-1"})
	var combined adapter.DecodeResult
	for _, observation := range observations {
		result, err := decoder.Observe(context.Background(), observation)
		if err != nil {
			t.Fatalf("Observe() error = %v", err)
		}
		combined = appendCursorDecodeResult(combined, result)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	return appendCursorDecodeResult(combined, flushed)
}

func assertCursorSessionDraft(t *testing.T, draft responseevents.Draft) {
	t.Helper()
	if draft.Kind != responseevents.KindSession || draft.Phase != responseevents.PhaseStarted {
		t.Fatalf("session kind/phase = %s/%s", draft.Kind, draft.Phase)
	}
	if draft.ProviderSessionRef != "cursor-session-123" || draft.ItemID != "cursor-session-123" {
		t.Fatalf("session correlation = %#v", draft)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.NativeEventType != "system" || draft.Provenance.NativeEventSubtype != "init" {
		t.Fatalf("session provenance = %#v", draft.Provenance)
	}
}

func assertCursorMessageDelta(t *testing.T, draft responseevents.Draft, wantText string) {
	t.Helper()
	if draft.Kind != responseevents.KindMessage || draft.Phase != responseevents.PhaseDelta {
		t.Fatalf("message kind/phase = %s/%s", draft.Kind, draft.Phase)
	}
	if draft.RunID != "run-cursor-1" || draft.DispatchID != "dispatch-cursor-1" || draft.ItemID != "cursor-message/run-cursor-1" {
		t.Fatalf("message correlation = %#v", draft)
	}
	if draft.ProviderSessionRef != "cursor-session-123" && draft.ProviderSessionRef != "cursor-session-safe" && draft.ProviderSessionRef != "cursor-session-unterminated" {
		t.Fatalf("provider session ref = %q", draft.ProviderSessionRef)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.NativeEventType != "assistant" || draft.Provenance.Representation != responseevents.RepresentationDelta {
		t.Fatalf("message provenance = %#v", draft.Provenance)
	}
	var payload responseevents.MessageDeltaPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("decode message payload: %v", err)
	}
	if payload.ContentBlockKind != responseevents.ContentBlockText || payload.TextDelta != wantText {
		t.Fatalf("message payload = %#v, want text %q", payload, wantText)
	}
}

func readCursorStreamFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
