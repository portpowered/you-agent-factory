package mapping

import (
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// DraftFromSequencedItem reconstructs the workers.Draft Project expects from
// one already-committed chatsessions.SequencedItem: item.ItemID and
// item.ParentItemID become the returned Draft's own ItemID/ParentItemID
// (Chat Sessions' sequencer-assigned identity, never a value this transport
// mints or remembers itself -- see final-proposal.md §5.3), and item.Kind,
// item.Phase, and item.Payload pass through unchanged as the source-native
// fields Project routes on. It is a pure, lossless field copy -- item is
// already a validated, committed envelope by the time a caller observes it
// through Events, so this never fails.
func DraftFromSequencedItem(item chatsessions.SequencedItem) workers.Draft {
	return workers.Draft{
		Kind:         item.Kind,
		Phase:        item.Phase,
		Payload:      item.Payload,
		ItemID:       item.ItemID,
		ParentItemID: item.ParentItemID,
	}
}
