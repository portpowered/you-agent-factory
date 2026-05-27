package projections_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildFactoryWorldView_ProjectsFailedTerminalWorkFailureDetails(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := failedTerminalWorkProjectionEvents(t0)

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)
	if got := len(activeView.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:failed"]); got != 0 {
		t.Fatalf("active tick failed occupancy = %#v, want none", activeView.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:failed"])
	}

	failedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState failed tick: %v", err)
	}
	assertFailedTerminalWorkProjection(t, BuildFactoryWorldView(failedState))
}

func TestBuildFactoryWorldView_ProjectsCanonicalDispatchAndProviderSessionInputsFromEvents(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := canonicalDispatchProviderSessionProjectionEvents(t0)

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)
	activeInput := activeView.Runtime.ActiveExecutionsByDispatchID["dispatch-1"].ConsumedInputs[0]
	if activeInput.TokenID != "work-1" ||
		activeInput.WorkItem == nil ||
		activeInput.WorkItem.TraceID != "trace-1" ||
		activeInput.WorkItem.Tags["priority"] != "high" {
		t.Fatalf("active consumed input = %#v, want traced work-1 input with tags", activeInput)
	}

	completedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	assertCanonicalDispatchProviderSessionProjection(t, BuildFactoryWorldView(completedState))
}

func TestBuildFactoryWorldView_HidesSystemTimeWorkFromNormalDashboardProjection(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		systemTimeInitialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-time", interfaces.FactoryWorkItem{
			ID:          "time-daily-refresh",
			WorkTypeID:  interfaces.SystemTimeWorkTypeID,
			DisplayName: "daily-refresh tick",
			TraceID:     "trace-time",
			PlaceID:     interfaces.SystemTimePendingPlaceID,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
				interfaces.TimeWorkTagKeyDueAt:           t0.Add(time.Second).Format(time.RFC3339Nano),
			},
		}),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-story", interfaces.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Customer story",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if got := worldState.PlaceOccupancyByID[interfaces.SystemTimePendingPlaceID].TokenCount; got != 1 {
		t.Fatalf("world state system time occupancy = %d, want retained debug token", got)
	}

	view := BuildFactoryWorldView(worldState)

	if _, ok := view.Topology.WorkstationNodesByID[interfaces.SystemTimeExpiryTransitionID]; ok {
		t.Fatalf("system expiry transition should be hidden from dashboard topology")
	}
	cronNode := view.Topology.WorkstationNodesByID["daily-refresh"]
	if reflect.DeepEqual(cronNode, interfaces.FactoryWorldWorkstationNode{}) {
		t.Fatalf("cron workstation node missing from dashboard topology")
	}
	if containsString(cronNode.InputPlaceIDs, interfaces.SystemTimePendingPlaceID) {
		t.Fatalf("cron dashboard input places = %#v, want internal time place hidden", cronNode.InputPlaceIDs)
	}
	if _, ok := view.Runtime.PlaceTokenCounts[interfaces.SystemTimePendingPlaceID]; ok {
		t.Fatalf("system time place token count should be hidden, got %#v", view.Runtime.PlaceTokenCounts)
	}
	if _, ok := view.Runtime.CurrentWorkItemsByPlaceID[interfaces.SystemTimePendingPlaceID]; ok {
		t.Fatalf("system time work items should be hidden from current work items")
	}
	if _, ok := view.Runtime.PlaceOccupancyWorkItemsByPlaceID[interfaces.SystemTimePendingPlaceID]; ok {
		t.Fatalf("system time work items should be hidden from place occupancy work items")
	}
	if got := view.Runtime.PlaceTokenCounts["task:init"]; got != 1 {
		t.Fatalf("customer task:init token count = %d, want 1", got)
	}
	if got := view.Runtime.CurrentWorkItemsByPlaceID["task:init"]; len(got) != 1 || got[0].WorkID != "work-1" {
		t.Fatalf("customer task:init current work = %#v, want work-1", got)
	}
	assertCanonicalFactoryKeepsSystemTimeWhileDashboardSurfacesHideIt(t, view)
}

