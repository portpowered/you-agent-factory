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
	if err := validateStreamGapPayload(gap); err != nil {
		return nil, err
	}

	chunk := &acpsdk.SessionUpdateAgentThoughtChunk{Content: acpsdk.TextBlock(gapNoticeText(gap))}
	if gap.AffectedItemID != "" {
		id := gap.AffectedItemID
		chunk.MessageId = &id
	}
	return &acpsdk.SessionUpdate{AgentThoughtChunk: chunk}, nil
}

// validateStreamGapPayload rejects a decodable-but-structurally-invalid
// StreamGapPayload rather than letting it fall through to gapNoticeText and
// render fabricated content. An item-scoped gap (AffectedItemID set) needs
// no numeric bounds. A retention-scoped gap (AffectedItemID empty -- this
// includes the zero-valued StreamGapPayload{} an empty JSON object `{}`
// decodes to) must declare a genuine resumable position: sequence 0 is a
// reserved sentinel that never names a real published record (see
// responseeventstore.retentionGapEventLocked's own "sequence zero is
// reserved" doc comment), so FirstAvailableSequence <= 0 can only mean the
// payload never declared where history resumes, and gapNoticeText would
// otherwise render the nonsensical "... history resumes at sequence 0."
// FromSequence == 0 is deliberately accepted on its own: production's own
// compaction-fallback gap (fragmentmap.streamGapPayloadFromCompaction, for a
// compaction fragment with no detailed summary) legitimately reports
// {FromSequence: 0, ToSequence: 0, FirstAvailableSequence: 1} when the exact
// evicted range is unknown, and that must keep projecting rather than start
// failing the turn's whole update drain.
func validateStreamGapPayload(gap workers.StreamGapPayload) error {
	if gap.AffectedItemID != "" {
		return nil
	}
	if gap.FirstAvailableSequence <= 0 {
		return fmt.Errorf(
			"%w: retention-scoped StreamGapPayload requires a positive firstAvailableSequence (the resumable position after the gap)",
			ErrMalformedRecord,
		)
	}
	if gap.FromSequence < 0 || gap.ToSequence < 0 || gap.ToSequence < gap.FromSequence || gap.FirstAvailableSequence <= gap.ToSequence {
		return fmt.Errorf(
			"%w: retention-scoped StreamGapPayload requires non-negative fromSequence <= toSequence < firstAvailableSequence",
			ErrMalformedRecord,
		)
	}
	return nil
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
