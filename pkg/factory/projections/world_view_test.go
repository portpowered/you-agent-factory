package projections

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestBuildFactoryWorldView_ProjectsFromReconstructedWorldState(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Write docs",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
	}
	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	view := BuildFactoryWorldView(worldState)

	if !reflect.DeepEqual(view.Topology.WorkstationNodeIDs, []string{"t-review"}) {
		t.Fatalf("WorkstationNodeIDs = %#v, want [t-review]", view.Topology.WorkstationNodeIDs)
	}
	node := view.Topology.WorkstationNodesByID["t-review"]
	if node.WorkstationName != "Review" {
		t.Fatalf("WorkstationName = %q, want Review", node.WorkstationName)
	}
	if !reflect.DeepEqual(node.InputWorkTypeIDs, []string{"task"}) {
		t.Fatalf("InputWorkTypeIDs = %#v, want [task]", node.InputWorkTypeIDs)
	}
	if view.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("InFlightDispatchCount = %d, want 1", view.Runtime.InFlightDispatchCount)
	}
	if !reflect.DeepEqual(view.Runtime.ActiveDispatchIDs, []string{"dispatch-1"}) {
		t.Fatalf("ActiveDispatchIDs = %#v, want [dispatch-1]", view.Runtime.ActiveDispatchIDs)
	}
	execution := view.Runtime.ActiveExecutionsByDispatchID["dispatch-1"]
	if len(execution.WorkItems) != 1 || execution.WorkItems[0].WorkID != "work-1" {
		t.Fatalf("active work items = %#v, want work-1", execution.WorkItems)
	}
	if len(execution.ConsumedInputs) != 1 || execution.ConsumedInputs[0].TokenID != "work-1" {
		t.Fatalf("consumed inputs = %#v, want work-1", execution.ConsumedInputs)
	}
	if view.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("DispatchedCount = %d, want 1", view.Runtime.Session.DispatchedCount)
	}
}

func TestBuildFactoryWorldViewWithActiveThrottlePauses_ProjectsRuntimePauseMetadata(t *testing.T) {
	view := BuildFactoryWorldViewWithActiveThrottlePauses(
		interfaces.FactoryWorldState{
			Topology: interfaces.InitialStructurePayload{
				Workers: []interfaces.FactoryWorker{
					{ID: "worker-claude", ModelProvider: "claude", Model: "claude-sonnet"},
					{ID: "worker-codex", ModelProvider: "codex", Model: "gpt-5-codex"},
				},
				Workstations: []interfaces.FactoryWorkstation{
					{
						ID:            "t-claude",
						Name:          "Claude Review",
						WorkerID:      "worker-claude",
						InputPlaceIDs: []string{"task:init", interfaces.SystemTimePendingPlaceID},
					},
					{
						ID:            "t-codex",
						Name:          "Codex Review",
						WorkerID:      "worker-codex",
						InputPlaceIDs: []string{"report:init"},
					},
				},
				Places: []interfaces.FactoryPlace{
					{ID: "task:init", TypeID: "task", Category: "INITIAL"},
					{ID: "report:init", TypeID: "report", Category: "INITIAL"},
					{ID: interfaces.SystemTimePendingPlaceID, TypeID: interfaces.SystemTimeWorkTypeID, Category: "PROCESSING"},
				},
			},
		},
		[]interfaces.ActiveThrottlePause{{
			LaneID:      "claude/claude-sonnet",
			Provider:    "claude",
			Model:       "claude-sonnet",
			PausedAt:    time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
			PausedUntil: time.Date(2026, 4, 30, 10, 5, 0, 0, time.UTC),
		}},
	)

	if len(view.Runtime.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(view.Runtime.ActiveThrottlePauses))
	}
	pause := view.Runtime.ActiveThrottlePauses[0]
	if pause.LaneID != "claude/claude-sonnet" || pause.Provider != "claude" || pause.Model != "claude-sonnet" {
		t.Fatalf("pause identity = %#v, want claude/claude-sonnet lane", pause)
	}
	if !pause.RecoverAt.Equal(pause.PausedUntil) {
		t.Fatalf("RecoverAt = %s, want PausedUntil %s", pause.RecoverAt, pause.PausedUntil)
	}
	if !reflect.DeepEqual(pause.AffectedTransitionIDs, []string{"t-claude"}) {
		t.Fatalf("affected transition IDs = %#v, want [t-claude]", pause.AffectedTransitionIDs)
	}
	if !reflect.DeepEqual(pause.AffectedWorkstationNames, []string{"Claude Review"}) {
		t.Fatalf("affected workstation names = %#v, want [Claude Review]", pause.AffectedWorkstationNames)
	}
	if !reflect.DeepEqual(pause.AffectedWorkerTypes, []string{"worker-claude"}) {
		t.Fatalf("affected worker types = %#v, want [worker-claude]", pause.AffectedWorkerTypes)
	}
	if !reflect.DeepEqual(pause.AffectedWorkTypeIDs, []string{"task"}) {
		t.Fatalf("affected work type IDs = %#v, want [task]", pause.AffectedWorkTypeIDs)
	}
}

