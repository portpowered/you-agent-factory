package factory_events

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIGetFactoryEventsReturnsOrderedDurableHistory proves retained Factory
// Event history is returned in durable ascending order through the public
// Factory Events API and that a second retained-history read preserves the same
// relative order for the same session generation.
func TestAPIGetFactoryEventsReturnsOrderedDurableHistory(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "ordered-durable-history")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})
	t.Cleanup(func() { server.Stop(t) })

	name := "ordered-durable-history-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "prove ordered durable history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	firstRead := server.GetFactoryEvents(t)
	if len(firstRead) < 4 {
		t.Fatalf("retained Factory Event count = %d, want at least 4 events after work completion", len(firstRead))
	}
	assertFactoryEventsAscendingOrder(t, firstRead)

	secondRead := server.GetFactoryEvents(t)
	if len(secondRead) != len(firstRead) {
		t.Fatalf(
			"second retained-history read count = %d, want %d for the same session generation",
			len(secondRead),
			len(firstRead),
		)
	}
	assertFactoryEventsSameRelativeOrder(t, firstRead, secondRead)
}

func assertFactoryEventsAscendingOrder(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	previousTick := -1
	previousSequence := -1
	for index, event := range events {
		if event.Context.Tick < previousTick {
			t.Fatalf(
				"Factory Event %d (%s) tick %d precedes previous tick %d",
				index,
				event.Id,
				event.Context.Tick,
				previousTick,
			)
		}
		if event.Context.Sequence < previousSequence {
			t.Fatalf(
				"Factory Event %d (%s) sequence %d precedes previous sequence %d",
				index,
				event.Id,
				event.Context.Sequence,
				previousSequence,
			)
		}
		previousTick = event.Context.Tick
		previousSequence = event.Context.Sequence
	}
}

func assertFactoryEventsSameRelativeOrder(
	t *testing.T,
	first []factoryapi.FactoryEvent,
	second []factoryapi.FactoryEvent,
) {
	t.Helper()

	if len(first) != len(second) {
		t.Fatalf("event count mismatch: first=%d second=%d", len(first), len(second))
	}
	for index := range first {
		if first[index].Id != second[index].Id {
			t.Fatalf(
				"retained-history reorder at index %d: first=%q second=%q",
				index,
				first[index].Id,
				second[index].Id,
			)
		}
		if first[index].Context.Sequence != second[index].Context.Sequence {
			t.Fatalf(
				"sequence changed at index %d for event %q between retained-history reads",
				index,
				first[index].Id,
			)
		}
	}
}
