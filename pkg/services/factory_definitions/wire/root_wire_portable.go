package wire

import (
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
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
	return snapshotsportabilityprepare.NewPreparer(
		contracts.CloneFactoryConfig,
		applySupportedFiles,
		applyStarterWork,
	)
}
