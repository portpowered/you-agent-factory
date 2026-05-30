package events

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryEventHistory_RecordWorkStateChange_OperatorMoveShape(t *testing.T) {
	eventTime := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return eventTime })

	history.RecordWorkStateChange(3, interfaces.WorkStateChangeRecord{
		WorkID:       "work-1",
		WorkTypeID:   "task",
		FromState:    "failed",
		ToState:      "in-progress",
		FromPlaceID:  "task:failed",
		ToPlaceID:    "task:in-progress",
		Source:       interfaces.WorkStateChangeSourceCLI,
		RequestID:    "move-request-1",
		TriggerWorkID: "work-parent",
		Reason:       "operator recovery",
	}, eventTime)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	event := events[0]
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
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkStateChange(4, interfaces.WorkStateChangeRecord{
		WorkID:      "work-utc",
		WorkTypeID:  "task",
		FromState:   "init",
		ToState:     "done",
		FromPlaceID: "task:init",
		ToPlaceID:   "task:done",
		Source:      interfaces.WorkStateChangeSourceAPI,
	}, eventTime)

	events := history.Events()
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-22T16:30:00Z")
}

func TestFactoryEventHistory_RecordWorkStateChange_MatchesRecordWorkRequestEventTimeHandling(t *testing.T) {
	localZone := time.FixedZone("Factory/Local", 7*60*60)
	eventTime := time.Date(2026, 4, 22, 23, 30, 0, 0, localZone)
	history := NewFactoryEventHistory(eventHistoryProjectionNet(), func() time.Time { return time.Unix(0, 0).UTC() })

	history.RecordWorkRequest(4, interfaces.WorkRequestRecord{
		RequestID: "request-utc",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-utc",
		WorkItems: []interfaces.FactoryWorkItem{{ID: "work-utc", WorkTypeID: "task"}},
	}, eventTime)
	history.RecordWorkStateChange(4, interfaces.WorkStateChangeRecord{
		WorkID:      "work-utc",
		WorkTypeID:  "task",
		FromState:   "init",
		ToState:     "done",
		FromPlaceID: "task:init",
		ToPlaceID:   "task:done",
		Source:      interfaces.WorkStateChangeSourceCLI,
	}, eventTime)

	events := history.Events()
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	assertEventTimeUTCJSON(t, events[0], "2026-04-22T16:30:00Z")
	assertEventTimeUTCJSON(t, events[1], "2026-04-22T16:30:00Z")
}
