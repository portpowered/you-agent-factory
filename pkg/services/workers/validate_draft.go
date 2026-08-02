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

// KnownKinds returns every declared response draft Kind, the sole normalized
// event vocabulary this package owns, in stable declaration order.
func KnownKinds() []Kind {
	internalKinds := workstationdraftvalidation.KnownKinds()
	out := make([]Kind, len(internalKinds))
	for i, kind := range internalKinds {
		out[i] = Kind(kind)
	}
	return out
}

// AllowedPhasesForKind returns the declared legal phases for kind and whether
// kind is a declared response draft kind. Callers must not treat the returned
// slice as exhaustive for an unknown kind.
func AllowedPhasesForKind(kind Kind) ([]Phase, bool) {
	internalPhases, ok := workstationdraftvalidation.AllowedPhases(workstationdraftvalidation.Kind(kind))
	if !ok {
		return nil, false
	}
	out := make([]Phase, len(internalPhases))
	for i, phase := range internalPhases {
		out[i] = Phase(phase)
	}
	return out, true
}
