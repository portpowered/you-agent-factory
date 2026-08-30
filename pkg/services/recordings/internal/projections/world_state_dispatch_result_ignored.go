package projections

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// applyDispatchResultIgnoredEvent validates the redacted stale-result
// diagnostic while intentionally leaving the reconstructed world unchanged.
// The corresponding dispatch response was never applied to the marking, so
// replay must preserve that same state no-op.
func (r *factoryWorldReducer) applyDispatchResultIgnoredEvent(event interfaces.FactoryEvent) error {
	var payload interfaces.DispatchResultIgnoredEventPayload
	return event.DecodePayload(&payload)
}
