// Package wire constructs the Factory Definitions catalog subservice from
// exact injected effect ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/internal/service"
)

// NewService constructs the private catalog subservice.
func NewService(
	paths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (catalog.Service, error) {
	if paths == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory path resolver is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: named Factory catalog filesystem is required")
	}
	service := catalogservice.New(paths, fileSystem)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions catalog: implementation rejected its dependencies")
	}
	return service, nil
}
