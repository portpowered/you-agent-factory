// Package wire constructs the Automations reconciliation subservice.
package wire

import (
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/internal/service"
)

// NewService constructs an inert reconciler with its required lifecycle owner.
// Construction never invokes lifecycle operations.
func NewService(lifecycle reconciliation.SourceLifecycle) reconciliation.Service {
	return reconciliationservice.New(lifecycle)
}
