package codex_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	provider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/codex"
)

const lifecycleFixture = `{"type":"thread.started","thread_id":"thread-codex-123","unrelated_session_id":"wrong"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item-message-1","type":"agent_message","text":"authoritative answer"}}
{"type":"turn.completed","usage":{"input_tokens":4,"output_tokens":2}}
`

func TestDecoderMapsThreadTurnAndCompletedAgentMessageExactly(t *testing.T) {
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-1", DispatchID: "dispatch-1"})
	cut := len(lifecycleFixture) / 2
	var drafts []responseevents.Draft
	for _, chunk := range []string{lifecycleFixture[:cut], lifecycleFixture[cut:]} {
		decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(chunk)})
		if err != nil {
			t.Fatalf("Observe() error = %v", err)
		}
		drafts = append(drafts, decoded.Drafts...)
	}
	flushed, err := decoder.Flush(context.Background(), adapter.FlushContext{Reason: adapter.FlushReasonCompleted})
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	drafts = append(drafts, flushed.Drafts...)
	if len(drafts) != 4 {
		t.Fatalf("draft count = %d, want 4: %#v", len(drafts), drafts)
	}
	wantKinds := []responseevents.Kind{responseevents.KindSession, responseevents.KindTurn, responseevents.KindMessage, responseevents.KindTurn}
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
		if draft.Kind != wantKinds[index] || draft.ProviderSessionRef != "thread-codex-123" {
			t.Fatalf("draft[%d] = %#v", index, draft)
		}
	}
	message := drafts[2]
	if message.ItemID != "item-message-1" || message.Phase != responseevents.PhaseCompleted || message.TurnID != drafts[1].TurnID {
		t.Fatalf("message correlation = %#v", message)
	}
	var payload responseevents.MessagePayload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != "authoritative answer" {
		t.Fatalf("message payload = %#v", payload)
	}
}

