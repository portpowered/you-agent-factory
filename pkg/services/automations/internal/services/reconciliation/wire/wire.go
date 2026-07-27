// Package wire constructs the Automations reconciliation subservice.
package wire

import (
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/internal/service"
)

// NewService constructs an inert reconciliation service. Reconciliation is
// pure and therefore has no effect dependencies to inject.
func NewService() reconciliation.Service {
	return reconciliationservice.New()
}
