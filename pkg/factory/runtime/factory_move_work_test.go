package runtime

import (
	"context"
	"testing"

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

	result, err := f.MoveWork(ctx, "work-paused-move", "complete")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}

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
