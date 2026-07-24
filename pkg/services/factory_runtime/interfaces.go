package factory

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontext "github.com/portpowered/infinite-you/pkg/services/factory_runtime/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// StateSnapshot is the public Factory Runtime observation returned to service
// consumers without requiring them to import Runtime implementation packages.
type StateSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

// Scheduler is the replaceable Factory Runtime transition-selection policy.
type Scheduler = scheduler.Scheduler

// WorkflowContextProvider exposes the immutable workflow context attached to a
// Factory runtime without exposing its implementation.
type WorkflowContextProvider interface {
	WorkflowContext() *factorycontext.FactoryContext
}

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
	// service-facing consumers. Legacy Petri-shaped aliases remain available for
	// migration; peers consuming published root observation slices use Observe.
	GetEngineStateSnapshot(ctx context.Context) (*StateSnapshot, error)
}

// Service is the singular Factory Runtime root contract and the only
// cross-service runtime authority for control, observation, dispatch-plan, and
// checkpoint slices published at this package root. Peers depend on this named
// interface rather than hosting bundles, run-loop engines, or JavaScript-only
// strategy seams. A service may route these operations to a replaceable hosted
// engine and therefore does not expose the engine run loop.
type Service interface {
	WorkMover
	APIFactory

	// Pause pauses the factory loop. No transitions fire until resumed.
	// Returns ErrNotRunning when the instance is not running.
	Pause(ctx context.Context) error

	// Resume resumes a paused factory loop and actively wakes the engine so
	// already-buffered submissions and worker results can drain. When the
	// factory is already running, resume is an accepted no-op.
	// Returns ErrNotRunning when the instance is not running.
	Resume(ctx context.Context) error

	// Terminate requests cooperative stop of the Factory Runtime instance using
	// the published plain terminate/stop control contract. Returns
	// ErrAlreadyStopped, ErrNotRunning, or ErrInvalidLifecycleTransition for
	// typed lifecycle failures. Nested IMP-RUN packets own durable stop wiring.
	Terminate(ctx context.Context, req TerminateRequest) (TerminateResult, error)

	// GetFactoryEvents returns the current-process canonical event history.
	GetFactoryEvents(ctx context.Context) ([]interfaces.FactoryEvent, error)

	// WaitToComplete returns a channel that is closed when all tokens reach
	// terminal or failed places and no dispatches are in flight. Callers can
	// block on this channel to know when the factory has finished all work
	// without having to manually drive ticks.
	WaitToComplete() <-chan struct{}

	// Observe returns a detached orchestration-neutral observation for live
	// status, progress, dispatch, result, resource, and retained health views.
	// Returns ErrNotRunning, ErrNotFound, or ErrInvalidObservationScope for typed
	// observation failures. Peers must not treat GetEngineStateSnapshot as the
	// source of truth for this published slice.
	Observe(ctx context.Context, req ObserveRequest) (ObserveResult, error)

	// PlanDispatch publishes a stable dispatch intent into Runtime-owned
	// planning/outbox vocabulary. Workers remains the execution owner. Returns
	// ErrDuplicateDispatchIntent, ErrNotRunning, or ErrNotFound for typed
	// dispatch-plan failures. Nested IMP-RUN packets own durable outbox wiring.
	PlanDispatch(ctx context.Context, req PlanDispatchRequest) (PlanDispatchResult, error)

	// AcceptDispatchResult accepts or retires a correlated worker result against
	// a previously planned dispatch intent, including idempotent duplicate
	// handling vocabulary on success. Returns ErrUnknownDispatchCorrelation,
	// ErrInvalidDispatchResultBoundary, ErrNotRunning, or ErrNotFound for typed
	// dispatch-plan failures.
	AcceptDispatchResult(ctx context.Context, req AcceptDispatchResultRequest) (AcceptDispatchResultResult, error)
}

// Factory extends Service with a blocking run loop for hosting-owned engine
// construction. Cross-service peers use Service, not Factory, as the runtime
// authority for published root-contract slices.
type Factory interface {
	Service
	// Run starts the factory loop. Blocks until ctx is cancelled or all
	// work reaches terminal states.
	Run(ctx context.Context) error
}

// WorldStateProjector reconstructs canonical query state from recorded events.
// Recordings supplies the implementation; Factory Runtime invokes the port.
type WorldStateProjector func(
	[]interfaces.FactoryEvent,
	int,
) (interfaces.FactoryWorldState, error)
