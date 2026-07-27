// Package wire constructs the Automations reconciliation subservice.
package wire

import (
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/internal/service"
)

// NewService constructs an inert reconciliation service with optional narrow
// lifecycle effects. Construction never invokes the supplied functions.
func NewService(effects ...reconciliation.Effects) reconciliation.Service {
	var supervision reconciliation.Effects
	if len(effects) > 0 {
		supervision = effects[0]
	}
	return reconciliationservice.New(supervision)
}
