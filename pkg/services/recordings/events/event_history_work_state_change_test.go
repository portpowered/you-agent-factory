package events

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_RecordWorkStateChange_OperatorMoveShape(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return eventTime })

	history.RecordWorkStateChange(3, work.WorkStateChangeRecord{
		WorkID:        "work-1",
		WorkTypeID:    "task",
		FromState:     "failed",
		ToState:       "in-progress",
		FromPlaceID:   "task:failed",
		ToPlaceID:     "task:in-progress",
		Source:        work.WorkStateChangeSourceCLI,
		RequestID:     "move-request-1",
		TriggerWorkID: "work-parent",
		Reason:        "operator recovery",
	}, eventTime)

	stream, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical history: %v", err)
	}
	if len(stream.History) != 1 {
		t.Fatalf("canonical history count = %d, want 1", len(stream.History))
	}
	var canonicalPayload interfaces.WorkStateChangeEventPayload
	if err := stream.History[0].DecodePayload(&canonicalPayload); err != nil {
		t.Fatalf("decode canonical payload: %v", err)
	}
	if canonicalPayload.WorkID != "work-1" || canonicalPayload.Source != work.WorkStateChangeSourceCLI {
		t.Fatalf("canonical payload = %#v, want owner-defined work state fields", canonicalPayload)
	}

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertWorkStateChangeOperatorEvent(t, events[0])
}

func assertWorkStateChangeOperatorEvent(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	assertWorkStateChangeEventEnvelope(t, event)
	assertWorkStateChangeOperatorPayload(t, event)
}

func assertWorkStateChangeEventEnvelope(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
		t.Fatalf("event type = %q, want WORK_STATE_CHANGE", event.Type)
	}
	if event.Id != "factory-event/work-state-change/work-1/3" {
		t.Fatalf("event id = %q, want stable work-state-change id", event.Id)
	}
	if event.Context.Tick != 3 || event.Context.Sequence != 0 {
		t.Fatalf("event context = %#v, want tick 3 sequence 0", event.Context)
	}
	if stringValueForEventHistoryTest(event.Context.RequestId) != "move-request-1" {
		t.Fatalf("requestId = %q, want move-request-1", stringValueForEventHistoryTest(event.Context.RequestId))
	}
	if event.Context.WorkIds == nil || len(*event.Context.WorkIds) != 1 || (*event.Context.WorkIds)[0] != "work-1" {
		t.Fatalf("workIds = %#v, want [work-1]", event.Context.WorkIds)
	}
}

func assertWorkStateChangeOperatorPayload(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	payload, err := event.Payload.AsWorkStateChangeEventPayload()
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.WorkId != "work-1" ||
		payload.WorkTypeName != "task" ||
		payload.FromState != "failed" ||
		payload.ToState != "in-progress" ||
		payload.FromPlaceId != "task:failed" ||
		payload.ToPlaceId != "task:in-progress" ||
		payload.Source != factoryapi.WorkStateChangeSourceCLI {
		t.Fatalf("payload = %#v, want canonical operator move fields", payload)
	}
	if stringValueForEventHistoryTest(payload.TriggerWorkId) != "work-parent" {
		t.Fatalf("triggerWorkId = %q, want work-parent", stringValueForEventHistoryTest(payload.TriggerWorkId))
	}
	if stringValueForEventHistoryTest(payload.Reason) != "operator recovery" {
		t.Fatalf("reason = %q, want operator recovery", stringValueForEventHistoryTest(payload.Reason))
	}
}

