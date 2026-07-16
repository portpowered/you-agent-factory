package validation

const (
	CodeFactoryPayloadInvalid = "factory.payload.invalid"
	CodeFactoryNameInvalid    = "factory.name.invalid"
	CodeFactoryVersionStale   = "factory.version.stale"
	CodeFactoryRuntimeNotIdle = "factory.runtime.notIdle"
	CodeFactorySessionField   = "factory.session.field"
	CodeFactorySessionTarget  = "factory.session.target"
)

// FormFactoryPayloadTarget reports invalid or unreadable factory request bodies.
func FormFactoryPayloadTarget() Target {
	return Target{
		Code:     CodeFactoryPayloadInvalid,
		Severity: SeverityError,
		Message:  "Factory request payload is invalid.",
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "",
			Location: SubjectLocationDefinition,
		},
	}
}

// InvalidFactoryNameTarget reports invalid named-factory identifiers.
func InvalidFactoryNameTarget() Target {
	return Target{
		Code:     CodeFactoryNameInvalid,
		Severity: SeverityError,
		Message:  "Factory name must be a safe directory segment without path separators and cannot be the reserved current-factory identifier.",
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "",
			Location: SubjectLocationDefinition,
		},
	}
}

// StaleFactoryVersionTarget reports optimistic-concurrency conflicts on editable saves.
func StaleFactoryVersionTarget() Target {
	return Target{
		Code:     CodeFactoryVersionStale,
		Severity: SeverityError,
		Message:  "Current factory definition is stale. Refresh the graph before saving.",
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "",
			Location: SubjectLocationDefinition,
		},
	}
}

// FactoryRuntimeNotIdleTarget reports activation blocked by active runtime work.
func FactoryRuntimeNotIdleTarget() Target {
	return Target{
		Code:     CodeFactoryRuntimeNotIdle,
		Severity: SeverityError,
		Message:  "Current factory runtime must be idle before activation.",
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "",
			Location: SubjectLocationDefinition,
		},
	}
}

// FactorySessionFieldTarget reports factory-session open validation failures.
func FactorySessionFieldTarget(reason, field, message string) Target {
	code := CodeFactorySessionField + "." + reason
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       field,
			Location: SubjectLocationReference,
		},
	}
}

// FactorySessionTargetTarget reports discovery-time factory target failures.
func FactorySessionTargetTarget(reason, targetID, message string) Target {
	code := CodeFactorySessionTarget + "." + reason
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       targetID,
			Location: SubjectLocationReference,
		},
	}
}
