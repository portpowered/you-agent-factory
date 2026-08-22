package mapping

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func sessionDraft(phase workers.Phase, payload json.RawMessage) workers.Draft {
	return workers.Draft{Kind: workers.KindSession, Phase: phase, Payload: payload}
}

func TestProjectSessionInfoUpdate(t *testing.T) {
	t.Parallel()

	title := "Renamed session"

	cases := []struct {
		name      string
		draft     workers.Draft
		wantTitle string
		wantNoop  bool
		wantErr   bool
	}{
		{
			name:      "a title change projects a session info update",
			draft:     sessionDraft(workers.PhaseUpdated, mustMarshal(t, workers.SessionPayload{Title: &title})),
			wantTitle: title,
		},
		{
			name:     "no title declared produces no update",
			draft:    sessionDraft(workers.PhaseUpdated, mustMarshal(t, workers.SessionPayload{Status: "active"})),
			wantNoop: true,
		},
		{
			name:     "a lifecycle phase never reaches session-info projection even with a title-bearing payload",
			draft:    sessionDraft(workers.PhaseStarted, mustMarshal(t, workers.SessionPayload{Title: &title})),
			wantNoop: true,
		},
		{
			name:    "malformed session payload is rejected",
			draft:   sessionDraft(workers.PhaseUpdated, json.RawMessage(`{"title":123}`)),
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			update, err := ProjectSessionInfoUpdate(tt.draft)

			if tt.wantErr {
				requireMalformed(t, update, err)
				return
			}
			if err != nil {
				t.Fatalf("ProjectSessionInfoUpdate() unexpected err = %v", err)
			}
			if tt.wantNoop {
				requireNoUpdate(t, update)
				return
			}
			if update == nil || update.SessionInfoUpdate == nil {
				t.Fatalf("ProjectSessionInfoUpdate() update = %+v, want a populated SessionInfoUpdate", update)
			}
			if update.SessionInfoUpdate.Title == nil || *update.SessionInfoUpdate.Title != tt.wantTitle {
				t.Fatalf("ProjectSessionInfoUpdate() Title = %v, want %q", update.SessionInfoUpdate.Title, tt.wantTitle)
			}
		})
	}
}

// TestProjectSessionInfoUpdate_ThroughDispatch proves the same behavior is
// reachable through Project's exhaustive dispatch, not only when calling
// ProjectSessionInfoUpdate directly.
func TestProjectSessionInfoUpdate_ThroughDispatch(t *testing.T) {
	t.Parallel()

	title := "Renamed session"
	update, err := Project(sessionDraft(workers.PhaseUpdated, mustMarshal(t, workers.SessionPayload{Title: &title})))
	if err != nil {
		t.Fatalf("Project() unexpected err = %v", err)
	}
	if update == nil || update.SessionInfoUpdate == nil {
		t.Fatalf("Project() update = %+v, want a populated SessionInfoUpdate", update)
	}
	if update.SessionInfoUpdate.Title == nil || *update.SessionInfoUpdate.Title != title {
		t.Fatalf("Project() Title = %v, want %q", update.SessionInfoUpdate.Title, title)
	}
}
