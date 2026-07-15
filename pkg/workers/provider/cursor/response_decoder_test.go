package cursor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
)

func TestResponseEventDecoder_SharedConformance(t *testing.T) {
	finalRecord := `{"type":"result","subtype":"success","is_error":false,"result":"conformance done","session_id":"cursor-conformance"}`
	toolLifecycle := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"cursor-conformance"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-conformance","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"cursor-conformance"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-conformance","tool_call":{"readToolCall":{"result":{"success":{"bytes":128}}}},"session_id":"cursor-conformance"}`,
		finalRecord,
	}, "\n"))
	privatePayload := "private conformance prompt"
	testkit.RunDecoderConformance(t, testkit.DecoderConformanceFixture{
		NewDecoder: func(input adapter.DecoderContext) adapter.Decoder { return NewResponseEventDecoder(input) },
		Lifecycle:  []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: toolLifecycle}},
		UnsafeAndRecovering: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(
			`{"type":"future_shape","prompt":"` + privatePayload + `"}` + "\n" + finalRecord + "\n",
		)}},
		UnterminatedFinal: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(finalRecord)}},
		Expected: testkit.DecoderConformanceExpected{
			ProviderRef: "cursor-conformance", MessageItemID: "cursor-message/run-decoder-conformance",
			ToolItemID: "cursor-tool/call-conformance", ToolCallID: "call-conformance", FinalContent: "conformance done",
		},
		ForbiddenDiagnostic: []string{privatePayload},
	})
}

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
	if readArguments["path"] != "README.md" {
		t.Fatalf("read arguments summary = %s, want useful path and redacted prompt", readStarted.ArgumentsSummary)
	}
	assertCursorSummaryRedactsKey(t, readArguments, readStarted.ArgumentsSummary, "prompt")
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

func TestResponseEventDecoder_RedactsCredentialVariantsInArgumentsAndResults(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"tool_call","subtype":"started","call_id":"call-credentials","tool_call":{"readToolCall":{"args":{"path":"README.md","api.key":"sk-live-secret","nested":{"value":"sk-nested-secret"}}}}}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-credentials","tool_call":{"readToolCall":{"result":{"success":{"output":"updated","access.token":"sk-result-secret","nested":[{"value":"ghp_nested_result_secret"}]}}}}}`,
	}, "\n"))
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})
	if len(result.Drafts) != 2 || len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want a correlated tool lifecycle", result)
	}

	var started, completed responseevents.ToolPayload
	decodeCursorToolPayload(t, result.Drafts[0], &started)
	decodeCursorToolPayload(t, result.Drafts[1], &completed)
	for label, summary := range map[string]json.RawMessage{
		"arguments": started.ArgumentsSummary,
		"results":   completed.ResultSummary,
	} {
		if len(summary) > cursorToolSummaryEncodedLimit {
			t.Fatalf("%s summary length = %d, want at most %d: %s", label, len(summary), cursorToolSummaryEncodedLimit, summary)
		}
		for _, forbidden := range []string{"api.key", "access.token", "sk-live-secret", "sk-nested-secret", "sk-result-secret", "ghp_nested_result_secret"} {
			if strings.Contains(string(summary), forbidden) {
				t.Fatalf("%s summary exposed credential material %q: %s", label, forbidden, summary)
			}
		}
		var decoded any
		if err := json.Unmarshal(summary, &decoded); err != nil {
			t.Fatalf("decode %s summary: %v", label, err)
		}
		if !cursorSummaryContainsValue(decoded, cursorToolRedactedValue) {
			t.Fatalf("%s summary did not retain a redaction marker: %s", label, summary)
		}
	}
	if !strings.Contains(string(started.ArgumentsSummary), `"path":"README.md"`) ||
		!strings.Contains(string(completed.ResultSummary), `"output":"updated"`) {
		t.Fatalf("summaries lost safe context: arguments=%s results=%s", started.ArgumentsSummary, completed.ResultSummary)
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

func TestResponseEventDecoder_ReconnectRetainsCorrelationForRecoveredCompletion(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"cursor-before-reconnect"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-recovered","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"cursor-before-reconnect"}`,
		`{"type":"system","subtype":"init","session_id":"cursor-after-reconnect"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-recovered","tool_call":{"readToolCall":{"result":{"success":{"bytes":128}}}},"session_id":"cursor-after-reconnect"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"cursor-after-reconnect"}`,
	}, "\n"))
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if gap := cursorDraftOfKind(result.Drafts, responseevents.KindStreamGap); gap != nil {
		t.Fatalf("recovered completion also emitted a gap: %#v", *gap)
	}
	tools := cursorDraftsOfKind(result.Drafts, responseevents.KindTool)
	if len(tools) != 2 {
		t.Fatalf("tool drafts = %#v, want one recovered lifecycle", tools)
	}
	assertCursorToolDraft(t, tools[0], responseevents.PhaseStarted, "call-recovered", "readToolCall", "started")
	assertCursorToolDraft(t, tools[1], responseevents.PhaseCompleted, "call-recovered", "readToolCall", "completed")
}

