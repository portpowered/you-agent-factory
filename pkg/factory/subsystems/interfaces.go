// Package subsystems defines the Subsystem interface and active tick-phase
// components used by the runtime.
package subsystems

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// TickGroup defines the ordering of subsystem execution within each tick.
// Lower values execute first. Negative values execute before the scheduler.
type TickGroup int

const (
	// CircuitBreaker runs first — may halt the tick entirely.
	CircuitBreaker TickGroup = -1
	// Scheduler is a placeholder tick group for scheduling logic.
	// The Dispatcher subsystem incorporates scheduling internally.
	Scheduler TickGroup = 3
	// Dispatcher selects enabled transitions and sends work to worker executors.
	// Runs before History and Transitioner so synchronous dispatch results can be
	// processed within the same tick cycle.
	Dispatcher TickGroup = 5
	// History computes token visit histories from snapshot.Results for callers
	// that want a derived view during the tick.
	History TickGroup = 10
	// Transitioner routes tokens to the correct arc set based on outcome,
	// constructs output/fanout tokens from raw dispatch snapshots, and handles
	// resource release and spawned work.
	Transitioner TickGroup = 11
	// CascadingFailure propagates failure from parent tokens to dependents.
	// Runs after Transitioner so it can see newly-failed tokens.
	CascadingFailure TickGroup = 15
	// Tracer records transition firings and token movements.
	Tracer TickGroup = 20
	// TerminationCheck determines if the workflow has completed.
	TerminationCheck TickGroup = 40
)

// Subsystem is a tick-phase component that observes the marking and produces
// mutations, dispatches, histories, and generated batches. The engine calls
// Execute on each subsystem in TickGroup order during every tick.
type Subsystem interface {
	// TickGroup returns the ordering constant for this subsystem.
	TickGroup() TickGroup

	// Execute runs the subsystem's logic for a single tick.
	// It receives an immutable snapshot of the full runtime state.
	Execute(ctx context.Context, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error)
}
