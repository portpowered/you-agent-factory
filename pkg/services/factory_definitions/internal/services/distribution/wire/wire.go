// Package wire constructs the Factory Definitions distribution subservice from
// exact injected distribute ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

// NewService constructs the private distribution owner from direct ports.
// Construction does not select host adapters or execute distribution operations.
func NewService(
	packagedCatalog factorydefinitions.PackagedFactoryCatalogOperations,
	packagedInstaller factorydefinitions.PackagedFactoryInstallationOperations,
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
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
