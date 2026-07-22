// Package factoryeventprojection maps public Factory events into the canonical
// Factory world-state projection boundary.
package factoryeventprojection

import (
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ReconstructFactoryWorldState converts public events at the transport
// boundary, then delegates ordering and reduction to the Factory owner.
func ReconstructFactoryWorldState(
	reconstruct recordings.WorldStateReconstructor,
	events []factoryapi.FactoryEvent,
	selectedTick int,
) (factorycontracts.FactoryWorldState, error) {
	canonicalEvents := make([]factorycontracts.FactoryEvent, 0, len(events))
	for _, event := range events {
		canonicalEvent, err := factorycontracts.NewFactoryEvent(event)
		if err != nil {
			return factorycontracts.FactoryWorldState{}, err
		}
		canonicalEvents = append(canonicalEvents, canonicalEvent)
	}
	return reconstruct(canonicalEvents, selectedTick)
}
