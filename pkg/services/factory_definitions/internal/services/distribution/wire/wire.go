// Package wire constructs the Factory Definitions distribution subservice from
// exact injected collaborators.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

// NewService constructs the private distribution capability.
func NewService(
	catalog []factorydefinitions.PackagedDefinition,
	installer factorydefinitions.PackagedFactoryInstaller,
	scaffold factorydefinitions.ScaffoldInitializer,
) (distribution.Service, error) {
	if installer == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: packaged Factory installer is required")
	}
	if scaffold == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: Factory scaffold initializer is required")
	}
	service := internalservice.New(catalog, installer, scaffold)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions distribution: implementation rejected its dependencies")
	}
	return service, nil
}
