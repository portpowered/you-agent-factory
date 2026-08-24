package projections

import (
	"reflect"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

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