func TestBuildFactoryWorldView_ProjectsExplicitDispatchChainingLineage(t *testing.T) {
	events := chainingTraceProjectionEvents()
	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)
	assertChainingTraceProjectionActiveView(t, activeView)

	completedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	completedView := BuildFactoryWorldView(completedState)
	assertChainingTraceProjectionCompletedView(t, completedView)
}

func TestBuildFactoryWorldView_ProjectsSubmitEligibleWorkTypesFromInitialStates(t *testing.T) {
	view := BuildFactoryWorldView(interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{
				{
					ID:   "task-internal",
					Name: "task",
					States: []interfaces.FactoryStateDefinition{
						{Value: "init", Category: string(interfaces.StateTypeInitial)},
						{Value: "done", Category: string(interfaces.StateTypeTerminal)},
					},
				},
				{
					ID:   "report-internal",
					Name: "report",
					States: []interfaces.FactoryStateDefinition{
						{Value: "queued", Category: string(interfaces.StateTypeInitial)},
						{Value: "done", Category: string(interfaces.StateTypeTerminal)},
					},
				},
				{
					ID:   "legacy-review",
					Name: "review",
					States: []interfaces.FactoryStateDefinition{
						{Value: "processing", Category: string(interfaces.StateTypeProcessing)},
						{Value: "done", Category: string(interfaces.StateTypeTerminal)},
					},
				},
				{
					ID: "fallback-id",
					States: []interfaces.FactoryStateDefinition{
						{Value: "queued", Category: string(interfaces.StateTypeInitial)},
						{Value: "done", Category: string(interfaces.StateTypeTerminal)},
					},
				},
				{
					ID: interfaces.SystemTimeWorkTypeID,
					States: []interfaces.FactoryStateDefinition{
						{Value: interfaces.SystemTimePendingState, Category: string(interfaces.StateTypeProcessing)},
					},
				},
			},
			Workstations: []interfaces.FactoryWorkstation{{
				ID:   "review",
				Name: "Review",
			}},
		},
	})

	want := []interfaces.FactoryWorldSubmitWorkType{
		{WorkTypeName: "fallback-id"},
		{WorkTypeName: "report"},
		{WorkTypeName: "task"},
	}
	if !reflect.DeepEqual(view.Topology.SubmitWorkTypes, want) {
		t.Fatalf("SubmitWorkTypes = %#v, want %#v", view.Topology.SubmitWorkTypes, want)
	}
}

func TestBuildFactoryWorldView_ProjectsSubmitEligibleWorkTypesWithoutWorkstations(t *testing.T) {
	view := BuildFactoryWorldView(interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{
				{
					ID:   "task-internal",
					Name: "task",
					States: []interfaces.FactoryStateDefinition{
						{Value: "init", Category: string(interfaces.StateTypeInitial)},
						{Value: "done", Category: string(interfaces.StateTypeTerminal)},
					},
				},
				{
					ID: "review",
					States: []interfaces.FactoryStateDefinition{
						{Value: "processing", Category: string(interfaces.StateTypeProcessing)},
					},
				},
			},
		},
	})

	want := []interfaces.FactoryWorldSubmitWorkType{
		{WorkTypeName: "task"},
	}
	if !reflect.DeepEqual(view.Topology.SubmitWorkTypes, want) {
		t.Fatalf("SubmitWorkTypes = %#v, want %#v", view.Topology.SubmitWorkTypes, want)
	}
	if len(view.Topology.WorkstationNodeIDs) != 0 {
		t.Fatalf("WorkstationNodeIDs = %#v, want empty", view.Topology.WorkstationNodeIDs)
	}
	if len(view.Topology.WorkstationNodesByID) != 0 {
		t.Fatalf("WorkstationNodesByID = %#v, want empty", view.Topology.WorkstationNodesByID)
	}
}

