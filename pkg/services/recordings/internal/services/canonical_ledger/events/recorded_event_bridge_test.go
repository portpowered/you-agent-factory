package events

import (
	"context"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestRecordedEventBridgePreservesCanonicalEvent(t *testing.T) {
	t.Parallel()

	history := newTestFactoryEventHistory(nil, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := history.Subscribe(ctx, nil, factorydefinitions.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	eventTime := time.Date(2026, time.July, 16, 1, 2, 3, 0, time.FixedZone("test", -7*60*60))
	history.AppendRecordedEvent(factorydefinitions.FactoryEvent{
		Id:      "recorded-event-1",
		Type:    factorydefinitions.FactoryEventTypeRunRequest,
		Context: factorydefinitions.FactoryEventContext{EventTime: eventTime},
	})

	recorded := history.CanonicalEvents()
	if len(recorded) != 1 {
		t.Fatalf("len(Events()) = %d, want 1", len(recorded))
	}
	if recorded[0].Id != "recorded-event-1" || recorded[0].SchemaVersion != factorydefinitions.FactoryEventSchemaVersionV1 {
		t.Fatalf("recorded event identity = (%q, %q), want preserved ID and canonical schema", recorded[0].Id, recorded[0].SchemaVersion)
	}
	if got, want := recorded[0].Context.EventTime, eventTime.UTC(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("recorded event time = %v (%v), want %v (UTC)", got, got.Location(), want)
	}

	select {
	case live := <-stream.Events:
		if live.Id != recorded[0].Id || live.SchemaVersion != recorded[0].SchemaVersion || !live.Context.EventTime.Equal(recorded[0].Context.EventTime) {
			t.Fatalf("live recorded event = %#v, want canonical history event %#v", live, recorded[0])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recorded event bridge live delivery")
	}
}
