package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRestoreRestoredActiveDispatchResolvesUniqueCanonicalPlace(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	marking := petri.NewMarking("test-net")
	cfg := &runtimeConfig{
		net:                buildSimpleNet(),
		clock:              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState: restored,
	}

	seeded, err := restoreRestoredWorkMarking(cfg, marking, time.Unix(0, 0).UTC(), nil, nil)
	if err != nil {
		t.Fatalf("restoreRestoredWorkMarking: %v", err)
	}
	if _, ok := seeded["work-missing-place"]; !ok {
		t.Fatalf("seeded Work IDs = %#v, want work-missing-place", seeded)
	}
	input := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0]
	if input.PlaceID != "task:init" {
		t.Fatalf("resolved dispatch input = %#v, want task:init", input)
	}
	tokens := marking.TokensInPlace("task:init")
	if len(tokens) != 1 || tokens[0].Color.WorkID != "work-missing-place" {
		t.Fatalf("restored task:init tokens = %#v, want the original Work identity", tokens)
	}
}

func TestRestoreRestoredMissingInitialWorkUsesCompleteGuardMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		mutate      func(*interfaces.FactoryWorldState)
		wantRecover bool
	}{
		{name: "U01 exact admitted initial Work", wantRecover: true},
		{
			name: "U02 duplicate admission",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.WorkRequestsByID["request-duplicate"] = restored.WorkRequestsByID["request-1"]
			},
		},
		{
			name: "U03 admission and indexes disagree on Work type",
			mutate: func(restored *interfaces.FactoryWorldState) {
				item := restored.WorkRequestsByID["request-1"].WorkItems[0]
				item.WorkTypeID = "other"
				restored.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
					RequestID: "request-1", WorkItems: []work.FactoryWorkItem{item},
				}
			},
		},
		{
			name: "U04 unknown Work type",
			mutate: func(restored *interfaces.FactoryWorldState) {
				item := restored.WorkItemsByID["work-recoverable"]
				item.WorkTypeID = "unknown"
				restored.WorkItemsByID[item.ID] = item
				restored.ActiveWorkItemsByID[item.ID] = item
				restored.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
					RequestID: "request-1", WorkItems: []work.FactoryWorkItem{item},
				}
			},
		},
		{
			name: "U05 state is not the sole initial state",
			mutate: func(restored *interfaces.FactoryWorldState) {
				item := restored.WorkItemsByID["work-recoverable"]
				item.State = "done"
				restored.WorkItemsByID[item.ID] = item
				restored.ActiveWorkItemsByID[item.ID] = item
			},
		},
		{
			name: "U06 multiple initial definitions",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.Topology.WorkTypes[0].States = append(
					restored.Topology.WorkTypes[0].States,
					interfaces.FactoryStateDefinition{Value: "ready", Category: "INITIAL"},
				)
			},
		},
		{
			name: "U07 later Work move",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.WorkStateChangesByWorkID = map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
					"work-recoverable": {{WorkID: "work-recoverable", ToPlaceID: "task:done", Sequence: 1}},
				}
			},
		},
		{
			name: "U08 active dispatch",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.ActiveDispatches = map[string]interfaces.FactoryWorldDispatch{
					"dispatch-active": {DispatchID: "dispatch-active", WorkItemIDs: []string{"work-recoverable"}},
				}
			},
		},
		{
			name: "U09 completed dispatch",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.CompletedDispatches = []interfaces.FactoryWorldDispatchCompletion{{
					DispatchID: "dispatch-completed", WorkItemIDs: []string{"work-recoverable"},
				}}
			},
		},
		{
			name: "U10 terminal failure approval conflict and malformed identity",
			mutate: func(restored *interfaces.FactoryWorldState) {
				restored.TerminalWorkByID = map[string]interfaces.FactoryTerminalWork{
					"work-recoverable": {WorkItem: restored.WorkItemsByID["work-recoverable"]},
				}
				restored.FailedWorkItemsByID = map[string]work.FactoryWorkItem{
					"work-recoverable": restored.WorkItemsByID["work-recoverable"],
				}
				restored.PendingHumanApprovalsByID = map[string]interfaces.FactoryWorldHumanApproval{
					"approval-1": {ApprovalID: "approval-1", WorkItemIDs: []string{"work-recoverable"}},
				}
				restored.PlaceOccupancyByID = map[string]interfaces.FactoryPlaceOccupancy{
					"task:init": {PlaceID: "task:init", WorkItemIDs: []string{"work-recoverable"}},
					"task:done": {PlaceID: "task:done", WorkItemIDs: []string{"work-recoverable"}},
				}
				tampered := restored.WorkItemsByID["work-recoverable"]
				tampered.ID = "different-work"
				restored.WorkItemsByID["work-recoverable"] = tampered
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			restored := restoredMissingInitialWorkFixture()
			if test.mutate != nil {
				test.mutate(restored)
			}
			marking := petri.NewMarking("test-net")
			cfg := &runtimeConfig{
				net:                buildSimpleNet(),
				clock:              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
				restoredWorldState: restored,
			}
			seeded, err := restoreRestoredWorkMarking(cfg, marking, time.Unix(0, 0).UTC(), nil, nil)
			if test.wantRecover {
				assertRecoveredMissingInitialWork(t, seeded, marking)
				return
			}
			if err == nil {
				t.Fatal("restoreRestoredWorkMarking succeeded for an ambiguous or conflicting recording")
			}
			if tokens := marking.TokensInPlace("task:init"); len(tokens) != 0 {
				t.Fatalf("fail-closed restore seeded tokens = %#v", tokens)
			}
		})
	}
}

