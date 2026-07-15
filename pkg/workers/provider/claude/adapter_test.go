package claude_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
	"github.com/portpowered/infinite-you/pkg/workers/provider/claude"
)

const (
	privateConformancePrompt = "private Claude prompt must not escape"
	privateConformanceToken  = "sk-claude-fixture-secret"
)

func TestAdapterBuildCommandRequestsStructuredPartialMessages(t *testing.T) {
	t.Parallel()

	built, err := claude.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		Request: workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.Claude), Model: "claude-sonnet-4",
			SessionID: "session-1", UserMessage: "inspect the workspace",
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	want := []string{"-p", "--model", "claude-sonnet-4", "--resume", "session-1", "--output-format", "stream-json", "--include-partial-messages", "inspect the workspace"}
	if !reflect.DeepEqual(built.Request.Args, want) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, want)
	}
}

func TestAdapterBuildCommandPreservesOptionalExecutionContext(t *testing.T) {
	t.Parallel()

	built, err := claude.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		SkipPermissions: true,
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-options"},
			WorkerType:       "agent-worker",
			WorkstationType:  "review-work",
			ProjectID:        "project-options",
			InputTokens:      []any{"input-token"},
			SystemPrompt:     "safe system prompt",
			UserMessage:      "review the workspace",
			EnvVars:          map[string]string{"CLAUDE_TEST_BOUNDARY": "value"},
			Worktree:         "story-worktree",
			WorkingDirectory: "workspace",
			Model:            "claude-sonnet-4",
			SessionID:        "session-options",
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	wantArgs := []string{
		"-p", "--dangerously-skip-permissions", "--worktree", "story-worktree",
		"--system-prompt", "safe system prompt", "--model", "claude-sonnet-4",
		"--resume", "session-options", "--output-format", "stream-json",
		"--include-partial-messages", "review the workspace",
	}
	if !reflect.DeepEqual(built.Request.Args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, wantArgs)
	}
	if built.Request.DispatchID != "dispatch-options" || built.Request.WorkerType != "agent-worker" ||
		built.Request.WorkstationName != "review-work" || built.Request.ProjectID != "project-options" ||
		built.Request.WorkDir != "workspace" || !reflect.DeepEqual(built.Request.InputTokens, []any{"input-token"}) {
		t.Fatalf("execution context = %#v", built.Request)
	}
	if !slices.Contains(built.Request.Env, "CLAUDE_TEST_BOUNDARY=value") {
		t.Fatalf("command env omitted explicit provider value: %#v", built.Request.Env)
	}
}

func TestAdapterReportsClaudeStreamingCapabilitiesAndFailureFacts(t *testing.T) {
	t.Parallel()

	providerAdapter := claude.NewAdapter()
	if providerAdapter.Identity() != "claude" {
		t.Fatalf("Identity() = %q", providerAdapter.Identity())
	}
	reported, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	capabilities := reported.Capabilities
	if !capabilities.NativeStreaming || !capabilities.MessageDeltas || !capabilities.MessageSnapshots || !capabilities.ToolLifecycle || !capabilities.StableItemIDs || capabilities.FinalOnly {
		t.Fatalf("Capabilities() = %#v", capabilities)
	}
	if failure := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{}); failure.Failure != nil {
		t.Fatalf("successful failure classification = %#v", failure)
	}
	failure := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{CommandResult: workerprocess.CommandResult{ExitCode: 1}})
	if failure.Failure == nil || failure.Failure.Type != workerexecution.WorkFailureTypeUnknown ||
		failure.Failure.Message != "claude exited with code 1" || failure.Failure.Retry.Retryable {
		t.Fatalf("failed classification = %#v", failure)
	}
}

