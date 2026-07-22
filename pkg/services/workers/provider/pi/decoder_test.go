package pi_test

import (
	"bytes"
	"context"
	"encoding/json"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/pi"
	"os"
	"strings"
	"testing"
)

func TestDecoderMapsSessionAgentTurnLifecycle(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/message_lifecycle_subtypes.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertValidPiDrafts(t, drafts)

	session := findPiDraft(drafts, factorysessions.ResponseEventKindSession, factorysessions.ResponseEventPhaseStarted)
	if session == nil || session.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("session draft = %#v", session)
	}
	runStarted := findPiDraft(drafts, factorysessions.ResponseEventKindRun, factorysessions.ResponseEventPhaseStarted)
	runCompleted := findPiDraft(drafts, factorysessions.ResponseEventKindRun, factorysessions.ResponseEventPhaseCompleted)
	if runStarted == nil || runCompleted == nil {
		t.Fatalf("run lifecycle missing: %#v", drafts)
	}
	if runStarted.ProviderSessionRef != "pi-session-lifecycle" || runCompleted.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("run drafts lost provider session ref: start=%#v end=%#v", runStarted, runCompleted)
	}

	turns := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindTurn, factorysessions.ResponseEventPhaseStarted)
	turnEnds := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindTurn, factorysessions.ResponseEventPhaseCompleted)
	if len(turns) != 1 || len(turnEnds) != 1 || turns[0].TurnID == "" || turns[0].TurnID != turnEnds[0].TurnID {
		t.Fatalf("turn lifecycle correlation missing: starts=%#v ends=%#v", turns, turnEnds)
	}
	var turnPayload factorysessions.ResponseEventTurn
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
	assertPiMessageSubtypeMapping(t, drafts)
}

func assertPiMessageSubtypeMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	assertPiMessageStarted(t, drafts)
	assertPiTextDeltaMapping(t, drafts)
	assertPiReasoningDeltaMapping(t, drafts)
	assertPiToolDeltaMapping(t, drafts)
	assertPiCompletedMessageMapping(t, drafts)
}

func assertPiMessageStarted(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	messageStarted := findPiDraft(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseStarted)
	if messageStarted == nil || messageStarted.ItemID != "msg-lifecycle" || messageStarted.ProviderSessionRef != "pi-session-lifecycle" {
		t.Fatalf("message start = %#v", messageStarted)
	}
}

func assertPiTextDeltaMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	textDeltas := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseDelta)
	if len(textDeltas) != 1 || textDeltas[0].ItemID != "msg-lifecycle" {
		t.Fatalf("text deltas = %#v", textDeltas)
	}
	var textPayload factorysessions.ResponseEventMessageDelta
	decodePiPayload(t, textDeltas[0], &textPayload)
	if textPayload.TextDelta != "Hello " || textPayload.ContentBlockKind != factorysessions.ResponseEventContentBlockText {
		t.Fatalf("text delta payload = %#v", textPayload)
	}
	if textDeltas[0].Provenance.NativeEventType != "text_delta" {
		t.Fatalf("text delta provenance = %#v", textDeltas[0].Provenance)
	}
}

func assertPiReasoningDeltaMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	reasoning := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseDelta)
	if len(reasoning) != 1 || reasoning[0].ItemID != "msg-lifecycle" {
		t.Fatalf("reasoning deltas = %#v", reasoning)
	}
	var reasoningPayload factorysessions.ResponseEventReasoning
	decodePiPayload(t, reasoning[0], &reasoningPayload)
	if reasoningPayload.SummaryDelta != "considering options" {
		t.Fatalf("reasoning payload = %#v", reasoningPayload)
	}
	if reasoning[0].Provenance.NativeEventType != "thinking_delta" {
		t.Fatalf("reasoning provenance = %#v", reasoning[0].Provenance)
	}
}

func assertPiToolDeltaMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	toolDeltas := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseDelta)
	if len(toolDeltas) != 2 {
		t.Fatalf("tool preview deltas = %d, want tool_call_delta and input_json_delta: %#v", len(toolDeltas), toolDeltas)
	}
	if toolDeltas[0].ItemID != "call-lifecycle" || toolDeltas[1].ItemID != "call-lifecycle" {
		t.Fatalf("tool preview identity changed: %#v", toolDeltas)
	}
	if toolDeltas[0].Provenance.NativeEventType != "tool_call_delta" || toolDeltas[1].Provenance.NativeEventType != "input_json_delta" {
		t.Fatalf("tool preview provenance = %#v and %#v", toolDeltas[0].Provenance, toolDeltas[1].Provenance)
	}
}

func assertPiCompletedMessageMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	completed := findPiDraft(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
	if completed == nil || completed.ItemID != "msg-lifecycle" {
		t.Fatalf("completed message = %#v", completed)
	}
	var completedPayload factorysessions.ResponseEventMessage
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

	completed := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
	if len(completed) != 1 {
		t.Fatalf("completed messages = %d, want one authoritative snapshot: %#v", len(completed), drafts)
	}
	var payload factorysessions.ResponseEventMessage
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
	if findPiDraft(drafts, factorysessions.ResponseEventKindSession, factorysessions.ResponseEventPhaseStarted) == nil {
		t.Fatalf("chunked decode lost session draft: %#v", drafts)
	}
	if findPiDraft(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted) == nil {
		t.Fatalf("chunked decode lost completed message: %#v", drafts)
	}
}

func TestDecoderFlushProcessesUnterminatedRecord(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/snapshot_authoritative_text.jsonl", true)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	completed := findPiDraft(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
	if completed == nil || completed.ItemID != "msg-snapshot" {
		t.Fatalf("flush did not process final unterminated record: %#v", drafts)
	}
}

func decodePiFixture(t *testing.T, path string, keepFinalRecordUnterminated bool) ([]factorysessions.ResponseEventDraft, []adapter.Diagnostic) {
	t.Helper()
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return decodePiFixtureBytes(t, fixture, keepFinalRecordUnterminated)
}

func assertValidPiDrafts(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	for index, draft := range drafts {
		if err := factorysessions.ValidateResponseEventDraft(draft); err != nil {
			t.Fatalf("draft[%d] %s/%s is invalid: %v", index, draft.Kind, draft.Phase, err)
		}
	}
}

func piDraftsByKindAndPhase(drafts []factorysessions.ResponseEventDraft, kind factorysessions.ResponseEventKind, phase factorysessions.ResponseEventPhase) []factorysessions.ResponseEventDraft {
	var matched []factorysessions.ResponseEventDraft
	for _, draft := range drafts {
		if draft.Kind == kind && draft.Phase == phase {
			matched = append(matched, draft)
		}
	}
	return matched
}

func findPiDraft(drafts []factorysessions.ResponseEventDraft, kind factorysessions.ResponseEventKind, phase factorysessions.ResponseEventPhase) *factorysessions.ResponseEventDraft {
	for index := range drafts {
		if drafts[index].Kind == kind && drafts[index].Phase == phase {
			return &drafts[index]
		}
	}
	return nil
}

func decodePiPayload(t *testing.T, draft factorysessions.ResponseEventDraft, target any) {
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
		if draft.Kind == factorysessions.ResponseEventKindProgress {
			t.Fatalf("subtype mapping collapsed to generic progress: %#v", draft)
		}
	}
	if len(piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindReasoning, factorysessions.ResponseEventPhaseDelta)) == 0 {
		t.Fatal("thinking_delta did not map to reasoning semantics")
	}
	toolDelta := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseDelta)
	if len(toolDelta) == 0 || !strings.Contains(toolDelta[0].Provenance.NativeEventType, "delta") {
		t.Fatalf("tool preview deltas missing: %#v", toolDelta)
	}
}

func TestDecoderCorrelatesToolLifecycleByStableCallIdentity(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/tool_lifecycle_correlation.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertValidPiDrafts(t, drafts)

	started := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseStarted, "call-weather")
	updates := piToolDraftsByCallID(drafts, factorysessions.ResponseEventPhaseDelta, "call-weather")
	completed := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseCompleted, "call-weather")
	if started == nil || completed == nil {
		t.Fatalf("tool lifecycle missing start or completion: %#v", drafts)
	}
	if len(updates) != 2 {
		t.Fatalf("tool updates = %d, want two partial updates for one call identity: %#v", len(updates), updates)
	}
	for _, draft := range append([]factorysessions.ResponseEventDraft{*started, *completed}, updates...) {
		if draft.ItemID != "call-weather" {
			t.Fatalf("tool draft changed call identity: %#v", draft)
		}
	}
	assertPiToolDraft(t, *started, factorysessions.ResponseEventPhaseStarted, "call-weather", "get_weather", "running")
	assertPiToolDraft(t, *completed, factorysessions.ResponseEventPhaseCompleted, "call-weather", "get_weather", "completed")
}

