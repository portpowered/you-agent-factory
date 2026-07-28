package portableconfig

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// NewPortableBundledFilesApplier binds the filesystem selected by Wire to
// portable authored-file discovery.
func NewPortableBundledFilesApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledFilesApplier, error) {
	if fileSystem == nil {
		return nil, errors.New("portable filesystem is required")
	}
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
		includeInlineContent bool,
		discoverUnlistedDocs bool,
	) error {
		return applySupportedFiles(
			factoryDir,
			factoryConfig,
			includeInlineContent,
			discoverUnlistedDocs,
			fileSystem,
		)
	}, nil
}

// NewFactoryStarterWorkApplier binds the filesystem traversal selected by
// Wire to starter-Work discovery.
func NewFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.FactoryStarterWorkApplier, error) {
	if fileSystem == nil {
		return nil, errors.New("portable filesystem is required")
	}
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
	) error {
		return applyStarterWork(factoryDir, factoryConfig, fileSystem)
	}, nil
}

// NewPortableBundledDocsPruner binds the filesystem traversal selected by
// Wire to obsolete authored-document cleanup.
func NewPortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledDocsPruner, error) {
	if fileSystem == nil {
		return nil, errors.New("portable filesystem is required")
	}
	return func(
		factoryDir string,
		factoryConfig *factorydefinitions.FactoryConfig,
	) error {
		return PruneRemovedDocs(fileSystem, factoryDir, factoryConfig)
	}, nil
}
