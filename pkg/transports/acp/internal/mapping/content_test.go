package mapping

import (
	"encoding/json"
	"errors"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// messageCase is one ProjectMessage table-driven expectation. Exactly one of
// wantErr, wantNoop, or wantText/wantItemID applies.
type messageCase struct {
	name       string
	draft      workers.Draft
	wantText   string
	wantNoop   bool
	wantErr    bool
	wantItemID string
}

// runMessageCases drives ProjectMessage for every case and asserts its
// declared outcome, shared by every TestProjectMessage_* function below so
// each stays a short table plus one call.
func runMessageCases(t *testing.T, cases []messageCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectMessage(tt.draft)

			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectMessage() unexpected err = %v", err)
			}
			if tt.wantNoop {
				requireNoUpdate(t, update)
				return
			}
			if update == nil || update.AgentMessageChunk == nil {
				t.Fatalf("ProjectMessage() update = %+v, want a populated AgentMessageChunk", update)
			}
			if got := textOf(update.AgentMessageChunk.Content); got != tt.wantText {
				t.Fatalf("ProjectMessage() text = %q, want %q", got, tt.wantText)
			}
			assertMessageID(t, update.AgentMessageChunk.MessageId, tt.wantItemID)
		})
	}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

func requireMalformed(t *testing.T, update *acpsdk.SessionUpdate, err error) {
	t.Helper()
	if !errors.Is(err, ErrMalformedRecord) {
		t.Fatalf("err = %v, want ErrMalformedRecord", err)
	}
	if update != nil {
		t.Fatalf("update = %+v, want nil on error", update)
	}
}

func requireNoUpdate(t *testing.T, update *acpsdk.SessionUpdate) {
	t.Helper()
	if update != nil {
		t.Fatalf("update = %+v, want nil (no-output)", update)
	}
}

func messageDraft(phase workers.Phase, payload json.RawMessage, itemID string) workers.Draft {
	return workers.Draft{Kind: workers.KindMessage, Phase: phase, Payload: payload, ItemID: itemID}
}

func TestProjectMessage_Delta(t *testing.T) {
	t.Parallel()

	runMessageCases(t, []messageCase{
		{
			name: "delta text projects a chunk",
			draft: messageDraft(workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{
				ContentBlockKind: workers.ContentBlockText, TextDelta: "hel",
			}), "item-1"),
			wantText:   "hel",
			wantItemID: "item-1",
		},
		{
			name: "delta with unsupported content-block kind produces no update",
			draft: messageDraft(workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{
				ContentBlockKind: workers.ContentBlockReasoningSummary, TextDelta: "not text",
			}), ""),
			wantNoop: true,
		},
		{
			name: "empty delta text produces no update",
			draft: messageDraft(workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{
				ContentBlockKind: workers.ContentBlockText, TextDelta: "",
			}), ""),
			wantNoop: true,
		},
		{
			name:    "malformed delta payload is rejected",
			draft:   messageDraft(workers.PhaseDelta, json.RawMessage(`{"contentBlockIndex":"not-a-number"}`), ""),
			wantErr: true,
		},
	})
}

func TestProjectMessage_Snapshot(t *testing.T) {
	t.Parallel()

	runMessageCases(t, []messageCase{
		{
			name: "completed snapshot from the assistant projects a chunk",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role:          "assistant",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}},
			}), "item-2"),
			wantText:   "hello",
			wantItemID: "item-2",
		},
		{
			name: "started snapshot from the assistant projects a chunk",
			draft: messageDraft(workers.PhaseStarted, mustMarshal(t, workers.MessagePayload{
				Role:          "assistant",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hi"}},
			}), ""),
			wantText: "hi",
		},
		{
			name: "multiple text blocks join in order",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role: "assistant",
				ContentBlocks: []workers.ContentBlock{
					{Kind: workers.ContentBlockText, Text: "first"},
					{Kind: workers.ContentBlockText, Text: "second"},
				},
			}), ""),
			wantText: "first\nsecond",
		},
		{
			name: "partial snapshot still projects its bounded text",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role: "assistant", Partial: true,
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "partial text"}},
			}), ""),
			wantText: "partial text",
		},
		{
			name:    "malformed snapshot payload is rejected",
			draft:   messageDraft(workers.PhaseCompleted, json.RawMessage(`{"role": 5}`), ""),
			wantErr: true,
		},
	})
}

