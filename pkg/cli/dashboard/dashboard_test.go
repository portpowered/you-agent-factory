package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// buildTestTopology creates a minimal topology with one work type for testing.
func buildTestTopology() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "processing", Category: state.StateCategoryProcessing},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}
	return &state.Net{
		ID:        "test-net",
		Places:    places,
		WorkTypes: map[string]*state.WorkType{"task": wt},
	}
}

func TestFormatSimpleDashboardWithRenderData_RendersSessionMetricsAndActiveRows(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	topology := buildTestTopology()

	output := FormatSimpleDashboardWithRenderData(
		activeRawEngineSnapshotForDashboardTest(now, topology),
		activeDashboardRenderDataForDashboardTest(now),
		now,
	)

	for _, want := range []string{
		"Active Workstations (1)",
		"story",
		"review-station",
		"11:59:15",
		"45s",
		"dashboard cleanup",
		"Queue Counts",
		"story:init",
		"Workstation Activity",
		"world-dispatch",
		"trace-dashboard",
		"Session Metrics",
		"Workstations Dispatched:  1  (story=1)",
		"Workstations Completed:   0",
		"Workstations Failed:      0",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	for _, absent := range []string{"raw-should-not-render", "raw-workstation"} {
		if strings.Contains(output, absent) {
			t.Errorf("output should not contain raw snapshot value %q:\n%s", absent, output)
		}
	}
}

func activeRawEngineSnapshotForDashboardTest(now time.Time, topology *state.Net) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
		Dispatches: map[string]*interfaces.DispatchEntry{
			"raw-dispatch": {
				TransitionID:    "raw-transition",
				WorkstationName: "raw-workstation",
				StartTime:       now.Add(-5 * time.Second),
				ConsumedTokens: []interfaces.Token{{
					ID:      "raw-token",
					PlaceID: "task:processing",
					Color:   interfaces.TokenColor{Name: "raw-should-not-render", WorkID: "raw-work", WorkTypeID: "task"},
				}},
			},
		},
		FactoryState:  "RUNNING",
		RuntimeStatus: interfaces.RuntimeStatusActive,
		Topology:      topology,
		Uptime:        10 * time.Minute,
	}
}

func activeDashboardRenderDataForDashboardTest(now time.Time) dashboardrender.SimpleDashboardRenderData {
	return dashboardrender.SimpleDashboardRenderData{
		InFlightDispatchCount: 1,
		ActiveExecutionsByDispatchID: map[string]dashboardrender.SimpleDashboardActiveExecution{
			"world-dispatch": {
				DispatchID:      "world-dispatch",
				TransitionID:    "review-transition",
				WorkstationName: "review-station",
				StartedAt:       now.Add(-45 * time.Second),
				WorkTypeIDs:     []string{"story"},
				WorkItems: []interfaces.FactoryWorldWorkItemRef{
					{WorkID: "work-1", WorkTypeID: "story", DisplayName: "dashboard cleanup"},
				},
			},
		},
		WorkstationActivityByNodeID: map[string]dashboardrender.SimpleDashboardWorkstationActivity{
			"review-transition": {
				NodeID:            "review-transition",
				WorkstationName:   "review-station",
				ActiveDispatchIDs: []string{"world-dispatch"},
				ActiveWorkItems: []interfaces.FactoryWorldWorkItemRef{
					{WorkID: "work-1", WorkTypeID: "story", DisplayName: "dashboard cleanup"},
				},
				TraceIDs: []string{"trace-dashboard"},
			},
		},
		PlaceTokenCounts: map[string]int{"story:init": 1},
		CurrentWorkItemsByPlaceID: map[string][]interfaces.FactoryWorldWorkItemRef{
			"story:init": {
				{WorkID: "work-1", WorkTypeID: "story", DisplayName: "dashboard cleanup"},
			},
		},
		Session: dashboardrender.SimpleDashboardSessionData{
			HasData:              true,
			DispatchedCount:      1,
			CompletedCount:       0,
			FailedCount:          0,
			DispatchedByWorkType: map[string]int{"story": 1},
		},
	}
}

