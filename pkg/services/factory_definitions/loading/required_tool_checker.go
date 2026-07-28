// Package loading provides the transitional Factory Definitions loading surface
// while effective-source construction is owned by the parent-private compilation
// subservice.
package loading

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
)

// NewPathRequiredToolChecker constructs the Factory Definitions external-tool
// availability adapter used by the application composition root.
func NewPathRequiredToolChecker(
	lookPath factorydefinitions.RequiredToolPathLookup,
	versionProbe factorydefinitions.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return internalloading.NewPathRequiredToolChecker(lookPath, versionProbe)
}
