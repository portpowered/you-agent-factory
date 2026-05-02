package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestHistorySubsystem_Execute_MergesHistoryFromDispatchConsumedTokens(t *testing.T) {
	timestamp := time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC)
	subsystem := NewHistory(nil)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Results: []interfaces.WorkResult{{
			DispatchID:   "dispatch-1",
			TransitionID: "transition-review",
			Outcome:      interfaces.OutcomeFailed,
		}},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-1": {
				DispatchID: "dispatch-1",
				ConsumedTokens: []interfaces.Token{
					{
						ID:      "token-1",
						PlaceID: "story:init",
						Color: interfaces.TokenColor{
							WorkID:     "story-1",
							WorkTypeID: "story",
						},
						History: interfaces.TokenHistory{
							TotalVisits: map[string]int{
								"transition-build": 2,
							},
							ConsecutiveFailures: map[string]int{
								"transition-review": 1,
							},
							PlaceVisits: map[string]int{
								"story:init": 3,
							},
							LastError: "previous failure",
							FailureLog: []interfaces.FailureRecord{{
								TransitionID: "transition-build",
								Timestamp:    timestamp,
								Error:        "build failed",
								Attempt:      1,
							}},
						},
					},
				},
			},
		},
	}

	result, err := subsystem.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil TickResult")
	}
	if len(result.Histories) != 1 {
		t.Fatalf("len(Histories) = %d, want 1", len(result.Histories))
	}

	history := result.Histories[0]
	if got := history.TotalVisits["transition-build"]; got != 2 {
		t.Fatalf("TotalVisits[transition-build] = %d, want 2", got)
	}
	if got := history.TotalVisits["transition-review"]; got != 1 {
		t.Fatalf("TotalVisits[transition-review] = %d, want 1", got)
	}
	if got := history.ConsecutiveFailures["transition-review"]; got != 2 {
		t.Fatalf("ConsecutiveFailures[transition-review] = %d, want 2", got)
	}
	if got := history.PlaceVisits["story:init"]; got != 3 {
		t.Fatalf("PlaceVisits[story:init] = %d, want 3", got)
	}
	if history.LastError != "previous failure" {
		t.Fatalf("LastError = %q, want %q", history.LastError, "previous failure")
	}
	if len(history.FailureLog) != 1 {
		t.Fatalf("len(FailureLog) = %d, want 1", len(history.FailureLog))
	}
	if history.FailureLog[0].Timestamp != timestamp {
		t.Fatalf("FailureLog[0].Timestamp = %s, want %s", history.FailureLog[0].Timestamp, timestamp)
	}
}

func TestBuildHistory_WhenDispatchLookupMissing_UsesOnlyCurrentResult(t *testing.T) {
	history := buildHistory(nil, &interfaces.WorkResult{
		DispatchID:   "dispatch-missing",
		TransitionID: "transition-review",
		Outcome:      interfaces.OutcomeAccepted,
	})

	if got := history.TotalVisits["transition-review"]; got != 1 {
		t.Fatalf("TotalVisits[transition-review] = %d, want 1", got)
	}
	if got := history.ConsecutiveFailures["transition-review"]; got != 0 {
		t.Fatalf("ConsecutiveFailures[transition-review] = %d, want 0", got)
	}
	if len(history.PlaceVisits) != 0 {
		t.Fatalf("PlaceVisits should be empty, got %+v", history.PlaceVisits)
	}
}
