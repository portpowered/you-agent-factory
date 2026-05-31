package runtime

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestMoveWork_AcceptsWhileFactoryPaused(t *testing.T) {
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{
		WorkID:     "work-paused-move",
		WorkTypeID: "task",
		TraceID:    "trace-paused-move",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	result, err := f.MoveWork(ctx, "work-paused-move", "complete", interfaces.WorkStateChangeSourceCLI, "")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}
	assertOperatorWorkStateChangeEvent(t, f, "work-paused-move", "init", "complete", factoryapi.WorkStateChangeSourceCLI)

	snap, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !markingContainsWorkAtPlace(&snap.Marking, "work-paused-move", "task:complete") {
		t.Fatalf("marking = %#v, want work-paused-move at task:complete", snap.Marking.Tokens)
	}
}

func buildMoveControlNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range wt.GeneratePlaces() {
		places[place.ID] = place
	}
	return &state.Net{
		ID:          "move-control-net",
		Places:      places,
		Transitions: make(map[string]*petri.Transition),
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func TestMoveWork_SubscribeReceivesWorkStateChangeInOrder(t *testing.T) {
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	submitCtx := context.Background()
	if _, err := submitWorkRequests(submitCtx, f, []interfaces.SubmitRequest{{
		WorkID:     "work-stream-move",
		WorkTypeID: "task",
		TraceID:    "trace-stream-move",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(submitCtx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := f.SubscribeFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}

	if _, err := f.MoveWork(submitCtx, "work-stream-move", "complete", interfaces.WorkStateChangeSourceAPI, ""); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}

	deadline := time.After(time.Second)
	for {
		select {
		case event := <-stream.Events:
			if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
				continue
			}
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("work state change payload: %v", err)
			}
			if payload.WorkId != "work-stream-move" || payload.Source != factoryapi.WorkStateChangeSourceAPI {
				t.Fatalf("payload = %#v, want api move for work-stream-move", payload)
			}
			goto verifiedLiveStream
		case <-deadline:
			t.Fatal("timed out waiting for WORK_STATE_CHANGE on live factory event stream")
		}
	}
verifiedLiveStream:

	events := runtimeGeneratedEvents(t, f)
	last := events[len(events)-1]
	if last.Type != factoryapi.FactoryEventTypeWorkStateChange {
		t.Fatalf("last event type = %q, want WORK_STATE_CHANGE", last.Type)
	}
}

func assertOperatorWorkStateChangeEvent(
	t *testing.T,
	f factory.Factory,
	workID string,
	fromState string,
	toState string,
	source factoryapi.WorkStateChangeSource,
) {
	t.Helper()
	events := runtimeGeneratedEvents(t, f)
	var found factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil {
			t.Fatalf("work state change payload: %v", err)
		}
		if payload.WorkId == workID && payload.FromState == fromState && payload.ToState == toState {
			found = event
			if payload.Source != source {
				t.Fatalf("payload source = %q, want %q", payload.Source, source)
			}
			break
		}
	}
	if found.Type == "" {
		t.Fatalf("events = %#v, want WORK_STATE_CHANGE for %s %s -> %s", events, workID, fromState, toState)
	}
	if found.Context.EventTime.Location() != time.UTC {
		t.Fatalf("eventTime location = %s, want UTC", found.Context.EventTime.Location())
	}
}