func TestResponseEventDecoder_ReconnectReportsEveryStillUnresolvedToolAsGap(t *testing.T) {
	result := decodeCursorObservations(t, []adapter.Observation{{
		Stream: adapter.OutputStreamStdout,
		Chunk:  readCursorStreamFixture(t, "tool_reconnect_gap.ndjson"),
	}})

	gaps := cursorDraftsOfKind(result.Drafts, responseevents.KindStreamGap)
	if len(gaps) != 2 {
		t.Fatalf("gap drafts = %#v, want both unresolved tools", gaps)
	}
	for index, callID := range []string{"call-edit", "call-read"} {
		assertCursorToolGapDraft(t, gaps[index], callID, cursorToolGapReconnect)
	}
	if last := result.Drafts[len(result.Drafts)-1]; last.Kind != responseevents.KindMessage || last.Phase != responseevents.PhaseCompleted {
		t.Fatalf("terminal draft = %#v, want successful message after explicit gaps", last)
	}
}

func TestResponseEventDecoder_TerminalFailureReportsUnresolvedToolGap(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`{"type":"tool_call","subtype":"started","call_id":"call-failed-run","tool_call":{"shellToolCall":{"args":{"command":"go test ./..."}}}}`,
		`{"type":"result","subtype":"api_error","is_error":true,"result":"Provider unavailable","session_id":"cursor-failed-run"}`,
	}, "\n"))
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})

	if len(result.Drafts) != 3 {
		t.Fatalf("drafts = %#v, want tool start, gap, and terminal failure", result.Drafts)
	}
	assertCursorToolGapDraft(t, result.Drafts[1], "call-failed-run", cursorToolGapFailure)
	if result.Drafts[2].Kind != responseevents.KindError || result.Drafts[2].Phase != responseevents.PhaseFailed {
		t.Fatalf("terminal draft = %#v, want ERROR/FAILED", result.Drafts[2])
	}
}

func TestResponseEventDecoder_ExplicitCancellationClosesUnresolvedToolsAsCanceled(t *testing.T) {
	started := adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(
		`{"type":"tool_call","subtype":"started","call_id":"call-canceled-run","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}` + "\n" +
			`{"type":"result","subtype":"canceled","is_error":true,"result":"User canceled","session_id":"cursor-canceled-run"}`,
	)}
	result := decodeCursorObservations(t, []adapter.Observation{started})

	if len(result.Drafts) != 3 {
		t.Fatalf("drafts = %#v, want tool start, tool cancellation, and run cancellation", result.Drafts)
	}
	assertCursorSynthesizedToolCancellation(t, result.Drafts[1], "call-canceled-run", "readToolCall", ResultTypeResult)
	if result.Drafts[2].Kind != responseevents.KindRun || result.Drafts[2].Phase != responseevents.PhaseCanceled {
		t.Fatalf("terminal draft = %#v, want RUN/CANCELED", result.Drafts[2])
	}
}

