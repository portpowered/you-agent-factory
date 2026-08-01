// Package wire constructs the Factory Definitions distribution subservice from
// exact injected distribute ports.
package wire

import (
	"fmt"
	factoryeffects "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/effects"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionpackagedcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/packagedcatalog"
	distributionpackagedinstallation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/packagedinstallation"
	distributionscaffoldfacts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/scaffoldfacts"
	distributionserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

func LocalFactoryNameResolver() func(string) (string, error) {
	return distributionscaffoldfacts.LocalFactoryNameResolver()
}

// NewPackagedFactoryCatalog constructs the distribution-owned packaged catalog
// behind root operation contracts.
func NewPackagedFactoryCatalog(
	definitions []factorydefinitions.PackagedDefinition,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	return distributionpackagedcatalog.New(definitions)
}

// NewPackagedFactoryInstaller constructs the distribution-owned installer
// behind the private ensure/install port.
func NewPackagedFactoryInstaller(
	persistence factorydefinitions.Persistence,
	fileSystem factoryeffects.PackagedInstallationFileSystem,
) factoryeffects.PackagedFactoryInstaller {
	return distributionpackagedinstallation.New(persistence, fileSystem)
}

// NewPackagedFactoryInstallationOperations adapts the distribution-owned
// installer to the root request operation used by application composition.
func NewPackagedFactoryInstallationOperations(
	persistence factorydefinitions.Persistence,
	fileSystem factoryeffects.PackagedInstallationFileSystem,
) factorydefinitions.PackagedFactoryInstallationOperations {
	installer := distributionpackagedinstallation.New(persistence, fileSystem)
	return factorydefinitions.PackagedFactoryInstallationOperations{
		Install: installer.InstallPackagedFactory,
	}
}

// NewService constructs the private distribution subservice from exact injected
// packaged-catalog, packaged-installation, and scaffold ports. Callers must
// supply Dependencies; this constructor does not select host filesystem/SQL/OS
// adapters or take Wire/root construction ownership.
func NewService(deps distributionservice.Dependencies) (distributionservice.Service, error) {
	if deps.PackagedCatalog.List == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory catalog list operation is required")
	}
	if deps.PackagedCatalog.Resolve == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory catalog resolve operation is required")
	}
	if deps.PackagedInstaller.Install == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory installer is required")
	}
	service := distributionserviceimpl.New(
		deps.PackagedCatalog,
		deps.PackagedInstaller,
		deps.ScaffoldInitializer,
		deps.ScaffoldFactoryNameResolver,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: implementation rejected its dependencies")
	}
	return service, nil
}
