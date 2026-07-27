package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProjectionQueriesProduceIdenticalDetachedLiveAndReplayResults(t *testing.T) {
	t.Parallel()

	projection := New()
	history := representativeHistory(t)
	original := cloneEvents(history)
	const selectedTick = 5

	var live factorydefinitions.FactoryWorldState
	for accepted := range history {
		var err error
		live, err = projection.ReconstructFactoryWorldState(history[:accepted+1], selectedTick)
		if err != nil {
			t.Fatalf("incremental reduction after event %d: %v", accepted, err)
		}
	}
	replayed, err := projection.ReconstructFactoryWorldState(history, selectedTick)
	if err != nil {
		t.Fatalf("retained-history reduction: %v", err)
	}

	if !reflect.DeepEqual(live, replayed) {
		t.Fatalf("live world state differs from replay:\nlive: %#v\nreplay: %#v", live, replayed)
	}
	if !reflect.DeepEqual(history, original) {
		t.Fatal("projection mutated caller-owned event input")
	}

	assertEquivalentDerivedQueries(t, projection, live, replayed)

	replayed.WorkItemsByID["work-1"] = work.FactoryWorkItem{ID: "mutated"}
	replayed.Topology.Places[0].ID = "mutated"
	replayed.CompletedDispatches[0].WorkItemIDs[0] = "mutated"
	again, err := projection.ReconstructFactoryWorldState(history, selectedTick)
	if err != nil {
		t.Fatalf("repeat retained-history reduction: %v", err)
	}
	if !reflect.DeepEqual(again, live) {
		t.Fatal("mutating a returned world state changed a later result")
	}
	if !reflect.DeepEqual(history, original) {
		t.Fatal("repeat projection mutated caller-owned event input")
	}
}

func TestProjectionQueriesRejectMalformedInputWithoutPartialState(t *testing.T) {
	t.Parallel()

	projection := New()
	valid := representativeHistory(t)
	malformed := append(cloneEvents(valid), canonicalEvent(
		t,
		factorydefinitions.FactoryEventTypeWorkRequest,
		"malformed",
		6,
		time.Date(2026, 7, 25, 12, 0, 6, 0, time.UTC),
		factorydefinitions.FactoryEventContext{},
		json.RawMessage(`{"type":`),
	))

	tests := []struct {
		name         string
		events       []factorydefinitions.FactoryEvent
		selectedTick int
	}{
		{name: "negative selected tick", events: valid, selectedTick: -1},
		{name: "malformed event payload", events: malformed, selectedTick: 6},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			state, err := projection.ReconstructFactoryWorldState(test.events, test.selectedTick)
			if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
				t.Fatalf("error = %v, want ErrInvalidProjectionInput", err)
			}
			if !reflect.DeepEqual(state, factorydefinitions.FactoryWorldState{}) {
				t.Fatalf("partial state = %#v, want zero result", state)
			}
		})
	}
}

func assertEquivalentDerivedQueries(
	t *testing.T,
	projection *Service,
	live factorydefinitions.FactoryWorldState,
	replayed factorydefinitions.FactoryWorldState,
) {
	t.Helper()

	liveDashboard := projection.SimpleDashboardRenderData(live)
	replayDashboard := projection.SimpleDashboardRenderData(replayed)
	if !reflect.DeepEqual(liveDashboard, replayDashboard) {
		t.Fatalf("live dashboard differs from replay: %#v != %#v", liveDashboard, replayDashboard)
	}

	liveRequests := projection.ProjectWorkstationRequests(live)
	replayRequests := projection.ProjectWorkstationRequests(replayed)
	if !reflect.DeepEqual(liveRequests, replayRequests) {
		t.Fatalf("live workstation requests differ from replay: %#v != %#v", liveRequests, replayRequests)
	}

	pauses := []factorydefinitions.ActiveThrottlePause{
		{
			LaneID:      "reviewer/openai/gpt-5",
			Provider:    "openai",
			Model:       "gpt-5",
			PausedAt:    time.Date(2026, 7, 25, 12, 0, 3, 0, time.UTC),
			PausedUntil: time.Date(2026, 7, 25, 12, 1, 3, 0, time.UTC),
		},
	}
	livePauses := projection.ProjectActiveThrottlePauses(live.Topology, pauses)
	replayPauses := projection.ProjectActiveThrottlePauses(replayed.Topology, pauses)
	if !reflect.DeepEqual(livePauses, replayPauses) {
		t.Fatalf("live throttle pauses differ from replay: %#v != %#v", livePauses, replayPauses)
	}

	again := projection.SimpleDashboardRenderData(live)
	if !reflect.DeepEqual(again, liveDashboard) {
		t.Fatal("repeated dashboard query is not stable")
	}
	if len(liveDashboard.Session.DispatchHistory) == 0 {
		t.Fatal("representative dashboard omitted dispatch history")
	}
	liveDashboard.Session.DispatchHistory[0].WorkItemIDs[0] = "mutated"
	detachedDashboard := projection.SimpleDashboardRenderData(live)
	if detachedDashboard.Session.DispatchHistory[0].WorkItemIDs[0] != "work-1" {
		t.Fatal("dashboard query result aliases a prior returned nested slice")
	}

	if liveRequests.WorkstationRequestsByDispatchId == nil {
		t.Fatal("representative workstation-request projection is empty")
	}
	requests := *liveRequests.WorkstationRequestsByDispatchId
	request := requests["dispatch-1"]
	if request.Request.InputWorkItems == nil {
		t.Fatal("representative workstation request omitted input work")
	}
	(*request.Request.InputWorkItems)[0].WorkId = "mutated"
	detachedRequests := projection.ProjectWorkstationRequests(live)
	if got := (*(*detachedRequests.WorkstationRequestsByDispatchId)["dispatch-1"].Request.InputWorkItems)[0].WorkId; got != "work-1" {
		t.Fatalf("workstation request query result aliases a prior result: %q", got)
	}
}

