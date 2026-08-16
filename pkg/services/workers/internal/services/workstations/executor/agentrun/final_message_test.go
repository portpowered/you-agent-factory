package agentrun

import (
	"encoding/json"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestPublishAgentFinalMessagePublishesCanonicalDraft(t *testing.T) {
	var got workerexecution.ProgressFragment
	publishAgentFinalMessage(func(fragment workerexecution.ProgressFragment) {
		got = fragment
	}, " dispatch-1 ", "  final answer  ")

	if got.DispatchID != "dispatch-1" || got.CanonicalDraft == nil {
		t.Fatalf("published fragment = %#v, want dispatch and canonical draft", got)
	}
	draft, ok := got.CanonicalDraft.(workerexecution.Draft)
	if !ok {
		t.Fatalf("canonical draft type = %T, want workers.Draft", got.CanonicalDraft)
	}
	if draft.DispatchID != "dispatch-1" || draft.ItemID != "dispatch-1-final-message" ||
		draft.Provenance.Provider != "agent-run" || draft.Phase != workerexecution.PhaseCompleted {
		t.Fatalf("draft = %#v, want final agent message metadata", draft)
	}
	var payload workerexecution.MessagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("draft payload: %v", err)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != "final answer" {
		t.Fatalf("draft payload = %#v, want trimmed final answer", payload)
	}

	publishAgentFinalMessage(nil, "dispatch-1", "ignored")
	publishAgentFinalMessage(func(workerexecution.ProgressFragment) {
		t.Fatal("empty final message was published")
	}, "dispatch-1", " ")
}
