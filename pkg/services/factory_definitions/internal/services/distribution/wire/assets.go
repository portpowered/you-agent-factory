package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire/assets"
)

// AssemblePackagedFactoryAssets exposes distribution-owned package asset
// assembly through the distribution owner boundary.
func AssemblePackagedFactoryAssets(
	definition factorydefinitions.PackagedFactoryAssetDefinition,
) ([]byte, error) {
	return distributionassets.AssemblePackagedFactoryAssets(definition)
}
