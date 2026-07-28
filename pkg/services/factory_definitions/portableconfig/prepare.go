// Package portableconfig prepares Factory Definitions for detached persistence
// and replay boundaries.
package portableconfig

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
)

// NewPreparer binds the authored-file operations selected by Wire to the
// Factory Definitions portable-preparation contract.
func NewPreparer(
	cloneFactoryConfig factorydefinitions.FactoryConfigCloner,
	applyBundledFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
) factorydefinitions.PortableFactoryConfigPreparer {
	return snapshotsportabilityprepare.NewPreparer(
		cloneFactoryConfig,
		applyBundledFiles,
		applyStarterWork,
	)
}

// Prepare clones one authored Factory and projects its supported portable
// files and shared starter Work.
func Prepare(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
	cloneFactoryConfig factorydefinitions.FactoryConfigCloner,
	applyBundledFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
) (*factorydefinitions.FactoryConfig, error) {
	return snapshotsportabilityprepare.PrepareConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		cloneFactoryConfig,
		applyBundledFiles,
		applyStarterWork,
	)
}
