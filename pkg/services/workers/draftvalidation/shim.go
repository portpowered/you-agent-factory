// Package draftvalidation is a transitional compile shim that re-exports draft
// validation from the private workstations destination.
package draftvalidation

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstationdraftvalidation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/draftvalidation"
)

type ValidationError = workstationdraftvalidation.ValidationError

func ValidateDraft(draft workers.Draft) error {
	return workstationdraftvalidation.ValidateDraft(workstationdraftvalidation.Draft{
		Kind:    workstationdraftvalidation.Kind(draft.Kind),
		Phase:   workstationdraftvalidation.Phase(draft.Phase),
		Payload: draft.Payload,
	})
}
