package internal

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
)

// NewPackagedFactoryInstaller constructs packaged Factory ensure/install
// operations from exact persistence and filesystem ports. The implementation
// lives in the Distribution private tree; Bootstrap and other peers consume
// only the returned PackagedFactoryInstaller root contract.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.PackagedFactoryPersistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	logger logging.Logger,
) factorydefinitions.PackagedFactoryInstaller {
	return NewPackagedFactoryInstallationService(persistence, fileSystem, directoryCreator, logger)
}

// NewPackagedFactoryInstallationService constructs the private packaged
// installation service for composition paths that need the concrete type.
func NewPackagedFactoryInstallationService(
	persistence factorydefinitions.PackagedFactoryPersistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
	directoryCreator factorydefinitions.PackagedInstallationDirectoryCreator,
	logger logging.Logger,
) *distributionpackagedinstallation.Service {
	return distributionpackagedinstallation.New(persistence, fileSystem, directoryCreator, logger)
}
