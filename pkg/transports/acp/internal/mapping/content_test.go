package mapping

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestProjectRetainedPreservesLiveProjectionAndFailureSemantics(t *testing.T) {
	tests := []struct {
		name        string
		draft       workers.Draft
		wantAgent   bool
		wantThought bool
		wantNoop    bool
		wantErr     bool
	}{
		{
			name: "assistant snapshot remains an agent message",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role:          "assistant",
				ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: "retained answer"}},
			}), "item-assistant"),
			wantAgent: true,
		},
		{
			name: "message delta remains an agent message",
			draft: messageDraft(workers.PhaseDelta, mustMarshal(t, workers.MessageDeltaPayload{
				ContentBlockKind: workers.ContentBlockText,
				TextDelta:        "partial answer",
			}), "item-delta"),
			wantAgent: true,
		},
		{
			name: "reasoning remains a thought update",
			draft: reasoningDraft(workers.PhaseCompleted, mustMarshal(t, workers.ReasoningPayload{
				Summary: "retained reasoning",
			}), "item-thought"),
			wantThought: true,
		},
		{
			name:    "malformed user snapshot is rejected",
			draft:   messageDraft(workers.PhaseCompleted, json.RawMessage(`{"role":5}`), "item-malformed"),
			wantErr: true,
		},
		{
			name: "empty user snapshot produces no update",
			draft: messageDraft(workers.PhaseCompleted, mustMarshal(t, workers.MessagePayload{
				Role: "user",
			}), "item-empty"),
			wantNoop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update, err := ProjectRetained(tt.draft)
			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectRetained() error = %v", err)
			}
			if tt.wantNoop {
				requireNoUpdate(t, update)
				return
			}
			if tt.wantAgent && (update == nil || update.AgentMessageChunk == nil) {
				t.Fatalf("ProjectRetained() update = %+v, want an agent message", update)
			}
			if tt.wantThought && (update == nil || update.AgentThoughtChunk == nil) {
				t.Fatalf("ProjectRetained() update = %+v, want an agent thought", update)
			}
		})
	}
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

// TestProjectPlan covers the session-level plan projection.
//
// A plan is the one record where an unrecognized value must fail rather than
// default: reporting a plan state the Factory never claimed is worse than
// reporting none, because the client renders it as the agent's own commitment.
func TestProjectPlan(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		payload    string
		wantNil    bool
		wantErr    bool
		wantStatus []acpsdk.PlanEntryStatus
	}{
		{
			name: "every declared status maps onto the ACP enum",
			payload: `{"steps":[
				{"id":"1","description":"pending by default"},
				{"id":"2","description":"explicitly pending","status":"pending"},
				{"id":"3","description":"running","status":"in_progress"},
				{"id":"4","description":"active alias","status":"ACTIVE"},
				{"id":"5","description":"done alias","status":"Done"}
			]}`,
			wantStatus: []acpsdk.PlanEntryStatus{
				acpsdk.PlanEntryStatusPending,
				acpsdk.PlanEntryStatusPending,
				acpsdk.PlanEntryStatusInProgress,
				acpsdk.PlanEntryStatusInProgress,
				acpsdk.PlanEntryStatusCompleted,
			},
		},
		{
			// Reporting an empty plan would clear whatever the client shows.
			name:    "a plan with no steps reports nothing",
			payload: `{"summary":"thinking about it"}`,
			wantNil: true,
		},
		{
			name:    "a step with no description is malformed",
			payload: `{"steps":[{"id":"1","description":"  "}]}`,
			wantErr: true,
		},
		{
			name:    "an unrecognized status fails closed",
			payload: `{"steps":[{"id":"1","description":"do it","status":"probably"}]}`,
			wantErr: true,
		},
		{
			name:    "a payload that is not a plan is malformed",
			payload: `["not","a","plan"]`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectPlan(workers.Draft{
				Kind: workers.KindPlan, Phase: workers.PhaseUpdated,
				Payload: json.RawMessage(tc.payload),
			})
			if tc.wantErr {
				if !errors.Is(err, ErrMalformedRecord) {
					t.Fatalf("ProjectPlan() error = %v, want ErrMalformedRecord", err)
				}
				if update != nil {
					t.Fatalf("ProjectPlan() update = %+v, want none alongside an error", update)
				}
				return
			}
			if err != nil {
				t.Fatalf("ProjectPlan() unexpected error = %v", err)
			}
			if tc.wantNil {
				if update != nil {
					t.Fatalf("ProjectPlan() update = %+v, want none", update)
				}
				return
			}
			if update == nil || update.Plan == nil {
				t.Fatalf("ProjectPlan() update = %+v, want a populated plan", update)
			}
			if len(update.Plan.Entries) != len(tc.wantStatus) {
				t.Fatalf("plan entries = %d, want %d", len(update.Plan.Entries), len(tc.wantStatus))
			}
			for index, entry := range update.Plan.Entries {
				if entry.Status != tc.wantStatus[index] {
					t.Fatalf("entry[%d] status = %q, want %q", index, entry.Status, tc.wantStatus[index])
				}
				// ACP has no omitempty on priority, so an unset value would
				// serialize as an invalid enum on the wire.
				if entry.Priority != defaultPlanEntryPriority {
					t.Fatalf("entry[%d] priority = %q, want the declared default %q",
						index, entry.Priority, defaultPlanEntryPriority)
				}
			}
		})
	}
}

// TestProjectAvailableCommands proves the advertisement is derived from the
// parser's own constant. Advertising a command the server would then reject is
// worse than advertising nothing.
func TestProjectAvailableCommands(t *testing.T) {
	t.Parallel()

	update := ProjectAvailableCommands()
	if update.AvailableCommandsUpdate == nil {
		t.Fatal("ProjectAvailableCommands() carried no advertisement")
	}
	commands := update.AvailableCommandsUpdate.AvailableCommands
	if len(commands) != 1 {
		t.Fatalf("advertised commands = %d, want exactly the one the parser implements", len(commands))
	}
	if commands[0].Name != FactoryCommandName {
		t.Fatalf("advertised command = %q, want %q", commands[0].Name, FactoryCommandName)
	}
	if strings.TrimSpace(commands[0].Description) == "" {
		t.Fatal("advertised command carries no description")
	}
	if commands[0].Input == nil || commands[0].Input.Unstructured == nil ||
		strings.TrimSpace(commands[0].Input.Unstructured.Hint) == "" {
		t.Fatalf("advertised command input = %+v, want an unstructured hint", commands[0].Input)
	}
}
