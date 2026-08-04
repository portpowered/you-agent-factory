package mapping

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func usageDraft(payload json.RawMessage) workers.Draft {
	return workers.Draft{Kind: workers.KindUsage, Phase: workers.PhaseUpdated, Payload: payload}
}

func TestProjectUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		draft    workers.Draft
		wantUsed int
		wantNoop bool
		wantErr  bool
	}{
		{
			name:     "total tokens projects a usage update",
			draft:    usageDraft(mustMarshal(t, workers.UsagePayload{TotalTokens: 120, Model: "gpt-5"})),
			wantUsed: 120,
		},
		{
			name:     "missing total tokens falls back to input plus output",
			draft:    usageDraft(mustMarshal(t, workers.UsagePayload{InputTokens: 30, OutputTokens: 15})),
			wantUsed: 45,
		},
		{
			name:     "model alone with no token counts produces no update",
			draft:    usageDraft(mustMarshal(t, workers.UsagePayload{Model: "gpt-5"})),
			wantNoop: true,
		},
		{
			name:     "zero-valued usage produces no update",
			draft:    usageDraft(mustMarshal(t, workers.UsagePayload{})),
			wantNoop: true,
		},
		{
			name:    "malformed usage payload is rejected",
			draft:   usageDraft(json.RawMessage(`{"totalTokens":"not-a-number"}`)),
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectUsage(tt.draft)

			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectUsage() unexpected err = %v", err)
			}
			if tt.wantNoop {
				requireNoUpdate(t, update)
				return
			}
			if update == nil || update.UsageUpdate == nil {
				t.Fatalf("ProjectUsage() update = %+v, want a populated UsageUpdate", update)
			}
			if got := update.UsageUpdate.Used; got != tt.wantUsed {
				t.Fatalf("ProjectUsage() Used = %d, want %d", got, tt.wantUsed)
			}
			if got := update.UsageUpdate.Size; got != 0 {
				t.Fatalf("ProjectUsage() Size = %d, want 0 (no source for context-window capacity)", got)
			}
		})
	}
}
