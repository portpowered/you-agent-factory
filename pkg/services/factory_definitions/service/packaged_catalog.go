package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionpackagedcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedcatalog"
)

// NewPackagedFactoryCatalog constructs deterministic packaged Factory catalog
// operations from validated embedded definitions. The implementation lives in the
// Distribution private tree; this constructor remains a transitional shim.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return distributionpackagedcatalog.New(definitions)
}