func TestDecoderMapsToolExecutionErrorToFailedTerminalStatus(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/tool_lifecycle_error.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}

	started := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseStarted, "call-fail")
	failed := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseFailed, "call-fail")
	if started == nil || failed == nil {
		t.Fatalf("tool error lifecycle missing: %#v", drafts)
	}
	assertPiToolDraft(t, *started, factorysessions.ResponseEventPhaseStarted, "call-fail", "write_file", "running")
	assertPiToolDraft(t, *failed, factorysessions.ResponseEventPhaseFailed, "call-fail", "write_file", "failed")
	if findPiToolDraft(drafts, factorysessions.ResponseEventPhaseCompleted, "call-fail") != nil {
		t.Fatalf("error end mapped to completed status: %#v", drafts)
	}
	if findPiDraft(drafts, factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseFailed) != nil {
		t.Fatalf("tool error leaked into unrelated error item: %#v", drafts)
	}
}

func TestDecoderMissingToolCallIDEmitsDiagnosticWithoutMergingTools(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		`{"type":"message_start","message":{"id":"msg-missing-call","role":"assistant","content":[]}}`,
		`{"type":"tool_execution_start","toolName":"orphan_tool","args":{"path":"README.md"}}`,
		`{"type":"tool_execution_start","toolCallId":"call-valid","toolName":"read_file","args":{"path":"README.md"}}`,
		`{"type":"tool_execution_end","toolCallId":"call-valid","toolName":"read_file","result":{"ok":true}}`,
	}, "\n") + "\n"
	drafts, diagnostics := decodePiRawFixture(t, raw)
	if len(diagnostics) != 1 || diagnostics[0].Code != "pi_missing_tool_call_id" {
		t.Fatalf("diagnostics = %#v, want one missing call identity diagnostic", diagnostics)
	}
	valid := piToolDraftsByCallID(drafts, factorysessions.ResponseEventPhaseStarted, "call-valid")
	completed := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseCompleted, "call-valid")
	if len(valid) != 1 || completed == nil {
		t.Fatalf("valid tool lifecycle = %#v, want isolated start/end for call-valid", drafts)
	}
	if findPiToolDraft(drafts, factorysessions.ResponseEventPhaseStarted, "") != nil {
		t.Fatal("missing toolCallId silently merged into anonymous tool item")
	}
}

func TestDecoderToolSummariesAreBoundedAndSanitized(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/tool_lifecycle_correlation.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	started := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseStarted, "call-weather")
	completed := findPiToolDraft(drafts, factorysessions.ResponseEventPhaseCompleted, "call-weather")
	if started == nil || completed == nil {
		t.Fatal("tool lifecycle missing")
	}
	var startedPayload factorysessions.ResponseEventTool
	decodePiPayload(t, *started, &startedPayload)
	var arguments map[string]string
	if err := json.Unmarshal(startedPayload.ArgumentsSummary, &arguments); err != nil {
		t.Fatalf("decode arguments summary: %v", err)
	}
	if arguments["city"] != "Oslo" || arguments["api_key"] != "<redacted>" {
		t.Fatalf("arguments summary = %#v", arguments)
	}
	var completedPayload factorysessions.ResponseEventTool
	decodePiPayload(t, *completed, &completedPayload)
	if !strings.Contains(string(completedPayload.ResultSummary), `"temperature":12`) ||
		strings.Contains(string(completedPayload.ResultSummary), "sk-fixture-secret") {
		t.Fatalf("result summary = %s", completedPayload.ResultSummary)
	}
	for _, draft := range piToolDraftsByCallID(drafts, factorysessions.ResponseEventPhaseDelta, "call-weather") {
		var delta factorysessions.ResponseEventToolDelta
		decodePiPayload(t, draft, &delta)
		if strings.Contains(delta.OutputDelta, "sk-fixture-secret") {
			t.Fatalf("partial summary leaked credential: %q", delta.OutputDelta)
		}
	}
}

