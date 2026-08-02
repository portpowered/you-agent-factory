// Package contracts contains the private snapshot and portability ports.
// These capabilities are construction details of Factory Definitions and do
// not cross the public unary root.
package contracts

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

type WorkstationLoader = factorycontracts.WorkstationLoader
type FactoryConfigJSONDecoder = factorycontracts.FactoryConfigJSONDecoder
type FactoryConfigCloner = factorycontracts.FactoryConfigCloner
type PortableBundledFilesApplier = factorycontracts.PortableBundledFilesApplier
type FactoryStarterWorkApplier = factorycontracts.FactoryStarterWorkApplier
type PortableFactoryConfigPreparer = factorycontracts.PortableFactoryConfigPreparer
type PortableBundledFilesMaterializer = factorycontracts.PortableBundledFilesMaterializer
type PortableBundledFileWritesValidator = factorycontracts.PortableBundledFileWritesValidator
type PortableBundledFilesCopier = factorycontracts.PortableBundledFilesCopier
type PortableBundledDocsPruner = factorycontracts.PortableBundledDocsPruner
type FactorySnapshotCapturer = factorycontracts.FactorySnapshotCapturer
type LoadedFactorySnapshotCapturer = factorycontracts.LoadedFactorySnapshotCapturer
type FactorySnapshotObjectMapper = factorycontracts.FactorySnapshotObjectMapper
type FactorySnapshotSource = factorycontracts.FactorySnapshotSource
type LoadedFactorySource = factorycontracts.LoadedFactorySource
type RuntimeDefinitionLookup = factorycontracts.RuntimeDefinitionLookup
type EditableFactoryValidationRequestMapper = factorycontracts.EditableFactoryValidationRequestMapper

// CanonicalFactoryLoader is the private canonical Factory loader needed by
// snapshot capture.
type CanonicalFactoryLoader = factorycontracts.CanonicalFactoryJSONLoader

// FactorySnapshotJSONDecoder decodes the detached snapshot representation.
type FactorySnapshotJSONDecoder func([]byte) (*factorydefinitions.FactorySnapshot, error)

// DefinitionValidationOperation is the private validation operation consumed
// by editable snapshot validation.
type DefinitionValidationOperation interface {
	ValidateDefinition(
		context.Context,
		factorydefinitions.DefinitionValidationRequest,
	) (factorydefinitions.ValidationResult, error)
}

// ReplayRuntimeConfig is the private replay lookup reconstructed from a
// snapshot. Replay consumers own their public presentation contracts.
type ReplayRuntimeConfig interface {
	FactoryConfig() *factorydefinitions.FactoryConfig
	FactoryDir() string
	RuntimeBaseDir() string
	Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool)
	Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
	WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
}
