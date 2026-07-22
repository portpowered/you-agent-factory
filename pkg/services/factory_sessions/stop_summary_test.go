package factorysessions_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProjectFactorySessionStopSummaryProjectsPetriDispatchStatuses(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		active  bool
		outcome workerexecution.WorkOutcome
		want    StopDispatchStatus
	}{
		{name: "running", active: true, want: StopDispatchStatusRunning},
		{name: "completed", outcome: workerexecution.OutcomeAccepted, want: StopDispatchStatusCompleted},
		{name: "failed", outcome: workerexecution.OutcomeFailed, want: StopDispatchStatusFailed},
		{name: "rejected", outcome: workerexecution.OutcomeRejected, want: StopDispatchStatusFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, token := stoppedWorkSnapshot(now, "blocked")
			if tc.active {
				snapshot.Dispatches = map[string]*interfaces.DispatchEntry{"dispatch-1": {DispatchID: "dispatch-1", WorkstationName: "review", ConsumedTokens: []workerexecution.Token{*token}}}
			} else {
				snapshot.DispatchHistory = []interfaces.CompletedDispatch{{DispatchID: "dispatch-1", WorkstationName: "review", Outcome: tc.outcome, EndTime: now, ConsumedTokens: []workerexecution.Token{*token}}}
			}
			summary := ProjectFactorySessionStopSummary("session-1", snapshot, nil)
			if summary == nil || summary.LatestDispatch == nil || summary.LatestDispatch.Status != tc.want {
				t.Fatalf("latest dispatch = %#v, want status %q", summary, tc.want)
			}
		})
	}
}

func TestProjectFactorySessionStopSummaryAppliesCanonicalPrecedence(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 30, 0, 0, time.UTC)
	snapshot, _ := stoppedWorkSnapshot(now, "blocked")
	javascript := &interfaces.FactorySessionJavaScriptRuntimeState{Dispatches: []interfaces.FactorySessionDispatchState{{ID: "js-1", Status: "INTERRUPTED", DispatchKind: "JAVASCRIPT_AGENT"}}}

	summary := ProjectFactorySessionStopSummary("session-1", snapshot, javascript)
	if summary == nil || summary.StopKind != StopKindInterrupted {
		t.Fatalf("stop summary = %#v, want JavaScript interruption before blocked Work", summary)
	}

	snapshot.LifecycleControlStatus = "PAUSED"
	summary = ProjectFactorySessionStopSummary("session-1", snapshot, javascript)
	if summary == nil || summary.StopKind != StopKindPaused {
		t.Fatalf("stop summary = %#v, want pause before interruption", summary)
	}
}

func stoppedWorkSnapshot(now time.Time, stateName string) (*factoryruntime.StateSnapshot, *factoryruntime.RuntimeToken) {
	placeID := "goal:" + stateName
	token := &factoryruntime.RuntimeToken{ID: "token-1", PlaceID: placeID, EnteredAt: now, Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1", WorkTypeID: "goal", Name: "Goal"}}
	return &factoryruntime.StateSnapshot{
		Marking:  factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{token.ID: token}},
		Topology: &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{placeID: {ID: placeID, TypeID: "goal", State: stateName}}},
	}, token
}
