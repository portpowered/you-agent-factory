// Package factoryevent maps canonical Factory events to the generated public
// transport contract.
package factoryevent

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ToAPI decodes one detached canonical Factory event into the generated
// OpenAPI union used at public transport boundaries.
func ToAPI(event interfaces.FactoryEvent) (factoryapi.FactoryEvent, error) {
	var mapped factoryapi.FactoryEvent
	if err := event.Decode(&mapped); err != nil {
		return factoryapi.FactoryEvent{}, fmt.Errorf("map canonical Factory event %q to API: %w", event.Id, err)
	}
	return mapped, nil
}

// SliceToAPI maps canonical Factory events in their existing order.
func SliceToAPI(events []interfaces.FactoryEvent) ([]factoryapi.FactoryEvent, error) {
	if events == nil {
		return nil, nil
	}
	mapped := make([]factoryapi.FactoryEvent, len(events))
	for index, event := range events {
		converted, err := ToAPI(event)
		if err != nil {
			return nil, err
		}
		mapped[index] = converted
	}
	return mapped, nil
}
