// Package wire constructs the parent-private Factory Runtime dispatch planner.
package wire

import (
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/internal/service"
)

// New constructs an inert dispatch-planning capability.
func New() dispatchplanning.Service {
	return internalservice.New()
}
