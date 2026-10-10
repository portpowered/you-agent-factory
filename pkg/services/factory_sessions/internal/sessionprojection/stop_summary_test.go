package sessionprojection_test

import (
	"fmt"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	. "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/legacysnapshot"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
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
				snapshot.Dispatches = map[string]*interfaces.DispatchEntry{"dispatch-1": {DispatchID: "dispatch-1", WorkstationName: "review", ConsumedTokens: []workerexecution.Token{{ID: token.ID, State: "blocked", Color: token.Color}}}}
			} else {
				snapshot.DispatchHistory = []interfaces.CompletedDispatch{{DispatchID: "dispatch-1", WorkstationName: "review", Outcome: tc.outcome, EndTime: now, ConsumedTokens: []workerexecution.Token{{ID: token.ID, State: "blocked", Color: token.Color}}}}
			}
			summary := sessionprojection.ProjectFactorySessionStopSummary("session-1", snapshot, nil)
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

	summary := sessionprojection.ProjectFactorySessionStopSummary("session-1", snapshot, javascript)
	if summary == nil || summary.StopKind != StopKindInterrupted {
		t.Fatalf("stop summary = %#v, want JavaScript interruption before blocked Work", summary)
	}

	snapshot.LifecycleControlStatus = "PAUSED"
	summary = sessionprojection.ProjectFactorySessionStopSummary("session-1", snapshot, javascript)
	if summary == nil || summary.StopKind != StopKindPaused {
		t.Fatalf("stop summary = %#v, want pause before interruption", summary)
	}
}

func TestProjectFactorySessionStopSummaryPreservesStructuredSchemaViolationReason(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 30, 0, 0, time.UTC)
	snapshot, token := stoppedWorkSnapshot(now, "blocked")
	snapshot.DispatchHistory = []interfaces.CompletedDispatch{{
		DispatchID: "dispatch-schema-violation",
		Outcome:    workerexecution.OutcomeFailed,
		Reason:     "structured output schema violation: missing property summary",
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyTerminal,
			Type:   workerexecution.WorkFailureTypeStructuredOutputSchemaViolation,
		},
		EndTime:        now,
		ConsumedTokens: []workerexecution.Token{{ID: token.ID, State: "blocked", Color: token.Color}},
	}}

	summary := sessionprojection.ProjectFactorySessionStopSummary("session-1", snapshot, nil)
	if summary == nil || summary.LatestDispatch == nil || summary.LatestDispatch.FailureDetail == nil {
		t.Fatalf("stop summary = %#v, want dispatch failure detail", summary)
	}
	if summary.LatestDispatch.FailureDetail.Reason != StopFailureType("structured_output_schema_violation") {
		t.Fatalf("failure reason = %q, want structured_output_schema_violation", summary.LatestDispatch.FailureDetail.Reason)
	}
}

func stoppedWorkSnapshot(now time.Time, stateName string) (*legacysnapshot.Snapshot, *workerexecution.Token) {
	placeID := "goal:" + stateName
	runtimeToken := &factoryruntime.RuntimeToken{ID: "token-1", PlaceID: placeID, EnteredAt: now, Color: factoryruntime.RuntimeTokenColor{WorkID: "work-1", WorkTypeID: "goal", Name: "Goal"}}
	token := &workerexecution.Token{ID: runtimeToken.ID, State: stateName, EnteredAt: now, Color: runtimeToken.Color}
	return &legacysnapshot.Snapshot{
		Marking:  factoryruntime.PetriMarkingSnapshot{Tokens: map[string]*factoryruntime.RuntimeToken{runtimeToken.ID: runtimeToken}},
		Topology: &factoryruntime.Net{Places: map[string]*factoryruntime.PetriPlace{placeID: {ID: placeID, TypeID: "goal", State: stateName}}},
	}, token
}

func TestProjectWorkStopSummaryIndexedPreservesStatesAndPausePrecedence(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		state  string
		paused bool
		want   StopKind
	}{
		{state: "init"}, {state: "blocked", want: StopKindBlocked}, {state: "needs-human", want: StopKindNeedsHuman}, {state: "interrupted", want: StopKindInterrupted}, {state: "blocked", paused: true, want: StopKindPaused},
	} {
		t.Run(tc.state+fmt.Sprint(tc.paused), func(t *testing.T) {
			t.Parallel()
			snapshot, token := stoppedWorkSnapshot(time.Unix(0, 0), tc.state)
			token.History.LastError = "safe failure"
			if tc.paused {
				snapshot.LifecycleControlStatus = "PAUSED"
			}
			snapshot.DispatchHistory = []interfaces.CompletedDispatch{{DispatchID: "dispatch-1", WorkstationName: "review", Outcome: workerexecution.OutcomeFailed, EndTime: time.Unix(1, 0), ConsumedTokens: []workerexecution.Token{*token}}}
			index := map[string]*workerexecution.Token{"work-1": token}
			summary := sessionprojection.ProjectWorkStopSummary("session-1", snapshot, token, nil, index)
			if tc.want == "" {
				if summary != nil {
					t.Fatalf("running Work has stop: %#v", summary)
				}
				return
			}
			if summary == nil || summary.StopKind != tc.want || summary.WorkID == nil || *summary.WorkID != "work-1" || summary.LatestDispatch == nil || summary.LatestDispatch.DispatchID != "dispatch-1" || summary.SuggestedRecoveryAction == nil {
				t.Fatalf("stop summary: %#v", summary)
			}
		})
	}
}

func TestProjectWorkStopSummaryIndexedUsesFirstMatchingWorkState(t *testing.T) {
	t.Parallel()
	snapshot, blocked := stoppedWorkSnapshot(time.Unix(0, 0), "blocked")
	running := *blocked
	running.ID = "token-2"
	running.State = "init"
	index := map[string]*workerexecution.Token{"work-1": blocked}
	if summary := sessionprojection.ProjectWorkStopSummary("session-1", snapshot, &running, nil, index); summary == nil || summary.StopKind != StopKindBlocked {
		t.Fatalf("matching stopped state: %#v", summary)
	}
}
