package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	distributionpackagedcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedcatalog"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
)

// NewPackagedFactoryCatalog constructs deterministic packaged Factory catalog
// operations from validated embedded definitions.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionsinternal.NewPackagedFactoryCatalog(definitions)
}

// LoadPublishedPackagedFactoryDefinitions loads the generated package
// publication through the Factory Definitions-owned catalog/materialization
// boundary. Consumers must not import the publication package or its loader.
func LoadPublishedPackagedFactoryDefinitions() ([]factorydefinitions.PackagedDefinition, error) {
	catalog, err := distributionpackagedcatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return nil, err
	}
	return catalog.All(), nil
}

// NewPublishedPackagedFactoryCatalog constructs the Definitions-owned catalog
// operations over the exact generated publication embedded in the executable.
func NewPublishedPackagedFactoryCatalog() (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	definitions, err := LoadPublishedPackagedFactoryDefinitions()
	if err != nil {
		return factorydefinitions.PackagedFactoryCatalogOperations{}, err
	}
	return NewPackagedFactoryCatalog(definitions)
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