func restoredMissingInitialWorkFixture() *interfaces.FactoryWorldState {
	item := work.FactoryWorkItem{
		ID: "work-recoverable", WorkTypeID: "task", State: "init", DisplayName: "Recover me",
		ChainingTraceDepth: 2, CurrentChainingTraceID: "trace-current", PreviousChainingTraceIDs: []string{"trace-parent"},
		TraceID: "trace-work", ParentID: "parent-work", Tags: map[string]string{"lane": "recovery"},
	}
	return &interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: "task", States: []interfaces.FactoryStateDefinition{
					{Value: "init", Category: "INITIAL"}, {Value: "done", Category: "TERMINAL"},
				},
			}},
			Places: []interfaces.FactoryPlace{{ID: "task:init", TypeID: "task", State: "init", Category: "INITIAL"}},
		},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-1": {RequestID: "request-1", WorkItems: []work.FactoryWorkItem{item}},
		},
		WorkItemsByID:       map[string]work.FactoryWorkItem{item.ID: item},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{item.ID: item},
		RelationsByWorkID: map[string][]work.FactoryRelation{item.ID: {{
			Type: "DEPENDS_ON", TargetWorkID: "parent-work", RequiredState: "done",
		}}},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
	}
}

func assertRecoveredMissingInitialWork(
	t *testing.T,
	seeded map[string]struct{},
	marking *petri.Marking,
) {
	t.Helper()
	if len(seeded) != 1 {
		t.Fatalf("seeded Work IDs = %#v, want one recovered Work", seeded)
	}
	tokens := marking.TokensInPlace("task:init")
	if len(tokens) != 1 || tokens[0].Color.WorkID != "work-recoverable" {
		t.Fatalf("recovered task:init tokens = %#v, want one exact Work", tokens)
	}
	token := tokens[0]
	if token.Color.RequestID != "request-1" || token.Color.Name != "Recover me" ||
		token.Color.TraceID != "trace-work" || token.Color.ParentID != "parent-work" ||
		token.Color.CurrentChainingTraceID != "trace-current" || token.Color.ChainingTraceDepth != 2 ||
		!reflect.DeepEqual(token.Color.PreviousChainingTraceIDs, []string{"trace-parent"}) ||
		!reflect.DeepEqual(token.Color.Tags, map[string]string{"lane": "recovery"}) ||
		len(token.Color.Relations) != 1 || token.Color.Relations[0].TargetWorkID != "parent-work" {
		t.Fatalf("recovered Work metadata = %#v, want recorded request/lineage/relation metadata", token.Color)
	}
}

