package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestEnablementEvaluator_ContextPassedThrough(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	marking := makeTestSnapshot(map[string]*interfaces.Token{})
	enabled := eval.FindEnabledTransitions(ctx, &state.Net{Transitions: map[string]*petri.Transition{}}, &marking)
	if len(enabled) != 0 {
		t.Fatalf("expected 0 enabled transitions, got %d", len(enabled))
	}
}

func TestEnablementEvaluator_UsesInjectedClockForCronTimeWindowGuard(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(2 * time.Minute)
	expiresAt := base.Add(7 * time.Minute)
	currentTime := dueAt.Add(-time.Nanosecond)
	eval := NewEnablementEvaluator(nil, WithEnablementClock(func() time.Time { return currentTime }))

	n := &state.Net{
		Places: map[string]*petri.Place{interfaces.SystemTimePendingPlaceID: {ID: interfaces.SystemTimePendingPlaceID}},
		Transitions: map[string]*petri.Transition{
			"cron-refresh": {
				ID:         "cron-refresh",
				WorkerType: "script",
				InputArcs:  []petri.Arc{{ID: "cron-time", Name: "time", PlaceID: interfaces.SystemTimePendingPlaceID, Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.CronTimeWindowGuard{Workstation: "refresh"}}},
			},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens:      map[string]*interfaces.Token{"time-refresh": schedulerCronTimeToken("time-refresh", "refresh", dueAt, expiresAt)},
		PlaceTokens: map[string][]string{interfaces.SystemTimePendingPlaceID: {"time-refresh"}},
	}

	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled before due = %d, want 0", len(enabled))
	}
	currentTime = dueAt
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
		t.Fatalf("enabled at due = %d, want 1", len(enabled))
	}
	currentTime = expiresAt.Add(-time.Nanosecond)
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 1 {
		t.Fatalf("enabled before expiry = %d, want 1", len(enabled))
	}
	currentTime = expiresAt
	if enabled := eval.FindEnabledTransitions(context.Background(), n, &marking); len(enabled) != 0 {
		t.Fatalf("enabled at expiry = %d, want 0", len(enabled))
	}
}

