package mapping

import (
	"encoding/json"
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProjectStreamGap projects one STREAM_GAP-kind record -- workers.PhaseUpdated,
// the only phase response-draft validation allows for this Kind -- into one
// explicit gap notice. Unlike every other projected record, a legal
// STREAM_GAP/UPDATED record always produces an update: retention eviction
// and item-scoped loss must be surfaced, never silently dropped.
//
// acp-go-sdk has no dedicated "gap" session/update variant, so this reports
// the gap as an agent_thought_chunk: thought chunks are the one ACP update
// kind clients already treat as agent-internal narration rather than
// accumulated response text, which keeps a gap notice from ever being
// mistaken for -- or concatenated into -- real message history. The chunk's
// text states only the bounds and reason the source payload itself declares;
// it never infers or fabricates what the missing records contained. An
// item-scoped gap (workers.StreamGapPayload.AffectedItemID set) carries that
// item's identity as MessageId so a client can associate the notice with the
// affected item; a retention-scoped gap carries no item identity.
func ProjectStreamGap(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var gap workers.StreamGapPayload
	if err := json.Unmarshal(draft.Payload, &gap); err != nil {
		return nil, fmt.Errorf("%w: payload must decode as StreamGapPayload: %v", ErrMalformedRecord, err)
	}

	chunk := &acpsdk.SessionUpdateAgentThoughtChunk{Content: acpsdk.TextBlock(gapNoticeText(gap))}
	if gap.AffectedItemID != "" {
		id := gap.AffectedItemID
		chunk.MessageId = &id
	}
	return &acpsdk.SessionUpdate{AgentThoughtChunk: chunk}, nil
}

// gapNoticeText renders the bounded, safe description of a gap: for an
// item-scoped gap, the declared reason; for a retention-scoped gap, the
// declared sequence bounds and first-available position, plus the reason
// when one is present.
func gapNoticeText(gap workers.StreamGapPayload) string {
	if gap.AffectedItemID != "" {
		if gap.Reason != "" {
			return fmt.Sprintf("Some records for this item are unavailable: %s", gap.Reason)
		}
		return "Some records for this item are unavailable."
	}

	text := fmt.Sprintf(
		"Records from sequence %d to %d are unavailable; history resumes at sequence %d.",
		gap.FromSequence, gap.ToSequence, gap.FirstAvailableSequence,
	)
	if gap.Reason != "" {
		text += " Reason: " + gap.Reason
	}
	return text
}
