// Package factorydefinitions contains Wire-owned bindings for concrete Factory
// Definitions implementations. It is composition code, not a service API.
package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// LoadedFactorySourceFactory binds the effective-source implementation to the
// Factory Definitions root constructor contract through owner wire composition.
func LoadedFactorySourceFactory() contracts.LoadedFactorySourceFactory {
	return factorydefinitionswire.LoadedFactorySourceFactory()
}
