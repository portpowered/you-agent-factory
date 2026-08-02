// Package wire constructs the Factory Definitions distribution subservice from
// exact injected distribute ports.
package wire

import (
	"fmt"

	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/contracts"
	distributionserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

// NewService constructs the private distribution subservice from exact injected
// packaged-catalog, packaged-installation, and scaffold ports. Each
// collaborator is a direct argument; this constructor does not select host
// filesystem/SQL/OS adapters or take Wire/root construction ownership.
func NewService(
	packagedCatalog distributioncontracts.PackagedFactoryCatalogOperations,
	packagedInstaller distributioncontracts.PackagedFactoryInstallationOperations,
	scaffoldInitializer distributioncontracts.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributioncontracts.ScaffoldFactoryNameResolver,
) (distributionservice.Service, error) {
	if packagedCatalog.List == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory catalog list operation is required")
	}
	if packagedCatalog.Resolve == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory catalog resolve operation is required")
	}
	if packagedInstaller.Install == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory installer is required")
	}
	service := distributionserviceimpl.New(
		packagedCatalog,
		packagedInstaller,
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: implementation rejected its dependencies")
	}
	return service, nil
}