func TestParseFinalOutputIsIndependentAndUsesOnlyThreadID(t *testing.T) {
	parsed, err := codex.ParseFinalOutput([]byte(lifecycleFixture))
	if err != nil {
		t.Fatalf("ParseFinalOutput() error = %v", err)
	}
	if parsed.Content != "authoritative answer" {
		t.Fatalf("content = %q", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.ID != "thread-codex-123" || parsed.ProviderSession.Kind != codex.ProviderSessionKindSessionID {
		t.Fatalf("provider session = %#v", parsed.ProviderSession)
	}
	if strings.Contains(parsed.Content, "thread.started") {
		t.Fatalf("content contains streamed observation: %q", parsed.Content)
	}
}

func TestCommandOutputNormalizerPublishesTypedCanonicalDrafts(t *testing.T) {
	var published []provider.InferenceProgressFragment
	normalizer := codex.NewCommandOutputNormalizer(provider.CommandRequest{
		Command: "codex", Args: []string{"exec", "--json", "-"}, DispatchID: "dispatch-stream-codex",
	}, func(fragment provider.InferenceProgressFragment) { published = append(published, fragment) })
	if normalizer == nil {
		t.Fatal("NewCommandOutputNormalizer() = nil")
	}
	normalizer.Observe("stdout", []byte(lifecycleFixture))
	normalizer.Flush()
	if len(published) != 4 {
		t.Fatalf("published = %#v, want four typed records", published)
	}
	for index, fragment := range published {
		draft, ok := fragment.CanonicalDraft.(*responseevents.Draft)
		if !ok {
			t.Fatalf("published[%d] canonical draft = %T", index, fragment.CanonicalDraft)
		}
		if err := responseevents.ValidateDraft(*draft); err != nil {
			t.Fatalf("published[%d] invalid: %v", index, err)
		}
		if draft.ProviderSessionRef != "thread-codex-123" {
			t.Fatalf("published[%d] session = %q", index, draft.ProviderSessionRef)
		}
	}
}

func TestDecoderPreservesEverySupportedItemSemanticAndIdentity(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "supported_items.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := codex.NewDecoder(adapter.DecoderContext{RunID: "run-items", DispatchID: "dispatch-items"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: fixture})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(decoded.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", decoded.Diagnostics)
	}
	if len(decoded.Drafts) != 18 {
		t.Fatalf("draft count = %d, want 18", len(decoded.Drafts))
	}
	for index, draft := range decoded.Drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] invalid: %v", index, err)
		}
		if draft.ProviderSessionRef != "thread-items-123" {
			t.Fatalf("draft[%d] provider session = %q", index, draft.ProviderSessionRef)
		}
	}

	byID := draftsByItemID(decoded.Drafts)
	if got, want := itemIDOrder(decoded.Drafts), []string{"reason-1", "reason-1", "command-1", "command-1", "files-1", "mcp-1", "mcp-1", "collab-1", "collab-1", "web-1", "web-1", "plan-1", "plan-1", "plan-1", "message-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("item order = %#v, want %#v", got, want)
	}
	assertItemLifecycle(t, byID["reason-1"], responseevents.KindReasoning, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["command-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["mcp-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["collab-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["web-1"], responseevents.KindTool, responseevents.PhaseStarted, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["plan-1"], responseevents.KindPlan, responseevents.PhaseUpdated, responseevents.PhaseUpdated, responseevents.PhaseUpdated)
	assertItemLifecycle(t, byID["message-1"], responseevents.KindMessage, responseevents.PhaseCompleted)
	assertItemLifecycle(t, byID["files-1"], responseevents.KindFileChange, responseevents.PhaseUpdated)

	assertItemPayloads(t, byID)
}

func TestDecoderClassifiesOnlyExactNestedItemTypes(t *testing.T) {
	decoder := codex.NewDecoder(adapter.DecoderContext{DispatchID: "dispatch-exact"})
	decoded, err := decoder.Observe(context.Background(), adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"near-match\",\"type\":\"command_execution_result\",\"command\":\"must not run\"}}\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Drafts) != 0 || len(decoded.Diagnostics) != 1 || decoded.Diagnostics[0].Code != "codex_unknown_item" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func draftsByItemID(drafts []responseevents.Draft) map[string][]responseevents.Draft {
	result := make(map[string][]responseevents.Draft)
	for _, draft := range drafts {
		if draft.ItemID != "" {
			result[draft.ItemID] = append(result[draft.ItemID], draft)
		}
	}
	return result
}

func itemIDOrder(drafts []responseevents.Draft) []string {
	var result []string
	for _, draft := range drafts {
		if draft.ItemID != "" {
			result = append(result, draft.ItemID)
		}
	}
	return result
}

func assertItemLifecycle(t *testing.T, drafts []responseevents.Draft, kind responseevents.Kind, phases ...responseevents.Phase) {
	t.Helper()
	got := make([]responseevents.Phase, len(drafts))
	completedSnapshots := 0
	for index, draft := range drafts {
		if draft.Kind != kind {
			t.Fatalf("item %q draft[%d] kind = %q, want %q", draft.ItemID, index, draft.Kind, kind)
		}
		got[index] = draft.Phase
		if draft.Provenance.NativeEventType == "item.completed" {
			completedSnapshots++
			if draft.Provenance.Representation != responseevents.RepresentationSnapshot {
				t.Fatalf("item %q completion representation = %q", draft.ItemID, draft.Provenance.Representation)
			}
		}
	}
	if !reflect.DeepEqual(got, phases) {
		t.Fatalf("phases = %#v, want %#v", got, phases)
	}
	if completedSnapshots != 1 {
		t.Fatalf("completed snapshots = %d, want 1", completedSnapshots)
	}
}

func assertItemPayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	assertReasoningCommandAndFilePayloads(t, byID)
	assertToolAndPlanPayloads(t, byID)
}

func assertReasoningCommandAndFilePayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	var reasoning responseevents.ReasoningPayload
	decodePayload(t, byID["reason-1"][1], &reasoning)
	if reasoning.Summary != "The provider adapter owns native decoding." {
		t.Fatalf("reasoning = %#v", reasoning)
	}

	var command responseevents.ToolPayload
	decodePayload(t, byID["command-1"][1], &command)
	if command.ToolCallID != "command-1" || command.ToolName != "command_execution" || !strings.Contains(string(command.ResultSummary), `"exitCode":0`) {
		t.Fatalf("command = %#v", command)
	}

	var files responseevents.FileChangePayload
	decodePayload(t, byID["files-1"][0], &files)
	if files.Path != "pkg/workers/provider/codex/items.go" || files.Operation != "update" || !strings.Contains(files.Summary, "adapter_test.go") {
		t.Fatalf("file change = %#v", files)
	}
}

func assertToolAndPlanPayloads(t *testing.T, byID map[string][]responseevents.Draft) {
	t.Helper()
	var mcp responseevents.ToolPayload
	decodePayload(t, byID["mcp-1"][1], &mcp)
	if mcp.ToolName != "mcp:docs/lookup" || len(mcp.ArgumentsSummary) != 0 || strings.Contains(string(mcp.ResultSummary), "not projected") {
		t.Fatalf("mcp = %#v", mcp)
	}

	var collaboration responseevents.ToolPayload
	decodePayload(t, byID["collab-1"][1], &collaboration)
	if collaboration.ToolName != "collaboration:spawn_agent" || strings.Contains(string(collaboration.ResultSummary), "not projected") {
		t.Fatalf("collaboration = %#v", collaboration)
	}

	var plan responseevents.PlanPayload
	decodePayload(t, byID["plan-1"][2], &plan)
	if len(plan.Steps) != 2 || plan.Steps[0].ID != "" || plan.Steps[0].Status != "completed" || plan.Steps[1].Status != "completed" {
		t.Fatalf("plan = %#v", plan)
	}
}

func decodePayload(t *testing.T, draft responseevents.Draft, target any) {
	t.Helper()
	if err := json.Unmarshal(draft.Payload, target); err != nil {
		t.Fatalf("decode %q payload: %v", draft.ItemID, err)
	}
}