func TestAdapterFullStreamConformance(t *testing.T) {
	t.Parallel()

	testkit.RunFullStream(t, testkit.FullStreamFixture{
		NewAdapter: func() adapter.Adapter { return claude.NewAdapter() },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-conformance"},
			Model:    "claude-sonnet-4", UserMessage: privateConformancePrompt,
		},
		ContentAndTools: claudeObservations(
			`{"type":"system","subtype":"init","session_id":"claude-session-conformance"}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"message_start","message":{"id":"msg_conformance","role":"assistant","content":[]}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"message_stop"}}`,
			`{"type":"assistant","session_id":"claude-session-conformance","message":{"id":"msg_conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}]}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"message_start","message":{"id":"msg_tool","role":"assistant","content":[]}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_conformance","name":"weather","input":{}}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Oslo\"}"}}}`,
			`{"type":"stream_event","session_id":"claude-session-conformance","event":{"type":"content_block_stop","index":0}}`,
		),
		RetryableFailure: claudeObservations(
			`{"type":"system","subtype":"api_retry","session_id":"claude-session-conformance","attempt":2,"retry_delay_ms":2000,"error_status":429}`,
		),
		UnsafeAndRecovering: claudeObservations(
			`{"type":"system","subtype":"init","session_id":"claude-session-conformance"}`,
			`{"type":`+privateConformancePrompt+`,"token":"`+privateConformanceToken+`"}`,
			`{"type":"future_shape","prompt":"`+privateConformancePrompt+`","token":"`+privateConformanceToken+`"}`,
			`{"type":"assistant","session_id":"claude-session-conformance","message":{"id":"msg_conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}]}}`,
		),
		UnterminatedFinal: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(
			`{"type":"assistant","session_id":"claude-session-conformance","message":{"id":"msg_conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}]}}`,
		)}},
		FinalResult: workerprocess.CommandResult{Stdout: []byte(
			`{"type":"result","subtype":"success","is_error":false,"result":"Hello world","session_id":"claude-session-conformance"}`,
		)},
		Expected: testkit.FullStreamExpected{
			ProviderSession: workerexecution.ProviderSessionMetadata{Provider: "claude", Kind: "session_id", ID: "claude-session-conformance"},
			ProviderRef:     "claude-session-conformance", MessageItemID: "msg_conformance",
			ToolItemID: "msg_tool/content-block/0", ToolCallID: "toolu_conformance",
			MessageDeltas: []string{"Hello ", "world"}, FinalContent: "Hello world",
			ToolOutputDelta: `{"city":"Oslo"}`, RetryAfter: 2,
		},
		ForbiddenDiagnostic: []string{privateConformancePrompt, privateConformanceToken},
	})
}

func TestDecoderMapsChunkedTextAndToolInputWithStableIdentity(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/content_blocks.jsonl", true)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	if len(drafts) != 7 {
		t.Fatalf("draft count = %d, want 7: %#v", len(drafts), drafts)
	}
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
	}
	assertMessageDrafts(t, drafts)
	assertToolDrafts(t, drafts)
}

func TestDecoderInvalidToolInputDoesNotLeakOrStopLaterMessage(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/invalid_tool_input.jsonl", false)
	if len(diagnostics) != 1 || diagnostics[0].Code != "claude_invalid_tool_input" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	encoded, err := json.Marshal(struct {
		Drafts      []responseevents.Draft `json:"drafts"`
		Diagnostics []adapter.Diagnostic   `json:"diagnostics"`
	}{drafts, diagnostics})
	if err != nil {
		t.Fatalf("marshal decoded output: %v", err)
	}
	for _, forbidden := range []string{"private prompt", "credential", "{invalid"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("decoded output leaked %q: %s", forbidden, encoded)
		}
	}
	completed := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if completed == nil || completed.ItemID != "msg_recovery" {
		t.Fatalf("later completed message missing: %#v", drafts)
	}
}

func TestDecoderNormalizesControlRecordsAndExplicitSubagentAttribution(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/control_records.jsonl", true)
	assertControlDiagnostics(t, diagnostics)
	assertCanonicalDrafts(t, drafts)
	assertControlObservations(t, drafts)
	assertControlAttribution(t, drafts)
	assertControlOutputSafe(t, drafts, diagnostics)
}

