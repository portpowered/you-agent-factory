package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedinstallation"
	distributionpackaging "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packaging"
)

// NewPackagedFactoryCatalog constructs deterministic packaged Factory catalog
// operations from validated embedded definitions.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return factorydefinitionsinternal.NewPackagedFactoryCatalog(definitions)
}

// NewPackagedFactoryCatalogService constructs the direct catalog capability
// consumed by the focused Packaging service.
func NewPackagedFactoryCatalogService(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalog, error) {
	return factorydefinitionsinternal.NewPackagedFactoryCatalogService(definitions)
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

// NewPackaging constructs the focused Packaging capability from the exact
// published definitions and already-selected persistence/filesystem ports.
// Construction is inert; package lookup, validation, and writes happen only
// when callers invoke the capability.
func NewPackaging(
	definitions []factorydefinitions.PackagedDefinition,
	persistence factorydefinitions.Persistence,
	fileSystem factorydefinitions.PackagedInstallationFileSystem,
) (factorydefinitions.Packaging, error) {
	catalog, err := NewPackagedFactoryCatalogService(definitions)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions packaging catalog: %w", err)
	}
	installation := NewPackagedFactoryInstallationService(persistence, fileSystem)
	capability, err := distributionpackaging.New(catalog, installation)
	if err != nil {
		return nil, err
	}
	return capability, nil
}