func assertCanonicalFactoryKeepsSystemTimeWhileDashboardSurfacesHideIt(
	t *testing.T,
	view interfaces.FactoryWorldView,
) {
	t.Helper()

	encodedFactory, err := json.Marshal(view.Factory)
	if err != nil {
		t.Fatalf("marshal canonical factory graph: %v", err)
	}
	if !strings.Contains(string(encodedFactory), interfaces.SystemTimeWorkTypeID) {
		t.Fatalf("canonical factory graph omitted raw system time identifier: %s", string(encodedFactory))
	}
	encodedDashboardSurfaces, err := json.Marshal(struct {
		Topology interfaces.FactoryWorldTopologyView `json:"topology"`
		Runtime  interfaces.FactoryWorldRuntimeView  `json:"runtime"`
	}{
		Topology: view.Topology,
		Runtime:  view.Runtime,
	})
	if err != nil {
		t.Fatalf("marshal dashboard topology/runtime: %v", err)
	}
	if strings.Contains(string(encodedDashboardSurfaces), interfaces.SystemTimeWorkTypeID) {
		t.Fatalf("dashboard topology/runtime leaked raw system time identifier: %s", string(encodedDashboardSurfaces))
	}
}

// portos:func-length-exception owner=agent-factory reason=system-time-expiry-dashboard-fixture review=2026-07-18 removal=share-system-time-fixture-builders-before-next-world-view-change
func TestBuildFactoryWorldView_LabelsSystemTimeExpiryDispatchForDashboard(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		systemTimeInitialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-time", interfaces.FactoryWorkItem{
			ID:          "time-daily-refresh",
			WorkTypeID:  interfaces.SystemTimeWorkTypeID,
			DisplayName: "daily-refresh tick",
			TraceID:     "trace-time",
			PlaceID:     interfaces.SystemTimePendingPlaceID,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
				interfaces.TimeWorkTagKeyDueAt:           t0.Add(time.Second).Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       t0.Add(time.Minute).Format(time.RFC3339Nano),
			},
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-expire",
			TransitionID: interfaces.SystemTimeExpiryTransitionID,
			Workstation:  interfaces.FactoryWorkstationRef{ID: interfaces.SystemTimeExpiryTransitionID, Name: interfaces.SystemTimeExpiryTransitionID},
			Inputs: []interfaces.WorkstationInput{{
				TokenID: "tok-time",
				PlaceID: interfaces.SystemTimePendingPlaceID,
				WorkItem: &interfaces.FactoryWorkItem{
					ID:         "time-daily-refresh",
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					TraceID:    "trace-time",
					PlaceID:    interfaces.SystemTimePendingPlaceID,
					Tags: map[string]string{
						interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
					},
				},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-expire",
			TransitionID:   interfaces.SystemTimeExpiryTransitionID,
			Workstation:    interfaces.FactoryWorkstationRef{ID: interfaces.SystemTimeExpiryTransitionID, Name: interfaces.SystemTimeExpiryTransitionID},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis: 10,
		}),
	}

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)
	if _, ok := activeView.Runtime.ActiveExecutionsByDispatchID["dispatch-expire"]; ok {
		t.Fatalf("active system-time-only expiry dispatch should stay hidden from normal dashboard executions")
	}

	completedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	view := BuildFactoryWorldView(completedState)

	if _, ok := view.Topology.WorkstationNodesByID[interfaces.SystemTimeExpiryTransitionID]; ok {
		t.Fatalf("raw system expiry transition should be hidden from dashboard topology")
	}
	if len(view.Runtime.Session.DispatchHistory) != 1 {
		t.Fatalf("dispatch history = %#v, want one expiry dispatch", view.Runtime.Session.DispatchHistory)
	}
	expiryDispatch := view.Runtime.Session.DispatchHistory[0]
	if expiryDispatch.TransitionID != interfaces.SystemTimeExpiryTransitionID ||
		expiryDispatch.Workstation.Name != interfaces.SystemTimeExpiryTransitionID {
		t.Fatalf("expiry dispatch = %#v, want canonical system-time transition id", expiryDispatch)
	}
	if len(expiryDispatch.ConsumedInputs) != 1 || expiryDispatch.ConsumedInputs[0].WorkItem == nil {
		t.Fatalf("expiry dispatch consumed inputs = %#v, want one time work input", expiryDispatch.ConsumedInputs)
	}
	timeInput := expiryDispatch.ConsumedInputs[0]
	if timeInput.PlaceID != interfaces.SystemTimePendingPlaceID ||
		timeInput.WorkItem.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("expiry dispatch input = %#v, want canonical time-work metadata", timeInput)
	}
}