func TestResponseEventDecoder_FlushClosesEveryUnresolvedToolFromBoundaryEvidence(t *testing.T) {
	observation := adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(
		`{"type":"tool_call","subtype":"started","call_id":"call-flush","tool_call":{"readToolCall":{"args":{"path":"README.md"}}}}` + "\n",
	)}
	testCases := []struct {
		name      string
		reason    adapter.FlushReason
		wantKind  responseevents.Kind
		wantPhase responseevents.Phase
		wantGap   string
	}{
		{name: "CompletedWithoutTerminalRecord", reason: adapter.FlushReasonCompleted, wantKind: responseevents.KindStreamGap, wantPhase: responseevents.PhaseUpdated, wantGap: cursorToolGapFlush},
		{name: "ProcessTerminated", reason: adapter.FlushReasonTerminated, wantKind: responseevents.KindStreamGap, wantPhase: responseevents.PhaseUpdated, wantGap: cursorToolGapTerminated},
		{name: "ProviderCanceled", reason: adapter.FlushReasonCanceled, wantKind: responseevents.KindTool, wantPhase: responseevents.PhaseCanceled},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := decodeCursorObservationsWithReason(t, []adapter.Observation{observation}, tc.reason)
			if len(result.Drafts) != 2 || result.Drafts[1].Kind != tc.wantKind || result.Drafts[1].Phase != tc.wantPhase {
				t.Fatalf("drafts = %#v, want start then %s/%s", result.Drafts, tc.wantKind, tc.wantPhase)
			}
			if tc.wantGap != "" {
				assertCursorToolGapDraft(t, result.Drafts[1], "call-flush", tc.wantGap)
			} else {
				assertCursorSynthesizedToolCancellation(t, result.Drafts[1], "call-flush", "readToolCall", "provider_boundary")
			}
		})
	}
}

func TestResponseEventDecoder_TerminalResultIsAuthoritativeSnapshot(t *testing.T) {
	testCases := []struct {
		name       string
		deltas     []string
		final      string
		wantDrafts int
	}{
		{name: "ExactMatch", deltas: []string{"final"}, final: "final", wantDrafts: 2},
		{name: "PrefixExtension", deltas: []string{"final"}, final: "final answer", wantDrafts: 2},
		{name: "Divergent", deltas: []string{"draft"}, final: "replacement", wantDrafts: 2},
		{name: "SnapshotOnly", final: "final", wantDrafts: 1},
		{name: "EmptySuccess", deltas: []string{"draft"}, final: "", wantDrafts: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			for index, delta := range tc.deltas {
				lines = append(lines, fmt.Sprintf(`{"type":"assistant","timestamp_ms":%d,"message":{"role":"assistant","content":[{"type":"text","text":%q}]},"session_id":"cursor-result-session"}`, index+1, delta))
			}
			lines = append(lines, fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"result":%q,"session_id":"cursor-result-session"}`, tc.final))

			result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(strings.Join(lines, "\n"))}})
			if len(result.Diagnostics) != 0 || len(result.Drafts) != tc.wantDrafts {
				t.Fatalf("result = %#v, want %d drafts and no diagnostics", result, tc.wantDrafts)
			}
			assertCursorMessageSnapshot(t, result.Drafts[len(result.Drafts)-1], tc.final)
			if got := reconstructCursorMessage(t, result.Drafts); got != tc.final {
				t.Fatalf("reconstructed message = %q, want authoritative snapshot %q", got, tc.final)
			}
			for _, draft := range result.Drafts {
				if err := responseevents.ValidateDraft(draft); err != nil {
					t.Fatalf("invalid draft: %v", err)
				}
			}
		})
	}
}

func TestResponseEventDecoder_TerminalFixtureAndInferenceResultStayAligned(t *testing.T) {
	raw := readCursorStreamFixture(t, "terminal_results.ndjson")
	decoded := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: raw}})
	if len(decoded.Drafts) != 4 || len(decoded.Diagnostics) != 0 {
		t.Fatalf("decoded = %#v, want session, two deltas, and snapshot", decoded)
	}
	assertCursorMessageSnapshot(t, decoded.Drafts[3], "Plan done")

	parsed, failure := ParseInferenceResult(string(modelprovider.Cursor), raw)
	if failure != nil || parsed == nil || parsed.Content != "Plan done" {
		t.Fatalf("parsed = %#v failure = %#v, want aligned terminal result", parsed, failure)
	}
}

func TestResponseEventDecoder_TerminalFailureAndCancellationDoNotCompleteMessage(t *testing.T) {
	testCases := []struct {
		name      string
		record    string
		wantKind  responseevents.Kind
		wantPhase responseevents.Phase
	}{
		{name: "Failure", record: `{"type":"result","subtype":"api_error","is_error":true,"result":"Provider unavailable","session_id":"cursor-result-session"}`, wantKind: responseevents.KindError, wantPhase: responseevents.PhaseFailed},
		{name: "ErrorFlaggedSuccess", record: `{"type":"result","subtype":"success","is_error":true,"result":"Request timed out","session_id":"cursor-result-session"}`, wantKind: responseevents.KindError, wantPhase: responseevents.PhaseFailed},
		{name: "Canceled", record: `{"type":"result","subtype":"canceled","is_error":true,"result":"User canceled","session_id":"cursor-result-session"}`, wantKind: responseevents.KindRun, wantPhase: responseevents.PhaseCanceled},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(tc.record)}})
			if len(result.Drafts) != 1 || result.Drafts[0].Kind != tc.wantKind || result.Drafts[0].Phase != tc.wantPhase {
				t.Fatalf("drafts = %#v, want %s/%s", result.Drafts, tc.wantKind, tc.wantPhase)
			}
			if err := responseevents.ValidateDraft(result.Drafts[0]); err != nil {
				t.Fatalf("invalid terminal draft: %v", err)
			}
		})
	}
}

func TestResponseEventDecoder_TerminalSnapshotIsBounded(t *testing.T) {
	final := strings.Repeat("x", PublishedTextLimit+20)
	record := fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"result":%q,"session_id":"cursor-result-session"}`, final)
	result := decodeCursorObservations(t, []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(record)}})
	if len(result.Drafts) != 1 {
		t.Fatalf("drafts = %#v, want one bounded snapshot", result.Drafts)
	}
	var payload responseevents.MessagePayload
	decodeCursorPayload(t, result.Drafts[0], &payload)
	if got := payload.ContentBlocks[0].Text; len(got) != PublishedTextLimit+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("snapshot length = %d, want %d with ellipsis", len(got), PublishedTextLimit+3)
	}
	if result.Drafts[0].Provenance.Fidelity != responseevents.FidelityLossy {
		t.Fatalf("snapshot provenance = %#v, want lossy", result.Drafts[0].Provenance)
	}
}

