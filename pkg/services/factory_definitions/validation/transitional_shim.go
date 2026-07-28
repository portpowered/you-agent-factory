// Package validation exposes transitional compile-time re-exports of the
// parent-private validation implementation under internal/services/validation.
// Production ownership lives in internal/services/validation/impl.
package validation

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	validationimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
)

type (
	Service                 = validationimpl.Service
	ConfigValidator         = validationimpl.ConfigValidator
	Finding                 = validationimpl.Finding
	Profile                 = validationimpl.Profile
	RequiredToolCheckResult = validationimpl.RequiredToolCheckResult
	RequiredToolChecker     = validationimpl.RequiredToolChecker
	RequiredToolFailureKind = validationimpl.RequiredToolFailureKind
	TopologyError           = validationimpl.TopologyError
	ValidationResult        = validationimpl.ValidationResult
	WorkTypeHandlingBehaviorOptions = validationimpl.WorkTypeHandlingBehaviorOptions
	WorkflowSourceReader    = validationimpl.WorkflowSourceReader
	WorkstationLoader       = validationimpl.WorkstationLoader
)

const (
	ProfileTopology   = validationimpl.ProfileTopology
	ProfilePrePersist = validationimpl.ProfilePrePersist

	RequiredToolFailureKindNone         = validationimpl.RequiredToolFailureKindNone
	RequiredToolFailureKindMissing      = validationimpl.RequiredToolFailureKindMissing
	RequiredToolFailureKindVersionProbe = validationimpl.RequiredToolFailureKindVersionProbe
)

// New constructs Factory Definition validation with the runtime-owned
// orchestrator semantic validator supplied explicitly.
func New(
	orchestrators factorydefinitions.OrchestratorDefinitionValidator,
	loadCanonical ...factorydefinitions.CanonicalFactoryJSONLoader,
) *Service {
	return validationimpl.New(orchestrators, loadCanonical...)
}

func NewConfigValidator(requiredToolChecker RequiredToolChecker) *ConfigValidator {
	return validationimpl.NewConfigValidator(requiredToolChecker)
}

func NewTopologyError(message string, targets []Target) *TopologyError {
	return validationimpl.NewTopologyError(message, targets)
}

var (
	BundledFileFindings              = validationimpl.BundledFileFindings
	CanonicalTargetSignatures        = validationimpl.CanonicalTargetSignatures
	EquivalentCanonicalTargetSignatures = validationimpl.EquivalentCanonicalTargetSignatures
	FactoryDefinitionFindings        = validationimpl.FactoryDefinitionFindings
	FactoryRuntimeNotIdleTarget      = validationimpl.FactoryRuntimeNotIdleTarget
	FactorySessionFieldTarget        = validationimpl.FactorySessionFieldTarget
	FactorySessionTargetTarget       = validationimpl.FactorySessionTargetTarget
	FormFactoryPayloadTarget         = validationimpl.FormFactoryPayloadTarget
	InvalidFactoryNameTarget         = validationimpl.InvalidFactoryNameTarget
	InvocationReturnTargets          = validationimpl.InvocationReturnTargets
	InvocationSignatureTargets       = validationimpl.InvocationSignatureTargets
	IsLayoutTargetCode               = validationimpl.IsLayoutTargetCode
	IsPetriOrchestratorValidationScope = validationimpl.IsPetriOrchestratorValidationScope
	LayoutSaveOutcomes               = validationimpl.LayoutSaveOutcomes
	ManagedRuntimeDependencyTargets  = validationimpl.ManagedRuntimeDependencyTargets
	MergeLayoutSaveOutcomes          = validationimpl.MergeLayoutSaveOutcomes
	OrchestratorTargets              = validationimpl.OrchestratorTargets
	PollerRunWorkstationKindTargets  = validationimpl.PollerRunWorkstationKindTargets
	PruneLayout                      = validationimpl.PruneLayout
	StaleFactoryVersionTarget        = validationimpl.StaleFactoryVersionTarget
	SubmittedTaxonomyCompatibilityTargets = validationimpl.SubmittedTaxonomyCompatibilityTargets
	Validate                         = validationimpl.Validate
	ValidateBlockingLoad             = validationimpl.ValidateBlockingLoad
	ValidateDeclarativeRequiredTools = validationimpl.ValidateDeclarativeRequiredTools
	ValidateGraphTopology            = validationimpl.ValidateGraphTopology
	ValidateLayout                   = validationimpl.ValidateLayout
	ValidateOrchestratorTargets      = validationimpl.ValidateOrchestratorTargets
	ValidatePortableBundledFilesForExpandOnPath = validationimpl.ValidatePortableBundledFilesForExpandOnPath
	ValidatePortableBundledFilesForExpandOnPathWithSourceResolver = validationimpl.ValidatePortableBundledFilesForExpandOnPathWithSourceResolver
	ValidatePortableResourceManifestOnPath = validationimpl.ValidatePortableResourceManifestOnPath
	ValidatePortableResourceManifestOnPathWithSourceResolver = validationimpl.ValidatePortableResourceManifestOnPathWithSourceResolver
	ValidateRequiredTools            = validationimpl.ValidateRequiredTools
	ValidateStructural               = validationimpl.ValidateStructural
	WorkPropagationTargets           = validationimpl.WorkPropagationTargets
	WorkTypeHandlingBehaviorTargets  = validationimpl.WorkTypeHandlingBehaviorTargets
	WorkerWorkstationBehaviorCompatibilityTargets = validationimpl.WorkerWorkstationBehaviorCompatibilityTargets
)
