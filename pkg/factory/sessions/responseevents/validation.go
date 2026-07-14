package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type ValidationError = shared.ValidationError

func ValidateEvent(event FactoryResponseEvent) error {
	return shared.ValidateEvent(event)
}
