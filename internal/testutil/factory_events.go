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
