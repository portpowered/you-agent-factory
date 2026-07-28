package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

// EffectiveCatalogService is the read-only Factory Definitions owner used by
// transports that do not require a Factory Session.
type EffectiveCatalogService = factorydefinitionsinternal.EffectiveCatalogService

// NewEffectiveCatalog constructs the stateless effective Factory catalog.
func NewEffectiveCatalog(
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
	normalize factorydefinitions.EffectiveFactoryDefinitionNormalizer,
) (factorydefinitions.EffectiveFactoryCatalogOperation, error) {
	return factorydefinitionsinternal.NewEffectiveCatalog(discovery, normalize)
}

// NewEffectiveCatalogService constructs the read-only Factory Definitions
// service slice used by transports that do not require a Factory Session.
func NewEffectiveCatalogService(
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
) (*EffectiveCatalogService, error) {
	return factorydefinitionsinternal.NewEffectiveCatalogService(listEffective)
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
