package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
)

func TestMoveWork_AcceptsWhileFactoryPaused(t *testing.T) {
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{{
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

	result, err := f.MoveWork(ctx, "work-paused-move", "complete", work.WorkStateChangeSourceCLI, "")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}
	assertOperatorWorkStateChangeRecord(t, history, "work-paused-move", "init", "complete", work.WorkStateChangeSourceCLI)

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
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	submitCtx := context.Background()
	if _, err := submitWorkRequests(submitCtx, f, []work.SubmitRequest{{
		WorkID:     "work-stream-move",
		WorkTypeID: "task",
		TraceID:    "trace-stream-move",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(submitCtx); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	stream, err := f.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream.StreamGenerationID == "" || history.CallCount("Subscribe") != 1 {
		t.Fatalf("subscription = %#v calls=%v, want injected ledger seam", stream, history.CallsSnapshot())
	}

	if _, err := f.MoveWork(submitCtx, "work-stream-move", "complete", work.WorkStateChangeSourceAPI, ""); err != nil {
		t.Fatalf("MoveWork: %v", err)
	}
	assertOperatorWorkStateChangeRecord(t, history, "work-stream-move", "init", "complete", work.WorkStateChangeSourceAPI)
	// Recordings owns publication of the root work-state-change fact to the
	// subscribed canonical stream and proves replay-before-live ordering.
}

func assertOperatorWorkStateChangeRecord(
	t *testing.T,
	history *recordingfixtures.ScriptedRuntimeLedger,
	workID string,
	fromState string,
	toState string,
	source work.WorkStateChangeSource,
) {
	t.Helper()
	for _, record := range history.WorkStateChanges {
		if record.WorkID == workID && record.FromState == fromState && record.ToState == toState {
			if record.Source != source {
				t.Fatalf("record source = %q, want %q", record.Source, source)
			}
			return
		}
	}
	t.Fatalf("records = %#v, want work-state change for %s %s -> %s", history.WorkStateChanges, workID, fromState, toState)
}