func TestBuildFactoryWorldView_HidesSystemTimeOnlyDispatchesFromSessionCounts(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		systemTimeInitialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-time", interfaces.FactoryWorkItem{
			ID:          "time-expired",
			WorkTypeID:  interfaces.SystemTimeWorkTypeID,
			DisplayName: "expired cron tick",
			TraceID:     "trace-time",
			PlaceID:     interfaces.SystemTimePendingPlaceID,
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-expire",
			TransitionID: interfaces.SystemTimeExpiryTransitionID,
			Workstation:  interfaces.FactoryWorkstationRef{ID: interfaces.SystemTimeExpiryTransitionID, Name: interfaces.SystemTimeExpiryTransitionID},
			Inputs: []interfaces.WorkstationInput{
				{
					TokenID: "tok-time",
					PlaceID: interfaces.SystemTimePendingPlaceID,
					WorkItem: &interfaces.FactoryWorkItem{
						ID:         "time-expired",
						WorkTypeID: interfaces.SystemTimeWorkTypeID,
						TraceID:    "trace-time",
						PlaceID:    interfaces.SystemTimePendingPlaceID,
					},
				},
			},
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if _, ok := worldState.ActiveDispatches["dispatch-expire"]; !ok {
		t.Fatalf("world state should retain system-time dispatch for reconstruction")
	}

	view := BuildFactoryWorldView(worldState)

	if view.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("InFlightDispatchCount = %d, want 0 for system-time-only dispatch", view.Runtime.InFlightDispatchCount)
	}
	if len(view.Runtime.ActiveExecutionsByDispatchID) != 0 {
		t.Fatalf("active executions = %#v, want none for system-time-only dispatch", view.Runtime.ActiveExecutionsByDispatchID)
	}
	if view.Runtime.Session.HasData || view.Runtime.Session.DispatchedCount != 0 {
		t.Fatalf("session = %#v, want no public session data for system-time-only dispatch", view.Runtime.Session)
	}
}

func TestBuildFactoryWorldView_RetainsSystemTimeConsumedTokenMetadataForDebugTrace(t *testing.T) {
	execution := buildWorldViewExecutionWithSystemTimeConsumedInput(t)
	if got := execution.WorkItems; len(got) != 1 || got[0].WorkID != "work-1" {
		t.Fatalf("active execution work items = %#v, want only customer work", got)
	}
	timeInput := requireSystemTimeConsumedInput(t, execution)
	if timeInput.PlaceID != interfaces.SystemTimePendingPlaceID {
		t.Fatalf("time input place = %q, want %q", timeInput.PlaceID, interfaces.SystemTimePendingPlaceID)
	}
	if timeInput.WorkItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "daily-refresh" ||
		timeInput.WorkItem.Tags[interfaces.TimeWorkTagKeyDueAt] == "" {
		t.Fatalf("time input tags = %#v, want cron workstation and due_at metadata", timeInput.WorkItem.Tags)
	}
}

func buildWorldViewExecutionWithSystemTimeConsumedInput(t *testing.T) interfaces.FactoryWorldActiveExecution {
	t.Helper()

	t0 := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		systemTimeInitialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-time", interfaces.FactoryWorkItem{
			ID:          "time-daily-refresh",
			WorkTypeID:  interfaces.SystemTimeWorkTypeID,
			DisplayName: "daily-refresh tick",
			TraceID:     "trace-time",
			PlaceID:     interfaces.SystemTimePendingPlaceID,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
				interfaces.TimeWorkTagKeyNominalAt:       t0.Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyDueAt:           t0.Add(time.Second).Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       t0.Add(time.Minute).Format(time.RFC3339Nano),
			},
		}),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-story", interfaces.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Customer story",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-cron",
			TransitionID: "daily-refresh",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "daily-refresh", Name: "Daily refresh"},
			Inputs: []interfaces.WorkstationInput{
				{
					TokenID:  "tok-story",
					PlaceID:  "task:init",
					WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Customer story", TraceID: "trace-1", PlaceID: "task:init"},
				},
				{
					TokenID: "tok-time",
					PlaceID: interfaces.SystemTimePendingPlaceID,
					WorkItem: &interfaces.FactoryWorkItem{
						ID:         "time-daily-refresh",
						WorkTypeID: interfaces.SystemTimeWorkTypeID,
						TraceID:    "trace-time",
						PlaceID:    interfaces.SystemTimePendingPlaceID,
						Tags: map[string]string{
							interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh",
							interfaces.TimeWorkTagKeyDueAt:           t0.Add(time.Second).Format(time.RFC3339Nano),
						},
					},
				},
			},
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	return BuildFactoryWorldView(worldState).Runtime.ActiveExecutionsByDispatchID["dispatch-cron"]
}

