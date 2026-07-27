// Package wire constructs the parent-private Factory Runtime instance host.
package wire

import (
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/internal/service"
)

// New constructs the inert instance host composed by the Factory Runtime root.
func New(dependencies instancehost.Dependencies) (instancehost.Service, error) {
	return internalservice.New(dependencies)
}