func assertControlDiagnostics(t *testing.T, diagnostics []adapter.Diagnostic) {
	t.Helper()
	if len(diagnostics) != 2 || diagnostics[0].Code != "claude_unknown_record" || diagnostics[1].Code != "claude_malformed_record" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func assertCanonicalDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
	}
}

func assertControlObservations(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	retry := findDraft(drafts, responseevents.KindError, responseevents.PhaseUpdated)
	if retry == nil {
		t.Fatalf("retry observation missing: %#v", drafts)
	}
	var retryPayload responseevents.ErrorPayload
	decodePayload(t, *retry, &retryPayload)
	if !retryPayload.Retryable || retryPayload.RetryAttempt == nil || *retryPayload.RetryAttempt != 2 || retryPayload.RetryAfterSeconds == nil || *retryPayload.RetryAfterSeconds != 2 {
		t.Fatalf("retry payload = %#v", retryPayload)
	}
	compaction := findDraft(drafts, responseevents.KindProgress, responseevents.PhaseUpdated)
	if compaction == nil {
		t.Fatalf("compaction observation missing: %#v", drafts)
	}
}

func assertControlAttribution(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	completed := draftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if len(completed) != 3 || completed[0].ParentItemID != "toolu_parent" || completed[1].ParentItemID != "" || completed[2].ParentItemID != "" {
		t.Fatalf("completed message attribution = %#v", completed)
	}
	delta := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseDelta)
	if delta == nil || delta.ParentItemID != "toolu_parent" || delta.ItemID != completed[0].ItemID {
		t.Fatalf("partial attribution or identity = %#v", delta)
	}
}

func assertControlOutputSafe(t *testing.T, drafts []responseevents.Draft, diagnostics []adapter.Diagnostic) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Drafts      []responseevents.Draft
		Diagnostics []adapter.Diagnostic
	}{drafts, diagnostics})
	if err != nil {
		t.Fatalf("marshal decoded controls: %v", err)
	}
	for _, forbidden := range []string{"private compacted transcript", "private future prompt", privateConformanceToken} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("normalized controls leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestDecoderOmitsOutOfRangeRetryMetadata(t *testing.T) {
	t.Parallel()

	decoder, err := claude.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{RunID: "run-bounds"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	result, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStdout,
		Chunk:  []byte(`{"type":"system","subtype":"api_retry","attempt":1000001,"retry_delay_ms":86400001}` + "\n"),
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	retry := findDraft(result.Drafts, responseevents.KindError, responseevents.PhaseUpdated)
	if retry == nil {
		t.Fatalf("retry observation missing: %#v", result)
	}
	var payload responseevents.ErrorPayload
	decodePayload(t, *retry, &payload)
	if payload.RetryAttempt != nil || payload.RetryAfterSeconds != nil {
		t.Fatalf("out-of-range retry metadata was retained: %#v", payload)
	}
}

func TestDecoderBoundsNonSemanticStreamDiagnostics(t *testing.T) {
	t.Parallel()

	decoder, err := claude.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	stderr, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStderr, Chunk: []byte("private provider stderr"),
	})
	if err != nil || len(stderr.Diagnostics) != 1 || stderr.Diagnostics[0].Code != "claude_stderr_ignored" {
		t.Fatalf("stderr result = %#v, err=%v", stderr, err)
	}
	ignored, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStream("future-stream"), Chunk: []byte("ignored"),
	})
	if err != nil || len(ignored.Drafts) != 0 || len(ignored.Diagnostics) != 0 {
		t.Fatalf("unknown stream result = %#v, err=%v", ignored, err)
	}
	oversized, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStdout, Chunk: bytes.Repeat([]byte("x"), 256*1024+1),
	})
	if err != nil || len(oversized.Diagnostics) != 1 || oversized.Diagnostics[0].Code != "claude_record_too_large" {
		t.Fatalf("oversized result = %#v, err=%v", oversized, err)
	}
}