func TestProjectMessage_NoOutput(t *testing.T) {
	t.Parallel()

	runMessageCases(t, []messageCase{
		{
			name: "non-assistant role produces no update",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role:          "user",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}},
			}), ""),
			wantNoop: true,
		},
		{
			name: "blank role produces no update",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "hello"}},
			}), ""),
			wantNoop: true,
		},
		{
			name: "unsupported content-block kind produces no update",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role:          "assistant",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockToolRequest, ToolCallID: "call-1", ToolName: "search"}},
			}), ""),
			wantNoop: true,
		},
		{
			name: "blank content blocks produce no update",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role:          "assistant",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: ""}},
			}), ""),
			wantNoop: true,
		},
	})
}

func TestProjectRetained_UserMessageUsesOriginalItemIdentity(t *testing.T) {
	draft := messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
		Role:          "user",
		ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "reload me"}},
	}), "item-user")

	update, err := ProjectRetained(draft)
	if err != nil {
		t.Fatalf("ProjectRetained() error = %v", err)
	}
	if update == nil || update.UserMessageChunk == nil {
		t.Fatalf("ProjectRetained() update = %+v, want a user_message_chunk", update)
	}
	if got := textOf(update.UserMessageChunk.Content); got != "reload me" {
		t.Fatalf("user_message_chunk text = %q, want %q", got, "reload me")
	}
	assertMessageID(t, update.UserMessageChunk.MessageId, "item-user")

	live, err := Project(draft)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	requireNoUpdate(t, live)
}

// reasoningCase is one ProjectReasoning table-driven expectation.
type reasoningCase struct {
	name       string
	draft      workers.Draft
	wantText   string
	wantNoop   bool
	wantErr    bool
	wantItemID string
}

func reasoningDraft(phase workers.Phase, payload json.RawMessage, itemID string) workers.Draft {
	return workers.Draft{Kind: workers.KindReasoning, Phase: phase, Payload: payload, ItemID: itemID}
}

func TestProjectReasoning(t *testing.T) {
	t.Parallel()

	cases := []reasoningCase{
		{
			name:       "delta summary projects a thought chunk",
			draft:      reasoningDraft(workers.PhaseDelta, mustMarshal(t, workers.ReasoningPayload{SummaryDelta: "thinking"}), "item-3"),
			wantText:   "thinking",
			wantItemID: "item-3",
		},
		{
			name:     "completed summary projects a thought chunk",
			draft:    reasoningDraft(workers.PhaseCompleted, mustMarshal(t, workers.ReasoningPayload{Summary: "final reasoning"}), ""),
			wantText: "final reasoning",
		},
		{
			name:     "started summary projects a thought chunk",
			draft:    reasoningDraft(workers.PhaseStarted, mustMarshal(t, workers.ReasoningPayload{Summary: "starting"}), ""),
			wantText: "starting",
		},
		{
			name:     "empty summary produces no update",
			draft:    reasoningDraft(workers.PhaseCompleted, mustMarshal(t, workers.ReasoningPayload{}), ""),
			wantNoop: true,
		},
		{
			name:     "empty summary delta produces no update",
			draft:    reasoningDraft(workers.PhaseDelta, mustMarshal(t, workers.ReasoningPayload{}), ""),
			wantNoop: true,
		},
		{
			name:    "malformed payload is rejected",
			draft:   reasoningDraft(workers.PhaseCompleted, json.RawMessage(`{"summary": 5}`), ""),
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectReasoning(tt.draft)

			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectReasoning() unexpected err = %v", err)
			}
			if tt.wantNoop {
				requireNoUpdate(t, update)
				return
			}
			if update == nil || update.AgentThoughtChunk == nil {
				t.Fatalf("ProjectReasoning() update = %+v, want a populated AgentThoughtChunk", update)
			}
			if got := textOf(update.AgentThoughtChunk.Content); got != tt.wantText {
				t.Fatalf("ProjectReasoning() text = %q, want %q", got, tt.wantText)
			}
			assertMessageID(t, update.AgentThoughtChunk.MessageId, tt.wantItemID)
		})
	}
}

// textOf extracts the plain text carried by an acpsdk.TextBlock-shaped
// content block, returning "" if the block is not text.
func textOf(block acpsdk.ContentBlock) string {
	if block.Text == nil {
		return ""
	}
	return block.Text.Text
}

func assertMessageID(t *testing.T, got *string, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("MessageId = %q, want nil (no ItemID on the source record)", *got)
		}
		return
	}
	if got == nil || *got != want {
		t.Fatalf("MessageId = %v, want %q", got, want)
	}
}
