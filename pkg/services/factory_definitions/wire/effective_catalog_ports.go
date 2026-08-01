package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

// NewEffectiveCatalog constructs the stateless effective Factory catalog.
func NewEffectiveCatalog(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	return factorydefinitionsinternal.NewEffectiveCatalog(discovery, normalize)
}

// NewEffectiveCatalogDiscovery constructs read-only disk and published-package
// discovery.
func NewEffectiveCatalogDiscovery(
	listRoot factorydefinitions.EffectiveFactoryRootListing,
	read factorydefinitions.EffectiveFactoryCandidateRead,
	packaged []factorydefinitions.PackagedDefinition,
) (factorydefinitions.EffectiveFactoryCatalogDiscovery, error) {
	return factorydefinitionsinternal.NewEffectiveCatalogDiscovery(listRoot, read, packaged)
}
