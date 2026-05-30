package validation

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ToErrorTargets maps canonical targets onto the legacy API ErrorTarget shape used by
// factory write responses until the OpenAPI contract adopts canonical subjects directly.
func ToErrorTargets(targets []Target) []factoryapi.ErrorTarget {
	if len(targets) == 0 {
		return nil
	}
	mapped := make([]factoryapi.ErrorTarget, 0, len(targets))
	for _, target := range targets {
		mapped = append(mapped, toErrorTarget(target))
	}
	return mapped
}

func toErrorTarget(target Target) factoryapi.ErrorTarget {
	kind, id, field := legacyErrorTargetFields(target)
	apiTarget := factoryapi.ErrorTarget{Kind: kind}
	if id != "" {
		apiTarget.Id = &id
	}
	if field != "" {
		apiTarget.Field = &field
	}
	return apiTarget
}

func legacyErrorTargetFields(target Target) (kind, id, field string) {
	if target.Path != "" {
		return legacyErrorTargetFromPath(target)
	}
	switch target.Subject.Type {
	case SubjectTypeWorkstation:
		switch target.Subject.Location {
		case SubjectLocationOnFailure, SubjectLocationOnRejection, SubjectLocationOutputs, SubjectLocationInputs:
			return "field", target.Subject.ID, ""
		default:
			return "field", target.Subject.ID, ""
		}
	case SubjectTypeWorkType:
		return "node", target.Subject.ID, ""
	case SubjectTypeWorkState:
		return "node", target.Subject.ID, ""
	case SubjectTypeWorker, SubjectTypeResource:
		return "field", target.Subject.ID, ""
	case SubjectTypeRoute:
		return "edge", target.Subject.ID, ""
	default:
		return "field", target.Subject.ID, ""
	}
}

func legacyErrorTargetFromPath(target Target) (kind, id, field string) {
	switch target.Code {
	case CodeDuplicateIdentifier:
		switch target.Subject.Type {
		case SubjectTypeWorkState:
			return "node", target.Subject.ID, target.Path
		case SubjectTypeWorkType, SubjectTypeWorker, SubjectTypeResource, SubjectTypeWorkstation:
			if target.Subject.ID == "" {
				return "field", "", target.Path
			}
			return "node", target.Subject.ID, target.Path
		default:
			return "field", target.Subject.ID, target.Path
		}
	case CodeDanglingPlaceReference, CodeDanglingResourceReference:
		return "edge", target.Subject.ID, target.Path
	case CodeDanglingWorkerReference:
		return "field", target.Subject.ID, target.Path
	case CodeWorkstationMissingFailureRoute, CodeWorkstationMissingRejectionRoute:
		return "field", target.Subject.ID, target.Path
	case CodeWorkstationConflictingOutputs:
		return "edge", target.Subject.ID, target.Path
	default:
		if target.Subject.Type == SubjectTypeRoute {
			return "edge", target.Subject.ID, target.Path
		}
		return "field", target.Subject.ID, target.Path
	}
}