func TestBuildFactoryWorldView_ProjectsCurrentWorkItemsByPlaceID(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Write docs",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
	}

	queuedState, err := ReconstructFactoryWorldState(events, 1)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState queued tick: %v", err)
	}
	queuedView := BuildFactoryWorldView(queuedState)
	queuedItems := queuedView.Runtime.CurrentWorkItemsByPlaceID["task:init"]
	if len(queuedItems) != 1 || queuedItems[0].WorkID != "work-1" {
		t.Fatalf("queued task:init work items = %#v, want work-1", queuedItems)
	}

	inFlightState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState in-flight tick: %v", err)
	}
	inFlightView := BuildFactoryWorldView(inFlightState)
	if got := inFlightView.Runtime.CurrentWorkItemsByPlaceID["task:init"]; len(got) != 0 {
		t.Fatalf("in-flight task:init work items = %#v, want empty after consumption", got)
	}
	if got, ok := inFlightView.Runtime.CurrentWorkItemsByPlaceID["task:review"]; !ok || len(got) != 0 {
		t.Fatalf("empty task:review work items = %#v, present=%t, want empty slice", got, ok)
	}
	if _, ok := inFlightView.Runtime.CurrentWorkItemsByPlaceID["task:complete"]; ok {
		t.Fatalf("terminal task:complete should not be exposed as current non-terminal work")
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-world-view-fixture review=2026-07-18 removal=split-terminal-and-failed-occupancy-assertions-before-next-world-view-change
func TestBuildFactoryWorldView_ProjectsSelectedTickTerminalAndFailedPlaceOccupancy(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-complete", WorkTypeID: "task", DisplayName: "Completed docs", TraceID: "trace-complete", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-complete",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-complete",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-complete", WorkTypeID: "task", DisplayName: "Completed docs", TraceID: "trace-complete", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-complete",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis: 2500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-complete-terminal",
				ToPlace:  "task:complete",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-complete", WorkTypeID: "task", DisplayName: "Completed docs", TraceID: "trace-complete", PlaceID: "task:complete"},
			}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-complete", WorkTypeID: "task", DisplayName: "Completed docs", TraceID: "trace-complete", PlaceID: "task:complete"},
				Status:   "TERMINAL",
			},
		}),
		workInputEvent(4, t0.Add(4*time.Second), interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked docs", TraceID: "trace-failed", PlaceID: "task:init"}),
		workstationRequestEvent(5, t0.Add(5*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-failed",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-failed",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked docs", TraceID: "trace-failed", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(6, t0.Add(6*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-failed",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "FAILED", FailureReason: "throttled", FailureMessage: "Provider rate limit exceeded."},
			DurationMillis: 500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-failed-terminal",
				ToPlace:  "task:failed",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked docs", TraceID: "trace-failed", PlaceID: "task:failed"},
			}},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-failed", WorkTypeID: "task", DisplayName: "Blocked docs", TraceID: "trace-failed", PlaceID: "task:failed"},
				Status:   "FAILED",
			},
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 6)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	view := BuildFactoryWorldView(worldState)

	if _, ok := view.Runtime.CurrentWorkItemsByPlaceID["task:complete"]; ok {
		t.Fatalf("terminal task:complete should not be exposed as current non-terminal work")
	}
	if _, ok := view.Runtime.CurrentWorkItemsByPlaceID["task:failed"]; ok {
		t.Fatalf("failed task:failed should not be exposed as current non-terminal work")
	}
	completedRefs := view.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:complete"]
	if len(completedRefs) != 1 || completedRefs[0].WorkID != "work-complete" || completedRefs[0].DisplayName != "Completed docs" {
		t.Fatalf("task:complete place occupancy refs = %#v, want work-complete", completedRefs)
	}
	failedRefs := view.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:failed"]
	if len(failedRefs) != 1 || failedRefs[0].WorkID != "work-failed" || failedRefs[0].DisplayName != "Blocked docs" {
		t.Fatalf("task:failed place occupancy refs = %#v, want work-failed", failedRefs)
	}
}

