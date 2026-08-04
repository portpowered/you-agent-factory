package mapping

import (
	"encoding/json"
	"testing"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestDraftFromSequencedItem_PreservesIdentityAndSourceFields(t *testing.T) {
	item := chatsessions.SequencedItem{
		ItemID:       "item-1",
		ParentItemID: "item-0",
		Kind:         workers.KindMessage,
		Phase:        workers.PhaseDelta,
		Payload:      json.RawMessage(`{"delta":{"kind":"TEXT","text":"hi"},"role":"ASSISTANT"}`),
	}

	got := DraftFromSequencedItem(item)

	if got.ItemID != item.ItemID {
		t.Errorf("ItemID = %q, want %q", got.ItemID, item.ItemID)
	}
	if got.ParentItemID != item.ParentItemID {
		t.Errorf("ParentItemID = %q, want %q", got.ParentItemID, item.ParentItemID)
	}
	if got.Kind != item.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, item.Kind)
	}
	if got.Phase != item.Phase {
		t.Errorf("Phase = %q, want %q", got.Phase, item.Phase)
	}
	if string(got.Payload) != string(item.Payload) {
		t.Errorf("Payload = %s, want %s", got.Payload, item.Payload)
	}
}

func TestDraftFromSequencedItem_BlankParentNeverFabricated(t *testing.T) {
	item := chatsessions.SequencedItem{
		ItemID:  "item-1",
		Kind:    workers.KindMessage,
		Phase:   workers.PhaseCompleted,
		Payload: json.RawMessage(`{"content":[{"kind":"TEXT","text":"hi"}],"role":"ASSISTANT"}`),
	}

	got := DraftFromSequencedItem(item)

	if got.ParentItemID != "" {
		t.Errorf("ParentItemID = %q, want empty for a top-level item", got.ParentItemID)
	}
}
