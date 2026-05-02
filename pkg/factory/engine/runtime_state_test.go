package engine

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestRuntimeState_Snapshot_Independence(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	marking.AddToken(&interfaces.Token{
		ID:      "tok-1",
		PlaceID: "place-a",
		Color:   interfaces.TokenColor{WorkID: "work-1", WorkTypeID: "type-1"},
	})

	rs := &RuntimeState{
		Marking: marking,
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-1": {
				DispatchID:   "dispatch-1",
				TransitionID: "trans-1",
				StartTime:    time.Now().Add(-time.Second),
				ConsumedTokens: []interfaces.Token{
					{
						ID:        "tok-1",
						PlaceID:   "place-a",
						CreatedAt: time.Unix(123, 0),
						EnteredAt: time.Unix(456, 0),
						Color: interfaces.TokenColor{
							WorkID:     "work-1",
							WorkTypeID: "type-1",
							Tags:       map[string]string{"source": "dispatcher"},
							Payload:    []byte("payload"),
						},
						History: interfaces.TokenHistory{
							TotalVisits:         map[string]int{"trans-1": 1},
							ConsecutiveFailures: map[string]int{"trans-1": 0},
							PlaceVisits:         map[string]int{"place-a": 1},
							FailureLog: []interfaces.FailureRecord{
								{TransitionID: "trans-0", Error: "first failure", Attempt: 1, Timestamp: time.Unix(120, 0)},
							},
						},
					},
				},
				HeldMutations: []interfaces.MarkingMutation{
					{Type: "CONSUME", TokenID: "tok-1", FromPlace: "place-a"},
				},
			},
		},
		Results: []interfaces.WorkResult{
			{
				DispatchID:   "dispatch-0",
				TransitionID: "trans-0",
				Outcome:      interfaces.OutcomeAccepted,
				ProviderSession: &interfaces.ProviderSessionMetadata{
					Provider: "codex",
					Kind:     "session_id",
					ID:       "sess-result-1",
				},
			},
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID:   "dispatch-0",
				TransitionID: "trans-0",
				ProviderSession: &interfaces.ProviderSessionMetadata{
					Provider: "codex",
					Kind:     "session_id",
					ID:       "sess-history-1",
				},
				StartTime: time.Now().Add(-2 * time.Second),
				EndTime:   time.Now().Add(-time.Second),
				Duration:  time.Second,
				ConsumedTokens: []interfaces.Token{
					{
						ID:      "tok-0",
						PlaceID: "place-z",
						Color: interfaces.TokenColor{
							WorkID:     "work-0",
							WorkTypeID: "type-0",
							Tags:       map[string]string{"history": "original"},
						},
					},
				},
				OutputMutations: []interfaces.TokenMutationRecord{
					{
						DispatchID:   "dispatch-0",
						TransitionID: "trans-0",
						Outcome:      interfaces.OutcomeAccepted,
						Type:         interfaces.MutationCreate,
						TokenID:      "work-0",
						ToPlace:      "place-complete",
						Token: &interfaces.Token{
							ID:      "work-0",
							PlaceID: "place-complete",
							Color:   interfaces.TokenColor{WorkID: "work-0", WorkTypeID: "type-0"},
						},
					},
				},
			},
		},
		TickCount: 5,
	}

	snap := rs.Snapshot()

	// --- Mutate the original and verify the snapshot is unaffected ---

	// Mutate marking
	rs.Marking.AddToken(&interfaces.Token{ID: "tok-2", PlaceID: "place-b"})
	if _, exists := snap.Marking.Tokens["tok-2"]; exists {
		t.Error("snapshot marking should not contain token added after snapshot")
	}

	// Mutate dispatches map
	rs.Dispatches["dispatch-2"] = &interfaces.DispatchEntry{DispatchID: "dispatch-2", TransitionID: "trans-2"}
	if _, exists := snap.Dispatches["dispatch-2"]; exists {
		t.Error("snapshot dispatches should not contain entry added after snapshot")
	}

	// Mutate held mutations in existing dispatch
	rs.Dispatches["dispatch-1"].HeldMutations = append(rs.Dispatches["dispatch-1"].HeldMutations,
		interfaces.MarkingMutation{Type: "MOVE", TokenID: "tok-extra"})
	if len(snap.Dispatches["dispatch-1"].HeldMutations) != 1 {
		t.Errorf("snapshot dispatch held mutations should have 1 entry, got %d", len(snap.Dispatches["dispatch-1"].HeldMutations))
	}

	// Mutate consumed tokens in existing dispatch
	rs.Dispatches["dispatch-1"].ConsumedTokens[0].Color.Tags["source"] = "mutated"
	rs.Dispatches["dispatch-1"].ConsumedTokens[0].History.TotalVisits["trans-new"] = 99
	rs.Dispatches["dispatch-1"].ConsumedTokens[0].History.FailureLog[0].Error = "mutated failure"
	if snap.Dispatches["dispatch-1"].ConsumedTokens[0].Color.Tags["source"] != "dispatcher" {
		t.Error("snapshot dispatch consumed token tags should not reflect mutations to original")
	}
	if _, exists := snap.Dispatches["dispatch-1"].ConsumedTokens[0].History.TotalVisits["trans-new"]; exists {
		t.Error("snapshot dispatch consumed token history should not reflect new visits added after snapshot")
	}
	if snap.Dispatches["dispatch-1"].ConsumedTokens[0].History.FailureLog[0].Error != "first failure" {
		t.Error("snapshot dispatch consumed token failure log should not reflect mutations to original")
	}

	// Mutate results slice
	rs.Results = append(rs.Results, interfaces.WorkResult{TransitionID: "trans-extra"})
	if len(snap.Results) != 1 {
		t.Errorf("snapshot results should have 1 entry, got %d", len(snap.Results))
	}
	rs.Results[0].ProviderSession.ID = "mutated-result-session"
	if snap.Results[0].ProviderSession.ID != "sess-result-1" {
		t.Error("snapshot result provider session should not reflect mutations to original")
	}

	// Mutate dispatch history slice
	rs.DispatchHistory = append(rs.DispatchHistory, interfaces.CompletedDispatch{TransitionID: "trans-extra"})
	if len(snap.DispatchHistory) != 1 {
		t.Errorf("snapshot dispatch history should have 1 entry, got %d", len(snap.DispatchHistory))
	}
	rs.DispatchHistory[0].ConsumedTokens[0].Color.Tags["history"] = "mutated"
	if snap.DispatchHistory[0].ConsumedTokens[0].Color.Tags["history"] != "original" {
		t.Error("snapshot dispatch history consumed token tags should not reflect mutations to original")
	}
	rs.DispatchHistory[0].OutputMutations[0].Token.Color.WorkID = "mutated-work"
	if snap.DispatchHistory[0].OutputMutations[0].Token.Color.WorkID != "work-0" {
		t.Error("snapshot dispatch history output token should not reflect mutations to original")
	}
	rs.DispatchHistory[0].ProviderSession.ID = "mutated-history-session"
	if snap.DispatchHistory[0].ProviderSession.ID != "sess-history-1" {
		t.Error("snapshot dispatch history provider session should not reflect mutations to original")
	}

	// Mutate tick count
	rs.TickCount = 999
	if snap.TickCount != 5 {
		t.Errorf("snapshot tick count should be 5, got %d", snap.TickCount)
	}
}

