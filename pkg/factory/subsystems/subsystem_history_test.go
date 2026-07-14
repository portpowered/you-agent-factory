package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
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

func TestBuildHistory_MergesSharedLineageVisitCountsWithMaxNotSum(t *testing.T) {
	consumed := []interfaces.Token{
		{
			Color: interfaces.TokenColor{WorkID: "task-1", WorkTypeID: "task"},
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"process": 3, "review": 2},
			},
		},
		{
			Color: interfaces.TokenColor{WorkID: "review-1", WorkTypeID: "review", ParentID: "task-1"},
			History: interfaces.TokenHistory{
				TotalVisits: map[string]int{"process": 3, "review": 1},
			},
		},
	}

	history := buildHistory(consumed, &interfaces.WorkResult{
		TransitionID: "review",
		Outcome:      interfaces.OutcomeContinue,
	}, "task-1")

	if got := history.TotalVisits["process"]; got != 3 {
		t.Fatalf("TotalVisits[process] = %d, want 3", got)
	}
	if got := history.TotalVisits["review"]; got != 3 {
		t.Fatalf("TotalVisits[review] = %d, want 3", got)
	}
}

func TestBuildHistory_ExcludesDifferentWorkOnSharedTrace(t *testing.T) {
	const sharedTrace = "batch-trace"
	consumed := []interfaces.Token{
		{
			Color:   interfaces.TokenColor{WorkID: "task-a", WorkTypeID: "task", CurrentChainingTraceID: sharedTrace},
			History: interfaces.TokenHistory{TotalVisits: map[string]int{"process": 1, "review": 0}},
		},
		{
			Color:   interfaces.TokenColor{WorkID: "review-b", WorkTypeID: "review", ParentID: "task-b", CurrentChainingTraceID: sharedTrace},
			History: interfaces.TokenHistory{TotalVisits: map[string]int{"process": 7, "review": 6}},
		},
	}

	history := buildHistory(consumed, &interfaces.WorkResult{
		TransitionID: "review",
		Outcome:      interfaces.OutcomeRejected,
	}, "task-a")

	if got := history.TotalVisits["process"]; got != 1 {
		t.Fatalf("TotalVisits[process] = %d, want 1 from task-a only", got)
	}
	if got := history.TotalVisits["review"]; got != 1 {
		t.Fatalf("TotalVisits[review] = %d, want first review visit for task-a", got)
	}
}

func TestBuildHistory_AccumulatesRepeatedCandidateCycles(t *testing.T) {
	consumed := []interfaces.Token{{
		Color:   interfaces.TokenColor{WorkID: "task-a", WorkTypeID: "task"},
		History: interfaces.TokenHistory{TotalVisits: map[string]int{"process": 2, "review": 3}},
	}}

	history := buildHistory(consumed, &interfaces.WorkResult{
		TransitionID: "review",
		Outcome:      interfaces.OutcomeRejected,
	}, "task-a")

	if got := history.TotalVisits["process"]; got != 2 {
		t.Fatalf("TotalVisits[process] = %d, want 2", got)
	}
	if got := history.TotalVisits["review"]; got != 4 {
		t.Fatalf("TotalVisits[review] = %d, want 4", got)
	}
}

func TestCandidateWorkID_UsesAuthoredInputOrderInsteadOfDispatchTokenOrder(t *testing.T) {
	net := &state.Net{Transitions: map[string]*petri.Transition{
		"review": {
			ID: "review",
			InputArcs: []petri.Arc{
				{PlaceID: "task:in-review"},
				{PlaceID: "review:init"},
			},
		},
	}}
	consumed := []interfaces.Token{
		{PlaceID: "review:init", Color: interfaces.TokenColor{WorkID: "review-b", ParentID: "task-b"}},
		{PlaceID: "task:in-review", Color: interfaces.TokenColor{WorkID: "task-a"}},
	}

	if got := candidateWorkID(net, "review", consumed); got != "task-a" {
		t.Fatalf("candidateWorkID() = %q, want task-a", got)
	}
}

func TestBuildHistory_WhenDispatchLookupMissing_UsesOnlyCurrentResult(t *testing.T) {
	history := buildHistory(nil, &interfaces.WorkResult{
		DispatchID:   "dispatch-missing",
		TransitionID: "transition-review",
		Outcome:      interfaces.OutcomeAccepted,
	}, "")

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
