package projections

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	modulePrefix              = "github.com/portpowered/infinite-you/"
	factoryRuntimeRoot        = modulePrefix + "pkg/services/factory_runtime"
	recordingsProjectionsRoot = modulePrefix + "pkg/services/recordings/internal/projections"
)

func TestFactoryWorldReducerHandlesCanonicalReplayEdges(t *testing.T) {
	t.Parallel()

	reducer := newFactoryWorldReducerWithReplayTopology(t)
	assertReplayFactoryChange(t, reducer)
	assertReplayReducerValidationEdges(t, reducer)
	assertReplayReducerOutputEdges(t, reducer)
}

func newFactoryWorldReducerWithReplayTopology(t *testing.T) *factoryWorldReducer {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "run-factory",
		"workTypes": []any{map[string]any{"name": "task", "states": []any{
			map[string]any{"name": "ready", "type": "INITIAL"},
			map[string]any{"name": "failed", "type": "FAILED"},
		}}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	reducer := newFactoryWorldReducer(3)
	runRequest := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventContext{Tick: 1}, interfaces.RunRequestEventPayload{Factory: snapshot})
	if err := reducer.applyStructureEvent(runRequest); err != nil {
		t.Fatalf("applyStructureEvent(run request): %v", err)
	}
	if reducer.stateValue.Topology.Name != "run-factory" {
		t.Fatalf("run request topology = %#v, want run-factory", reducer.stateValue.Topology)
	}
	incompleteRun := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventContext{Tick: 2}, interfaces.RunRequestEventPayload{})
	if err := reducer.applyStructureEvent(incompleteRun); err != nil {
		t.Fatalf("applyStructureEvent(replayed incomplete run request): %v", err)
	}
	if reducer.stateValue.Topology.Name != "run-factory" {
		t.Fatalf("replayed incomplete run request changed topology = %#v", reducer.stateValue.Topology)
	}
	return reducer
}

func assertReplayFactoryChange(t *testing.T, reducer *factoryWorldReducer) {
	t.Helper()
	changedSnapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "changed-factory",
		"workTypes": []any{map[string]any{"name": "task", "states": []any{
			map[string]any{"name": "ready", "type": "INITIAL"},
			map[string]any{"name": "done", "type": "TERMINAL"},
		}}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot(changed): %v", err)
	}
	factoryChange := canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeFactoryChange,
		interfaces.FactoryEventContext{Tick: 3}, interfaces.FactoryChangeEventPayload{Factory: changedSnapshot})
	if err := reducer.applyStructureEvent(factoryChange); err != nil {
		t.Fatalf("applyStructureEvent(factory change): %v", err)
	}
	if reducer.stateValue.Topology.Name != "changed-factory" || len(reducer.stateValue.Topology.Places) != 2 {
		t.Fatalf("factory change topology = %#v, want changed factory with two places", reducer.stateValue.Topology)
	}
}

func assertReplayReducerValidationEdges(t *testing.T, reducer *factoryWorldReducer) {
	t.Helper()
	if err := reducer.applyCanonicalFactory(nil); err == nil {
		t.Fatal("applyCanonicalFactory(nil) error = nil, want required snapshot error")
	}
	if err := reducer.applyWorkRequestEvent(canonicalWorldProjectionEvent(t,
		interfaces.FactoryEventTypeWorkRequest, interfaces.FactoryEventContext{}, work.WorkRequestEventPayload{})); err == nil {
		t.Fatal("applyWorkRequestEvent(missing type) error = nil, want validation error")
	}
	if err := reducer.applyRelationshipChangeEvent(canonicalWorldProjectionEvent(t,
		interfaces.FactoryEventTypeRelationshipChangeRequest, interfaces.FactoryEventContext{}, work.RelationshipChangeRequestEventPayload{})); err == nil {
		t.Fatal("applyRelationshipChangeEvent(missing type) error = nil, want validation error")
	}
	if err := reducer.apply(interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeRunResponse}); err != nil {
		t.Fatalf("apply(run response): %v", err)
	}
	if err := reducer.apply(interfaces.FactoryEvent{Type: interfaces.FactoryEventType("unknown")}); err != nil {
		t.Fatalf("apply(unknown): %v", err)
	}
}