func TestCursorSafeToolSummaryIsDeterministicBoundedAndRedacted(t *testing.T) {
	sensitiveKey := "prompt: private-key-text-must-not-leak"
	oversizedKey := strings.Repeat("oversized-key-must-not-leak", cursorToolSummaryKeyLimit)
	large := map[string]any{
		"path":          "README.md",
		"authorization": "Bearer must-not-leak",
		"output":        strings.Repeat("x", cursorToolSummaryStringLimit+20),
		sensitiveKey:    "submitted prompt",
		oversizedKey:    "bounded key value",
		"nested": map[string]any{
			"escaped": strings.Repeat("\x00", cursorToolSummaryStringLimit),
			"details": strings.Repeat("nested output", cursorToolSummaryStringLimit),
		},
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
	if strings.Contains(string(first), "must-not-leak") || strings.Contains(string(first), sensitiveKey) ||
		strings.Contains(string(first), oversizedKey) {
		t.Fatalf("summary exposed sensitive or oversized key text: %s", first)
	}
	if len(first) > cursorToolSummaryEncodedLimit {
		t.Fatalf("encoded summary length = %d, want at most %d: %s", len(first), cursorToolSummaryEncodedLimit, first)
	}
	if summary["path"] != "README.md" {
		t.Fatalf("summary lost useful safe context: %s", first)
	}
	if !cursorSummaryContainsValue(summary, cursorToolRedactedValue) {
		t.Fatalf("summary did not redact credential: %s", first)
	}
	output, _ := summary["output"].(string)
	if len(output) > cursorToolSummaryStringLimit+3 || !strings.HasSuffix(output, "...") {
		t.Fatalf("bounded output = %q", output)
	}
}

func TestCursorSafeToolSummaryRedactsSeparatorVariantsAndStandaloneCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		value     map[string]any
		safeKey   string
		safeValue string
		forbidden []string
	}{
		{
			name: "Arguments",
			value: map[string]any{
				"path":    "README.md",
				"api.key": "sk-live-secret",
				"nested": map[string]any{
					"value": "sk-nested-secret",
				},
			},
			safeKey:   "path",
			safeValue: "README.md",
			forbidden: []string{"api.key", "sk-live-secret", "sk-nested-secret"},
		},
		{
			name: "Results",
			value: map[string]any{
				"output":       "updated",
				"access.token": "sk-result-secret",
				"nested": []any{
					map[string]any{"value": "ghp_nested_result_secret"},
				},
			},
			safeKey:   "output",
			safeValue: "updated",
			forbidden: []string{"access.token", "sk-result-secret", "ghp_nested_result_secret"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			summary := cursorSafeToolSummary(raw)
			var decoded map[string]any
			if err := json.Unmarshal(summary, &decoded); err != nil {
				t.Fatalf("summary is not valid JSON: %v", err)
			}
			if decoded[tc.safeKey] != tc.safeValue {
				t.Fatalf("summary lost useful safe context: %s", summary)
			}
			if !cursorSummaryContainsValue(decoded, cursorToolRedactedValue) {
				t.Fatalf("summary did not retain a redaction marker: %s", summary)
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(string(summary), forbidden) {
					t.Fatalf("summary exposed credential material %q: %s", forbidden, summary)
				}
			}
			if len(summary) > cursorToolSummaryEncodedLimit {
				t.Fatalf("encoded summary length = %d, want at most %d: %s", len(summary), cursorToolSummaryEncodedLimit, summary)
			}
		})
	}
}