func TestEnablementEvaluator_OrdersEnabledTransitionsByID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)

	n := &state.Net{
		Places: map[string]*petri.Place{"p-alpha": {ID: "p-alpha"}, "p-beta": {ID: "p-beta"}, "p-zeta": {ID: "p-zeta"}},
		Transitions: map[string]*petri.Transition{
			"transition-zeta":  {ID: "transition-zeta", WorkerType: "script", InputArcs: []petri.Arc{{ID: "arc-zeta", Name: "work", PlaceID: "p-zeta", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}},
			"transition-alpha": {ID: "transition-alpha", WorkerType: "script", InputArcs: []petri.Arc{{ID: "arc-alpha", Name: "work", PlaceID: "p-alpha", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}},
			"transition-beta":  {ID: "transition-beta", WorkerType: "script", InputArcs: []petri.Arc{{ID: "arc-beta", Name: "work", PlaceID: "p-beta", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens:      map[string]*interfaces.Token{"tok-zeta": {ID: "tok-zeta", PlaceID: "p-zeta"}, "tok-alpha": {ID: "tok-alpha", PlaceID: "p-alpha"}, "tok-beta": {ID: "tok-beta", PlaceID: "p-beta"}},
		PlaceTokens: map[string][]string{"p-zeta": {"tok-zeta"}, "p-alpha": {"tok-alpha"}, "p-beta": {"tok-beta"}},
	}

	for i := 0; i < 10; i++ {
		enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
		if got, want := strings.Join(transitionIDs(enabled), ","), "transition-alpha,transition-beta,transition-zeta"; got != want {
			t.Fatalf("iteration %d enabled transition order = %v, want %v", i, transitionIDs(enabled), []string{"transition-alpha", "transition-beta", "transition-zeta"})
		}
	}
}

func TestEnablementEvaluator_SelectsOrdinaryTokensByStableID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{"p-work": {ID: "p-work"}},
		Transitions: map[string]*petri.Transition{
			"transition-work": {ID: "transition-work", WorkerType: "script", InputArcs: []petri.Arc{{ID: "arc-work", Name: "work", PlaceID: "p-work", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 2}}}},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"tok-c": {ID: "tok-c", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"tok-a": {ID: "tok-a", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"tok-b": {ID: "tok-b", PlaceID: "p-work", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
		},
		PlaceTokens: map[string][]string{"p-work": {"tok-c", "tok-a", "tok-b"}},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got, want := strings.Join(tokenIDs(enabled[0].Bindings["work"]), ","), "tok-a,tok-b"; got != want {
		t.Fatalf("bound ordinary tokens = %v, want %v", tokenIDs(enabled[0].Bindings["work"]), []string{"tok-a", "tok-b"})
	}
}

func TestEnablementEvaluator_SelectsResourceTokensByStableID(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{"slot:available": {ID: "slot:available"}},
		Transitions: map[string]*petri.Transition{
			"transition-slot": {ID: "transition-slot", WorkerType: "script", InputArcs: []petri.Arc{{ID: "arc-slot", Name: "slot", PlaceID: "slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}}}},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"slot-2": {ID: "slot-2", PlaceID: "slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
			"slot-1": {ID: "slot-1", PlaceID: "slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
		},
		PlaceTokens: map[string][]string{"slot:available": {"slot-2", "slot-1"}},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("enabled transitions = %d, want 1", len(enabled))
	}
	if got, want := strings.Join(tokenIDs(enabled[0].Bindings["slot"]), ","), "slot-1"; got != want {
		t.Fatalf("bound resource tokens = %v, want %v", tokenIDs(enabled[0].Bindings["slot"]), []string{"slot-1"})
	}
}

func TestEnablementEvaluator_ExpandsRepeatedWorkAndResourceBindingsForSameTransition(t *testing.T) {
	eval := NewEnablementEvaluator(nil)
	n := &state.Net{
		Places: map[string]*petri.Place{"task:init": {ID: "task:init"}, "executor-slot:available": {ID: "executor-slot:available"}},
		Transitions: map[string]*petri.Transition{
			"process": {ID: "process", WorkerType: "processor", InputArcs: []petri.Arc{
				{ID: "work-in", Name: "work", PlaceID: "task:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}, Guard: &petri.DependencyGuard{}},
				{ID: "slot-in", Name: "slot", PlaceID: "executor-slot:available", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 1}},
			}},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens: map[string]*interfaces.Token{
			"work-b": {ID: "work-b", PlaceID: "task:init", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"work-a": {ID: "work-a", PlaceID: "task:init", Color: interfaces.TokenColor{DataType: interfaces.DataTypeWork}},
			"slot-2": {ID: "slot-2", PlaceID: "executor-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
			"slot-1": {ID: "slot-1", PlaceID: "executor-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}},
		},
		PlaceTokens: map[string][]string{"task:init": {"work-b", "work-a"}, "executor-slot:available": {"slot-2", "slot-1"}},
	}

	enabled := eval.FindEnabledTransitions(context.Background(), n, &marking)
	if len(enabled) != 1 {
		t.Fatalf("base enabled candidates = %d, want 1", len(enabled))
	}
	expanded := ExpandRepeatedBindings(n, &marking, enabled)
	if len(expanded) != 2 {
		t.Fatalf("expanded candidates = %d, want 2", len(expanded))
	}
	if got := strings.Join(append(tokenIDs(expanded[0].Bindings["work"]), tokenIDs(expanded[0].Bindings["slot"])...), ","); got != "work-a,slot-1" {
		t.Fatalf("first candidate tokens = %v, want [work-a slot-1]", got)
	}
	if got := strings.Join(append(tokenIDs(expanded[1].Bindings["work"]), tokenIDs(expanded[1].Bindings["slot"])...), ","); got != "work-b,slot-2" {
		t.Fatalf("second candidate tokens = %v, want [work-b slot-2]", got)
	}
}