func TestNew_WithRestoredActiveDispatchMissingPlaceResolvesCanonicalPlace(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	tokens := snapshot.Marking.PlaceTokens["task:init"]
	if len(tokens) != 1 {
		t.Fatalf("restored active Work token IDs = %#v, want one token at task:init", tokens)
	}
	token := snapshot.Marking.Tokens[tokens[0]]
	if token == nil || token.Color.WorkID != "work-missing-place" {
		t.Fatalf("restored active Work token = %#v, want work-missing-place", token)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "task:init" {
		t.Fatalf("resolved dispatch input PlaceID = %q, want task:init", got)
	}
}

func TestRestoreRestoredActiveDispatchUsesLatestCanonicalMutation(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.Topology.Workstations = nil
	restored.WorkStateChangesByWorkID = map[string][]interfaces.FactoryWorldWorkStateChangeRecord{
		"work-missing-place": {
			{WorkID: "work-missing-place", ToPlaceID: "task:init-a", Sequence: 1},
			{WorkID: "work-missing-place", ToPlaceID: "task:init-b", Sequence: 2},
		},
	}
	net := buildSimpleNet()
	net.Places["task:init-a"] = &petri.Place{ID: "task:init-a", TypeID: "task", State: "init"}
	net.Places["task:init-b"] = &petri.Place{ID: "task:init-b", TypeID: "task", State: "init"}
	net.Transitions["t-process"].InputArcs = []petri.Arc{
		{ID: "a-b", PlaceID: "task:init-b", Direction: petri.ArcInput},
		{ID: "a-a", PlaceID: "task:init-a", Direction: petri.ArcInput},
	}

	if err := materializeRestoredDispatchInputPlaces(restored, net, restoredWorkItems(restored)); err != nil {
		t.Fatalf("materializeRestoredDispatchInputPlaces: %v", err)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "task:init-b" {
		t.Fatalf("resolved dispatch input PlaceID = %q, want latest task:init-b mutation", got)
	}
}

func TestRestoreRestoredActiveDispatchIgnoresEmptyRecordedWorkstationPlaces(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.Topology.Workstations[0].InputPlaceIDs = []string{""}

	if err := materializeRestoredDispatchInputPlaces(restored, buildSimpleNet(), restoredWorkItems(restored)); err != nil {
		t.Fatalf("materializeRestoredDispatchInputPlaces: %v", err)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "task:init" {
		t.Fatalf("resolved dispatch input PlaceID = %q, want loaded transition place task:init", got)
	}
}

func TestRestoreRestoredActiveDispatchUsesLoadedArcWhenCurrentWorkStateAdvanced(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.Topology.Workstations[0].InputPlaceIDs = nil
	restored.WorkItemsByID["work-missing-place"] = work.FactoryWorkItem{
		ID: "work-missing-place", WorkTypeID: "task", State: "complete",
	}
	net := buildSimpleNet()
	net.Places["task:complete"] = &petri.Place{ID: "task:complete", TypeID: "task", State: "complete"}

	if err := materializeRestoredDispatchInputPlaces(restored, net, restoredWorkItems(restored)); err != nil {
		t.Fatalf("materializeRestoredDispatchInputPlaces: %v", err)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "task:init" {
		t.Fatalf("resolved dispatch input PlaceID = %q, want loaded transition place task:init", got)
	}
}

func TestRestoreRestoredWorkMarkingSkipsActiveDispatchClaimsForReplay(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.WorkItemsByID["work-missing-place"] = work.FactoryWorkItem{
		ID: "work-missing-place", WorkTypeID: "task", State: "done",
	}
	restored.PlaceOccupancyByID = map[string]interfaces.FactoryPlaceOccupancy{
		"task:done": {PlaceID: "task:done", WorkItemIDs: []string{"work-missing-place"}},
	}
	net := buildSimpleNet()
	marking := petri.NewMarking("test-net")
	cfg := &runtimeConfig{
		net:                                net,
		clock:                              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState:                 restored,
		skipRestoredDispatchReconciliation: true,
	}

	seeded, err := restoreRestoredWorkMarking(
		cfg, marking, time.Unix(0, 0).UTC(), nil,
		map[string]struct{}{"work-missing-place": {}},
	)
	if err != nil {
		t.Fatalf("restoreRestoredWorkMarking: %v", err)
	}
	if len(seeded) != 0 || len(marking.Tokens) != 0 {
		t.Fatalf("replay seed = %#v tokens=%#v, want no active dispatch board claim", seeded, marking.Tokens)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "" {
		t.Fatalf("replay source dispatch input PlaceID = %q, want source unchanged", got)
	}
}

func TestRestoreRestoredActiveDispatchRejectsMissingPlaceWithoutMutation(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.Topology.Workstations = nil
	net := buildSimpleNet()
	net.Transitions["t-process"].InputArcs = nil
	marking := petri.NewMarking("test-net")
	beforeMarking := marking.Snapshot()
	cfg := &runtimeConfig{
		net:                net,
		clock:              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState: restored,
	}

	_, err := restoreRestoredWorkMarking(cfg, marking, time.Unix(0, 0).UTC(), nil, nil)
	if err == nil {
		t.Fatal("materializeRestoredDispatchInputPlaces returned nil, want missing-place error")
	}
	var placeErr *restoredDispatchPlaceError
	if !errors.As(err, &placeErr) {
		t.Fatalf("materialization error = %T %v, want restoredDispatchPlaceError", err, err)
	}
	if placeErr.Reason != restoredDispatchPlaceMissing || placeErr.DispatchID != "dispatch-missing-place" ||
		placeErr.WorkID != "work-missing-place" || placeErr.TransitionID != "t-process" || len(placeErr.Candidates) != 0 {
		t.Fatalf("typed missing-place error = %#v, want stable empty-candidate diagnostics", placeErr)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "" {
		t.Fatalf("missing-place dispatch input = %q, want unchanged empty place", got)
	}
	if !reflect.DeepEqual(marking.Snapshot(), beforeMarking) {
		t.Fatalf("marking changed after missing-place rejection: got %#v, want %#v", marking.Snapshot(), beforeMarking)
	}
}

func TestRestoreRestoredActiveDispatchRejectsAmbiguousPlacesWithSortedCandidates(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.Topology.Workstations[0].InputPlaceIDs = []string{"task:init-b", "task:init-a"}
	net := buildSimpleNet()
	net.Places["task:init-a"] = &petri.Place{ID: "task:init-a", TypeID: "task", State: "init"}
	net.Places["task:init-b"] = &petri.Place{ID: "task:init-b", TypeID: "task", State: "init"}
	net.Transitions["t-process"].InputArcs = []petri.Arc{
		{ID: "a-b", Name: "input-b", PlaceID: "task:init-b", Direction: petri.ArcInput},
		{ID: "a-a", Name: "input-a", PlaceID: "task:init-a", Direction: petri.ArcInput},
	}
	marking := petri.NewMarking("test-net")
	beforeMarking := marking.Snapshot()
	cfg := &runtimeConfig{
		net:                net,
		clock:              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState: restored,
	}

	_, err := restoreRestoredWorkMarking(cfg, marking, time.Unix(0, 0).UTC(), nil, nil)
	if err == nil {
		t.Fatal("materializeRestoredDispatchInputPlaces returned nil, want ambiguous-place error")
	}
	var placeErr *restoredDispatchPlaceError
	if !errors.As(err, &placeErr) {
		t.Fatalf("materialization error = %T %v, want restoredDispatchPlaceError", err, err)
	}
	if placeErr.Reason != restoredDispatchPlaceAmbiguous {
		t.Fatalf("typed ambiguous-place reason = %q, want ambiguous", placeErr.Reason)
	}
	if !reflect.DeepEqual(placeErr.Candidates, []string{"task:init-a", "task:init-b"}) {
		t.Fatalf("ambiguous candidates = %#v, want sorted exact IDs", placeErr.Candidates)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "" {
		t.Fatalf("ambiguous dispatch input = %q, want unchanged empty place", got)
	}
	if !reflect.DeepEqual(marking.Snapshot(), beforeMarking) {
		t.Fatalf("marking changed after ambiguous-place rejection: got %#v, want %#v", marking.Snapshot(), beforeMarking)
	}
}

func TestRestoreRestoredActiveDispatchResolvesAtomicallyAcrossBatch(t *testing.T) {
	restored := restoredMissingPlaceDispatchFixture()
	restored.ActiveDispatches["dispatch-ambiguous"] = interfaces.FactoryWorldDispatch{
		DispatchID:   "dispatch-ambiguous",
		TransitionID: "t-ambiguous",
		WorkItemIDs:  []string{"work-ambiguous"},
		Inputs: []interfaces.WorkstationInput{{
			TokenID:  "work-ambiguous",
			WorkItem: &work.FactoryWorkItem{ID: "work-ambiguous"},
		}},
	}
	restored.WorkItemsByID["work-ambiguous"] = work.FactoryWorkItem{ID: "work-ambiguous", WorkTypeID: "task", State: "init"}
	restored.Topology.Workstations = append(restored.Topology.Workstations, interfaces.FactoryWorkstation{
		ID: "t-ambiguous", InputPlaceIDs: []string{"task:init-a", "task:init-b"},
	})
	net := buildSimpleNet()
	net.Places["task:init-a"] = &petri.Place{ID: "task:init-a", TypeID: "task", State: "init"}
	net.Places["task:init-b"] = &petri.Place{ID: "task:init-b", TypeID: "task", State: "init"}
	net.Transitions["t-ambiguous"] = &petri.Transition{
		ID: "t-ambiguous",
		InputArcs: []petri.Arc{
			{ID: "a-b", PlaceID: "task:init-b", Direction: petri.ArcInput},
			{ID: "a-a", PlaceID: "task:init-a", Direction: petri.ArcInput},
		},
	}
	marking := petri.NewMarking("test-net")
	beforeMarking := marking.Snapshot()
	cfg := &runtimeConfig{
		net:                net,
		clock:              platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second),
		restoredWorldState: restored,
	}

	_, err := restoreRestoredWorkMarking(cfg, marking, time.Unix(0, 0).UTC(), nil, nil)
	if err == nil {
		t.Fatal("materializeRestoredDispatchInputPlaces returned nil, want later ambiguous-place error")
	}
	var placeErr *restoredDispatchPlaceError
	if !errors.As(err, &placeErr) || placeErr.DispatchID != "dispatch-ambiguous" {
		t.Fatalf("batch materialization error = %T %v, want ambiguous dispatch diagnostic", err, err)
	}
	if got := restored.ActiveDispatches["dispatch-missing-place"].Inputs[0].PlaceID; got != "" {
		t.Fatalf("earlier unique dispatch input = %q, want no partial resolution", got)
	}
	if got := restored.ActiveDispatches["dispatch-ambiguous"].Inputs[0].PlaceID; got != "" {
		t.Fatalf("ambiguous dispatch input = %q, want unchanged empty place", got)
	}
	if !reflect.DeepEqual(marking.Snapshot(), beforeMarking) {
		t.Fatalf("marking changed after batch rejection: got %#v, want %#v", marking.Snapshot(), beforeMarking)
	}
}

func restoredMissingPlaceDispatchFixture() *interfaces.FactoryWorldState {
	return &interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			Workstations: []interfaces.FactoryWorkstation{{
				ID: "t-process", InputPlaceIDs: []string{"task:init"},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-missing-place": {ID: "work-missing-place", WorkTypeID: "task", State: "init"},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-missing-place": {
				DispatchID:   "dispatch-missing-place",
				TransitionID: "t-process",
				WorkItemIDs:  []string{"work-missing-place"},
				Inputs: []interfaces.WorkstationInput{{
					TokenID:  "work-missing-place",
					WorkItem: &work.FactoryWorkItem{ID: "work-missing-place"},
				}},
			},
		},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
	}
}

func TestNewRestoresCompletedDispatchHistoryForWorkReads(t *testing.T) {
	t.Parallel()

	workID := "work-restored-failure"
	startedAt := time.Date(2026, time.August, 11, 9, 30, 0, 0, time.UTC)
	completedAt := startedAt.Add(750 * time.Millisecond)
	item := work.FactoryWorkItem{
		ID: workID, WorkTypeID: "task", State: "failed", DisplayName: "restored failure",
		TraceID: "trace-restored-failure", PreviousChainingTraceIDs: []string{"trace-parent"},
	}
	failure := &workerexecution.FailureDetail{
		Reason:  workerexecution.WorkFailureTypeAuthFailure,
		Message: "Provider authentication failed.",
	}
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{workID: item},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID:     "dispatch-restored-failure",
			TransitionID:   "process",
			Workstation:    interfaces.FactoryWorkstationRef{Name: "processor"},
			StartedAt:      startedAt,
			CompletedAt:    completedAt,
			DurationMillis: 750,
			WorkItemIDs:    []string{workID},
			ConsumedInputs: []interfaces.WorkstationInput{{
				TokenID:  "token-restored-failure",
				WorkItem: &item,
			}},
			InputWorkItems: []work.FactoryWorkItem{item},
			Result: interfaces.WorkstationResult{
				Outcome:       string(workerexecution.OutcomeFailed),
				Error:         failure.Message,
				FailureDetail: failure,
			},
		}},
	}
	f, err := newTestFactory(withNet(buildSimpleNetWithFailureArc()), withRestoredWorldState(restored))
	if err != nil {
		t.Fatalf("New with restored dispatch history: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("restored dispatch history = %d entries, want one", len(snapshot.DispatchHistory))
	}
	completed := snapshot.DispatchHistory[0]
	assertRestoredFailureDispatch(t, completed, workID, startedAt, completedAt, failure)
	completed.FailureDetail.Message = "mutated detached snapshot"
	second, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after mutation: %v", err)
	}
	assertRestoredDispatchFailureIsDetached(t, second.DispatchHistory[0], failure.Message)
}

func assertRestoredFailureDispatch(
	t *testing.T,
	completed interfaces.CompletedDispatch,
	workID string,
	startedAt, completedAt time.Time,
	failure *workerexecution.FailureDetail,
) {
	t.Helper()
	if completed.DispatchID != "dispatch-restored-failure" || completed.Outcome != workerexecution.OutcomeFailed ||
		!completed.StartTime.Equal(startedAt) || !completed.EndTime.Equal(completedAt) || completed.Duration != 750*time.Millisecond ||
		len(completed.ConsumedTokens) != 1 || completed.ConsumedTokens[0].Color.WorkID != workID ||
		completed.FailureDetail == nil || *completed.FailureDetail != *failure {
		t.Fatalf("restored completed dispatch = %#v, want Work-associated failure and exact timing", completed)
	}
}

func assertRestoredDispatchFailureIsDetached(t *testing.T, completed interfaces.CompletedDispatch, wantMessage string) {
	t.Helper()
	if completed.FailureDetail == nil || completed.FailureDetail.Message != wantMessage {
		t.Fatalf("mutated restored dispatch history leaked into runtime: %#v", completed.FailureDetail)
	}
}

func TestWorkerRecordingSessionStartedAt(t *testing.T) {
	t.Parallel()

	want := time.Date(2026, time.August, 11, 16, 30, 0, 0, time.UTC)
	tests := []struct {
		name    string
		session recordings.WorkerSessionRecordingSnapshot
		want    *time.Time
		wantErr bool
	}{
		{name: "empty history", session: recordings.WorkerSessionRecordingSnapshot{WorkerSessionID: "worker-1"}},
		{
			name:    "malformed draft",
			session: workerRecordingSessionSnapshot("worker-1", `{`),
			wantErr: true,
		},
		{
			name:    "wrong kind",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"RUN","phase":"STARTED","payload":{}}`),
			wantErr: true,
		},
		{
			name:    "wrong phase",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"SESSION","phase":"UPDATED","payload":{}}`),
			wantErr: true,
		},
		{
			name:    "malformed session payload",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"SESSION","phase":"STARTED","payload":"invalid"}`),
			wantErr: true,
		},
		{
			name:    "opening identity mismatch",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"other"}}`),
			wantErr: true,
		},
		{
			name:    "legacy timestamp absent",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker-1"}}`),
		},
		{
			name:    "source timestamp normalized to UTC",
			session: workerRecordingSessionSnapshot("worker-1", `{"kind":"SESSION","phase":"STARTED","payload":{"workerSessionId":"worker-1","startedAt":"2026-08-11T09:30:00-07:00"}}`),
			want:    &want,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := workerRecordingSessionStartedAt(test.session)
			if (err != nil) != test.wantErr {
				t.Fatalf("workerRecordingSessionStartedAt() error = %v, want error %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if test.want == nil {
				if got != nil {
					t.Fatalf("workerRecordingSessionStartedAt() = %v, want no timestamp", got)
				}
				return
			}
			if got == nil || !got.Equal(*test.want) || got.Location() != time.UTC {
				t.Fatalf("workerRecordingSessionStartedAt() = %v, want %v in UTC", got, test.want)
			}
		})
	}
}

