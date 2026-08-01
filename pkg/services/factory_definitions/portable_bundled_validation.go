package factorydefinitions

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

// PortableBundledFileValidationKind identifies the Factory Definition field or
// policy rejected by portable bundled-file validation.
type PortableBundledFileValidationKind = factorycontracts.PortableBundledFileValidationKind

const (
	PortableBundledFileValidationType             = factorycontracts.PortableBundledFileValidationType
	PortableBundledFileValidationTargetPath       = factorycontracts.PortableBundledFileValidationTargetPath
	PortableBundledFileValidationTargetRoot       = factorycontracts.PortableBundledFileValidationTargetRoot
	PortableBundledFileValidationTargetRootHelper = factorycontracts.PortableBundledFileValidationTargetRootHelper
)

// PortableBundledFileValidationError preserves structured validation meaning
// while allowing transports and authored-format adapters to choose their own
// finding representation.
type PortableBundledFileValidationError = factorycontracts.PortableBundledFileValidationError

var (
	ValidatePortableBundledFileType          = factorycontracts.ValidatePortableBundledFileType
	ValidatePortableBundledFileTarget        = factorycontracts.ValidatePortableBundledFileTarget
	ShouldOmitSupportedPortableBundledInline = factorycontracts.ShouldOmitSupportedPortableBundledInline
)