func requireSystemTimeConsumedInput(t *testing.T, execution interfaces.FactoryWorldActiveExecution) *interfaces.WorkstationInput {
	t.Helper()

	for i := range execution.ConsumedInputs {
		if execution.ConsumedInputs[i].WorkItem != nil &&
			execution.ConsumedInputs[i].WorkItem.WorkTypeID == interfaces.SystemTimeWorkTypeID {
			return &execution.ConsumedInputs[i]
		}
	}

	t.Fatalf("consumed inputs = %#v, want retained system time input", execution.ConsumedInputs)
	return nil
}

func failedTerminalWorkProjectionEvents(t0 time.Time) []factoryapi.FactoryEvent {
	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:          "work-failed",
			WorkTypeID:  "task",
			DisplayName: "Blocked story",
			TraceID:     "trace-failed",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-failed",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-failed/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeFailed,
			DurationMillis:     500,
			ErrorClass:         stringPtrForProjectionTest("throttled"),
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-failed"}),
			Diagnostics: generatedWorkDiagnosticsForProjectionTest(&interfaces.SafeWorkDiagnostics{
				Provider: &interfaces.SafeProviderDiagnostic{
					Provider: "codex",
					Model:    "gpt-5.4",
					ResponseMetadata: map[string]string{
						"retry_count": "1",
					},
				},
			}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:      "dispatch-failed",
			TransitionID:    "t-review",
			Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:          interfaces.WorkstationResult{Outcome: "FAILED", Error: "provider throttled", FailureReason: "throttled", FailureMessage: "Provider rate limit exceeded."},
			DurationMillis:  500,
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-failed"},
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-failed-terminal",
				ToPlace:  "task:failed",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
			}},
			TraceData: &interfaces.FactoryTraceData{TraceID: "trace-failed", WorkIDs: []string{"work-failed"}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked story", TraceID: "trace-failed", PlaceID: "task:failed"},
				Status:   "FAILED",
			},
		}),
	}
}

func assertFailedTerminalWorkProjection(t *testing.T, failedView interfaces.FactoryWorldView) {
	t.Helper()

	if failedView.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("failed tick completed count = %d, want 0", failedView.Runtime.Session.CompletedCount)
	}
	failedItems := failedView.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:failed"]
	if len(failedItems) != 1 || failedItems[0].DisplayName != "Blocked story" {
		t.Fatalf("failed occupancy = %#v, want Blocked story in task:failed", failedItems)
	}
	if len(failedView.Runtime.Session.DispatchHistory) != 1 ||
		failedView.Runtime.Session.DispatchHistory[0].Result.FailureReason != "throttled" {
		t.Fatalf("dispatch history = %#v, want retained failure reason", failedView.Runtime.Session.DispatchHistory)
	}
	if len(failedView.Runtime.Session.ProviderSessions) != 1 ||
		failedView.Runtime.Session.ProviderSessions[0].FailureMessage != "Provider rate limit exceeded." {
		t.Fatalf("provider sessions = %#v, want retained failure message", failedView.Runtime.Session.ProviderSessions)
	}
}

func canonicalDispatchProviderSessionProjectionEvents(t0 time.Time) []factoryapi.FactoryEvent {
	input := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Review draft",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
		Tags:        map[string]string{"priority": "high"},
	}
	output := interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Reviewed draft",
		TraceID:     "trace-1",
		PlaceID:     "task:complete",
		Tags:        map[string]string{"priority": "high"},
	}
	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEventWithToken(1, t0.Add(time.Second), "tok-work-1", input),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "tok-work-1",
				PlaceID:  "task:init",
				WorkItem: &input,
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     1200,
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"}),
			Diagnostics: generatedWorkDiagnosticsForProjectionTest(&interfaces.SafeWorkDiagnostics{
				Provider: &interfaces.SafeProviderDiagnostic{
					Provider: "codex",
					Model:    "gpt-5.4",
				},
			}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:      "dispatch-1",
			TransitionID:    "t-review",
			Workstation:     interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:          interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis:  1200,
			OutputWork:      []interfaces.FactoryWorkItem{output},
			TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
			TerminalWork:    &interfaces.FactoryTerminalWork{WorkItem: output, Status: "TERMINAL"},
		}),
	}
}