func representativeHistory(t *testing.T) []factorydefinitions.FactoryEvent {
	t.Helper()

	t0 := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	sessionID := "factory-session-1"
	dispatchID := "dispatch-1"
	requestID := "request-1"
	workIDs := []string{"work-1"}
	traceIDs := []string{"trace-1"}
	snapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name": "projection-query-fixture",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]any{
				{"name": "ready", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
			},
		}},
		"workstations": []map[string]any{{
			"id":      "review",
			"name":    "Review",
			"worker":  "reviewer",
			"inputs":  []map[string]any{{"workType": "task", "state": "ready"}},
			"outputs": []map[string]any{{"workType": "task", "state": "done"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	return []factorydefinitions.FactoryEvent{
		canonicalEvent(t, factorydefinitions.FactoryEventTypeInitialStructureRequest, "structure", 0, t0,
			factorydefinitions.FactoryEventContext{Sequence: 1, SessionID: &sessionID},
			factorydefinitions.InitialStructureRequestEventPayload{Factory: snapshot}),
		canonicalEvent(t, factorydefinitions.FactoryEventTypeSessionStarted, "session-started", 1, t0.Add(time.Second),
			factorydefinitions.FactoryEventContext{Sequence: 2, SessionID: &sessionID},
			factorydefinitions.FactorySessionStartedEventPayload{StartedAt: t0.Add(time.Second)}),
		canonicalEvent(t, factorydefinitions.FactoryEventTypeWorkRequest, "work-request", 2, t0.Add(2*time.Second),
			factorydefinitions.FactoryEventContext{
				Sequence: 3, SessionID: &sessionID, RequestID: &requestID,
				WorkIDs: &workIDs, TraceIDs: &traceIDs,
			},
			work.WorkRequestEventPayload{
				Type: work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.WorkRequestEventWork{{
					Name: "Write projection", WorkID: "work-1", RequestID: requestID,
					WorkTypeID: "task", TraceID: "trace-1",
					State: &work.WorkEventState{Name: "ready", Type: "INITIAL"},
					Tags:  map[string]string{"priority": "high"},
				}},
			}),
		canonicalEvent(t, factorydefinitions.FactoryEventTypeDispatchRequest, "dispatch-request", 3, t0.Add(3*time.Second),
			factorydefinitions.FactoryEventContext{
				Sequence: 4, SessionID: &sessionID, DispatchID: &dispatchID,
				WorkIDs: &workIDs, TraceIDs: &traceIDs,
			},
			factorydefinitions.DispatchRequestEventPayload{
				TransitionID: "review",
				Inputs:       []factorydefinitions.DispatchConsumedWorkRef{{WorkID: "work-1"}},
			}),
		canonicalEvent(t, factorydefinitions.FactoryEventTypeDispatchResponse, "dispatch-response", 4, t0.Add(4*time.Second),
			factorydefinitions.FactoryEventContext{
				Sequence: 5, SessionID: &sessionID, DispatchID: &dispatchID,
				WorkIDs: &workIDs, TraceIDs: &traceIDs,
			},
			workerexecution.DispatchResponseEventPayload{
				TransitionID: "review",
				Outcome:      workerexecution.WorkOutcome("ACCEPTED"),
			}),
		canonicalEvent(t, factorydefinitions.FactoryEventTypeSessionCompleted, "session-completed", 5, t0.Add(5*time.Second),
			factorydefinitions.FactoryEventContext{Sequence: 6, SessionID: &sessionID},
			factorydefinitions.FactorySessionCompletedEventPayload{
				CompletedAt: t0.Add(5 * time.Second),
				FinalStatus: factorydefinitions.FactorySessionLifecycleStatus("COMPLETED"),
			}),
	}
}

func canonicalEvent(
	t *testing.T,
	eventType factorydefinitions.FactoryEventType,
	id string,
	tick int,
	eventTime time.Time,
	context factorydefinitions.FactoryEventContext,
	payload any,
) factorydefinitions.FactoryEvent {
	t.Helper()

	var encoded json.RawMessage
	switch value := payload.(type) {
	case json.RawMessage:
		encoded = append(json.RawMessage(nil), value...)
	default:
		var err error
		encoded, err = json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s payload: %v", eventType, err)
		}
	}
	context.Tick = tick
	context.EventTime = eventTime
	return factorydefinitions.FactoryEvent{
		Context:       context,
		Id:            id,
		Payload:       encoded,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          eventType,
	}
}

func cloneEvents(events []factorydefinitions.FactoryEvent) []factorydefinitions.FactoryEvent {
	cloned := make([]factorydefinitions.FactoryEvent, len(events))
	for index, event := range events {
		cloned[index] = event.Clone()
	}
	return cloned
}