func TestDecoderCompleteTextSnapshotSupersedesDeltasAndIsIdempotent(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/snapshot_text.jsonl", true)
	assertValidDrafts(t, drafts, diagnostics)
	completed := draftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if len(completed) != 2 {
		t.Fatalf("completed messages = %d, want authoritative and distinct later message: %#v", len(completed), drafts)
	}
	assertMessageContent(t, completed[0], "msg_text", []string{"authoritative text"})
	assertMessageContent(t, completed[1], "msg_text_2", []string{"later message"})
	if encoded, err := json.Marshal(completed); err != nil || bytes.Contains(encoded, []byte("draft text")) {
		t.Fatalf("completed snapshots retained accumulated draft text: %s (err=%v)", encoded, err)
	}
}

func TestDecoderCompleteToolSnapshotReusesLogicalToolIdentity(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/snapshot_tool.jsonl", false)
	assertValidDrafts(t, drafts, diagnostics)
	completedTools := draftsByKindAndPhase(drafts, responseevents.KindTool, responseevents.PhaseCompleted)
	if len(completedTools) != 2 {
		t.Fatalf("completed tools = %d, want partial completion and authoritative update: %#v", len(completedTools), drafts)
	}
	for _, draft := range completedTools {
		if draft.ItemID != "msg_tool/content-block/0" || draft.ParentItemID != "msg_tool" {
			t.Fatalf("tool snapshot allocated a second logical identity: %#v", draft)
		}
	}
	var authoritative responseevents.ToolPayload
	decodePayload(t, completedTools[1], &authoritative)
	var summary map[string]any
	if err := json.Unmarshal(authoritative.ArgumentsSummary, &summary); err != nil {
		t.Fatalf("decode authoritative tool summary: %v", err)
	}
	if authoritative.ToolCallID != "toolu_snapshot" || summary["city"] != "Bergen" || summary["api_token"] != "<redacted>" {
		t.Fatalf("authoritative tool snapshot = %#v", authoritative)
	}
	if bytes.Contains(authoritative.ArgumentsSummary, []byte("snapshot-secret")) {
		t.Fatalf("authoritative tool summary leaked provider input: %s", authoritative.ArgumentsSummary)
	}
	if got := len(draftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)); got != 1 {
		t.Fatalf("completed messages = %d, want one authoritative snapshot", got)
	}
}

func TestDecoderCompleteMixedSnapshotPreservesProviderBlockOrder(t *testing.T) {
	t.Parallel()

	drafts, diagnostics := decodeFixture(t, "testdata/snapshot_mixed.jsonl", true)
	assertValidDrafts(t, drafts, diagnostics)
	completed := draftsByKindAndPhase(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
	if len(completed) != 1 {
		t.Fatalf("completed messages = %d, want one: %#v", len(completed), drafts)
	}
	var payload responseevents.MessagePayload
	decodePayload(t, completed[0], &payload)
	if len(payload.ContentBlocks) != 3 || payload.ContentBlocks[0].Text != "final first" || payload.ContentBlocks[1].ToolCallID != "toolu_mixed" || payload.ContentBlocks[2].Text != "final last" {
		t.Fatalf("mixed snapshot order = %#v", payload.ContentBlocks)
	}
}

func TestParseFinalKeepsTerminalResultAuthoritative(t *testing.T) {
	t.Parallel()

	stdout := []byte("{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"partial\"}]}}\n" +
		"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"authoritative final\",\"session_id\":\"claude-session-42\"}\n")
	result, err := claude.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: stdout},
	})
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != "authoritative final" || result.Response.ProviderSession == nil || result.Response.ProviderSession.ID != "claude-session-42" {
		t.Fatalf("ParseFinal() = %#v", result)
	}
	if len(result.Drafts) != 0 {
		t.Fatalf("ParseFinal() emitted semantic drafts: %#v", result.Drafts)
	}
}

func decodeFixture(t *testing.T, path string, keepFinalRecordUnterminated bool) ([]responseevents.Draft, []adapter.Diagnostic) {
	t.Helper()
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if keepFinalRecordUnterminated {
		fixture = bytes.TrimSuffix(fixture, []byte("\n"))
	}
	decoder, err := claude.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{RunID: "run-1", DispatchID: "dispatch-1"})
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
		decoded, observeErr := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: fixture[offset : offset+size]})
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

