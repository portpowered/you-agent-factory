package codex_test

import (
	"context"
	"encoding/json"
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
