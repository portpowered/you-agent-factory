// Package factorydefinitions contains Wire-owned bindings for concrete Factory
// Definitions implementations. It is composition code, not a service API.
package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloadedsource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loadedsource"
)

// LoadedFactorySourceFactory binds the effective-source implementation to the
// Factory Definitions root constructor contract.
func LoadedFactorySourceFactory() contracts.LoadedFactorySourceFactory {
	return func(
		factoryDir string,
		factoryConfig *contracts.FactoryConfig,
		runtimeDefinitions contracts.RuntimeDefinitionLookup,
		replacements []contracts.PortableBundledFileReplacement,
	) (contracts.MutableLoadedFactorySource, error) {
		return factoryloadedsource.New(
			factoryDir,
			factoryConfig,
			runtimeDefinitions,
			replacements,
		)
	}
}
