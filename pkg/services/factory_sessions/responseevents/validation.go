package responseevents

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

type ValidationError = workers.ValidationError

// ValidateDraft validates a Workers-owned provider draft.
func ValidateDraft(draft Draft) error {
	return workers.ValidateDraft(draft)
}

// ValidateEvent validates the semantic portion of a published session event.
func ValidateEvent(event FactoryResponseEvent) error {
	return workers.ValidateDraft(workers.Draft{
		Kind: event.Kind, Phase: event.Phase, Payload: event.Payload,
	})
}
