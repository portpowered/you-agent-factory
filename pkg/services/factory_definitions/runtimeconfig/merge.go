// Package runtimeconfig provides the transitional runtime-config merge surface
// while merge behavior is owned by the parent-private compilation subservice.
package runtimeconfig

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalruntimeconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimeconfig"
)

// Merge returns a detached Factory Definition with runtime Worker and
// Workstation definitions applied to the authored topology.
func Merge(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) (*factorydefinitions.FactoryConfig, error) {
	return internalruntimeconfig.Merge(factoryConfig, runtimeDefinitions)
}

// NormalizeCanonicalWorkstationRuntime applies the shared authored/runtime
// normalization used when a Workstation definition enters effective runtime
// state or is written back to an authored layout.
func NormalizeCanonicalWorkstationRuntime(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	internalruntimeconfig.NormalizeCanonicalWorkstationRuntime(workstation)
}