// portos:func-length-exception owner=agent-factory reason=dashboard-world-view-formatting-fixture review=2026-07-18 removal=split-fixture-builders-when-dashboard-fixtures-are-refactored
func TestFormatSimpleDashboardWithRenderData_RendersTerminalProviderAndDispatchDetails(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	topology := buildTestTopology()

	es := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
		DispatchHistory: []interfaces.CompletedDispatch{{
			DispatchID:      "raw-dispatch",
			TransitionID:    "raw-transition",
			WorkstationName: "raw-workstation",
			Outcome:         interfaces.OutcomeAccepted,
			ConsumedTokens: []interfaces.Token{
				{ID: "raw-token", Color: interfaces.TokenColor{Name: "raw-input", WorkID: "raw-work", WorkTypeID: "task"}},
			},
		}},
		FactoryState:  "RUNNING",
		RuntimeStatus: interfaces.RuntimeStatusIdle,
		Topology:      topology,
		Uptime:        20 * time.Minute,
	}
	renderData := dashboardrender.SimpleDashboardRenderData{
		InFlightDispatchCount: 1,
		ActiveExecutionsByDispatchID: map[string]dashboardrender.SimpleDashboardActiveExecution{
			"dispatch-active": {
				DispatchID:      "dispatch-active",
				TransitionID:    "plan",
				WorkstationName: "Planner",
				StartedAt:       now.Add(-25 * time.Second),
				WorkTypeIDs:     []string{"story"},
				WorkItems: []interfaces.FactoryWorldWorkItemRef{
					{WorkID: "work-active", WorkTypeID: "story", DisplayName: "Plan rollout"},
				},
			},
		},
		PlaceOccupancyWorkItemsByPlaceID: map[string][]interfaces.FactoryWorldWorkItemRef{
			"story:complete": {
				{WorkID: "work-complete", WorkTypeID: "story", DisplayName: "Docs complete"},
			},
			"story:failed": {
				{WorkID: "work-failed", WorkTypeID: "story", DisplayName: "Blocked change"},
			},
		},
		PlaceCategoriesByID: map[string]string{
			"story:complete": "TERMINAL",
			"story:failed":   "FAILED",
		},
		Session: dashboardrender.SimpleDashboardSessionData{
			HasData:              true,
			DispatchedCount:      3,
			CompletedCount:       1,
			FailedCount:          1,
			DispatchedByWorkType: map[string]int{"story": 3},
			CompletedByWorkType:  map[string]int{"story": 1},
			FailedByWorkType:     map[string]int{"story": 1},
			DispatchHistory: []interfaces.FactoryWorldDispatchCompletion{
				{
					DispatchID:      "dispatch-complete",
					TransitionID:    "write",
					Workstation:     interfaces.FactoryWorkstationRef{Name: "Writer"},
					InputWorkItems:  []interfaces.FactoryWorkItem{{ID: "work-complete", WorkTypeID: "story", DisplayName: "Draft docs"}},
					OutputWorkItems: []interfaces.FactoryWorkItem{{ID: "work-complete", WorkTypeID: "story", DisplayName: "Docs complete"}},
					Result:          interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
					StartedAt:       now.Add(-70 * time.Second),
					CompletedAt:     now.Add(-65 * time.Second),
					DurationMillis:  5000,
				},
				{
					DispatchID:      "dispatch-rejected",
					TransitionID:    "review",
					Workstation:     interfaces.FactoryWorkstationRef{Name: "Reviewer"},
					InputWorkItems:  []interfaces.FactoryWorkItem{{ID: "work-rejected", WorkTypeID: "story", DisplayName: "Review draft"}},
					OutputWorkItems: []interfaces.FactoryWorkItem{{ID: "work-rejected", WorkTypeID: "story", DisplayName: "Needs rewrite"}},
					Result:          interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeRejected), Feedback: "missing acceptance tests"},
					StartedAt:       now.Add(-60 * time.Second),
					CompletedAt:     now.Add(-45 * time.Second),
					DurationMillis:  15000,
				},
				{
					DispatchID:      "dispatch-failed",
					TransitionID:    "ship",
					Workstation:     interfaces.FactoryWorkstationRef{Name: "Publisher"},
					InputWorkItems:  []interfaces.FactoryWorkItem{{ID: "work-failed", WorkTypeID: "story", DisplayName: "Ship change"}},
					OutputWorkItems: []interfaces.FactoryWorkItem{{ID: "work-failed", WorkTypeID: "story", DisplayName: "Blocked change"}},
					Result:          interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeFailed), FailureReason: "throttled", FailureMessage: "provider unavailable"},
					StartedAt:       now.Add(-40 * time.Second),
					CompletedAt:     now.Add(-20 * time.Second),
					DurationMillis:  20000,
				},
			},
			ProviderSessions: []interfaces.FactoryWorldProviderSessionRecord{{
				DispatchID:      "dispatch-failed",
				TransitionID:    "ship",
				WorkstationName: "Publisher",
				ConsumedInputs:  []interfaces.WorkstationInput{{WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "story", DisplayName: "Blocked change"}}},
				Outcome:         string(interfaces.OutcomeFailed),
				FailureReason:   "throttled",
				FailureMessage:  "provider unavailable",
				ProviderSession: interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-failed"},
			}},
		},
	}

	output := FormatSimpleDashboardWithRenderData(es, renderData, now)

	for _, want := range []string{
		"Active Workstations (1)",
		"Planner",
		"Plan rollout",
		"Completed Workstations",
		"Success",
		"Rejected",
		"Failed",
		"Writer",
		"Reviewer",
		"Publisher",
		"Draft docs",
		"Docs complete",
		"Review draft",
		"Needs rewrite",
		"Ship change",
		"Blocked change",
		"missing acceptance tests",
		"throttled - provider unavailable",
		"Provider sessions:",
		"Blocked change [dispatch-failed] codex / session_id / sess-failed",
		"Blocked change [dispatch-failed] Publisher throttled - provider unavailable",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	for _, absent := range []string{"raw-dispatch", "raw-workstation", "raw-input"} {
		if strings.Contains(output, absent) {
			t.Errorf("output should not contain raw snapshot value %q:\n%s", absent, output)
		}
	}
}