func assertReplayReducerOutputEdges(t *testing.T, reducer *factoryWorldReducer) {
	t.Helper()
	if got := reducer.outputPlaceForWork("missing-workstation", workerexecution.OutcomeFailed, "task"); got != "" {
		t.Fatalf("missing workstation output place = %q, want empty", got)
	}
	reducer.applyInitialStructure(interfaces.InitialStructurePayload{
		Places:       []interfaces.FactoryPlace{{ID: "task:failed", TypeID: "task", State: "failed", Category: "FAILED"}},
		Workstations: []interfaces.FactoryWorkstation{{ID: "review", Name: "Review"}},
	})
	if got := reducer.outputPlaceForWork("review", workerexecution.OutcomeFailed, "task"); got != "task:failed" {
		t.Fatalf("failed output place = %q, want task:failed", got)
	}
	if got := buildSimpleDashboardWorkstationNodes(interfaces.InitialStructurePayload{
		Workstations: []interfaces.FactoryWorkstation{{ID: interfaces.SystemTimeExpiryTransitionID}},
	}); got != nil {
		t.Fatalf("system-only dashboard nodes = %#v, want nil", got)
	}
	workIDs := []string{"work-target"}
	if got := sourceWorkIDFromCanonicalContext(interfaces.FactoryEventContext{WorkIDs: &workIDs}, "work-target"); got != "" {
		t.Fatalf("source work ID for target-only context = %q, want empty", got)
	}
	beforeTokens := len(reducer.tokenPlaces)
	reducer.removeToken("")
	if len(reducer.tokenPlaces) != beforeTokens {
		t.Fatalf("empty token removal changed token index = %#v", reducer.tokenPlaces)
	}
	emptyResource := []interfaces.DispatchResourceRef{{}}
	if got := reducer.consumeResourceUnits(&emptyResource); len(got) != 0 {
		t.Fatalf("unnamed resource consumption = %#v, want none", got)
	}
	if got := firstConsumedResourceIndex(
		[]interfaces.FactoryResourceUnit{{ResourceID: "gpu"}}, []bool{false}, "cpu"); got != -1 {
		t.Fatalf("unmatched consumed resource index = %d, want -1", got)
	}
}

// TestProjectionsImportRuntimeRootOnly seals CUT-REC-RUN story 004: Recordings
// projection and observation surfaces may depend on Factory Runtime only through
// the service root contract.

// TestProjectionsConstructRuntimeObservationAndResultShapesThroughRoot proves
// projection/observation edges construct Runtime-owned marking, result, and
// observation vocabulary through the sealed Runtime root boundary.
func TestProjectionsConstructRuntimeObservationAndResultShapesThroughRoot(t *testing.T) {
	t.Parallel()

	uptime := 11 * time.Second
	engineSnapshot := factoryruntime.DashboardEngineStateSnapshot(
		"RUNNING",
		interfaces.RuntimeStatusActive,
		11,
		uptime,
	)
	if engineSnapshot.FactoryState != "RUNNING" || engineSnapshot.TickCount != 11 || engineSnapshot.Uptime != uptime {
		t.Fatalf("engine snapshot = %#v, want RUNNING tick=11 uptime=%v", engineSnapshot, uptime)
	}

	observeResult := factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Progress: factoryruntime.ObservationProgress{
				InFlightDispatchCount: 1,
				TickCount:             engineSnapshot.TickCount,
			},
			Health: factoryruntime.ObservationHealth{
				FactoryState: engineSnapshot.FactoryState,
				Uptime:       engineSnapshot.Uptime,
			},
		},
	}
	if observeResult.Observation.Progress.TickCount != 11 {
		t.Fatalf("observation tick count = %d, want 11", observeResult.Observation.Progress.TickCount)
	}
	if observeResult.Observation.Health.FactoryState != "RUNNING" {
		t.Fatalf("observation factory state = %q, want RUNNING", observeResult.Observation.Health.FactoryState)
	}

	sessionID := "session-projection-root"
	primaryJSON, err := json.Marshal(map[string]any{"ok": true})
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	resultParts := []work.WorkContentPart{{
		Type: work.WorkContentPartTypeJSON,
		JSON: primaryJSON,
	}}
	ownerProjection := factoryruntime.SessionResultProjection{
		Durable: factoryruntime.SessionResult{
			SessionID:     sessionID,
			ResultStatus:  factoryruntime.ResultStatusFinal,
			PrimaryResult: resultParts,
		},
		Updated: factoryruntime.SessionResultUpdatedPayload{
			ResultStatus:  interfaces.FactorySessionResultStatusFinal,
			ResultSummary: resultParts,
		},
	}
	eventPayload := apisurface.WorkflowSessionResultUpdatedPayloadToAPI(ownerProjection.Updated)
	t0 := time.Date(2026, 7, 27, 18, 0, 0, 0, time.UTC)
	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResultUpdated, interfaces.FactoryEventContext{
			EventTime: t0.Add(2 * time.Second),
			Tick:      2,
		}, interfaces.FactorySessionResultUpdatedEventPayload{
			ResultStatus:  interfaces.FactorySessionResultStatusFinal,
			ResultSummary: resultParts,
		}),
	}
	worldState, err := ReconstructCanonicalFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructCanonicalFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || worldState.SessionBracket.ResultStatus != string(interfaces.FactorySessionResultStatusFinal) {
		t.Fatalf("session bracket = %#v, want FINAL result status", worldState.SessionBracket)
	}
	durableResult := apisurface.WorkflowSessionResultToAPI(ownerProjection.Durable)
	if durableResult.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", durableResult.SessionId, sessionID)
	}
	if eventPayload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFinal {
		t.Fatalf("event payload result status = %q, want FINAL", eventPayload.ResultStatus)
	}
}

