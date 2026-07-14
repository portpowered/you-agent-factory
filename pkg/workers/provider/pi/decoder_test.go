package pi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/pi"
)

func TestDecoderMapsSessionAgentTurnLifecycle(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/message_lifecycle_subtypes.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertValidPiDrafts(t, drafts)

	session := findPiDraft(drafts, responseevents.KindSession, responseevents.PhaseStarted)
	if session == nil || session.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("session draft = %#v", session)
	}
	runStarted := findPiDraft(drafts, responseevents.KindRun, responseevents.PhaseStarted)
	runCompleted := findPiDraft(drafts, responseevents.KindRun, responseevents.PhaseCompleted)
	if runStarted == nil || runCompleted == nil {
		t.Fatalf("run lifecycle missing: %#v", drafts)
	}
	if runStarted.ProviderSessionRef != "pi-session-lifecycle" || runCompleted.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("run drafts lost provider session ref: start=%#v end=%#v", runStarted, runCompleted)
	}

	turns := piDraftsByKindAndPhase(drafts, responseevents.KindTurn, responseevents.PhaseStarted)
	turnEnds := piDraftsByKindAndPhase(drafts, responseevents.KindTurn, responseevents.PhaseCompleted)
	if len(turns) != 1 || len(turnEnds) != 1 || turns[0].TurnID == "" || turns[0].TurnID != turnEnds[0].TurnID {
		t.Fatalf("turn lifecycle correlation missing: starts=%#v ends=%#v", turns, turnEnds)
	}
	var turnPayload responseevents.TurnPayload
	decodePiPayload(t, turns[0], &turnPayload)
	if turnPayload.TurnIndex != 1 || turnPayload.Status != "started" {
		t.Fatalf("turn start payload = %#v", turnPayload)
	}
}

func TestDecoderMapsMessageSubtypesByExactAssistantMessageEvent(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/message_lifecycle_subtypes.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}

	messageStarted := findPiDraft(drafts, responseevents.KindMessage, responseevents.PhaseStarted)
	if messageStarted == nil || messageStarted.ItemID != "msg-lifecycle" || messageStarted.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("message start = %#v", messageStarted)
	}

	textDeltas := piDraftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseDelta)
	if len(textDeltas) != 1 || textDeltas[0].ItemID != "msg-lifecycle" {
		t.Fatalf("text deltas = %#v", textDeltas)
	}
	var textPayload responseevents.MessageDeltaPayload
	decodePiPayload(t, textDeltas[0], &textPayload)
	if textPayload.TextDelta != "Hello " || textPayload.ContentBlockKind != responseevents.ContentBlockText {
		t.Fatalf("text delta payload = %#v", textPayload)
	}
	if textDeltas[0].Provenance.NativeEventType != "text_delta" {
		t.Fatalf("text delta provenance = %#v", textDeltas[0].Provenance)
	}

	reasoning := piDraftsByKindAndPhase(drafts, responseevents.KindReasoning, responseevents.PhaseDelta)
	if len(reasoning) != 1 || reasoning[0].ItemID != "msg-lifecycle" {
		t.Fatalf("reasoning deltas = %#v", reasoning)
	}
	var reasoningPayload responseevents.ReasoningPayload
	decodePiPayload(t, reasoning[0], &reasoningPayload)
	if reasoningPayload.SummaryDelta != "considering options" {
		t.Fatalf("reasoning payload = %#v", reasoningPayload)
	}
	if reasoning[0].Provenance.NativeEventType != "thinking_delta" {
		t.Fatalf("reasoning provenance = %#v", reasoning[0].Provenance)
	}

	toolDeltas := piDraftsByKindAndPhase(drafts, responseevents.KindTool, responseevents.PhaseDelta)
	if len(toolDeltas) != 2 {
		t.Fatalf("tool preview deltas = %d, want tool_call_delta and input_json_delta: %#v", len(toolDeltas), toolDeltas)
	}
	if toolDeltas[0].ItemID != "call-lifecycle" || toolDeltas[1].ItemID != "call-lifecycle" {
		t.Fatalf("tool preview identity changed: %#v", toolDeltas)
	}
	if toolDeltas[0].Provenance.NativeEventType != "tool_call_delta" || toolDeltas[1].Provenance.NativeEventType != "input_json_delta" {
		t.Fatalf("tool preview provenance = %#v and %#v", toolDeltas[0].Provenance, toolDeltas[1].Provenance)
	}

	completed := findPiDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if completed == nil || completed.ItemID != "msg-lifecycle" {
		t.Fatalf("completed message = %#v", completed)
	}
	var completedPayload responseevents.MessagePayload
	decodePiPayload(t, *completed, &completedPayload)
	if len(completedPayload.ContentBlocks) != 1 || completedPayload.ContentBlocks[0].Text != "Hello world" {
		t.Fatalf("completed message payload = %#v", completedPayload)
	}
}