func assertCanonicalDispatchProviderSessionProjection(t *testing.T, completedView interfaces.FactoryWorldView) {
	t.Helper()

	dispatch := completedView.Runtime.Session.DispatchHistory[0]
	if len(dispatch.ConsumedInputs) != 1 || dispatch.ConsumedInputs[0].WorkItem == nil || dispatch.ConsumedInputs[0].WorkItem.ID != "work-1" {
		t.Fatalf("dispatch consumed inputs = %#v, want work-1", dispatch.ConsumedInputs)
	}
	if len(dispatch.OutputWorkItems) == 0 || dispatch.OutputWorkItems[0].PlaceID != "task:complete" {
		t.Fatalf("dispatch output work items = %#v, want completed work item", dispatch.OutputWorkItems)
	}
	if dispatch.ConsumedInputs[0].PlaceID != "task:init" || dispatch.OutputWorkItems[0].DisplayName != "Reviewed draft" {
		t.Fatalf("dispatch route/details = %#v, want task:init -> Reviewed draft", dispatch)
	}
	if dispatch.TerminalWork == nil || dispatch.TerminalWork.WorkItem.TraceID != "trace-1" {
		t.Fatalf("terminal work = %#v, want trace-backed terminal work", dispatch.TerminalWork)
	}
	providerSession := completedView.Runtime.Session.ProviderSessions[0]
	if len(providerSession.ConsumedInputs) != 1 || providerSession.ConsumedInputs[0].WorkItem == nil || providerSession.ConsumedInputs[0].WorkItem.ID != "work-1" {
		t.Fatalf("provider session consumed inputs = %#v, want work-1", providerSession.ConsumedInputs)
	}
}

func systemTimeInitialStructureEvent(eventTime time.Time) factoryapi.FactoryEvent {
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{
				{
					Name: "task",
					States: []factoryapi.WorkState{
						{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
						{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
					},
				},
				{
					Name: interfaces.SystemTimeWorkTypeID,
					States: []factoryapi.WorkState{
						{Name: interfaces.SystemTimePendingState, Type: factoryapi.WorkStateTypePROCESSING},
					},
				},
			},
			Workstations: &[]factoryapi.Workstation{
				{
					Id:       stringPtrForProjectionTest("daily-refresh"),
					Name:     "Daily refresh",
					Behavior: workstationKindPtrForWorldViewTest(factoryapi.WorkstationKindCron),
					Worker:   "refresh-worker",
					Inputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}, {WorkType: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}},
					Outputs:  &[]factoryapi.WorkstationIO{{WorkType: "task", State: "done"}},
				},
				{
					Id:      stringPtrForProjectionTest(interfaces.SystemTimeExpiryTransitionID),
					Name:    interfaces.SystemTimeExpiryTransitionID,
					Worker:  "",
					Inputs:  []factoryapi.WorkstationIO{{WorkType: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}},
					Outputs: nil,
				},
			},
		},
	}
	return generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial-with-time", 0, eventTime, factoryapi.FactoryEventContext{}, payload)
}