func workerRecordingSessionSnapshot(workerSessionID, payload string) recordings.WorkerSessionRecordingSnapshot {
	return recordings.WorkerSessionRecordingSnapshot{
		WorkerSessionID: workerSessionID,
		Records:         []events.Record{{Payload: []byte(payload)}},
	}
}

func TestRestoredCompletionInputTokensPreservesRecordedSources(t *testing.T) {
	t.Parallel()

	workOne := work.FactoryWorkItem{ID: "work-one", WorkTypeID: "task", State: "review"}
	consumed := restoredCompletionInputTokens(interfaces.FactoryWorldDispatchCompletion{
		ConsumedInputs: []interfaces.WorkstationInput{
			{TokenID: "missing-work"},
			{WorkItem: &work.FactoryWorkItem{}},
			{WorkItem: &workOne},
			{TokenID: "duplicate", WorkItem: &workOne},
		},
	})
	if len(consumed) != 1 || consumed[0].ID != workOne.ID || consumed[0].Color.WorkID != workOne.ID {
		t.Fatalf("restored consumed tokens = %#v, want one deduplicated Work token", consumed)
	}

	workTwo := work.FactoryWorkItem{ID: "work-two", WorkTypeID: "task", State: "review"}
	inputFallback := restoredCompletionInputTokens(interfaces.FactoryWorldDispatchCompletion{
		ConsumedInputs: []interfaces.WorkstationInput{{}, {WorkItem: &work.FactoryWorkItem{}}},
		InputWorkItems: []work.FactoryWorkItem{{}, workTwo, workTwo},
	})
	if len(inputFallback) != 1 || inputFallback[0].ID != workTwo.ID {
		t.Fatalf("restored input-work fallback = %#v, want one deduplicated Work token", inputFallback)
	}

	workIDFallback := restoredCompletionInputTokens(interfaces.FactoryWorldDispatchCompletion{
		ConsumedInputs: []interfaces.WorkstationInput{{}},
		InputWorkItems: []work.FactoryWorkItem{{}},
		WorkItemIDs:    []string{"", "work-three", "work-three"},
	})
	if len(workIDFallback) != 1 || workIDFallback[0].ID != "work-three" {
		t.Fatalf("restored Work ID fallback = %#v, want one Work token", workIDFallback)
	}

	if tokens := restoredCompletionInputTokens(interfaces.FactoryWorldDispatchCompletion{}); len(tokens) != 0 {
		t.Fatalf("restored empty inputs = %#v, want no tokens", tokens)
	}
}

