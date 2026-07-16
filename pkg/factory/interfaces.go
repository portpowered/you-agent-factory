package factory

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
)

// WorkMover is synchronous operator control ingress for relocating work tokens.
type WorkMover interface {
	MoveWork(ctx context.Context, workID string, stateName string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error)
}

// APIFactory is the factory boundary required by the HTTP API server.
type APIFactory interface {
	// SubmitWorkRequest injects a canonical work request batch idempotently.
	SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error)

	// SubscribeFactoryEvents returns canonical factory event history followed by
	// live events. The live stream closes when ctx is canceled. When reconnect
	// is non-nil, only events newer than the acknowledged cursor are replayed.
	SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error)

	// GetEngineStateSnapshot returns the aggregate observability snapshot for
	// service-facing consumers.
	GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
}

// Factory is the top-level interface for a CPN-based workflow engine.
type Factory interface {
	WorkMover
	// Run starts the factory loop. Blocks until ctx is cancelled or all
	// work reaches terminal states.
	Run(ctx context.Context) error

	APIFactory

	// Pause pauses the factory loop. No transitions fire until resumed.
	Pause(ctx context.Context) error

	// Resume resumes a paused factory loop and actively wakes the engine so
	// already-buffered submissions and worker results can drain. When the
	// factory is already running, resume is an accepted no-op.
	Resume(ctx context.Context) error

	// GetFactoryEvents returns the current-process canonical event history.
	GetFactoryEvents(ctx context.Context) ([]interfaces.FactoryEvent, error)

	// WaitToComplete returns a channel that is closed when all tokens reach
	// terminal or failed places and no dispatches are in flight. Callers can
	// block on this channel to know when the factory has finished all work
	// without having to manually drive ticks.
	WaitToComplete() <-chan struct{}
}