func cursorSummaryContainsValue(value any, want string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, nested := range typed {
			if cursorSummaryContainsValue(nested, want) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if cursorSummaryContainsValue(nested, want) {
				return true
			}
		}
	case string:
		return typed == want
	}
	return false
}

func assertCursorSummaryRedactsKey(t *testing.T, decoded map[string]any, summary json.RawMessage, forbiddenKey string) {
	t.Helper()
	if !cursorSummaryContainsValue(decoded, cursorToolRedactedValue) {
		t.Fatalf("summary did not retain a redaction marker: %s", summary)
	}
	if strings.Contains(string(summary), forbiddenKey) {
		t.Fatalf("summary exposed sensitive key %q: %s", forbiddenKey, summary)
	}
}

func TestCursorSafeToolSummaryDecodesStructuredFunctionArgumentsAndRejectsUnsafeToolNames(t *testing.T) {
	summary := cursorSafeToolSummary(json.RawMessage(`"{\"path\":\"README.md\",\"password\":\"must-not-leak\"}"`))
	var decoded map[string]any
	if err := json.Unmarshal(summary, &decoded); err != nil {
		t.Fatalf("decode structured string summary: %v", err)
	}
	if decoded["path"] != "README.md" {
		t.Fatalf("structured string summary = %s", summary)
	}
	assertCursorSummaryRedactsKey(t, decoded, summary, "password")
	if got := cursorSafeToolName("private prompt: reveal credentials"); got != cursorToolFallbackName {
		t.Fatalf("unsafe tool name = %q, want fallback", got)
	}
}

func decodeCursorObservations(t *testing.T, observations []adapter.Observation) adapter.DecodeResult {
	return decodeCursorObservationsWithReason(t, observations, adapter.FlushReasonCompleted)
}

func decodeCursorObservationsWithReason(t *testing.T, observations []adapter.Observation, reason adapter.FlushReason) adapter.DecodeResult {
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
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: reason})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	return appendCursorDecodeResult(combined, flushed)
}

func cursorDraftsOfKind(drafts []responseevents.Draft, kind responseevents.Kind) []responseevents.Draft {
	var matching []responseevents.Draft
	for _, draft := range drafts {
		if draft.Kind == kind {
			matching = append(matching, draft)
		}
	}
	return matching
}