func claudeObservations(records ...string) []adapter.Observation {
	observations := make([]adapter.Observation, 0, len(records)+1)
	for index, record := range records {
		chunk := []byte(strings.TrimSpace(record) + "\n")
		if index == 1 && len(chunk) > 7 {
			observations = append(observations,
				adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk[:7]},
				adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk[7:]},
			)
			continue
		}
		observations = append(observations, adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: chunk})
	}
	return observations
}

func assertValidDrafts(t *testing.T, drafts []responseevents.Draft, diagnostics []adapter.Diagnostic) {
	t.Helper()
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diagnostics)
	}
	assertCanonicalDrafts(t, drafts)
}

func draftsByKindAndPhase(drafts []responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase) []responseevents.Draft {
	var matched []responseevents.Draft
	for _, draft := range drafts {
		if draft.Kind == kind && draft.Phase == phase {
			matched = append(matched, draft)
		}
	}
	return matched
}

func assertMessageContent(t *testing.T, draft responseevents.Draft, itemID string, texts []string) {
	t.Helper()
	if draft.ItemID != itemID {
		t.Fatalf("message item ID = %q, want %q", draft.ItemID, itemID)
	}
	var payload responseevents.MessagePayload
	decodePayload(t, draft, &payload)
	if len(payload.ContentBlocks) != len(texts) {
		t.Fatalf("message blocks = %#v, want text count %d", payload.ContentBlocks, len(texts))
	}
	for index, text := range texts {
		if payload.ContentBlocks[index].Text != text {
			t.Fatalf("message block[%d] = %#v, want text %q", index, payload.ContentBlocks[index], text)
		}
	}
}

func assertMessageDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for _, index := range []int{1, 2, 6} {
		if drafts[index].ItemID != "msg_01" || drafts[index].ProviderSessionRef != "claude-session-42" {
			t.Fatalf("message draft[%d] lost identity: %#v", index, drafts[index])
		}
	}
	for index, want := range []string{"Hello ", "world"} {
		var payload responseevents.MessageDeltaPayload
		decodePayload(t, drafts[index+1], &payload)
		if payload.TextDelta != want || payload.ContentBlockIndex != 0 {
			t.Fatalf("message delta[%d] = %#v", index, payload)
		}
	}
	var completed responseevents.MessagePayload
	decodePayload(t, drafts[6], &completed)
	if len(completed.ContentBlocks) != 2 || completed.ContentBlocks[0].Text != "Hello world" || completed.ContentBlocks[1].ToolCallID != "toolu_42" {
		t.Fatalf("completed message = %#v", completed)
	}
}

func assertToolDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for _, index := range []int{3, 4, 5} {
		if drafts[index].ItemID != "msg_01/content-block/1" || drafts[index].ParentItemID != "msg_01" {
			t.Fatalf("tool draft[%d] lost identity: %#v", index, drafts[index])
		}
	}
	var started, completed responseevents.ToolPayload
	decodePayload(t, drafts[3], &started)
	decodePayload(t, drafts[5], &completed)
	var delta responseevents.ToolDeltaPayload
	decodePayload(t, drafts[4], &delta)
	if started.ToolCallID != "toolu_42" || delta.ToolCallID != "toolu_42" || completed.ToolCallID != "toolu_42" {
		t.Fatalf("tool call identity changed: start=%#v delta=%#v completed=%#v", started, delta, completed)
	}
	if !strings.Contains(delta.OutputDelta, `"city":"Oslo"`) || !strings.Contains(delta.OutputDelta, `"api_token":"<redacted>"`) || strings.Contains(delta.OutputDelta, "fixture-secret") {
		t.Fatalf("tool summary was not safely normalized: %q", delta.OutputDelta)
	}
}

func findDraft(drafts []responseevents.Draft, kind responseevents.Kind, phase responseevents.Phase) *responseevents.Draft {
	for index := range drafts {
		if drafts[index].Kind == kind && drafts[index].Phase == phase {
			return &drafts[index]
		}
	}
	return nil
}

func decodePayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
}
