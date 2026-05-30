package validation

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

const (
	CodeFactoryPayloadInvalid     = "factory.payload.invalid"
	CodeFactoryNameInvalid        = "factory.name.invalid"
	CodeFactoryVersionStale       = "factory.version.stale"
	CodeFactoryRuntimeNotIdle     = "factory.runtime.notIdle"
	CodeFactorySessionField       = "factory.session.field"
)

// FormFactoryPayloadTarget reports invalid or unreadable factory request bodies.
func FormFactoryPayloadTarget() factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     CodeFactoryPayloadInvalid,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "Factory request payload is invalid.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeFactory,
			Id:       "",
			Location: factoryapi.FactoryValidationSubjectLocationDefinition,
		},
	}
}

// InvalidFactoryNameTarget reports invalid named-factory identifiers.
func InvalidFactoryNameTarget() factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     CodeFactoryNameInvalid,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeFactory,
			Id:       "",
			Location: factoryapi.FactoryValidationSubjectLocationDefinition,
		},
	}
}

// StaleFactoryVersionTarget reports optimistic-concurrency conflicts on editable saves.
func StaleFactoryVersionTarget() factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     CodeFactoryVersionStale,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "Current factory definition is stale. Refresh the graph before saving.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeFactory,
			Id:       "",
			Location: factoryapi.FactoryValidationSubjectLocationDefinition,
		},
	}
}

// FactoryRuntimeNotIdleTarget reports activation blocked by active runtime work.
func FactoryRuntimeNotIdleTarget() factoryapi.FactoryValidationTarget {
	return factoryapi.FactoryValidationTarget{
		Code:     CodeFactoryRuntimeNotIdle,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  "Current factory runtime must be idle before activation.",
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeFactory,
			Id:       "",
			Location: factoryapi.FactoryValidationSubjectLocationDefinition,
		},
	}
}

// FactorySessionFieldTarget reports factory-session open validation failures.
func FactorySessionFieldTarget(reason, field, message string) factoryapi.FactoryValidationTarget {
	code := CodeFactorySessionField + "." + reason
	return factoryapi.FactoryValidationTarget{
		Code:     code,
		Severity: factoryapi.FactoryValidationSeverityError,
		Message:  message,
		Subject: factoryapi.FactoryValidationSubject{
			Type:     factoryapi.FactoryValidationSubjectTypeFactory,
			Id:       field,
			Location: factoryapi.FactoryValidationSubjectLocationReference,
		},
	}
}