func cursorDraftOfKind(drafts []responseevents.Draft, kind responseevents.Kind) *responseevents.Draft {
	matching := cursorDraftsOfKind(drafts, kind)
	if len(matching) == 0 {
		return nil
	}
	return &matching[0]
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

func assertCursorMessageSnapshot(t *testing.T, draft responseevents.Draft, wantText string) {
	t.Helper()
	if draft.Kind != responseevents.KindMessage || draft.Phase != responseevents.PhaseCompleted {
		t.Fatalf("message snapshot kind/phase = %s/%s", draft.Kind, draft.Phase)
	}
	if draft.ItemID != "cursor-message/run-cursor-1" || (draft.ProviderSessionRef != "cursor-result-session" && draft.ProviderSessionRef != "cursor-terminal-session") {
		t.Fatalf("message snapshot correlation = item:%q provider:%q", draft.ItemID, draft.ProviderSessionRef)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.NativeEventType != "result" || draft.Provenance.NativeEventSubtype != "success" || draft.Provenance.Representation != responseevents.RepresentationSnapshot {
		t.Fatalf("message snapshot provenance = %#v", draft.Provenance)
	}
	var payload responseevents.MessagePayload
	decodeCursorPayload(t, draft, &payload)
	if payload.Role != "assistant" || len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Kind != responseevents.ContentBlockText || payload.ContentBlocks[0].Text != wantText {
		t.Fatalf("message snapshot payload = %#v, want %q", payload, wantText)
	}
}

func reconstructCursorMessage(t *testing.T, drafts []responseevents.Draft) string {
	t.Helper()
	var assembled string
	for _, draft := range drafts {
		if draft.Kind != responseevents.KindMessage {
			continue
		}
		if draft.Phase == responseevents.PhaseDelta {
			var payload responseevents.MessageDeltaPayload
			decodeCursorPayload(t, draft, &payload)
			assembled += payload.TextDelta
			continue
		}
		if draft.Phase == responseevents.PhaseCompleted {
			var payload responseevents.MessagePayload
			decodeCursorPayload(t, draft, &payload)
			assembled = payload.ContentBlocks[0].Text
		}
	}
	return assembled
}

func decodeCursorPayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode Cursor payload: %v", err)
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

func assertCursorToolGapDraft(t *testing.T, draft responseevents.Draft, callID, reason string) {
	t.Helper()
	if draft.Kind != responseevents.KindStreamGap || draft.Phase != responseevents.PhaseUpdated || draft.ItemID != "cursor-tool/"+callID {
		t.Fatalf("tool gap correlation = %#v", draft)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.Delivery != responseevents.DeliverySynthesized {
		t.Fatalf("tool gap provenance = %#v", draft.Provenance)
	}
	var payload responseevents.StreamGapPayload
	decodeCursorPayload(t, draft, &payload)
	if payload.AffectedItemID != draft.ItemID || payload.ToolCallID != callID || payload.Reason != reason {
		t.Fatalf("tool gap payload = %#v, want call=%q reason=%q", payload, callID, reason)
	}
	if err := responseevents.ValidateDraft(draft); err != nil {
		t.Fatalf("invalid tool gap draft: %v", err)
	}
}

func assertCursorSynthesizedToolCancellation(t *testing.T, draft responseevents.Draft, callID, name, nativeType string) {
	t.Helper()
	if draft.Kind != responseevents.KindTool || draft.Phase != responseevents.PhaseCanceled || draft.ItemID != "cursor-tool/"+callID {
		t.Fatalf("synthesized tool cancellation = %#v", draft)
	}
	if draft.Provenance.Provider != "cursor" || draft.Provenance.Delivery != responseevents.DeliverySynthesized || draft.Provenance.NativeEventType != nativeType {
		t.Fatalf("synthesized tool cancellation provenance = %#v", draft.Provenance)
	}
	var payload responseevents.ToolPayload
	decodeCursorToolPayload(t, draft, &payload)
	if payload.ToolCallID != callID || payload.ToolName != name || payload.Status != "canceled" {
		t.Fatalf("synthesized tool cancellation payload = %#v", payload)
	}
	if err := responseevents.ValidateDraft(draft); err != nil {
		t.Fatalf("invalid synthesized tool cancellation: %v", err)
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
