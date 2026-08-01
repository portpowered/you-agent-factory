package assets

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire/assets"
)

// AssemblePackagedFactoryAssets exposes the distribution-owned asset assembly
// operation to repository tooling without importing the root application wire.
func AssemblePackagedFactoryAssets(
	definition factorydefinitions.PackagedFactoryAssetDefinition,
) ([]byte, error) {
	return distributionassets.AssemblePackagedFactoryAssets(definition)
}