func TestBuildFactoryWorldView_ProjectsCompletedFailedTerminalAndProviderSessions(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     2500,
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis: 2500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-1",
				ToPlace:  "task:complete",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:complete"},
			}},
			TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:complete"},
				Status:   "TERMINAL",
			},
		}),
	}
	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	view := BuildFactoryWorldView(worldState)

	if view.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("InFlightDispatchCount = %d, want 0", view.Runtime.InFlightDispatchCount)
	}
	if view.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("CompletedCount = %d, want 1", view.Runtime.Session.CompletedCount)
	}
	if len(view.Runtime.Session.DispatchHistory) != 1 || view.Runtime.Session.DispatchHistory[0].DispatchID != "dispatch-1" {
		t.Fatalf("DispatchHistory = %#v, want dispatch-1", view.Runtime.Session.DispatchHistory)
	}
	dispatch := view.Runtime.Session.DispatchHistory[0]
	if len(dispatch.InputWorkItems) != 1 || dispatch.InputWorkItems[0].DisplayName != "Write docs" {
		t.Fatalf("dispatch input work items = %#v, want Write docs", dispatch.InputWorkItems)
	}
	if len(dispatch.OutputWorkItems) == 0 || dispatch.OutputWorkItems[0].DisplayName != "Write docs" {
		t.Fatalf("dispatch output work items = %#v, want Write docs", dispatch.OutputWorkItems)
	}
	if len(view.Runtime.Session.ProviderSessions) != 1 || view.Runtime.Session.ProviderSessions[0].ProviderSession.ID != "sess-1" {
		t.Fatalf("ProviderSessions = %#v, want sess-1", view.Runtime.Session.ProviderSessions)
	}
	if got := view.Runtime.PlaceTokenCounts["task:complete"]; got != 1 {
		t.Fatalf("task:complete count = %d, want 1", got)
	}
}

func TestBuildFactoryWorldView_ProjectsRejectedDispatchFeedbackAndOutputLabels(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Review draft", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-rejected",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Review draft", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-rejected",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "REJECTED", Feedback: "missing tests"},
			DurationMillis: 1500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-1",
				ToPlace:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Needs rewrite", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
	}
	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	view := BuildFactoryWorldView(worldState)
	history := view.Runtime.Session.DispatchHistory
	if len(history) != 1 {
		t.Fatalf("dispatch history = %#v, want one rejected dispatch", history)
	}
	if history[0].Result.Feedback != "missing tests" {
		t.Fatalf("dispatch feedback = %q, want missing tests", history[0].Result.Feedback)
	}
	if history[0].InputWorkItems[0].DisplayName != "Review draft" {
		t.Fatalf("input labels = %#v, want Review draft", history[0].InputWorkItems)
	}
	if history[0].OutputWorkItems[0].DisplayName != "Needs rewrite" {
		t.Fatalf("output labels = %#v, want Needs rewrite", history[0].OutputWorkItems)
	}
}

func TestBuildFactoryWorldView_CountsMultiTokenProviderDispatchOnce(t *testing.T) {
	state := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs"},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{
				DispatchID:     "dispatch-1",
				TransitionID:   "t-review",
				Workstation:    interfaces.FactoryWorkstationRef{Name: "Review"},
				Result:         interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
				WorkItemIDs:    []string{"work-1", "work-1"},
				DurationMillis: 1000,
			},
		},
		ProviderSessions: []interfaces.FactoryWorldProviderSessionRecord{
			{
				DispatchID:      "dispatch-1",
				TransitionID:    "t-review",
				WorkstationName: "Review",
				Outcome:         string(interfaces.OutcomeAccepted),
				ProviderSession: interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
				WorkItemIDs:     []string{"work-1", "work-1"},
			},
			{
				DispatchID:      "dispatch-1",
				TransitionID:    "t-review",
				WorkstationName: "Review",
				Outcome:         string(interfaces.OutcomeAccepted),
				ProviderSession: interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-retry"},
				WorkItemIDs:     []string{"work-1", "work-1"},
			},
		},
	}

	view := BuildFactoryWorldView(state)

	if view.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("DispatchedCount = %d, want 1", view.Runtime.Session.DispatchedCount)
	}
	if got := view.Runtime.Session.DispatchedByWorkType["task"]; got != 1 {
		t.Fatalf("DispatchedByWorkType[task] = %d, want 1", got)
	}
	if len(view.Runtime.Session.ProviderSessions) != 2 {
		t.Fatalf("ProviderSessions = %#v, want two retained attempts", view.Runtime.Session.ProviderSessions)
	}
}

