package projections_test

import (
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildFactoryWorldView_ProjectsFromReconstructedWorldState(t *testing.T) {
	t0 := time.Date(2026, 4, 16, 8, 0, 0, 0, time.UTC)
	events := []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:                 "work-1",
			WorkTypeID:         "task",
			DisplayName:        "Write docs",
			ChainingTraceDepth: 3,
			TraceID:            "trace-1",
			PlaceID:            "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", ChainingTraceDepth: 3, TraceID: "trace-1", PlaceID: "task:init"},
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

func TestBuildFactoryWorldView_ExposesCanonicalFactoryGraphFromStructureEvents(t *testing.T) {
	t0 := time.Date(2026, 5, 27, 1, 0, 0, 0, time.UTC)
	workerType := factoryapi.WorkerTypeModelWorker
	workerProvider := factoryapi.WorkerModelProviderCodex
	workstationKind := factoryapi.WorkstationKindStandard
	maxRetries := 3
	promptBody := "Review the work and either continue, reject, or fail it."
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			Name:      "graph-source",
			Resources: &[]factoryapi.Resource{{Name: "agent-slot", Capacity: 2}},
			Workers: &[]factoryapi.Worker{{
				Name:          "reviewer",
				Type:          &workerType,
				ModelProvider: &workerProvider,
				Model:         stringPtrForProjectionTest("gpt-5.4"),
				Resources:     &[]factoryapi.ResourceRequirement{{Name: "agent-slot", Capacity: 1}},
			}},
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "story",
				States: []factoryapi.WorkState{
					{Name: "new", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "review", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "continue", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "rejected", Type: factoryapi.WorkStateTypePROCESSING},
					{Name: "done", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				},
			}, {
				Name: interfaces.SystemTimeWorkTypeID,
				States: []factoryapi.WorkState{
					{Name: "pending", Type: factoryapi.WorkStateTypePROCESSING},
				},
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:         stringPtrForProjectionTest("review"),
				Name:       "Review",
				Worker:     "reviewer",
				Behavior:   &workstationKind,
				Body:       &promptBody,
				Inputs:     []factoryapi.WorkstationIO{{WorkType: "story", State: "new"}},
				Outputs:    &[]factoryapi.WorkstationIO{{WorkType: "story", State: "done"}},
				OnContinue: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "continue"}},
				OnRejection: &[]factoryapi.WorkstationIO{{
					WorkType: "story",
					State:    "rejected",
				}},
				OnFailure: &[]factoryapi.WorkstationIO{{WorkType: "story", State: "failed"}},
				Resources: &[]factoryapi.ResourceRequirement{{Name: "agent-slot", Capacity: 1}},
				Limits:    &factoryapi.WorkstationLimits{MaxRetries: &maxRetries},
			}, {
				Id:      stringPtrForProjectionTest(interfaces.SystemTimeExpiryTransitionID),
				Name:    interfaces.SystemTimeExpiryTransitionID,
				Inputs:  []factoryapi.WorkstationIO{{WorkType: interfaces.SystemTimeWorkTypeID, State: "pending"}},
				Outputs: &[]factoryapi.WorkstationIO{},
				Worker:  "",
			}},
		},
	}
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "initial-canonical-factory", 0, t0, factoryapi.FactoryEventContext{}, payload),
	}
	worldState, err := ReconstructFactoryWorldState(events, 0)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	view := BuildFactoryWorldView(worldState)

	assertCanonicalFactoryGraphPreserved(t, worldState, view, payload.Factory)
	assertCanonicalFactoryWorkstationDetailsPreserved(t, *view.Factory, promptBody, maxRetries)
}

