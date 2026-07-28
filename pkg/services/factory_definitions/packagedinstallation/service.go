// Package packagedinstallation is a transitional shim over the Distribution-owned
// packaged Factory installation implementation.
package packagedinstallation

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
)

// Service installs packaged Factory definitions into a named-Factory root.
type Service = distributionpackagedinstallation.Service

// New constructs packaged Factory installation from exact persistence and
// filesystem ports. The implementation lives in the Distribution private tree.
func New(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *Service {
	return distributionpackagedinstallation.New(persistence, fileSystem)
}