func TestFormatSimpleDashboardWithRenderData_MapsSystemTimeCompatibilityAtCliBoundary(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	topology := buildTestTopology()

	output := FormatSimpleDashboardWithRenderData(
		interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking:         petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
			FactoryState:    "RUNNING",
			RuntimeStatus:   interfaces.RuntimeStatusIdle,
			Uptime:          2 * time.Minute,
			Topology:        topology,
			TickCount:       4,
			DispatchHistory: nil,
		},
		dashboardrender.SimpleDashboardRenderData{
			Session: dashboardrender.SimpleDashboardSessionData{
				HasData:         true,
				DispatchedCount: 1,
				CompletedCount:  0,
				FailedCount:     1,
				DispatchHistory: []interfaces.FactoryWorldDispatchCompletion{
					{
						DispatchID:     "dispatch-expire",
						TransitionID:   interfaces.SystemTimeExpiryTransitionID,
						Workstation:    interfaces.FactoryWorkstationRef{Name: interfaces.SystemTimeExpiryTransitionID},
						Result:         interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeFailed), FailureReason: "expired"},
						StartedAt:      now.Add(-15 * time.Second),
						CompletedAt:    now.Add(-10 * time.Second),
						DurationMillis: 5000,
					},
				},
				ProviderSessions: []interfaces.FactoryWorldProviderSessionRecord{{
					DispatchID:      "dispatch-expire",
					TransitionID:    interfaces.SystemTimeExpiryTransitionID,
					WorkstationName: interfaces.SystemTimeExpiryTransitionID,
					Outcome:         string(interfaces.OutcomeFailed),
					FailureReason:   "expired",
					ProviderSession: interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-expire"},
				}},
			},
		},
		now,
	)

	for _, want := range []string{
		"time:expire",
		"codex / session_id / sess-expire",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, interfaces.SystemTimeExpiryTransitionID) {
		t.Fatalf("output should not expose raw system-time transition id:\n%s", output)
	}
}

