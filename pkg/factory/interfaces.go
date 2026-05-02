package factory

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// APIFactory is the factory boundary required by the HTTP API server.
type APIFactory interface {
	// SubmitWorkRequest injects a canonical work request batch idempotently.
	SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error)

	// SubscribeFactoryEvents returns canonical factory event history followed by
	// live events. The live stream closes when ctx is canceled.
	SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error)

	// GetEngineStateSnapshot returns the aggregate observability snapshot for
	// service-facing consumers.
	GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error)
}

// Factory is the top-level interface for a CPN-based workflow engine.
type Factory interface {
	// Run starts the factory loop. Blocks until ctx is cancelled or all
	// work reaches terminal states.
	Run(ctx context.Context) error

	APIFactory

	// Pause pauses the factory loop. No transitions fire until resumed.
	Pause(ctx context.Context) error

	// GetFactoryEvents returns the current-process canonical event history.
	GetFactoryEvents(ctx context.Context) ([]factoryapi.FactoryEvent, error)

	// WaitToComplete returns a channel that is closed when all tokens reach
	// terminal or failed places and no dispatches are in flight. Callers can
	// block on this channel to know when the factory has finished all work
	// without having to manually drive ticks.
	WaitToComplete() <-chan struct{}
}
