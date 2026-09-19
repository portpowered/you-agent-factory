package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
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
	if completed.DispatchID != "dispatch-restored-failure" || completed.Outcome != workerexecution.OutcomeFailed ||
		!completed.StartTime.Equal(startedAt) || !completed.EndTime.Equal(completedAt) || completed.Duration != 750*time.Millisecond ||
		len(completed.ConsumedTokens) != 1 || completed.ConsumedTokens[0].Color.WorkID != workID ||
		completed.FailureDetail == nil || *completed.FailureDetail != *failure {
		t.Fatalf("restored completed dispatch = %#v, want Work-associated failure and exact timing", completed)
	}
	completed.FailureDetail.Message = "mutated detached snapshot"
	second, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after mutation: %v", err)
	}
	if second.DispatchHistory[0].FailureDetail == nil || second.DispatchHistory[0].FailureDetail.Message != failure.Message {
		t.Fatalf("mutated restored dispatch history leaked into runtime: %#v", second.DispatchHistory[0].FailureDetail)
	}
}
