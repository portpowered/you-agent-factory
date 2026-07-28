// Package loadedsource provides the transitional effective loaded-source surface
// while construction is owned by the parent-private compilation subservice.
package loadedsource

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalloadedsource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource"
)

// Source is the effective Factory Definition retained by a live runtime.
type Source = internalloadedsource.Source

// New constructs an effective loaded source from an authored Factory
// Definition and optional runtime definitions.
func New(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
	portableBundledReplacements []factorydefinitions.PortableBundledFileReplacement,
) (*Source, error) {
	return internalloadedsource.New(
		factoryDir,
		factoryConfig,
		runtimeDefinitions,
		portableBundledReplacements,
	)
}
