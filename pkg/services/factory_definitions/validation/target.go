package validation

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

type (
	Severity        = factorydefinitions.ValidationSeverity
	SubjectType     = factorydefinitions.ValidationSubjectType
	SubjectLocation = factorydefinitions.ValidationSubjectLocation
	Subject         = factorydefinitions.ValidationSubject
	Target          = factorydefinitions.ValidationTarget
	Result          = factorydefinitions.ValidationResult
)

const (
	SeverityError   = factorydefinitions.ValidationSeverityError
	SeverityWarning = factorydefinitions.ValidationSeverityWarning
	SeverityHint    = factorydefinitions.ValidationSeverityHint

	SubjectTypeFactory     = factorydefinitions.ValidationSubjectTypeFactory
	SubjectTypeWorkstation = factorydefinitions.ValidationSubjectTypeWorkstation
	SubjectTypeWorkType    = factorydefinitions.ValidationSubjectTypeWorkType
	SubjectTypeWorkState   = factorydefinitions.ValidationSubjectTypeWorkState
	SubjectTypeWorker      = factorydefinitions.ValidationSubjectTypeWorker
	SubjectTypeResource    = factorydefinitions.ValidationSubjectTypeResource
	SubjectTypeRoute       = factorydefinitions.ValidationSubjectTypeRoute

	SubjectLocationOnRejection = factorydefinitions.ValidationSubjectLocationOnRejection
	SubjectLocationOnFailure   = factorydefinitions.ValidationSubjectLocationOnFailure
	SubjectLocationOutputs     = factorydefinitions.ValidationSubjectLocationOutputs
	SubjectLocationInputs      = factorydefinitions.ValidationSubjectLocationInputs
	SubjectLocationStates      = factorydefinitions.ValidationSubjectLocationStates
	SubjectLocationTerminal    = factorydefinitions.ValidationSubjectLocationTerminal
	SubjectLocationReference   = factorydefinitions.ValidationSubjectLocationReference
	SubjectLocationDefinition  = factorydefinitions.ValidationSubjectLocationDefinition
)

// DefaultTopologyValidationMessage is the operator-facing save rejection message
// when callers do not supply a custom topology validation message.
const DefaultTopologyValidationMessage = factorydefinitions.DefaultTopologyValidationMessage
