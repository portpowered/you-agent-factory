// Package reconciliation defines the Automations-owned desired/observed
// reconciliation capability. Trigger implementations and callers outside
// Automations consume the outer Automations service instead of this private
// subservice contract.
package reconciliation

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

// Service owns detached reconciliation decisions and explicit source
// lifecycle operations. Only lifecycle commands can apply supervision effects.
type Service interface {
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

// Effects applies source-specific lifecycle effects without owning
// reconciliation policy. Start is invoked only after the reconciler commits
// the authoritative starting observation. Wait observes one already-started
// transition; it must not activate or stop a source.
type Effects struct {
	Start func(context.Context, StartEffect) error
	Stop  func(context.Context, StopEffect) error
	Wait  func(context.Context, WaitEffect) (automations.SourceObservation, error)
}

// StartEffect identifies the one logical source activation to apply.
type StartEffect struct {
	Kind        string
	Observation automations.SourceObservation
}

// StopEffect identifies the one logical source deactivation to apply.
type StopEffect struct {
	Observation automations.SourceObservation
}

// WaitEffect identifies the transition whose latest observation is requested.
type WaitEffect struct {
	Desired     automations.DesiredLifecycleState
	Observation automations.SourceObservation
}
