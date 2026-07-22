package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// OrchestratorDefinitionValidator validates runtime-owned orchestrator
// semantics while returning Factory Definition-owned targets.
//
// Factory Definitions declares this port so its validation implementation
// does not depend on Factory Runtime internals. Factory Runtime supplies the
// implementation at composition time.
type OrchestratorDefinitionValidator = contracts.OrchestratorDefinitionValidator

// Validator is the public Factory Definition validation boundary. Parameters
// remain flat so composition can inject collaborators directly.
type Validator = contracts.Validator

type DefinitionValidationRequest = contracts.DefinitionValidationRequest
type DefinitionValidationOperation = contracts.DefinitionValidationOperation
type EditableFactoryValidationRequestMapper = contracts.EditableFactoryValidationRequestMapper
type SubmittedDefinitionValidationRequest = contracts.SubmittedDefinitionValidationRequest
type SubmittedDefinitionValidationOperation = contracts.SubmittedDefinitionValidationOperation
type EffectiveDefinitionValidationRequest = contracts.EffectiveDefinitionValidationRequest
type EffectiveDefinitionValidationOperation = contracts.EffectiveDefinitionValidationOperation
type SubmittedDefinitionTaxonomy = contracts.SubmittedDefinitionTaxonomy
type SubmittedWorkerTaxonomy = contracts.SubmittedWorkerTaxonomy
type SubmittedWorkstationTaxonomy = contracts.SubmittedWorkstationTaxonomy

// RequiredToolCheckResult captures the availability result for one declarative
// external tool dependency.
type RequiredToolCheckResult = contracts.RequiredToolCheckResult

// RequiredToolFailureKind classifies the source of a required-tool failure.
type RequiredToolFailureKind = contracts.RequiredToolFailureKind

const (
	RequiredToolFailureKindNone         = contracts.RequiredToolFailureKindNone
	RequiredToolFailureKindMissing      = contracts.RequiredToolFailureKindMissing
	RequiredToolFailureKindVersionProbe = contracts.RequiredToolFailureKindVersionProbe
)

// RequiredToolChecker is the Factory Definitions-owned external process port
// used to verify declarative tool dependencies.
type RequiredToolChecker = contracts.RequiredToolChecker
