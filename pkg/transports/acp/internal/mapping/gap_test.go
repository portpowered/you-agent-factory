package mapping

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func gapDraft(payload json.RawMessage) workers.Draft {
	return workers.Draft{Kind: workers.KindStreamGap, Phase: workers.PhaseUpdated, Payload: payload}
}

func TestProjectStreamGap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		draft      workers.Draft
		wantText   string
		wantItemID string
		wantErr    bool
	}{
		{
			name: "retention gap reports bounds and reason",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 5, ToSequence: 9, FirstAvailableSequence: 10, Reason: "retention eviction",
			})),
			wantText: "Records from sequence 5 to 9 are unavailable; history resumes at sequence 10. Reason: retention eviction",
		},
		{
			name: "retention gap without a reason omits the reason clause",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 1, ToSequence: 2, FirstAvailableSequence: 3,
			})),
			wantText: "Records from sequence 1 to 2 are unavailable; history resumes at sequence 3.",
		},
		{
			name: "item gap reports the declared reason and carries item identity",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				AffectedItemID: "item-7", Reason: "provider reconnect lost buffered deltas",
			})),
			wantText:   "Some records for this item are unavailable: provider reconnect lost buffered deltas",
			wantItemID: "item-7",
		},
		{
			name:    "malformed gap payload is rejected",
			draft:   gapDraft(json.RawMessage(`{"fromSequence":"not-a-number"}`)),
			wantErr: true,
		},
		{
			name:    "empty retention payload is rejected rather than fabricating a resumes-at-sequence-0 notice",
			draft:   gapDraft(json.RawMessage(`{}`)),
			wantErr: true,
		},
		{
			name: "retention payload with a non-positive firstAvailableSequence is rejected",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 1, ToSequence: 2, FirstAvailableSequence: 0,
			})),
			wantErr: true,
		},
		{
			name: "retention payload with inverted bounds is rejected",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 9, ToSequence: 5, FirstAvailableSequence: 10,
			})),
			wantErr: true,
		},
		{
			name: "retention payload whose first-available position does not exceed toSequence is rejected",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 1, ToSequence: 3, FirstAvailableSequence: 3,
			})),
			wantErr: true,
		},
		{
			name: "retention payload with an unknown fromSequence floor of zero is accepted, matching production's compaction-fallback gap",
			draft: gapDraft(mustMarshal(t, workers.StreamGapPayload{
				FromSequence: 0, ToSequence: 0, FirstAvailableSequence: 1, Reason: "compaction",
			})),
			wantText: "Records from sequence 0 to 0 are unavailable; history resumes at sequence 1. Reason: compaction",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectStreamGap(tt.draft)

			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectStreamGap() unexpected err = %v", err)
			}
			if update == nil || update.AgentThoughtChunk == nil {
				t.Fatalf("ProjectStreamGap() update = %+v, want a populated AgentThoughtChunk", update)
			}
			if got := textOf(update.AgentThoughtChunk.Content); got != tt.wantText {
				t.Fatalf("ProjectStreamGap() text = %q, want %q", got, tt.wantText)
			}
			assertMessageID(t, update.AgentThoughtChunk.MessageId, tt.wantItemID)
		})
	}
}
