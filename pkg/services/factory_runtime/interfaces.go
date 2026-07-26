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

// LegacyEngineObservation retains the migration-only API and Factory Sessions
// signature without making that snapshot a member of Service. New peers use
// Observe and do not need this alias.
type LegacyEngineObservation = StateSnapshot

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

// APIFactory is the migration-only factory boundary required by legacy HTTP
// API and Factory Sessions adapters. New cross-service peers use Service,
// which does not expose GetEngineStateSnapshot.
type APIFactory interface {
	// SubmitWorkRequest injects a canonical work request batch idempotently.
	SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error)

	// SubscribeFactoryEvents returns canonical factory event history followed by
	// live events. The live stream closes when ctx is canceled. When reconnect
	// is non-nil, only events newer than the acknowledged cursor are replayed.
	SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error)

	// GetEngineStateSnapshot returns the aggregate observability snapshot for
	// migration-era consumers.
	GetEngineStateSnapshot(ctx context.Context) (*StateSnapshot, error)
}

// Service is the singular Factory Runtime root contract and the only
// cross-service runtime authority for control, observation, dispatch-plan, and
// checkpoint slices published at this package root. Peers depend on this named
// interface rather than hosting bundles, run-loop engines, or JavaScript-only
// strategy seams. A service may route these operations to a replaceable hosted
// engine and therefore does not expose the engine run loop.
type Service interface {
	// ControlPause pauses the factory loop. No transitions fire until resumed.
	// Returns ErrNotRunning when the instance is not running.
	ControlPause(ctx context.Context, req PauseRequest) (PauseResult, error)

	// ControlResume resumes a paused factory loop and actively wakes the engine so
	// already-buffered submissions and worker results can drain. When the
	// factory is already running, resume is an accepted no-op.
	// Returns ErrNotRunning when the instance is not running.
	ControlResume(ctx context.Context, req ResumeRequest) (ResumeResult, error)

	// ControlTerminate requests cooperative stop of the Factory Runtime instance using
	// the published plain terminate/stop control contract. Returns
	// ErrAlreadyStopped, ErrNotRunning, or ErrInvalidLifecycleTransition for
	// typed lifecycle failures. Nested IMP-RUN packets own durable stop wiring.
	ControlTerminate(ctx context.Context, req TerminateRequest) (TerminateResult, error)

	// ControlWaitToComplete returns a channel that is closed when all tokens reach
	// terminal or failed places and no dispatches are in flight. Callers can
	// block on this channel to know when the factory has finished all work
	// without having to manually drive ticks.
	ControlWaitToComplete(req WaitToCompleteRequest) WaitToCompleteResult

	// ControlMoveWork relocates Work through Runtime-owned plain vocabulary.
	// Returns ErrMoveWorkNotFound or ErrMoveWorkRequestConflict for typed
	// missing-work and idempotency-conflict failures.
	ControlMoveWork(ctx context.Context, req MoveWorkRequest) (MoveWorkResult, error)

	// Observe returns a detached orchestration-neutral observation for live
	// status, progress, dispatch, result, resource, and retained health views.
	// Returns ErrNotRunning, ErrNotFound, or ErrInvalidObservationScope for typed
	// observation failures. Peers must not treat GetEngineStateSnapshot as the
	// source of truth for this published slice.
	Observe(ctx context.Context, req ObserveRequest) (ObserveResult, error)

	// PlanDispatch publishes a stable dispatch intent into Runtime-owned
	// planning/outbox vocabulary. Workers remains the execution owner. Returns
	// ErrDuplicateDispatchIntent, ErrNotRunning, or ErrNotFound for typed
	// dispatch-plan failures, or ErrCapabilityUnavailable until the canonical
	// outbox implementation lands. Nested IMP-RUN packets own durable wiring.
	PlanDispatch(ctx context.Context, req PlanDispatchRequest) (PlanDispatchResult, error)

	// AcceptDispatchResult accepts or retires a correlated worker result against
	// a previously planned dispatch intent, including idempotent duplicate
	// handling vocabulary on success. Returns ErrUnknownDispatchCorrelation,
	// ErrInvalidDispatchResultBoundary, ErrNotRunning, or ErrNotFound for typed
	// dispatch-plan failures, or ErrCapabilityUnavailable until the canonical
	// result-ingress implementation lands.
	AcceptDispatchResult(ctx context.Context, req AcceptDispatchResultRequest) (AcceptDispatchResultResult, error)

	// CaptureCheckpoint captures a versioned Runtime execution checkpoint with
	// opaque strategy payload bytes. Returns ErrNotRunning or ErrNotFound for
	// typed availability failures. Does not claim Recordings immutable history
	// ownership. Returns ErrCapabilityUnavailable until nested IMP-RUN packets
	// provide canonical execution-state codec wiring.
	CaptureCheckpoint(ctx context.Context, req CaptureCheckpointRequest) (CaptureCheckpointResult, error)

	// LoadCheckpoint loads or inspects compatibility of a previously captured
	// checkpoint without restoring it. Returns ErrCheckpointNotFound,
	// ErrCorruptCheckpoint, ErrIncompatibleCheckpoint, ErrNotRunning, or
	// ErrNotFound for typed failures, or ErrCapabilityUnavailable until the
	// canonical checkpoint store implementation lands.
	LoadCheckpoint(ctx context.Context, req LoadCheckpointRequest) (LoadCheckpointResult, error)

	// RestoreCheckpoint restores a compatible opaque checkpoint into mutable
	// Runtime execution state. Returns ErrCheckpointNotFound,
	// ErrCorruptCheckpoint, ErrIncompatibleCheckpoint, ErrNotRunning, or
	// ErrNotFound for typed failures, or ErrCapabilityUnavailable until the
	// canonical mutable-state restore implementation lands.
	RestoreCheckpoint(ctx context.Context, req RestoreCheckpointRequest) (RestoreCheckpointResult, error)
}

// Factory retains the migration-era engine and blocking run-loop surface for
// hosting-owned construction. Concrete hosted runtimes also implement Service;
// cross-service root-slice peers depend on Service rather than this engine seam.
type Factory interface {
	APIFactory
	WorkMover
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	GetFactoryEvents(ctx context.Context) ([]interfaces.FactoryEvent, error)
	WaitToComplete() <-chan struct{}
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
