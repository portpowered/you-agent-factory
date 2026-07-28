package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
)

// NewPackagedFactoryCatalog constructs deterministic packaged Factory catalog
// operations from validated embedded definitions.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionsinternal.NewPackagedFactoryCatalog(definitions)
}

// NewPackagedFactoryInstaller constructs packaged Factory ensure/install
// operations from exact persistence and filesystem ports.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstaller {
	return factorydefinitionsinternal.NewPackagedFactoryInstaller(persistence, fileSystem)
}

// NewPackagedFactoryInstallationService constructs the private packaged
// installation service for composition paths that need the concrete type.
func NewPackagedFactoryInstallationService(
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) *distributionpackagedinstallation.Service {
	return factorydefinitionsinternal.NewPackagedFactoryInstallationService(persistence, fileSystem)
}
