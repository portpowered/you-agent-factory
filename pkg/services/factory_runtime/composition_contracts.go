package factory

import (
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Root contract typed errors for orchestration-neutral Runtime control and
// observation. Peers branch with errors.Is; nested IMP-RUN implementations map
// concrete lifecycle failures onto these sentinels.
var (
	// ErrNotRunning indicates the targeted Factory Runtime instance is not in a
	// running state for the requested control or observation operation.
	ErrNotRunning = errors.New("factory runtime is not running")

	// ErrNotFound indicates the targeted Factory Runtime instance or work scope
	// does not exist for the requested operation.
	ErrNotFound = errors.New("factory runtime target not found")

	// ErrAlreadyStopped indicates terminate/stop was requested against an
	// instance that has already stopped.
	ErrAlreadyStopped = errors.New("factory runtime is already stopped")

	// ErrInvalidLifecycleTransition indicates the requested control operation is
	// not valid from the instance's current lifecycle state.
	ErrInvalidLifecycleTransition = errors.New("factory runtime invalid lifecycle transition")

	// ErrInvalidObservationScope indicates the observation request asked for a
	// scope outside the published orchestration-neutral observation vocabulary.
	ErrInvalidObservationScope = errors.New("factory runtime invalid observation scope")

	// ErrIncompleteDrain identifies a finite runtime that became quiescent with
	// admitted customer Work still in a non-terminal state.
	ErrIncompleteDrain = errors.New("factory session drained with non-terminal work")

	// ErrDuplicateDispatchIntent indicates a plan/publish request conflicted with
	// an existing dispatch intent that is not eligible for idempotent re-delivery.
	ErrDuplicateDispatchIntent = dispatchplanning.ErrDuplicateDispatchIntent

	// ErrUnknownDispatchCorrelation indicates accept/retire targeted a correlation
	// that is not present in the Runtime dispatch outbox.
	ErrUnknownDispatchCorrelation = dispatchplanning.ErrUnknownDispatchCorrelation

	// ErrInvalidDispatchResultBoundary indicates the correlated worker result fell
	// outside the published result-boundary vocabulary peers may submit.
	ErrInvalidDispatchResultBoundary = dispatchplanning.ErrInvalidDispatchResultBoundary
)

// IncompleteDrainError preserves the authoritative non-terminal Work count
// while allowing callers to branch on ErrIncompleteDrain.
type IncompleteDrainError struct {
	NonTerminalWorkCount int
}

func (e *IncompleteDrainError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("factory session drained with %d non-terminal work items; run is incomplete", e.NonTerminalWorkCount)
}

func (e *IncompleteDrainError) Unwrap() error {
	return ErrIncompleteDrain
}

type WorkersMockCommandRunnerFactory func(
	*workers.MockWorkersConfig,
	factorydefinitions.RuntimeDefinitionLookup,
	platformprocess.CommandRunner,
) platformprocess.CommandRunner

// WorkerSessionsFactory constructs the per-session Worker Sessions service
// (W4 Runtime dispatch cutover) from the directly injected Workers execution
// service and the canonical runtime clock. Wire composes
// the one construction path (worker_sessions/wire.NewService plus its Events,
// logging, and Provider Sessions dependencies) behind this factory so Factory
// Runtime never imports a peer service's wire or internal packages directly.
type WorkerSessionsFactory func(workers.Service, platformclock.Source) (workersessions.Service, error)