func TestDashboardSessionViewFromRenderData_FallsBackToDispatchHistoryWorkItems(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)

	renderData := dashboardrender.SimpleDashboardRenderData{
		Session: dashboardrender.SimpleDashboardSessionData{
			HasData:              true,
			DispatchedCount:      5,
			CompletedCount:       2,
			FailedCount:          2,
			DispatchedByWorkType: map[string]int{"story": 5},
			CompletedByWorkType:  map[string]int{"story": 2},
			FailedByWorkType:     map[string]int{"story": 2},
			DispatchHistory: []interfaces.FactoryWorldDispatchCompletion{
				{
					DispatchID:   "accepted-terminal",
					TransitionID: "write",
					Workstation:  interfaces.FactoryWorkstationRef{Name: "Writer"},
					TerminalWork: &interfaces.FactoryTerminalWork{
						Status:   "COMPLETE",
						WorkItem: interfaces.FactoryWorkItem{ID: "completed-terminal", WorkTypeID: "story", DisplayName: "Published draft"},
					},
					OutputWorkItems: []interfaces.FactoryWorkItem{
						{ID: "completed-terminal", WorkTypeID: "story", DisplayName: "should not replace terminal"},
					},
					Result:         interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
					StartedAt:      now.Add(-50 * time.Second),
					CompletedAt:    now.Add(-40 * time.Second),
					DurationMillis: 10000,
				},
				{
					DispatchID:   "accepted-output",
					TransitionID: "review",
					Workstation:  interfaces.FactoryWorkstationRef{Name: "Reviewer"},
					TerminalWork: &interfaces.FactoryTerminalWork{
						Status:   "FAILED",
						WorkItem: interfaces.FactoryWorkItem{ID: "completed-output", WorkTypeID: "story", DisplayName: "should skip failed terminal"},
					},
					OutputWorkItems: []interfaces.FactoryWorkItem{
						{ID: "completed-output", WorkTypeID: "story", DisplayName: "Review ready"},
					},
					Result:         interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
					StartedAt:      now.Add(-39 * time.Second),
					CompletedAt:    now.Add(-30 * time.Second),
					DurationMillis: 9000,
				},
				{
					DispatchID:     "accepted-input-only",
					TransitionID:   "draft",
					Workstation:    interfaces.FactoryWorkstationRef{Name: "Drafter"},
					InputWorkItems: []interfaces.FactoryWorkItem{{ID: "completed-input-only", WorkTypeID: "story", DisplayName: "should stay hidden"}},
					Result:         interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
					StartedAt:      now.Add(-35 * time.Second),
					CompletedAt:    now.Add(-31 * time.Second),
					DurationMillis: 4000,
				},
				{
					DispatchID:   "failed-terminal",
					TransitionID: "ship",
					Workstation:  interfaces.FactoryWorkstationRef{Name: "Publisher"},
					TerminalWork: &interfaces.FactoryTerminalWork{
						Status:   "FAILED",
						WorkItem: interfaces.FactoryWorkItem{ID: "failed-terminal", WorkTypeID: "story", DisplayName: "Publish blocked"},
					},
					OutputWorkItems: []interfaces.FactoryWorkItem{
						{ID: "failed-terminal", WorkTypeID: "story", DisplayName: "should not replace failed terminal"},
					},
					Result: interfaces.WorkstationResult{
						Outcome:        string(interfaces.OutcomeFailed),
						FailureReason:  "throttled",
						FailureMessage: "provider unavailable",
					},
					StartedAt:      now.Add(-29 * time.Second),
					CompletedAt:    now.Add(-20 * time.Second),
					DurationMillis: 9000,
				},
				{
					DispatchID:   "failed-output-and-input-fallback",
					TransitionID: interfaces.SystemTimeExpiryTransitionID,
					Workstation:  interfaces.FactoryWorkstationRef{Name: interfaces.SystemTimeExpiryTransitionID},
					InputWorkItems: []interfaces.FactoryWorkItem{
						{ID: "failed-output", WorkTypeID: "story", DisplayName: "should not replace failed output"},
						{ID: "failed-input", WorkTypeID: "story", DisplayName: "Retry later"},
					},
					OutputWorkItems: []interfaces.FactoryWorkItem{
						{ID: "failed-output", WorkTypeID: "story", DisplayName: "Expired artifact"},
					},
					Result: interfaces.WorkstationResult{
						Outcome:       string(interfaces.OutcomeFailed),
						FailureReason: "expired",
					},
					StartedAt:      now.Add(-19 * time.Second),
					CompletedAt:    now.Add(-10 * time.Second),
					DurationMillis: 9000,
				},
			},
		},
	}
	view := dashboardSessionViewFromRenderData(renderData)

	if got, want := view.CompletedWorkLabels, []string{"Published draft", "Review ready"}; !equalStrings(got, want) {
		t.Fatalf("CompletedWorkLabels = %v, want %v", got, want)
	}
	if got, want := view.FailedWorkLabels, []string{"Expired artifact", "Publish blocked", "Retry later"}; !equalStrings(got, want) {
		t.Fatalf("FailedWorkLabels = %v, want %v", got, want)
	}
	if len(view.FailedWorkDetails) != 3 {
		t.Fatalf("len(FailedWorkDetails) = %d, want 3", len(view.FailedWorkDetails))
	}

	detailsByLabel := make(map[string]dashboardFailedWorkDetail, len(view.FailedWorkDetails))
	for _, detail := range view.FailedWorkDetails {
		detailsByLabel[detail.WorkItem.DisplayName] = detail
	}

	publishBlocked := detailsByLabel["Publish blocked"]
	if publishBlocked.DispatchID != "failed-terminal" ||
		publishBlocked.WorkstationName != "Publisher" ||
		publishBlocked.FailureReason != "throttled" ||
		publishBlocked.FailureMessage != "provider unavailable" {
		t.Fatalf("Publish blocked detail = %+v", publishBlocked)
	}

	expiredArtifact := detailsByLabel["Expired artifact"]
	if expiredArtifact.DispatchID != "failed-output-and-input-fallback" ||
		expiredArtifact.TransitionID != interfaces.SystemTimeDashboardExpiryTransitionID ||
		expiredArtifact.WorkstationName != interfaces.SystemTimeDashboardExpiryTransitionID ||
		expiredArtifact.FailureReason != "expired" ||
		expiredArtifact.FailureMessage != "" {
		t.Fatalf("Expired artifact detail = %+v", expiredArtifact)
	}

	retryLater := detailsByLabel["Retry later"]
	if retryLater.DispatchID != "failed-output-and-input-fallback" ||
		retryLater.TransitionID != interfaces.SystemTimeDashboardExpiryTransitionID ||
		retryLater.WorkstationName != interfaces.SystemTimeDashboardExpiryTransitionID ||
		retryLater.FailureReason != "expired" ||
		retryLater.FailureMessage != "" {
		t.Fatalf("Retry later detail = %+v", retryLater)
	}

	output := FormatSimpleDashboardWithRenderData(
		interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking:       petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
			FactoryState:  "RUNNING",
			RuntimeStatus: interfaces.RuntimeStatusIdle,
			Topology:      buildTestTopology(),
			Uptime:        5 * time.Minute,
		},
		renderData,
		now,
	)

	for _, want := range []string{
		"Failed work: 3",
		"Expired artifact [failed-output-and-input-fallback] time:expire expired",
		"Publish blocked [failed-terminal] Publisher throttled - provider unavailable",
		"Retry later [failed-output-and-input-fallback] time:expire expired",
		"Completed work: 2",
		"Published draft",
		"Review ready",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "should skip failed terminal") {
		t.Fatalf("output should not contain failed terminal completed label:\n%s", output)
	}
}

