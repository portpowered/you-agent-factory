package testutil

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// GeneratedFactoryEvent decodes a canonical domain envelope for assertions at
// the generated transport compatibility boundary.
func GeneratedFactoryEvent(t testing.TB, event interfaces.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	var generated factoryapi.FactoryEvent
	if err := event.Decode(&generated); err != nil {
		t.Fatalf("decode canonical FactoryEvent fixture: %v", err)
	}
	return generated
}

// GeneratedFactoryEvents decodes canonical domain envelopes for generated
// transport compatibility assertions.
func GeneratedFactoryEvents(t testing.TB, events []interfaces.FactoryEvent) []factoryapi.FactoryEvent {
	t.Helper()
	generated := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		generated = append(generated, GeneratedFactoryEvent(t, event))
	}
	return generated
}
