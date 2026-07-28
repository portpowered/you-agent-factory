package factorydefinitions

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/loading"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// FactoryLayoutFlattener binds the concrete Factory Definitions loader selected
// by Wire to the root layout contract.
func FactoryLayoutFlattener(
	loader *factoryloading.Loader,
) contracts.FactoryLayoutFlattener {
	return loader.FlattenFactoryConfig
}

// PortableFactoryConfigPreparer binds the canonical Factory Definition
// authored-file operations to portable preparation.
func PortableFactoryConfigPreparer(
	applySupportedFiles contracts.PortableBundledFilesApplier,
	applyStarterWork contracts.FactoryStarterWorkApplier,
) contracts.PortableFactoryConfigPreparer {
	return factorydefinitionswire.PortableFactoryConfigPreparer(
		applySupportedFiles,
		applyStarterWork,
	)
}
