package factorydefinitions

import (
	"io/fs"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

// PortableBundledFileInspection is the exact filesystem effect used to verify
// that a resolved portable bundled-file source names a regular filesystem
// entry. Source resolution policy remains a separate Factory Definitions role.
type PortableBundledFileInspection interface {
	Stat(string) (fs.FileInfo, error)
}

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