func TestDecoderToolNameRemainsStableAcrossLifecycle(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/tool_lifecycle_correlation.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	for _, draft := range piToolDraftsByCallID(drafts, "", "call-weather") {
		var payload factorysessions.ResponseEventTool
		if draft.Phase == factorysessions.ResponseEventPhaseDelta {
			var delta factorysessions.ResponseEventToolDelta
			decodePiPayload(t, draft, &delta)
			continue
		}
		decodePiPayload(t, draft, &payload)
		if payload.ToolName != "get_weather" {
			t.Fatalf("tool name changed across lifecycle: %#v payload=%#v", draft, payload)
		}
	}
}

func TestDecoderDuplicateToolUpdatesDoNotAllocateNewItems(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		`{"type":"message_start","message":{"id":"msg-dedupe","role":"assistant","content":[]}}`,
		`{"type":"tool_execution_start","toolCallId":"call-dedupe","toolName":"search","args":{"query":"docs"}}`,
		`{"type":"tool_execution_update","toolCallId":"call-dedupe","partialResult":{"page":1}}`,
		`{"type":"tool_execution_update","toolCallId":"call-dedupe","partialResult":{"page":1}}`,
		`{"type":"tool_execution_end","toolCallId":"call-dedupe","toolName":"search","result":{"page":1}}`,
	}, "\n") + "\n"
	drafts, diagnostics := decodePiRawFixture(t, raw)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	updates := piToolDraftsByCallID(drafts, factorysessions.ResponseEventPhaseDelta, "call-dedupe")
	if len(updates) != 1 {
		t.Fatalf("duplicate partial updates allocated %d tool items, want 1: %#v", len(updates), updates)
	}
}

func TestDecoderMapsRemainingControlRecords(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/control_records.jsonl", true)
	if len(diagnostics) != 2 || diagnostics[0].Code != "pi_unknown_record" || diagnostics[1].Code != "pi_malformed_record" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	assertValidPiDrafts(t, drafts)
	assertPiControlRecordMapping(t, drafts, diagnostics)
}

