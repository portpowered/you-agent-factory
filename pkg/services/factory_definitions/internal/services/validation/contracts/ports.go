// Package contracts contains the private validation ports. These capabilities
// are construction details of Factory Definitions and never cross the public
// unary root.
package contracts

import (
	"context"
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

// CanonicalFactoryLoader loads one canonical Factory payload for validation.
type CanonicalFactoryLoader = factorycontracts.CanonicalFactoryJSONLoader

// WorkstationLoader resolves authored workstation configuration during a
// canonical load.
type WorkstationLoader = factorycontracts.WorkstationLoader

// WorkflowSourceReader resolves file-backed JavaScript workflow source.
type WorkflowSourceReader = factorycontracts.WorkflowSourceReader

// DefinitionValidationOperation is the complete pre-persist validation
// operation supplied by Factory Definitions composition.
type DefinitionValidationOperation interface {
	ValidateDefinition(
		context.Context,
		factorydefinitions.DefinitionValidationRequest,
	) (factorydefinitions.ValidationResult, error)
}

// EffectiveDefinitionValidationOperation validates an already-loaded
// effective Factory Definition with the fixed invocation profile.
type EffectiveDefinitionValidationOperation interface {
	ValidateEffectiveDefinition(
		context.Context,
		factorydefinitions.EffectiveDefinitionValidationRequest,
	) (factorydefinitions.ValidationResult, error)
}

// RequiredToolChecker verifies declarative external-tool requirements.
type RequiredToolChecker = factorycontracts.RequiredToolChecker

// OrchestratorDefinitionValidator is the Runtime-owned semantic validation
// port supplied to Factory Definitions at composition time.
type OrchestratorDefinitionValidator = factorycontracts.OrchestratorDefinitionValidator

// PortableBundledFileSourceResolver resolves an authored bundled-file source.
type PortableBundledFileSourceResolver = factorycontracts.PortableBundledFileSourceResolver

// PortableBundledFileInspection is the exact filesystem capability needed by
// portable bundled-file validation.
type PortableBundledFileInspection interface {
	Stat(string) (fs.FileInfo, error)
}