func assertCanonicalFactoryGraphPreserved(
	t *testing.T,
	worldState interfaces.FactoryWorldState,
	view interfaces.FactoryWorldView,
	want factoryapi.Factory,
) {
	t.Helper()

	if worldState.Factory == nil {
		t.Fatal("world state factory = nil, want canonical factory graph")
	}
	if view.Factory == nil {
		t.Fatal("world view factory = nil, want canonical factory graph")
	}
	if !reflect.DeepEqual(*worldState.Factory, want) {
		t.Fatalf("world state factory = %#v, want canonical payload", *worldState.Factory)
	}
	if !reflect.DeepEqual(*view.Factory, want) {
		t.Fatalf("world view factory = %#v, want canonical payload", *view.Factory)
	}
}

func assertCanonicalFactoryWorkstationDetailsPreserved(
	t *testing.T,
	factory factoryapi.Factory,
	promptBody string,
	maxRetries int,
) {
	t.Helper()

	workstation := (*factory.Workstations)[0]
	assertCanonicalFactoryRoutePreserved(t, workstation.OnContinue, "continue", "onContinue")
	assertCanonicalFactoryRoutePreserved(t, workstation.OnRejection, "rejected", "onRejection")
	assertCanonicalFactoryRoutePreserved(t, workstation.OnFailure, "failed", "onFailure")
	if workstation.Body == nil || *workstation.Body != promptBody {
		t.Fatalf("body = %#v, want prompt body preserved", workstation.Body)
	}
	if workstation.Limits == nil || workstation.Limits.MaxRetries == nil || *workstation.Limits.MaxRetries != maxRetries {
		t.Fatalf("limits = %#v, want max retries preserved", workstation.Limits)
	}
}

func assertCanonicalFactoryRoutePreserved(
	t *testing.T,
	routes *[]factoryapi.WorkstationIO,
	wantState string,
	label string,
) {
	t.Helper()

	if routes == nil || len(*routes) != 1 || (*routes)[0].State != wantState {
		t.Fatalf("%s = %#v, want %s route", label, routes, wantState)
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
			ID:                 "work-1",
			WorkTypeID:         "task",
			DisplayName:        "Write docs",
			ChainingTraceDepth: 3,
			TraceID:            "trace-1",
			PlaceID:            "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID:  "work-1",
				PlaceID:  "task:init",
				WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "Write docs", ChainingTraceDepth: 3, TraceID: "trace-1", PlaceID: "task:init"},
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
	if queuedItems[0].ChainingTraceDepth != 3 {
		t.Fatalf("queued chaining trace depth = %d, want 3", queuedItems[0].ChainingTraceDepth)
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
	activeInput := inFlightView.Runtime.ActiveExecutionsByDispatchID["dispatch-1"].ConsumedInputs[0]
	if activeInput.WorkItem == nil || activeInput.WorkItem.ChainingTraceDepth != 3 {
		t.Fatalf("active consumed input depth = %#v, want depth 3", activeInput.WorkItem)
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

func TestBuildFactoryWorldView_CountsFailedWorkItemsForCustomerSummary(t *testing.T) {
	failedDispatch := interfaces.FactoryWorldDispatchCompletion{
		DispatchID:   "dispatch-1",
		TransitionID: "review",
		WorkItemIDs:  []string{"work-1", "work-2", "work-3"},
		Workstation:  interfaces.FactoryWorkstationRef{Name: "Review"},
		Result:       interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeFailed)},
	}
	state := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{failedDispatch},
		FailedWorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{failedDispatch},
	}

	view := BuildFactoryWorldView(state)

	if view.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("DispatchedCount = %d, want 1 failed dispatch", view.Runtime.Session.DispatchedCount)
	}
	if view.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("CompletedCount = %d, want 0 accepted completions", view.Runtime.Session.CompletedCount)
	}
	if view.Runtime.Session.FailedCount != 3 {
		t.Fatalf("FailedCount = %d, want 3 failed work items", view.Runtime.Session.FailedCount)
	}
	if got := view.Runtime.Session.FailedByWorkType["story"]; got != 3 {
		t.Fatalf("FailedByWorkType[story] = %d, want 3", got)
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