func TestRestoredCompletionOutputMutationsSkipEmptyWork(t *testing.T) {
	t.Parallel()

	mutations := restoredCompletionOutputMutations(interfaces.FactoryWorldDispatchCompletion{
		DispatchID:   "dispatch-output",
		TransitionID: "process",
		Result:       interfaces.WorkstationResult{Outcome: "COMPLETED"},
		OutputWorkItems: []work.FactoryWorkItem{
			{},
			{ID: "work-output", WorkTypeID: "task", State: "review"},
		},
	})
	if len(mutations) != 1 || mutations[0].TokenID != "work-output" || mutations[0].Token == nil || mutations[0].Token.Color.WorkID != "work-output" {
		t.Fatalf("restored output mutations = %#v, want one output Work mutation", mutations)
	}
}

func TestReconcileRuntimeRestoredDispatches(t *testing.T) {
	t.Parallel()

	t.Run("disabled reconciliation", func(t *testing.T) {
		if err := reconcileRuntimeRestoredDispatches(&runtimeConfig{skipRestoredDispatchReconciliation: true}, nil); err != nil {
			t.Fatalf("reconcile disabled Factory Runtime: %v", err)
		}
	})
	t.Run("preserves reconciliation errors", func(t *testing.T) {
		cfg := &runtimeConfig{restoredWorldState: &interfaces.FactoryWorldState{
			ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
				"dispatch-without-identity": {TransitionID: "process"},
			},
		}}
		err := reconcileRuntimeRestoredDispatches(cfg, &recordingfixtures.ScriptedRuntimeLedger{})
		if err == nil {
			t.Fatal("reconcile invalid restored dispatch: want an identity error")
		}
	})
}
