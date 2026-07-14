package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type Draft = shared.Draft

func ValidateDraft(draft Draft) error {
	return ValidateEvent(FactoryResponseEvent{Kind: draft.Kind, Phase: draft.Phase, Payload: draft.Payload})
}
