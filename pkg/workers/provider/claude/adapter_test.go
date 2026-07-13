package claude_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/claude"
)

func TestAdapterBuildCommandRequestsStructuredPartialMessages(t *testing.T) {
	t.Parallel()

	built, err := claude.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		Request: interfaces.ProviderInferenceRequest{
			ModelProvider: string(interfaces.ModelProviderClaude), Model: "claude-sonnet-4",
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
	if failure.Failure == nil || failure.Failure.Message != "Claude invocation failed." || failure.Failure.Retry.Retryable {
		t.Fatalf("failed classification = %#v", failure)
	}
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
