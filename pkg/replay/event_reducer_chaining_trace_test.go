package replay

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestReplayDispatchFromEvent_PreservesConsumedInputChainingLineage(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "merge",
		Inputs:       []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-generated"}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromEvent(factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-1",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 5, 0, 0, time.UTC),
			Tick:       5,
			DispatchId: stringPtrIfNotEmpty("dispatch-1"),
			TraceIds:   slicePtr([]string{"trace-generated"}),
			WorkIds:    slicePtr([]string{"work-generated"}),
		},
		Payload: union,
	}, map[string]interfaces.Work{
		"work-generated": {
			WorkID:                   "work-generated",
			Name:                     "generated-merge-input",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-generated",
			PreviousChainingTraceIDs: []string{"trace-parent-a", "trace-parent-z"},
			TraceID:                  "trace-generated",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	tokens := workers.WorkDispatchInputTokens(replayed.dispatch)
	if len(tokens) != 1 {
		t.Fatalf("replayed input tokens = %#v, want one token", tokens)
	}
	if tokens[0].Color.CurrentChainingTraceID != "trace-generated" {
		t.Fatalf("replayed token current chaining trace ID = %q, want trace-generated", tokens[0].Color.CurrentChainingTraceID)
	}
	if got := tokens[0].Color.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "trace-parent-a" || got[1] != "trace-parent-z" {
		t.Fatalf("replayed token previous chaining trace IDs = %#v, want [trace-parent-a trace-parent-z]", got)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 1 || got[0] != "trace-generated" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [trace-generated]", got)
	}
}

func TestReplayDispatchFromEvent_PrefersContextChainingLineageOverPayloadCompatibilityCopy(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             "merge",
		CurrentChainingTraceId:   stringPtrIfNotEmpty("payload-current"),
		PreviousChainingTraceIds: slicePtr([]string{"payload-a", "payload-z"}),
		Inputs:                   []factoryapi.DispatchConsumedWorkRef{{WorkId: "work-generated"}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromEvent(factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-context-first",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:                time.Date(2026, 4, 22, 19, 7, 0, 0, time.UTC),
			Tick:                     6,
			DispatchId:               stringPtrIfNotEmpty("dispatch-context-first"),
			CurrentChainingTraceId:   stringPtrIfNotEmpty("context-current"),
			PreviousChainingTraceIds: slicePtr([]string{"context-a", "context-z"}),
			TraceIds:                 slicePtr([]string{"trace-generated"}),
			WorkIds:                  slicePtr([]string{"work-generated"}),
		},
		Payload: union,
	}, map[string]interfaces.Work{
		"work-generated": {
			WorkID:                   "work-generated",
			Name:                     "generated-merge-input",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "trace-generated",
			PreviousChainingTraceIDs: []string{"trace-parent-a", "trace-parent-z"},
			TraceID:                  "trace-generated",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if replayed.dispatch.CurrentChainingTraceID != "context-current" {
		t.Fatalf("replayed dispatch current chaining trace ID = %q, want context-current", replayed.dispatch.CurrentChainingTraceID)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "context-a" || got[1] != "context-z" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [context-a context-z]", got)
	}
}

func TestReplayDispatchFromEvent_FallsBackToContextWorkIDsWhenConsumedRefsOmitWorkID(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: "process",
		Inputs:       []factoryapi.DispatchConsumedWorkRef{{}},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromEvent(factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-legacy",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 6, 0, 0, time.UTC),
			Tick:       1,
			DispatchId: stringPtrIfNotEmpty("dispatch-legacy"),
			TraceIds:   slicePtr([]string{"trace-task-1"}),
			WorkIds:    slicePtr([]string{"work-task-1"}),
		},
		Payload: union,
	}, map[string]interfaces.Work{
		"work-task-1": {
			WorkID:                 "work-task-1",
			Name:                   "task-1",
			WorkTypeID:             "task",
			CurrentChainingTraceID: "trace-task-1",
			TraceID:                "trace-task-1",
		},
	})
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if got := replayed.dispatch.Execution.WorkIDs; len(got) != 1 || got[0] != "work-task-1" {
		t.Fatalf("replayed dispatch work IDs = %#v, want [work-task-1]", got)
	}
	tokens := workers.WorkDispatchInputTokens(replayed.dispatch)
	if len(tokens) != 1 {
		t.Fatalf("replayed input tokens = %#v, want one token", tokens)
	}
	if tokens[0].Color.WorkID != "work-task-1" {
		t.Fatalf("replayed token work ID = %q, want work-task-1", tokens[0].Color.WorkID)
	}
	if tokens[0].ID != "work-task-1" {
		t.Fatalf("replayed token ID = %q, want work-task-1", tokens[0].ID)
	}
}

func TestReplayDispatchFromEvent_FallsBackToPayloadChainingLineageForLegacyEvents(t *testing.T) {
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId:             "legacy",
		CurrentChainingTraceId:   stringPtrIfNotEmpty("payload-current"),
		PreviousChainingTraceIds: slicePtr([]string{"payload-a", "payload-z"}),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch payload: %v", err)
	}

	replayed, err := replayDispatchFromEvent(factoryapi.Factory{}, factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/dispatch-payload-fallback",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, 4, 22, 19, 8, 0, 0, time.UTC),
			Tick:       7,
			DispatchId: stringPtrIfNotEmpty("dispatch-payload-fallback"),
			TraceIds:   slicePtr([]string{"trace-generated"}),
		},
		Payload: union,
	}, nil)
	if err != nil {
		t.Fatalf("replayDispatchFromEvent: %v", err)
	}

	if replayed.dispatch.CurrentChainingTraceID != "payload-current" {
		t.Fatalf("replayed dispatch current chaining trace ID = %q, want payload-current", replayed.dispatch.CurrentChainingTraceID)
	}
	if got := replayed.dispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "payload-a" || got[1] != "payload-z" {
		t.Fatalf("replayed dispatch previous chaining trace IDs = %#v, want [payload-a payload-z]", got)
	}
}
