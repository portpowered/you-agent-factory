// Package wire constructs the private Models Runtime Scopes subservice.
package wire

import (
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/internal/service"
)

// NewService constructs an inert Runtime Scopes registry.
func NewService() runtimescopes.Service {
	return internalservice.New()
}
