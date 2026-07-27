// Package reconciliation defines the Automations-owned desired/observed
// reconciliation capability. Trigger implementations and callers outside
// Automations consume the outer Automations service instead of this private
// subservice contract.
package reconciliation

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

// Service computes detached convergence outcomes without applying source
// effects. Lifecycle supervision remains a separate explicit operation.
type Service interface {
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
}
