// Package wire constructs the parent-private Factory Runtime dispatch planner.
package wire

import (
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/internal/service"
)

// New constructs a dispatch-planning capability over the Workers publication
// edge supplied by its Factory Runtime parent.
func New(
	publisher dispatchplanning.WorkersPublisher,
	canceler dispatchplanning.WorkersCanceler,
) dispatchplanning.Service {
	return internalservice.NewWithCancellation(publisher, canceler)
}
