// Package wire constructs the Factory Definitions distribution subservice from
// exact injected collaborators.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

// NewService constructs the private distribution capability from a built-in
// catalog snapshot plus exact injected install/scaffold ports. Callers must
// supply Dependencies; this constructor does not select host filesystem
// adapters or take Wire/root construction ownership.
func NewService(
	catalog []factorydefinitions.PackagedDefinition,
	deps distribution.Dependencies,
) (distribution.Service, error) {
	if deps.Installer == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory installer is required")
	}
	if deps.Scaffold == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: Factory scaffold initializer is required")
	}
	service := internalservice.New(catalog, deps.Installer, deps.Scaffold)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: implementation rejected its dependencies")
	}
	return service, nil
}
