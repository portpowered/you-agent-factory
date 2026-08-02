package workers

import workstationdraftvalidation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/draftvalidation"

// ValidationError identifies the response-event field that failed draft validation.
type ValidationError = workstationdraftvalidation.ValidationError

// ValidateDraft checks a provider draft before it crosses the publication boundary.
func ValidateDraft(draft Draft) error {
	return workstationdraftvalidation.ValidateDraft(workstationdraftvalidation.Draft{
		Kind:    workstationdraftvalidation.Kind(draft.Kind),
		Phase:   workstationdraftvalidation.Phase(draft.Phase),
		Payload: draft.Payload,
	})
}
