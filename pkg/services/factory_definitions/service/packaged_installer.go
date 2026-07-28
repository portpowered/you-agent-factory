package service

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
)

// NewPackagedFactoryInstaller constructs packaged Factory installation operations
// from exact persistence and filesystem ports. The implementation lives in the
// Distribution private tree; this constructor remains a transitional shim.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstallationOperations {
	installer := NewPackagedFactoryInstallationService(persistence, fileSystem)
	return factorydefinitions.PackagedFactoryInstallationOperations{
		Install: installer.InstallPackagedFactory,
	}
}

// NewPackagedFactoryInstallationService constructs the private packaged
// installation service for composition paths that need the concrete type.
func NewPackagedFactoryInstallationService(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *distributionpackagedinstallation.Service {
	return distributionpackagedinstallation.New(persistence, fileSystem)
}
