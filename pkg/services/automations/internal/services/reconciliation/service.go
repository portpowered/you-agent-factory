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
	RuntimeSourceControl
	Reconcile(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	StartSource(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSource(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSource(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	SourceStatus(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	GetStatus(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

// RuntimeSourceControl keeps lifecycle transitions independent when runtimes share
// public source identities. Empty RuntimeID selects the detached public scope.
type RuntimeSourceControl interface {
	StartSourceForRuntime(context.Context, string, automations.StartSourceRequest) (automations.StartSourceResult, error)
	StopSourceForRuntime(context.Context, string, automations.StopSourceRequest) (automations.StopSourceResult, error)
	WaitSourceForRuntime(context.Context, string, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
}

// SourceLifecycle applies source-specific lifecycle effects without owning
// reconciliation policy. Start runs after the authoritative starting observation
// is committed. Wait observes a transition without activating or stopping it.
type SourceLifecycle interface {
	Start(context.Context, StartEffect) error
	Stop(context.Context, StopEffect) error
	Wait(context.Context, WaitEffect) (automations.SourceObservation, error)
}

// StartEffect identifies the one logical source activation to apply.
type StartEffect struct {
	RuntimeID   string
	Kind        string
	Observation automations.SourceObservation
}

// StopEffect identifies the one logical source deactivation to apply.
type StopEffect struct {
	RuntimeID   string
	Observation automations.SourceObservation
}

// WaitEffect identifies the transition whose latest observation is requested.
type WaitEffect struct {
	RuntimeID   string
	Desired     automations.DesiredLifecycleState
	Observation automations.SourceObservation
}
