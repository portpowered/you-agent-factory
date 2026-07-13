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

func TestResponseEventDecoder_CorrelatesSafeToolLifecycleFixture(t *testing.T) {
	result := decodeCursorObservations(t, []adapter.Observation{{
		Stream: adapter.OutputStreamStdout,
		Chunk:  readCursorStreamFixture(t, "tool_lifecycle.ndjson"),
	}})

	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
	if len(result.Drafts) != 5 {
		t.Fatalf("draft count = %d, want session and four tool records: %#v", len(result.Drafts), result.Drafts)
	}
	assertCursorToolDraft(t, result.Drafts[1], responseevents.PhaseStarted, "call-read-1", "readToolCall", "started")
	assertCursorToolDraft(t, result.Drafts[2], responseevents.PhaseCompleted, "call-read-1", "readToolCall", "completed")
	assertCursorToolDraft(t, result.Drafts[3], responseevents.PhaseStarted, "call-shell-2", "shell", "started")
	assertCursorToolDraft(t, result.Drafts[4], responseevents.PhaseFailed, "call-shell-2", "shell", "failed")

	var readStarted, readCompleted, shellStarted, shellCompleted responseevents.ToolPayload
	decodeCursorToolPayload(t, result.Drafts[1], &readStarted)
	decodeCursorToolPayload(t, result.Drafts[2], &readCompleted)
	decodeCursorToolPayload(t, result.Drafts[3], &shellStarted)
	decodeCursorToolPayload(t, result.Drafts[4], &shellCompleted)
	for label, summary := range map[string]json.RawMessage{
		"read started":   readStarted.ArgumentsSummary,
		"read completed": readCompleted.ArgumentsSummary,
		"shell started":  shellStarted.ArgumentsSummary,
	} {
		text := string(summary)
		if strings.Contains(text, "private submitted prompt") || strings.Contains(text, "cursor-secret") || strings.Contains(text, "secret-value") {
			t.Fatalf("%s summary exposed sensitive input: %s", label, text)
		}
	}
	var readArguments map[string]any
	if err := json.Unmarshal(readStarted.ArgumentsSummary, &readArguments); err != nil {
		t.Fatalf("decode read arguments: %v", err)
	}
	if readArguments["path"] != "README.md" || readArguments["prompt"] != cursorToolRedactedValue {
		t.Fatalf("read arguments summary = %s, want useful path and redacted prompt", readStarted.ArgumentsSummary)
	}
	if !reflect.DeepEqual(readStarted.ArgumentsSummary, readCompleted.ArgumentsSummary) {
		t.Fatalf("completed arguments = %s, want retained start summary %s", readCompleted.ArgumentsSummary, readStarted.ArgumentsSummary)
	}
	if !strings.Contains(string(readCompleted.ResultSummary), `"path":"README.md"`) ||
		!strings.Contains(string(shellCompleted.ResultSummary), `"message":"command failed"`) {
		t.Fatalf("result summaries lost useful context: read=%s shell=%s", readCompleted.ResultSummary, shellCompleted.ResultSummary)
	}
	for index, draft := range result.Drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
	}
}

func TestResponseEventDecoder_MapsExplicitToolFailureAndCancellationStatuses(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"tool_call","subtype":"started","call_id":"call-fail","tool_call":{"writeToolCall":{"args":{"path":"safe.txt"}}}}`,
		`{"type":"tool_call","subtype":"failed","call_id":"call-fail","tool_call":{"writeToolCall":{"result":{"failure":{"message":"denied"}}}}}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-cancel","tool_call":{"searchToolCall":{"args":{"query":"public docs"}}}}`,
		`{"type":"tool_call","subtype":"canceled","call_id":"call-cancel","tool_call":{"searchToolCall":{"result":{"canceled":{"reason":"user request"}}}}}`,
	}, "\n"))
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if len(result.Drafts) != 4 || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want four tool drafts", result)
	}
	assertCursorToolDraft(t, result.Drafts[1], responseevents.PhaseFailed, "call-fail", "writeToolCall", "failed")
	assertCursorToolDraft(t, result.Drafts[3], responseevents.PhaseCanceled, "call-cancel", "searchToolCall", "canceled")
}

