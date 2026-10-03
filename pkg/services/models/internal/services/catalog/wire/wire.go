// Package wire constructs the private Models Catalog subservice.
package wire

import (
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert catalog over the supplied Runtime Scopes
// authority.
func NewService(
	scopes runtimescopes.Service,
	readiness catalog.ReadinessQuery,
) (catalog.Service, error) {
	return internalservice.New(scopes, readiness), nil
}
