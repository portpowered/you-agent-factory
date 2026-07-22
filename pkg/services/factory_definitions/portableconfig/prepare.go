// Package portableconfig prepares Factory Definitions for detached persistence
// and replay boundaries.
package portableconfig

import (
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// NewPreparer binds the authored-file operations selected by Wire to the
// Factory Definitions portable-preparation contract.
func NewPreparer(
	cloneFactoryConfig factorydefinitions.FactoryConfigCloner,
	applyBundledFiles factorydefinitions.PortableBundledFilesApplier,
	applyStarterWork factorydefinitions.FactoryStarterWorkApplier,
) factorydefinitions.PortableFactoryConfigPreparer {
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		includeInlineContent bool,
	) (*factorydefinitions.FactoryConfig, error) {
		return Prepare(
			factoryDir,
			factoryConfig,
			includeInlineContent,
			cloneFactoryConfig,
			applyBundledFiles,
			applyStarterWork,
		)
	}
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
	if cloneFactoryConfig == nil {
		return nil, errors.New("Factory Definition cloner is required")
	}
	if applyBundledFiles == nil {
		return nil, errors.New("portable bundled-files applier is required")
	}
	if applyStarterWork == nil {
		return nil, errors.New("Factory starter-Work applier is required")
	}
	cloned, err := cloneFactoryConfig(factoryConfig)
	if err != nil {
		return nil, err
	}
	if err := applyBundledFiles(
		factoryDir,
		cloned,
		includeInlineContent,
		false,
	); err != nil {
		return nil, err
	}
	if err := applyStarterWork(factoryDir, cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