func TestResponseEventDecoder_InvalidToolCallIDIsDiagnosticAndDoesNotMergeTools(t *testing.T) {
	privatePrompt := "private tool prompt"
	raw := []byte(
		`{"type":"tool_call","subtype":"started","call_id":" ","tool_call":{"readToolCall":{"args":{"prompt":"` + privatePrompt + `"}}}}` + "\n" +
			`{"type":"tool_call","subtype":"started","call_id":"call-valid","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}` + "\n" +
			`{"type":"tool_call","subtype":"completed","call_id":"call-valid","tool_call":{"readToolCall":{"result":{"success":{}}}}}`,
	)
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != cursorDiagnosticInvalidToolCall {
		t.Fatalf("diagnostics = %#v, want one invalid-call diagnostic", result.Diagnostics)
	}
	if strings.Contains(result.Diagnostics[0].Message, privatePrompt) {
		t.Fatalf("diagnostic exposed private prompt: %#v", result.Diagnostics[0])
	}
	if len(result.Drafts) != 2 || result.Drafts[0].ItemID != "cursor-tool/call-valid" || result.Drafts[1].ItemID != "cursor-tool/call-valid" {
		t.Fatalf("drafts = %#v, want isolated valid lifecycle", result.Drafts)
	}
}

func TestCursorSafeToolSummaryIsDeterministicBoundedAndRedacted(t *testing.T) {
	large := map[string]any{
		"path":          "README.md",
		"authorization": "Bearer must-not-leak",
		"output":        strings.Repeat("x", cursorToolSummaryStringLimit+20),
	}
	raw, err := json.Marshal(large)
	if err != nil {
		t.Fatal(err)
	}
	first := cursorSafeToolSummary(raw)
	second := cursorSafeToolSummary(raw)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("summaries are not deterministic: %s != %s", first, second)
	}
	var summary map[string]any
	if err := json.Unmarshal(first, &summary); err != nil {
		t.Fatalf("summary is not valid JSON: %v", err)
	}
	if strings.Contains(string(first), "must-not-leak") || summary["authorization"] != cursorToolRedactedValue {
		t.Fatalf("summary did not redact credential: %s", first)
	}
	output, _ := summary["output"].(string)
	if len(output) > cursorToolSummaryStringLimit+3 || !strings.HasSuffix(output, "...") {
		t.Fatalf("bounded output = %q", output)
	}
}

func TestCursorSafeToolSummaryDecodesStructuredFunctionArgumentsAndRejectsUnsafeToolNames(t *testing.T) {
	summary := cursorSafeToolSummary(json.RawMessage(`"{\"path\":\"README.md\",\"password\":\"must-not-leak\"}"`))
	var decoded map[string]any
	if err := json.Unmarshal(summary, &decoded); err != nil {
		t.Fatalf("decode structured string summary: %v", err)
	}
	if decoded["path"] != "README.md" || decoded["password"] != cursorToolRedactedValue {
		t.Fatalf("structured string summary = %s", summary)
	}
	if got := cursorSafeToolName("private prompt: reveal credentials"); got != cursorToolFallbackName {
		t.Fatalf("unsafe tool name = %q, want fallback", got)
	}
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

func assertCursorToolDraft(t *testing.T, draft responseevents.Draft, phase responseevents.Phase, callID, name, status string) {
	t.Helper()
	if draft.Kind != responseevents.KindTool || draft.Phase != phase {
		t.Fatalf("tool kind/phase = %s/%s, want TOOL/%s", draft.Kind, draft.Phase, phase)
	}
	if draft.RunID != "run-cursor-1" || draft.DispatchID != "dispatch-cursor-1" || draft.ItemID != "cursor-tool/"+callID {
		t.Fatalf("tool correlation = %#v", draft)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.NativeEventType != "tool_call" || draft.Provenance.Representation != responseevents.RepresentationNotification {
		t.Fatalf("tool provenance = %#v", draft.Provenance)
	}
	var payload responseevents.ToolPayload
	decodeCursorToolPayload(t, draft, &payload)
	if payload.ToolCallID != callID || payload.ToolName != name || payload.Status != status {
		t.Fatalf("tool payload = %#v, want call=%q name=%q status=%q", payload, callID, name, status)
	}
}

func decodeCursorToolPayload(t *testing.T, draft responseevents.Draft, target *responseevents.ToolPayload) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode tool payload: %v", err)
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