func TestCompletedDispatch_Timing(t *testing.T) {
	start := time.Now().Add(-500 * time.Millisecond)
	end := time.Now()
	duration := end.Sub(start)

	cd := interfaces.CompletedDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "trans-1",
		StartTime:    start,
		EndTime:      end,
		Duration:     duration,
	}

	if cd.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
	if cd.EndTime.IsZero() {
		t.Error("EndTime should not be zero")
	}
	if cd.Duration <= 0 {
		t.Errorf("Duration should be positive, got %v", cd.Duration)
	}
	if cd.EndTime.Before(cd.StartTime) {
		t.Error("EndTime should not be before StartTime")
	}
	// Duration should be consistent with start/end times (within tolerance for time precision).
	expectedDuration := cd.EndTime.Sub(cd.StartTime)
	if cd.Duration != expectedDuration {
		t.Errorf("Duration %v does not match EndTime-StartTime %v", cd.Duration, expectedDuration)
	}
}

func TestRuntimeState_Snapshot_EmptyState(t *testing.T) {
	rs := &RuntimeState{
		Marking: petri.NewMarking("wf-empty"),
	}

	snap := rs.Snapshot()

	if snap.Marking.WorkflowID != "wf-empty" {
		t.Errorf("expected workflow ID 'wf-empty', got %q", snap.Marking.WorkflowID)
	}
	if snap.Dispatches != nil {
		t.Error("empty dispatches should snapshot as nil")
	}
	if snap.Results != nil {
		t.Error("empty results should snapshot as nil")
	}
	if snap.DispatchHistory != nil {
		t.Error("empty dispatch history should snapshot as nil")
	}
}
