package workers

import workstationdraftvalidation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/draftvalidation"

// ValidationError identifies the response-event field that failed draft validation.
type ValidationError = workstationdraftvalidation.ValidationError

// ValidateDraft checks a provider draft before it crosses the publication
// boundary. Canonical Kind and Phase vocabulary validation runs first so an
// unknown Kind or unknown Phase is reported distinctly from a known Kind
// paired with a known but disallowed Phase, which the existing internal
// legal-pair policy owner reports as a phase-policy failure.
func ValidateDraft(draft Draft) error {
	if err := draft.Kind.Validate(); err != nil {
		return err
	}
	if err := draft.Phase.Validate(); err != nil {
		return err
	}
	return workstationdraftvalidation.ValidateDraft(workstationdraftvalidation.Draft{
		Kind:    workstationdraftvalidation.Kind(draft.Kind),
		Phase:   workstationdraftvalidation.Phase(draft.Phase),
		Payload: draft.Payload,
	})
}
