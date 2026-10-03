// Package reconciliation defines the Automations-owned desired/observed
// reconciliation capability. Trigger implementations and callers outside
// Automations consume the outer Automations service instead of this private
// subservice contract.
package reconciliation

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
)

// Service owns detached reconciliation decisions and explicit source
// lifecycle operations. Only lifecycle commands can apply supervision effects.
type Service interface {
	// Runtime-keyed controls isolate sources sharing a public identity. An empty
	// runtime ID selects the detached public scope.
	StartSourceForRuntime(context.Context, string, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSourceForRuntime(context.Context, string, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSourceForRuntime(context.Context, string, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

// Value aliases preserve lifecycle effect identity.
type StartEffect = sourcelifecycle.StartEffect
type StopEffect = sourcelifecycle.StopEffect
type WaitEffect = sourcelifecycle.WaitEffect