func TestFactoryEventHistory_RecordWorkStateChange_NormalizesEventTimeToUTC(t *testing.T) {
	localZone := time.FixedZone("Factory/Local", 7*60*60)
	eventTime := time.Date(2026, 4, 22, 23, 30, 0, 0, localZone)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkStateChange(4, work.WorkStateChangeRecord{
		WorkID:      "work-utc",
		WorkTypeID:  "task",
		FromState:   "init",
		ToState:     "done",
		FromPlaceID: "task:init",
		ToPlaceID:   "task:done",
		Source:      work.WorkStateChangeSourceAPI,
	}, eventTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-22T16:30:00Z")
}

func TestFactoryEventHistory_RecordWorkStateChange_MatchesRecordWorkRequestEventTimeHandling(t *testing.T) {
	localZone := time.FixedZone("Factory/Local", 7*60*60)
	eventTime := time.Date(2026, 4, 22, 23, 30, 0, 0, localZone)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(4, work.WorkRequestRecord{
		RequestID: "request-utc",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-utc",
		WorkItems: []work.FactoryWorkItem{{ID: "work-utc", WorkTypeID: "task"}},
	}, eventTime)
	history.RecordWorkStateChange(4, work.WorkStateChangeRecord{
		WorkID:      "work-utc",
		WorkTypeID:  "task",
		FromState:   "init",
		ToState:     "done",
		FromPlaceID: "task:init",
		ToPlaceID:   "task:done",
		Source:      work.WorkStateChangeSourceCLI,
	}, eventTime)

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-22T16:30:00Z")
	assertEventTimeUTCJSON(t, events[1], "2026-04-22T16:30:00Z")
}

func TestFactoryEventHistory_StateEventsUseCanonicalPayloadsAndRetainPublicWireShape(t *testing.T) {
	startedAt := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Second)
	history := newTestFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return startedAt })

	history.RecordRunResponse(7, interfaces.FactoryStateCompleted, "factory complete", finishedAt)
	history.RecordFactoryStateChange(8, interfaces.FactoryStateRunning, interfaces.FactoryStateCompleted, "all work complete", finishedAt)

	stream, err := history.Subscribe(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("subscribe canonical history: %v", err)
	}
	if len(stream.History) != 2 {
		t.Fatalf("canonical history count = %d, want 2", len(stream.History))
	}
	assertCanonicalFactoryStateEvents(t, stream.History, finishedAt)
	assertPublicFactoryStateEvents(t, generatedHistoryEvents(t, history))
}

func assertCanonicalFactoryStateEvents(t *testing.T, events []interfaces.FactoryEvent, finishedAt time.Time) {
	t.Helper()
	var runPayload interfaces.RunResponseEventPayload
	if err := events[0].DecodePayload(&runPayload); err != nil {
		t.Fatalf("decode canonical run response: %v", err)
	}
	if runPayload.State == nil || *runPayload.State != interfaces.FactoryStateCompleted || runPayload.WallClock == nil || runPayload.WallClock.FinishedAt == nil || !runPayload.WallClock.FinishedAt.Equal(finishedAt) {
		t.Fatalf("canonical run response = %#v, want completed state and wall clock", runPayload)
	}

	var statePayload interfaces.FactoryStateResponseEventPayload
	if err := events[1].DecodePayload(&statePayload); err != nil {
		t.Fatalf("decode canonical factory state response: %v", err)
	}
	if statePayload.PreviousState == nil || *statePayload.PreviousState != interfaces.FactoryStateRunning || statePayload.State != interfaces.FactoryStateCompleted {
		t.Fatalf("canonical factory state response = %#v, want running to completed", statePayload)
	}
}

func assertPublicFactoryStateEvents(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	runPublic, err := events[0].Payload.AsRunResponseEventPayload()
	if err != nil {
		t.Fatalf("decode public run response: %v", err)
	}
	if runPublic.State == nil || *runPublic.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("public run state = %#v, want COMPLETED", runPublic.State)
	}
	statePublic, err := events[1].Payload.AsFactoryStateResponseEventPayload()
	if err != nil {
		t.Fatalf("decode public factory state response: %v", err)
	}
	if statePublic.PreviousState == nil || *statePublic.PreviousState != factoryapi.FactoryStateRunning || statePublic.State != factoryapi.FactoryStateCompleted {
		t.Fatalf("public factory state response = %#v, want RUNNING to COMPLETED", statePublic)
	}
}