func TestFormatSimpleDashboard_SnapshotOnlyDoesNotRenderSessionRows(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	topology := buildTestTopology()

	es := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
		DispatchHistory: []interfaces.CompletedDispatch{{
			DispatchID:      "raw-dispatch",
			TransitionID:    "raw-transition",
			WorkstationName: "raw-workstation",
			Outcome:         interfaces.OutcomeAccepted,
			ConsumedTokens: []interfaces.Token{
				{ID: "raw-token", PlaceID: "task:processing", Color: interfaces.TokenColor{Name: "raw-input", WorkID: "raw-work", WorkTypeID: "task"}},
			},
		}},
		FactoryState: "RUNNING",
	}

	output := FormatSimpleDashboard(es, topology, now)

	for _, absent := range []string{"Session Metrics", "Completed Workstations", "raw-workstation", "raw-input"} {
		if strings.Contains(output, absent) {
			t.Errorf("output should not contain snapshot-only session value %q:\n%s", absent, output)
		}
	}
}

func TestFormatSimpleDashboard_NoRemovedSections(t *testing.T) {
	now := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	topology := buildTestTopology()

	es := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		TickCount:     1,
		RuntimeStatus: interfaces.RuntimeStatusFinished,
		Marking: petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-1": {ID: "tok-1", PlaceID: "task:failed", Color: interfaces.TokenColor{WorkTypeID: "task"}},
			},
		},
		FactoryState: "RUNNING",
	}

	output := FormatSimpleDashboard(es, topology, now)

	if !strings.Contains(output, "Runtime: FINISHED") {
		t.Fatalf("output missing runtime status:\n%s", output)
	}

	for _, absent := range []string{"Resources", "Bottlenecks", "Failures", "Active Work Items", "Work Summary"} {
		if strings.Contains(output, absent) {
			t.Errorf("output should not contain %q section:\n%s", absent, output)
		}
	}
}

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 15*time.Minute, "2h15m"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDurationShort(tt.d)
			if got != tt.want {
				t.Errorf("formatDurationShort(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatDashboardTime(t *testing.T) {
	value := time.Date(2026, 4, 3, 12, 0, 0, 0, time.Local)
	got := formatDashboardTime(value)
	if got != "12:00:00" {
		t.Fatalf("formatDashboardTime() = %q, want %q", got, "12:00:00")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