func TestDecoderCompleteTextSnapshotSupersedesDeltasAndIsIdempotent(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/snapshot_authoritative_text.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertValidPiDrafts(t, drafts)

	completed := piDraftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if len(completed) != 1 {
		t.Fatalf("completed messages = %d, want one authoritative snapshot: %#v", len(completed), drafts)
	}
	var payload responseevents.MessagePayload
	decodePiPayload(t, completed[0], &payload)
	if payload.ContentBlocks[0].Text != "authoritative text" {
		t.Fatalf("completed payload = %#v, want authoritative text", payload)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed drafts: %v", err)
	}
	if bytes.Contains(encoded, []byte("draft text")) {
		t.Fatalf("completed snapshot retained accumulated draft text: %s", encoded)
	}
}

func TestDecoderChunkedDeliveryPreservesOrderAndIdentity(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/message_lifecycle_subtypes.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	if findPiDraft(drafts, responseevents.KindSession, responseevents.PhaseStarted) == nil {
		t.Fatalf("chunked decode lost session draft: %#v", drafts)
	}
	if findPiDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted) == nil {
		t.Fatalf("chunked decode lost completed message: %#v", drafts)
	}
}

func TestDecoderFlushProcessesUnterminatedRecord(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/snapshot_authoritative_text.jsonl", true)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	completed := findPiDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if completed == nil || completed.ItemID != "msg-snapshot" {
		t.Fatalf("flush did not process final unterminated record: %#v", drafts)
	}
}

func decodePiFixture(t *testing.T, path string, keepFinalRecordUnterminated bool) ([]responseevents.Draft, []adapter.Diagnostic) {
	t.Helper()
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if keepFinalRecordUnterminated {
		fixture = bytes.TrimSuffix(fixture, []byte("\n"))
	}
	decoder, err := pi.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{
		RunID: "run-pi-lifecycle", DispatchID: "dispatch-pi-lifecycle",
	})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	var drafts []responseevents.Draft
	var diagnostics []adapter.Diagnostic
	for offset := 0; offset < len(fixture); {
		size := 17
		if remaining := len(fixture) - offset; remaining < size {
			size = remaining
		}
		decoded, observeErr := decoder.Observe(context.Background(), adapter.Observation{
			Stream: adapter.OutputStreamStdout,
			Chunk:  fixture[offset : offset+size],
		})
		if observeErr != nil {
			t.Fatalf("Observe() error = %v", observeErr)
		}
		drafts = append(drafts, decoded.Drafts...)
		diagnostics = append(diagnostics, decoded.Diagnostics...)
		offset += size
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	return append(drafts, flushed.Drafts...), append(diagnostics, flushed.Diagnostics...)
}

func assertValidPiDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] %s/%s is invalid: %v", index, draft.Kind, draft.Phase, err)
		}
	}
}

func piDraftsByKindAndPhase(drafts []responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase) []responseevents.Draft {
	var matched []responseevents.Draft
	for _, draft := range drafts {
		if draft.Kind == kind && draft.Phase == phase {
			matched = append(matched, draft)
		}
	}
	return matched
}

func findPiDraft(drafts []responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase) *responseevents.Draft {
	for index := range drafts {
		if drafts[index].Kind == kind && drafts[index].Phase == phase {
			return &drafts[index]
		}
	}
	return nil
}

func decodePiPayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
}

func TestDecoderSubtypeDraftsDoNotCollapseToGenericProgress(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/message_lifecycle_subtypes.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	for _, draft := range drafts {
		if draft.Kind == responseevents.KindProgress {
			t.Fatalf("subtype mapping collapsed to generic progress: %#v", draft)
		}
	}
	if len(piDraftsByKindAndPhase(drafts, responseevents.KindReasoning, responseevents.PhaseDelta)) == 0 {
		t.Fatal("thinking_delta did not map to reasoning semantics")
	}
	toolDelta := piDraftsByKindAndPhase(drafts, responseevents.KindTool, responseevents.PhaseDelta)
	if len(toolDelta) == 0 || !strings.Contains(toolDelta[0].Provenance.NativeEventType, "delta") {
		t.Fatalf("tool preview deltas missing: %#v", toolDelta)
	}
}
