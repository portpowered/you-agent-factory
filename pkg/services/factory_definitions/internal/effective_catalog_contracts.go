package internal

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// EffectiveCatalogDiscovery is a private source bundle used only inside the
// Definitions catalog implementation. The root Service exposes the resulting
// detached entries, never these source callbacks.
type EffectiveCatalogDiscovery struct {
	ListRoot     func(context.Context, string) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error)
	ListPackaged func(context.Context) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error)
}

// EffectiveDefinitionNormalizer is the private effective-source decoder used
// by the catalog owner.
type EffectiveDefinitionNormalizer func(
	context.Context,
	factorydefinitions.EffectiveFactoryCatalogCandidate,
) (*factorydefinitions.FactoryConfig, error)

// EffectiveCatalogOperation is private to the Definitions implementation and
// is used by the eventual root service fold.
type EffectiveCatalogOperation func(
	context.Context,
	factorydefinitions.ListEffectiveFactoriesRequest,
) (factorydefinitions.ListEffectiveFactoriesResult, error)

// EffectiveCatalogRootListing and EffectiveCatalogCandidateRead are private
// source adapters for disk-backed effective discovery.
type EffectiveCatalogRootListing func(string) ([]factorydefinitions.NamedFactoryListEntry, error)
type EffectiveCatalogCandidateRead func(string) ([]byte, error)
