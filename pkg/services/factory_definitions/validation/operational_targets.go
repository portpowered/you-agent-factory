package validation

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

const (
	CodeFactoryPayloadInvalid = factorydefinitions.ValidationCodeFactoryPayloadInvalid
	CodeFactoryNameInvalid    = factorydefinitions.ValidationCodeFactoryNameInvalid
	CodeFactoryVersionStale   = factorydefinitions.ValidationCodeFactoryVersionStale
	CodeFactoryRuntimeNotIdle = factorydefinitions.ValidationCodeFactoryRuntimeNotIdle
	CodeFactorySessionField   = factorydefinitions.ValidationCodeFactorySessionField
	CodeFactorySessionTarget  = factorydefinitions.ValidationCodeFactorySessionTarget
)

// FormFactoryPayloadTarget reports invalid or unreadable factory request bodies.
func FormFactoryPayloadTarget() Target {
	return factorydefinitions.FormFactoryPayloadValidationTarget()
}

// InvalidFactoryNameTarget reports invalid named-factory identifiers.
func InvalidFactoryNameTarget() Target {
	return factorydefinitions.InvalidFactoryNameValidationTarget()
}

// StaleFactoryVersionTarget reports optimistic-concurrency conflicts on editable saves.
func StaleFactoryVersionTarget() Target {
	return factorydefinitions.StaleFactoryVersionValidationTarget()
}

// FactoryRuntimeNotIdleTarget reports activation blocked by active runtime work.
func FactoryRuntimeNotIdleTarget() Target {
	return factorydefinitions.FactoryRuntimeNotIdleValidationTarget()
}

// FactorySessionFieldTarget reports factory-session open validation failures.
func FactorySessionFieldTarget(reason, field, message string) Target {
	return factorydefinitions.FactorySessionFieldValidationTarget(reason, field, message)
}

// FactorySessionTargetTarget reports discovery-time factory target failures.
func FactorySessionTargetTarget(reason, targetID, message string) Target {
	return factorydefinitions.FactorySessionTargetValidationTarget(reason, targetID, message)
}
