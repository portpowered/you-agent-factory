// Package wire constructs the Factory Definitions catalog subservice from
// exact injected effect ports.
package wire

import (
	"fmt"

	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/service"
)

// NewService constructs the private catalog subservice from exact injected
// path and catalog-filesystem ports. Callers must supply Dependencies; this
// constructor does not select host filesystem/SQL/OS adapters or take
// Wire/root construction ownership.
func NewService(deps catalog.Dependencies) (catalog.Service, error) {
	if deps.Paths == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory path resolver is required")
	}
	if deps.FileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory catalog filesystem is required")
	}
	service := catalogservice.New(deps.Paths, deps.FileSystem)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: implementation rejected its dependencies")
	}
	return service, nil
}