func TestBuildFactoryWorldView_SelectedTickProjectionComesFromEventHistory(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceResponseEvent(3, t0.Add(2500*time.Millisecond), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeSucceeded,
			DurationMillis:     2500,
			ProviderSession:    generatedProviderSessionForProjectionTest(&interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"}),
		}),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED"},
			DurationMillis: 2500,
			Outputs: []interfaces.WorkstationOutput{{
				Type:     string(interfaces.MutationMove),
				TokenID:  "work-1",
				ToPlace:  "task:complete",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:complete"},
			}},
			TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
			ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "responses", ID: "sess-1"},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:complete"},
				Status:   "TERMINAL",
			},
		}),
	}

	activeState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState active tick: %v", err)
	}
	activeView := BuildFactoryWorldView(activeState)

	completedState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	completedView := BuildFactoryWorldView(completedState)

	if activeView.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("active tick InFlightDispatchCount = %d, want 1", activeView.Runtime.InFlightDispatchCount)
	}
	if activeView.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("active tick CompletedCount = %d, want 0", activeView.Runtime.Session.CompletedCount)
	}
	if len(activeView.Runtime.Session.ProviderSessions) != 0 {
		t.Fatalf("active tick ProviderSessions = %#v, want none before response", activeView.Runtime.Session.ProviderSessions)
	}
	if got := activeView.Runtime.PlaceTokenCounts["task:complete"]; got != 0 {
		t.Fatalf("active tick task:complete count = %d, want 0", got)
	}
	if completedView.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("completed tick InFlightDispatchCount = %d, want 0", completedView.Runtime.InFlightDispatchCount)
	}
	if completedView.Runtime.Session.CompletedCount != 1 {
		t.Fatalf("completed tick CompletedCount = %d, want 1", completedView.Runtime.Session.CompletedCount)
	}
	if len(completedView.Runtime.Session.ProviderSessions) != 1 || completedView.Runtime.Session.ProviderSessions[0].ProviderSession.ID != "sess-1" {
		t.Fatalf("completed tick ProviderSessions = %#v, want sess-1", completedView.Runtime.Session.ProviderSessions)
	}
	if got := completedView.Runtime.PlaceTokenCounts["task:complete"]; got != 1 {
		t.Fatalf("completed tick task:complete count = %d, want 1", got)
	}
}

func TestBuildFactoryWorldView_ProjectsResourceCountSmokeSnapshots(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 16, 0, 0, 0, time.UTC)
	events := resourceCountProjectionEvents(t0)
	cases := []struct {
		name              string
		tick              int
		wantResourceCount int
		wantInFlight      int
	}{
		{name: "idle", tick: 1, wantResourceCount: 2, wantInFlight: 0},
		{name: "active dispatch", tick: 3, wantResourceCount: 1, wantInFlight: 1},
		{name: "released resource", tick: 4, wantResourceCount: 2, wantInFlight: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worldState, err := ReconstructFactoryWorldState(events, tc.tick)
			if err != nil {
				t.Fatalf("ReconstructFactoryWorldState tick %d: %v", tc.tick, err)
			}

			view := BuildFactoryWorldView(worldState)

			if got := view.Runtime.PlaceTokenCounts["agent-slot:available"]; got != tc.wantResourceCount {
				t.Fatalf("tick %d agent-slot:available count = %d, want %d", tc.tick, got, tc.wantResourceCount)
			}
			if got := view.Runtime.InFlightDispatchCount; got != tc.wantInFlight {
				t.Fatalf("tick %d InFlightDispatchCount = %d, want %d", tc.tick, got, tc.wantInFlight)
			}
			if got := view.Topology.WorkstationNodesByID["implement"].InputPlaces; !hasResourcePlaceRef(got, "agent-slot:available") {
				t.Fatalf("implement input places = %#v, want agent-slot:available resource place", got)
			}
		})
	}
}

