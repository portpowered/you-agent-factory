package projectionquery_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestCloneFactoryWorldDispatchCompletion_PreservesProjectionShape(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	original := recordings.FactoryWorldDispatchCompletion{
		DispatchID:    "dispatch-1",
		TransitionID:  "release review",
		CompletedTick: 3,
		CompletedAt:   completedAt,
		WorkItemIDs:   []string{"work-1", "work-2"},
	}

	cloned := recordings.CloneFactoryWorldDispatchCompletion(original)
	if !reflect.DeepEqual(cloned, original) {
		t.Fatalf("clone = %#v, want %#v", cloned, original)
	}

	encoded, err := json.Marshal(cloned)
	if err != nil {
		t.Fatalf("marshal cloned completion: %v", err)
	}
	var roundTripped recordings.FactoryWorldDispatchCompletion
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal cloned completion: %v", err)
	}
	if roundTripped.DispatchID != original.DispatchID || roundTripped.TransitionID != original.TransitionID {
		t.Fatalf("round trip = %#v, want dispatch identity preserved from %#v", roundTripped, original)
	}
}

func TestFactoryWorldState_RoundTripsDetachedProjectionPayload(t *testing.T) {
	t.Parallel()

	state := recordings.FactoryWorldState{
		Tick:      3,
		EventTime: time.Date(2026, 7, 28, 5, 15, 0, 0, time.UTC),
		TerminalWorkByID: map[string]recordings.FactoryTerminalWork{
			"work-1": {
				WorkItem: work.FactoryWorkItem{ID: "work-1", WorkTypeID: "goal", State: "done"},
				Status:   "TERMINAL",
			},
		},
	}

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	var decoded recordings.FactoryWorldState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal world state: %v", err)
	}
	if decoded.Tick != state.Tick || decoded.TerminalWorkByID["work-1"].Status != "TERMINAL" {
		t.Fatalf("decoded = %#v, want tick and terminal status from %#v", decoded, state)
	}
}
