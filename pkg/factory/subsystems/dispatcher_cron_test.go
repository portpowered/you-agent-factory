package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// portos:func-length-exception owner=agent-factory reason=cron-dispatch-fixture review=2026-07-18 removal=split-cron-dispatch-fixture-before-next-cron-dispatch-change
func TestDispatcher_CronTransitionDispatchesThroughWorkerPathWithTimeToken(t *testing.T) {
	currentTime := time.Date(2026, time.April, 18, 12, 0, 0, 0, time.UTC)
	n := &state.Net{
		Places: map[string]*petri.Place{
			"signal:init":                       {ID: "signal:init"},
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"poll-with-input": {
				ID:         "poll-with-input",
				Name:       "poll-with-input",
				WorkerType: "cron-worker",
				InputArcs: []petri.Arc{
					{
						ID:          "signal-in",
						Name:        "signal",
						PlaceID:     "signal:init",
						Direction:   petri.ArcInput,
						Mode:        interfaces.ArcModeConsume,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					},
					{
						ID:          "time-in",
						Name:        "time",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Mode:        interfaces.ArcModeConsume,
						Guard:       &petri.CronTimeWindowGuard{Workstation: "poll-with-input"},
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					},
				},
			},
		},
	}

	dispatcher := NewDispatcher(
		n,
		scheduler.NewFIFOScheduler(),
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return currentTime }),
	)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"signal-token": {
				ID:      "signal-token",
				PlaceID: "signal:init",
				Color: interfaces.TokenColor{
					RequestID:  "request-signal",
					WorkID:     "signal-work",
					WorkTypeID: "signal",
					DataType:   interfaces.DataTypeWork,
					TraceID:    "trace-signal",
				},
			},
			"time-work": dispatcherCronTimeToken("time-work", "poll-with-input", currentTime.Add(-time.Second), currentTime.Add(time.Minute)),
		}),
		TickCount: 42,
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one cron dispatch", result)
	}

	dispatch := result.Dispatches[0].Dispatch
	if dispatch.WorkerType != "cron-worker" {
		t.Fatalf("worker type = %q, want cron-worker", dispatch.WorkerType)
	}
	inputTokens := workers.WorkDispatchInputTokens(dispatch)
	if len(inputTokens) != 2 {
		t.Fatalf("input token count = %d, want signal and time tokens: %#v", len(inputTokens), inputTokens)
	}
	if !dispatchHasInputWorkID(inputTokens, "signal-work") || !dispatchHasInputWorkID(inputTokens, "time-work") {
		t.Fatalf("cron dispatch inputs = %#v, want signal and time work tokens", inputTokens)
	}
	if dispatch.Execution.RequestID != "request-signal" {
		t.Fatalf("execution request ID = %q, want request-signal", dispatch.Execution.RequestID)
	}
	if dispatch.Execution.TraceID != "trace-signal" {
		t.Fatalf("execution trace ID = %q, want trace-signal", dispatch.Execution.TraceID)
	}
	if strings.Join(dispatch.Execution.WorkIDs, ",") != "signal-work" {
		t.Fatalf("execution work IDs = %#v, want only customer work signal-work", dispatch.Execution.WorkIDs)
	}
	if dispatch.Execution.ReplayKey != "poll-with-input/trace-signal/signal-work" {
		t.Fatalf("replay key = %q, want customer-work replay key", dispatch.Execution.ReplayKey)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatcher-determinism-fixture review=2026-07-18 removal=split-determinism-fixture-before-next-dispatcher-determinism-change
func TestDispatcher_RepeatedRunsProduceStableDispatchAndTokenSequences(t *testing.T) {
	n := &state.Net{
		Places: map[string]*petri.Place{
			"p-work-a":         {ID: "p-work-a"},
			"p-work-b":         {ID: "p-work-b"},
			"slot-a:available": {ID: "slot-a:available"},
			"slot-b:available": {ID: "slot-b:available"},
			"p-done":           {ID: "p-done"},
		},
		Transitions: map[string]*petri.Transition{
			"transition-b": {
				ID:         "transition-b",
				Name:       "step-b",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-work-b", Name: "work", PlaceID: "p-work-b", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "arc-slot-b", Name: "slot", PlaceID: "slot-b:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "arc-out-b", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
			"transition-a": {
				ID:         "transition-a",
				Name:       "step-a",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{ID: "arc-work-a", Name: "work", PlaceID: "p-work-a", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "arc-slot-a", Name: "slot", PlaceID: "slot-a:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "arc-out-a", Name: "out", PlaceID: "p-done", Direction: petri.ArcOutput},
				},
			},
		},
	}

	wantDispatches := []string{"transition-a", "transition-b"}
	wantWorkTokens := []string{"tok-a", "tok-b"}
	wantResourceTokens := []string{"slot-a-1", "slot-b-1"}

	for i := 0; i < 10; i++ {
		dispatcher := NewDispatcher(n, scheduler.NewFIFOScheduler(), nil, nil)
		snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{
				Tokens: map[string]*interfaces.Token{
					"tok-b":    {ID: "tok-b", PlaceID: "p-work-b", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
					"tok-a":    {ID: "tok-a", PlaceID: "p-work-a", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
					"slot-a-2": {ID: "slot-a-2", PlaceID: "slot-a:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-a-1": {ID: "slot-a-1", PlaceID: "slot-a:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-b-2": {ID: "slot-b-2", PlaceID: "slot-b:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
					"slot-b-1": {ID: "slot-b-1", PlaceID: "slot-b:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
				},
				PlaceTokens: map[string][]string{
					"p-work-b":         {"tok-b"},
					"p-work-a":         {"tok-a"},
					"slot-a:available": {"slot-a-2", "slot-a-1"},
					"slot-b:available": {"slot-b-2", "slot-b-1"},
				},
			},
		}

		result, err := dispatcher.Execute(context.Background(), &snapshot)
		if err != nil {
			t.Fatalf("iteration %d unexpected error: %v", i, err)
		}
		if result == nil {
			t.Fatalf("iteration %d expected dispatch result", i)
		}

		gotDispatches, gotWorkTokens, gotResourceTokens := dispatchSequences(result.Dispatches)
		if strings.Join(gotDispatches, ",") != strings.Join(wantDispatches, ",") {
			t.Fatalf("iteration %d dispatch sequence = %v, want %v", i, gotDispatches, wantDispatches)
		}
		if strings.Join(gotWorkTokens, ",") != strings.Join(wantWorkTokens, ",") {
			t.Fatalf("iteration %d work token sequence = %v, want %v", i, gotWorkTokens, wantWorkTokens)
		}
		if strings.Join(gotResourceTokens, ",") != strings.Join(wantResourceTokens, ",") {
			t.Fatalf("iteration %d resource token sequence = %v, want %v", i, gotResourceTokens, wantResourceTokens)
		}
	}
}

func TestDispatcher_UsesDispatcherClockForCronTimeWindowGuard(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(2 * time.Minute)
	expiresAt := base.Add(7 * time.Minute)
	currentTime := dueAt.Add(-time.Nanosecond)

	n := &state.Net{
		Places: map[string]*petri.Place{
			interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID},
		},
		Transitions: map[string]*petri.Transition{
			"cron-refresh": {
				ID:         "cron-refresh",
				Name:       "refresh",
				WorkerType: "script",
				InputArcs: []petri.Arc{
					{
						ID:          "cron-time",
						Name:        "time",
						PlaceID:     interfaces.SystemTimePendingPlaceID,
						Direction:   petri.ArcInput,
						Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
						Guard:       &petri.CronTimeWindowGuard{Workstation: "refresh"},
					},
				},
			},
		},
	}

	dispatcher := NewDispatcher(
		n,
		scheduler.NewFIFOScheduler(),
		nil,
		nil,
		WithDispatcherClock(func() time.Time { return currentTime }),
	)
	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: makeDispatcherSnapshot(map[string]*interfaces.Token{
			"time-refresh": dispatcherCronTimeToken("time-refresh", "refresh", dueAt, expiresAt),
		}),
	}

	result, err := dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error before due: %v", err)
	}
	if result != nil {
		t.Fatalf("result before due = %#v, want nil", result)
	}

	currentTime = dueAt
	result, err = dispatcher.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("unexpected error at due: %v", err)
	}
	if result == nil || len(result.Dispatches) != 1 {
		t.Fatalf("dispatches at due = %#v, want one dispatch", result)
	}
	if result.Dispatches[0].Dispatch.TransitionID != "cron-refresh" {
		t.Fatalf("transition id = %q, want cron-refresh", result.Dispatches[0].Dispatch.TransitionID)
	}
}
