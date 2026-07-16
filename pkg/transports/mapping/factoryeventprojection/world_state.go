// Package factoryeventprojection maps public Factory events into the canonical
// Factory world-state projection boundary.
package factoryeventprojection

import (
	factorycontracts "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ReconstructFactoryWorldState converts public events at the transport
// boundary, then delegates ordering and reduction to the Factory owner.
func ReconstructFactoryWorldState(
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
	return projections.ReconstructCanonicalFactoryWorldState(canonicalEvents, selectedTick)
}