// portos:func-length-exception owner=agent-factory reason=resource-count-event-fixture-builder review=2026-07-19 removal=split-resource-count-event-builders-before-next-world-view-fixture-change
func resourceCountProjectionEvents(eventTime time.Time) []factoryapi.FactoryEvent {
	workstationID := "implement"
	workstation := factoryapi.Workstation{
		Id:      &workstationID,
		Name:    "Implement",
		Worker:  "agent",
		Inputs:  []factoryapi.WorkstationIO{{WorkType: "story", State: "new"}, {WorkType: "agent-slot", State: "available"}},
		Outputs: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
	}
	resources := []factoryapi.Resource{{Name: "agent-slot", Capacity: 2}}
	workTypes := []factoryapi.WorkType{{
		Name:   "story",
		States: []factoryapi.WorkState{{Name: "new", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "done", Type: factoryapi.WorkStateTypeTERMINAL}},
	}}
	workstations := []factoryapi.Workstation{workstation}
	work := factoryapi.Work{
		Name:         "Resource Occupancy Story",
		TraceId:      stringPtrForProjectionTest("trace-resource-count"),
		WorkId:       stringPtrForProjectionTest("work-resource-count"),
		WorkTypeName: stringPtrForProjectionTest("story"),
	}
	dispatchContext := factoryapi.FactoryEventContext{
		DispatchId: stringPtrForProjectionTest("dispatch-resource-count"),
		TraceIds:   stringSlicePtrForProjectionTest([]string{"trace-resource-count"}),
		WorkIds:    stringSlicePtrForProjectionTest([]string{"work-resource-count"}),
	}

	return []factoryapi.FactoryEvent{
		resourceCountInitialStructureEvent(eventTime, resources, workTypes, workstations),
		resourceCountWorkRequestEvent(eventTime, work),
		resourceCountDispatchCreatedEvent(eventTime, dispatchContext, work, workstation),
		resourceCountDispatchCompletedEvent(eventTime, dispatchContext, work, workstation),
	}
}

func resourceCountInitialStructureEvent(
	eventTime time.Time,
	resources []factoryapi.Resource,
	workTypes []factoryapi.WorkType,
	workstations []factoryapi.Workstation,
) factoryapi.FactoryEvent {
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeInitialStructureRequest,
		"resource-count-structure",
		1,
		eventTime,
		factoryapi.FactoryEventContext{},
		factoryapi.InitialStructureRequestEventPayload{
			Factory: factoryapi.Factory{
				Resources:    &resources,
				WorkTypes:    &workTypes,
				Workstations: &workstations,
			},
		},
	)
}

func resourceCountWorkRequestEvent(eventTime time.Time, work factoryapi.Work) factoryapi.FactoryEvent {
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeWorkRequest,
		"resource-count-work-input",
		2,
		eventTime.Add(time.Second),
		factoryapi.FactoryEventContext{
			RequestId: stringPtrForProjectionTest("request-resource-count"),
			TraceIds:  stringSlicePtrForProjectionTest([]string{"trace-resource-count"}),
			WorkIds:   stringSlicePtrForProjectionTest([]string{"work-resource-count"}),
		},
		factoryapi.WorkRequestEventPayload{
			Type:  factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works: &[]factoryapi.Work{work},
		},
	)
}

func resourceCountDispatchCreatedEvent(
	eventTime time.Time,
	dispatchContext factoryapi.FactoryEventContext,
	work factoryapi.Work,
	workstation factoryapi.Workstation,
) factoryapi.FactoryEvent {
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeDispatchRequest,
		"resource-count-request",
		3,
		eventTime.Add(2*time.Second),
		dispatchContext,
		factoryapi.DispatchRequestEventPayload{
			Inputs:       []factoryapi.DispatchConsumedWorkRef{{WorkId: stringValueForProjectionTest(work.WorkId)}},
			Resources:    &[]factoryapi.Resource{{Name: "agent-slot"}},
			TransitionId: "implement",
		},
	)
}

func resourceCountDispatchCompletedEvent(
	eventTime time.Time,
	dispatchContext factoryapi.FactoryEventContext,
	work factoryapi.Work,
	workstation factoryapi.Workstation,
) factoryapi.FactoryEvent {
	return generatedProjectionEvent(
		factoryapi.FactoryEventTypeDispatchResponse,
		"resource-count-response",
		4,
		eventTime.Add(3*time.Second),
		dispatchContext,
		factoryapi.DispatchResponseEventPayload{
			DurationMillis:  int64PtrForProjectionTest(1000),
			Outcome:         factoryapi.WorkOutcomeAccepted,
			OutputResources: &[]factoryapi.Resource{{Name: "agent-slot"}},
			OutputWork:      &[]factoryapi.Work{work},
			TransitionId:    "implement",
		},
	)
}

func workstationKindPtrForWorldViewTest(value factoryapi.WorkstationKind) *factoryapi.WorkstationKind {
	return &value
}

func hasResourcePlaceRef(refs []interfaces.FactoryWorldPlaceRef, placeID string) bool {
	for _, ref := range refs {
		if ref.PlaceID == placeID && ref.Kind == "resource" {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
