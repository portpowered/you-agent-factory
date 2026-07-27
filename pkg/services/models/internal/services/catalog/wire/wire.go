// Package wire constructs the private Models Catalog subservice.
package wire

import (
	"fmt"

	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert catalog over the supplied Runtime Scopes
// authority.
func NewService(scopes runtimescopes.Service) (catalog.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf("Models Catalog runtime scopes service is required")
	}
	return internalservice.New(scopes), nil
}
