package testutil

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FactoryEvent converts a generated boundary fixture to the canonical domain
// envelope. Tests should keep generated payload construction at the transport
// edge and put only domain events into FactoryEventStream.
func FactoryEvent(t testing.TB, event factoryapi.FactoryEvent) interfaces.FactoryEvent {
	t.Helper()
	converted, err := interfaces.NewFactoryEvent(event)
	if err != nil {
		t.Fatalf("convert generated FactoryEvent fixture: %v", err)
	}
	return converted
}

// FactoryEvents converts generated boundary fixtures to detached domain events.
func FactoryEvents(t testing.TB, events []factoryapi.FactoryEvent) []interfaces.FactoryEvent {
	t.Helper()
	converted := make([]interfaces.FactoryEvent, 0, len(events))
	for _, event := range events {
		converted = append(converted, FactoryEvent(t, event))
	}
	return converted
}

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
