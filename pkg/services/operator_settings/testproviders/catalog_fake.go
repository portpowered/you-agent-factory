// Package testproviders is a transitional shim over internal Settings test fakes.
// Implementation lives under operator_settings/internal/testproviders.
package testproviders

import (
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// CatalogFake is a behavioral double for providers.Service used by Settings tests.
type CatalogFake = internaltestproviders.CatalogFake

// NewCatalogFake constructs a catalog fake from the supplied descriptors.
var NewCatalogFake = internaltestproviders.NewCatalogFake

// StandardCatalog returns a root-typed Providers fake with selectable catalog entries.
func StandardCatalog() providers.Service {
	return internaltestproviders.StandardCatalog()
}
