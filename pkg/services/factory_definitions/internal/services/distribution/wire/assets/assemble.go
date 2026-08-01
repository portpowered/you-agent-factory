package assets

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionpackageassets "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/packageassets"
)

// AssemblePackagedFactoryAssets is the distribution-owned package asset
// operation used by both catalog generation and application composition.
func AssemblePackagedFactoryAssets(
	definition factorydefinitions.PackagedFactoryAssetDefinition,
) ([]byte, error) {
	return distributionpackageassets.Assemble(definition)
}