func assertPiControlRecordMapping(t *testing.T, drafts []factorysessions.ResponseEventDraft, diagnostics []adapter.Diagnostic) {
	t.Helper()

	retry := findPiDraft(drafts, factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseUpdated)
	if retry == nil {
		t.Fatalf("retry observation missing: %#v", drafts)
	}
	var retryPayload factorysessions.ResponseEventErrorPayload
	decodePiPayload(t, *retry, &retryPayload)
	if !retryPayload.Retryable || retryPayload.RetryAttempt == nil || *retryPayload.RetryAttempt != 2 ||
		retryPayload.RetryAfterSeconds == nil || *retryPayload.RetryAfterSeconds != 2 {
		t.Fatalf("retry payload = %#v", retryPayload)
	}

	compaction := piDraftsByKindAndPhase(drafts, factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated)
	if len(compaction) != 2 {
		t.Fatalf("compaction observations = %d, want start and end: %#v", len(compaction), compaction)
	}

	completed := findPiDraft(drafts, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
	if completed == nil || completed.ItemID != "msg-controls" {
		t.Fatalf("later valid completion missing: %#v", drafts)
	}

	encoded, err := json.Marshal(struct {
		Drafts      []factorysessions.ResponseEventDraft `json:"drafts"`
		Diagnostics []adapter.Diagnostic                 `json:"diagnostics"`
	}{drafts, diagnostics})
	if err != nil {
		t.Fatalf("marshal decoded controls: %v", err)
	}
	for _, forbidden := range []string{"private compacted transcript", "private future prompt", "private queued prompt", "sk-pi-fixture-secret"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("normalized controls leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestDecoderExplicitlyIgnoresQueueUpdateWithoutDrafts(t *testing.T) {
	t.Parallel()

	raw := strings.Join([]string{
		`{"type":"session","id":"pi-session-queue"}`,
		`{"type":"queue_update","queuedPrompt":"private queued prompt must not escape","queueDepth":3}`,
		`{"type":"message_start","message":{"id":"msg-queue","role":"assistant","content":[]}}`,
		`{"type":"message_end","message":{"id":"msg-queue","role":"assistant","content":[{"type":"text","text":"done"}],"stopReason":"stop"}}`,
	}, "\n") + "\n"
	drafts, diagnostics := decodePiRawFixture(t, raw)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for ignored queue_update", diagnostics)
	}
	for _, draft := range drafts {
		if draft.Kind == factorysessions.ResponseEventKindProgress && strings.Contains(string(draft.Payload), "queued") {
			t.Fatalf("queue_update mapped to steering work item: %#v", draft)
		}
	}
	encoded, err := json.Marshal(drafts)
	if err != nil {
		t.Fatalf("marshal drafts: %v", err)
	}
	if bytes.Contains(encoded, []byte("private queued prompt")) {
		t.Fatalf("queue_update leaked prompt text: %s", encoded)
	}
}

func TestDecoderEventMatrixCoversDocumentedLifecycleEvents(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodePiFixture(t, "testdata/event_matrix.jsonl", false)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertValidPiDrafts(t, drafts)

	required := []struct {
		kind  factorysessions.ResponseEventKind
		phase factorysessions.ResponseEventPhase
	}{
		{factorysessions.ResponseEventKindSession, factorysessions.ResponseEventPhaseStarted},
		{factorysessions.ResponseEventKindRun, factorysessions.ResponseEventPhaseStarted},
		{factorysessions.ResponseEventKindTurn, factorysessions.ResponseEventPhaseStarted},
		{factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseStarted},
		{factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted},
		{factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseStarted},
		{factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseCompleted},
		{factorysessions.ResponseEventKindProgress, factorysessions.ResponseEventPhaseUpdated},
		{factorysessions.ResponseEventKindError, factorysessions.ResponseEventPhaseUpdated},
	}
	for _, want := range required {
		if findPiDraft(drafts, want.kind, want.phase) == nil {
			t.Fatalf("event matrix missing %s/%s: %#v", want.kind, want.phase, drafts)
		}
	}
}

func decodePiRawFixture(t *testing.T, raw string) ([]factorysessions.ResponseEventDraft, []adapter.Diagnostic) {
	t.Helper()
	return decodePiFixtureBytes(t, []byte(raw), false)
}

func decodePiFixtureBytes(t *testing.T, fixture []byte, keepFinalRecordUnterminated bool) ([]factorysessions.ResponseEventDraft, []adapter.Diagnostic) {
	t.Helper()
	if keepFinalRecordUnterminated {
		fixture = bytes.TrimSuffix(fixture, []byte("\n"))
	}
	decoder, err := pi.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{
		RunID: "run-pi-lifecycle", DispatchID: "dispatch-pi-lifecycle",
	})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	var drafts []factorysessions.ResponseEventDraft
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

func piToolDraftsByCallID(drafts []factorysessions.ResponseEventDraft, phase factorysessions.ResponseEventPhase, callID string) []factorysessions.ResponseEventDraft {
	var matched []factorysessions.ResponseEventDraft
	for _, draft := range drafts {
		if draft.Kind != factorysessions.ResponseEventKindTool || draft.ItemID != callID {
			continue
		}
		if phase != "" && draft.Phase != phase {
			continue
		}
		matched = append(matched, draft)
	}
	return matched
}

func findPiToolDraft(drafts []factorysessions.ResponseEventDraft, phase factorysessions.ResponseEventPhase, callID string) *factorysessions.ResponseEventDraft {
	for index := range drafts {
		if drafts[index].Kind == factorysessions.ResponseEventKindTool && drafts[index].Phase == phase && drafts[index].ItemID == callID {
			return &drafts[index]
		}
	}
	return nil
}

func assertPiToolDraft(t *testing.T, draft factorysessions.ResponseEventDraft, phase factorysessions.ResponseEventPhase, callID, name, status string) {
	t.Helper()
	if draft.Kind != factorysessions.ResponseEventKindTool || draft.Phase != phase || draft.ItemID != callID {
		t.Fatalf("tool draft = %#v, want TOOL/%s for %s", draft, phase, callID)
	}
	var payload factorysessions.ResponseEventTool
	decodePiPayload(t, draft, &payload)
	if payload.ToolCallID != callID || payload.ToolName != name || payload.Status != status {
		t.Fatalf("tool payload = %#v, want call=%s name=%s status=%s", payload, callID, name, status)
	}
}