func TestReconstructCanonicalFactoryWorldState_OrderedDetachedInputSkipsCopyAndSort(t *testing.T) {
	eventTime := time.Date(2026, 8, 23, 18, 0, 0, 0, time.UTC)
	events := []interfaces.FactoryEvent{
		{Id: "event-1", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: eventTime}},
		{Id: "event-2", Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 2, EventTime: eventTime.Add(time.Second)}},
	}
	wantInput := append([]interfaces.FactoryEvent(nil), events...)
	stats := &worldStateReplayStats{}

	if _, err := reconstructCanonicalFactoryWorldState(events, 2, stats); err != nil {
		t.Fatalf("reconstructCanonicalFactoryWorldState: %v", err)
	}
	if stats.eventCopies != 0 || stats.sortPasses != 0 {
		t.Fatalf("ordered replay operations = %#v, want no copy or sort", stats)
	}
	if !reflect.DeepEqual(events, wantInput) {
		t.Fatalf("ordered replay mutated caller input: got %#v, want %#v", events, wantInput)
	}
}

func TestDispatchProjectionRetainsRunnerSelectionMetadata(t *testing.T) {
	t.Parallel()

	reducer := newFactoryWorldReducer(1)
	applyDispatch := func(dispatchID string, metadata *interfaces.DispatchRequestEventMetadata) interfaces.FactoryWorldDispatch {
		event := interfaces.FactoryEvent{
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
		}
		reducer.applyDispatchCreated(event, interfaces.DispatchRequestEventPayload{Metadata: metadata})
		return reducer.stateValue.ActiveDispatches[dispatchID]
	}

	withoutMetadata := applyDispatch("dispatch-without-runner-metadata", nil)
	if withoutMetadata.RunnerID != "" || withoutMetadata.RunnerSelectionSource != "" {
		t.Fatalf("dispatch without metadata = %#v, want empty runner facts", withoutMetadata)
	}

	runnerID := "claude"
	withoutSource := applyDispatch("dispatch-without-runner-source", &interfaces.DispatchRequestEventMetadata{
		RunnerID: &runnerID,
	})
	if withoutSource.RunnerID != runnerID || withoutSource.RunnerSelectionSource != "" {
		t.Fatalf("dispatch without source = %#v, want runner ID and empty source", withoutSource)
	}

	source := workerexecution.RunnerSelectionSourceWorkstation
	selected := applyDispatch("dispatch-with-runner-selection", &interfaces.DispatchRequestEventMetadata{
		RunnerID:              &runnerID,
		RunnerSelectionSource: &source,
	})
	if selected.RunnerID != runnerID || selected.RunnerSelectionSource != source {
		t.Fatalf("dispatch runner selection = %#v, want ID=%q source=%q", selected, runnerID, source)
	}
}

func TestReconstructCanonicalFactoryWorldState_OutOfOrderInputCopiesBeforeSort(t *testing.T) {
	eventTime := time.Date(2026, 8, 23, 18, 5, 0, 0, time.UTC)
	earlier := interfaces.FactoryEvent{
		Id:      "event-earlier",
		Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: eventTime},
	}
	later := interfaces.FactoryEvent{
		Id:      "event-later",
		Context: interfaces.FactoryEventContext{Tick: 2, Sequence: 2, EventTime: eventTime.Add(time.Second)},
	}
	events := []interfaces.FactoryEvent{later, earlier}
	wantInput := []interfaces.FactoryEvent{later.Clone(), earlier.Clone()}
	stats := &worldStateReplayStats{}

	if _, err := reconstructCanonicalFactoryWorldState(events, 2, stats); err != nil {
		t.Fatalf("reconstructCanonicalFactoryWorldState: %v", err)
	}
	if stats.eventCopies != len(events) || stats.sortPasses != 1 {
		t.Fatalf("out-of-order replay operations = %#v, want %d copies and one sort", stats, len(events))
	}
	if !reflect.DeepEqual(events, wantInput) {
		t.Fatalf("out-of-order replay mutated caller input: got %#v, want %#v", events, wantInput)
	}
}

func TestFactoryEventsInReplayOrder_UsesDeterministicTieBreakers(t *testing.T) {
	eventTime := time.Date(2026, 8, 23, 18, 10, 0, 0, time.UTC)
	ordered := []interfaces.FactoryEvent{
		{Id: "event-a", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: eventTime}},
		{Id: "event-b", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 1, EventTime: eventTime}},
	}
	if !factoryEventsInReplayOrder(ordered) {
		t.Fatal("events with equal ordering metadata should preserve stable input order")
	}
	if factoryEventsInReplayOrder([]interfaces.FactoryEvent{ordered[1], ordered[0]}) {
		t.Fatal("event ID tie-breaker should classify reversed input as out of order")
	}
}
