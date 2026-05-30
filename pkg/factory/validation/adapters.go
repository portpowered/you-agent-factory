package validation

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ToValidationTargets maps canonical targets onto the validation-only API response shape.
func ToValidationTargets(targets []Target) []factoryapi.FactoryValidationTarget {
	if len(targets) == 0 {
		return []factoryapi.FactoryValidationTarget{}
	}
	mapped := make([]factoryapi.FactoryValidationTarget, 0, len(targets))
	for _, target := range targets {
		mapped = append(mapped, toValidationTarget(target))
	}
	return mapped
}

func toValidationTarget(target Target) factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     target.Code,
		Severity: toValidationSeverity(target.Severity),
		Message:  target.Message,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     toValidationSubjectType(target.Subject.Type),
			Id:       target.Subject.ID,
			Location: toValidationSubjectLocation(target.Subject.Location),
		},
	}
}

func toValidationSeverity(severity Severity) factoryapi.FactoryValidationSeverity {
	switch severity {
	case SeverityWarning:
		return factoryapi.FactoryValidationSeverityWarning
	case SeverityHint:
		return factoryapi.FactoryValidationSeverityHint
	default:
		return factoryapi.FactoryValidationSeverityError
	}
}

func toValidationSubjectType(subjectType SubjectType) factoryapi.FactoryValidationSubjectType {
	switch subjectType {
	case SubjectTypeWorkstation:
		return factoryapi.FactoryValidationSubjectTypeWorkstation
	case SubjectTypeWorkType:
		return factoryapi.FactoryValidationSubjectTypeWorkType
	case SubjectTypeWorkState:
		return factoryapi.FactoryValidationSubjectTypeWorkState
	case SubjectTypeWorker:
		return factoryapi.FactoryValidationSubjectTypeWorker
	case SubjectTypeResource:
		return factoryapi.FactoryValidationSubjectTypeResource
	case SubjectTypeRoute:
		return factoryapi.FactoryValidationSubjectTypeRoute
	default:
		return factoryapi.FactoryValidationSubjectTypeFactory
	}
}

func toValidationSubjectLocation(location SubjectLocation) factoryapi.FactoryValidationSubjectLocation {
	switch location {
	case SubjectLocationOnRejection:
		return factoryapi.FactoryValidationSubjectLocationOnRejection
	case SubjectLocationOnFailure:
		return factoryapi.FactoryValidationSubjectLocationOnFailure
	case SubjectLocationOutputs:
		return factoryapi.FactoryValidationSubjectLocationOutputs
	case SubjectLocationInputs:
		return factoryapi.FactoryValidationSubjectLocationInputs
	case SubjectLocationStates:
		return factoryapi.FactoryValidationSubjectLocationStates
	case SubjectLocationTerminal:
		return factoryapi.FactoryValidationSubjectLocationTerminal
	case SubjectLocationReference:
		return factoryapi.FactoryValidationSubjectLocationReference
	default:
		return factoryapi.FactoryValidationSubjectLocationDefinition
	}
}

