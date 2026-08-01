package internal

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

// NewPackagedFactoryInstaller constructs packaged Factory ensure/install
// operations from exact persistence and filesystem ports. The implementation
// lives in the Distribution private tree; Bootstrap and other peers consume
// only the returned PackagedFactoryInstaller root contract.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.Persistence,
	fileSystem factoryeffects.PackagedInstallationFileSystem,
) factoryeffects.PackagedFactoryInstaller {
	return distributionwire.NewPackagedFactoryInstaller(persistence, fileSystem)
}

// NewPackagedFactoryInstallationService constructs the private packaged
// installation service for composition paths that need the concrete type.
func NewPackagedFactoryInstallationService(
	persistence factorydefinitions.Persistence,
	fileSystem factoryeffects.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstallationOperations {
	return distributionwire.NewPackagedFactoryInstallationOperations(persistence, fileSystem)
}
