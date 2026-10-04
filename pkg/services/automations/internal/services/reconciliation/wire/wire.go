// Package wire constructs the Automations reconciliation subservice.
package wire

import (
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/internal/service"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
)

// NewService constructs an inert reconciler with its required lifecycle owner.
// Construction never invokes lifecycle operations.
func NewService(lifecycle sourcelifecycle.SourceLifecycle) reconciliation.Service {
	return reconciliationservice.New(lifecycle)
}
