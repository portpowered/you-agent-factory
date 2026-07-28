// Package wire constructs the Factory Definitions distribution subservice from
// exact injected distribute ports.
package wire

import (
	"fmt"

	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

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
	if deps.ScaffoldInitializer == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: scaffold initializer is required")
	}
	if deps.ScaffoldFactoryNameResolver == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: scaffold factory name resolver is required")
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
