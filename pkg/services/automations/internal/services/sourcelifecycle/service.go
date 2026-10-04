// Package sourcelifecycle owns Automation source registration and supervision.
package sourcelifecycle

import (
	"context"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type RuntimeSourceConfiguration struct {
	RuntimeID     string
	CursorBaseDir string
	// WorkflowID preserves the caller's script identity, including a blank value.
	// Scheduler identity remains independently selected in the snapshot.
	WorkflowID       string
	FactorySessionID string
	Snapshot         factorydefinitions.RuntimeSnapshot
	Inputs           automations.RuntimeActivationInputs
}

type StartEffect struct {
	RuntimeID   string
	Kind        string
	Observation automations.SourceObservation
}

type StopEffect struct {
	RuntimeID   string
	Observation automations.SourceObservation
}

type WaitEffect struct {
	RuntimeID   string
	Desired     automations.DesiredLifecycleState
	Observation automations.SourceObservation
}

type SourceLifecycle interface {
	ConfigureRuntimeSource(context.Context, RuntimeSourceConfiguration) error
	ReleaseRuntimeSource(context.Context, string) error
	Start(context.Context, StartEffect) error
	Stop(context.Context, StopEffect) error
	Wait(context.Context, WaitEffect) (automations.SourceObservation, error)
}