func TestBuildFactoryWorldView_ProjectsInferenceAttemptsForDashboard(t *testing.T) {
	t0 := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	exitCode := 1
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", TraceID: "trace-1", PlaceID: "task:init"},
			}},
		}),
		inferenceRequestEvent(3, t0.Add(3*time.Second), factoryapi.InferenceRequestEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			WorkingDirectory:   "/work/project",
			Worktree:           "/work/project/.worktrees/story",
			Prompt:             "Review the story.",
		}),
		inferenceResponseEvent(4, t0.Add(4*time.Second), factoryapi.InferenceResponseEventPayload{
			InferenceRequestId: "dispatch-1/inference-request/1",
			Attempt:            1,
			Outcome:            factoryapi.InferenceOutcomeFailed,
			DurationMillis:     875,
			ExitCode:           &exitCode,
			ErrorClass:         stringPtrForProjectionTest("rate_limited"),
		}),
	}

	pendingState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState pending tick: %v", err)
	}
	pendingView := BuildFactoryWorldView(pendingState)
	pendingAttempt := pendingView.Runtime.InferenceAttemptsByDispatchID["dispatch-1"]["dispatch-1/inference-request/1"]
	if pendingAttempt.RequestTime.IsZero() || pendingAttempt.Outcome != "" {
		t.Fatalf("pending inference attempt view = %#v, want request time without outcome", pendingAttempt)
	}

	completedState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState completed tick: %v", err)
	}
	completedView := BuildFactoryWorldView(completedState)
	completedAttempt := completedView.Runtime.InferenceAttemptsByDispatchID["dispatch-1"]["dispatch-1/inference-request/1"]
	if completedAttempt.Outcome != string(factoryapi.InferenceOutcomeFailed) ||
		completedAttempt.DurationMillis != 875 ||
		completedAttempt.ExitCode == nil ||
		*completedAttempt.ExitCode != 1 ||
		completedAttempt.ErrorClass != "rate_limited" ||
		completedAttempt.ResponseTime.IsZero() {
		t.Fatalf("completed inference attempt view = %#v, want failed response details", completedAttempt)
	}
}

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
	encodedView, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal dashboard view: %v", err)
	}
	if strings.Contains(string(encodedView), interfaces.SystemTimeWorkTypeID) {
		t.Fatalf("dashboard view leaked raw system time identifier: %s", string(encodedView))
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
					Id:      stringPtrForProjectionTest("daily-refresh"),
					Name:    "Daily refresh",
					Behavior: workstationKindPtrForWorldViewTest(factoryapi.WorkstationKindCron),
					Worker:  "refresh-worker",
					Inputs:  []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}, {WorkType: interfaces.SystemTimeWorkTypeID, State: interfaces.SystemTimePendingState}},
					Outputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "done"}},
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
		Outputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
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

func TestWorkItemRefsForProjectionOwners_FilterCustomerWorkAndPreserveLineage(t *testing.T) {
	itemsByID := map[string]interfaces.FactoryWorkItem{
		"work-2": {ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}, TraceID: "trace-2"},
		"work-1": {ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}, TraceID: "trace-1"},
		"time-1": {ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"},
	}

	refsByID := workItemRefsForIDs([]string{"work-2", "time-1", "work-1", "work-2"}, itemsByID)
	if len(refsByID) != 2 || refsByID[0].WorkID != "work-1" || refsByID[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForIDs = %#v, want sorted customer refs", refsByID)
	}
	if refsByID[0].CurrentChainingTraceID != "chain-1" || len(refsByID[1].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("workItemRefsForIDs lineage = %#v, want explicit chaining fields", refsByID)
	}

	refsForItems := workItemRefsForItems([]interfaces.FactoryWorkItem{
		itemsByID["work-2"],
		itemsByID["time-1"],
		itemsByID["work-2"],
		itemsByID["work-1"],
	})
	if len(refsForItems) != 2 || refsForItems[0].WorkID != "work-2" || refsForItems[1].WorkID != "work-1" {
		t.Fatalf("workItemRefsForItems = %#v, want first-occurrence customer refs", refsForItems)
	}

	refsForInputs := workItemRefsForInputs([]interfaces.WorkstationInput{
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}}},
	})
	if len(refsForInputs) != 2 || refsForInputs[0].WorkID != "work-1" || refsForInputs[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForInputs = %#v, want first-occurrence customer refs", refsForInputs)
	}
}
