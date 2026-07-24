package runtime

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// Terminate publishes the root terminate/stop control entrypoint. Durable stop
// wiring remains an IMP-RUN concern; this maps lifecycle states onto the plain
// root typed errors and accepted outcome without mutating run-loop internals.
func (f *factoryImpl) Terminate(_ context.Context, _ factory.TerminateRequest) (factory.TerminateResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
	default:
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
}

// Observe returns a detached orchestration-neutral observation projected from
// live runtime state. Petri markings, nets, tokens, and enabled transitions are
// not included in the published observation vocabulary.
func (f *factoryImpl) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		// Observable lifecycle states may return a sanitized observation.
	default:
		return factory.ObserveResult{}, factory.ErrNotRunning
	}

	snap, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factory.ObserveResult{}, err
	}
	return factory.ObserveResult{Observation: projectRootObservation(snap, req.Scope)}, nil
}

// PlanDispatch publishes a stable dispatch intent through the root dispatch-plan
// contract. Nested IMP-RUN packets own durable outbox wiring; this method maps
// lifecycle availability onto typed root errors for compile continuity.
func (f *factoryImpl) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return factory.PlanDispatchResult{
			Outcome:       factory.DispatchPlanOutcomeAccepted,
			DispatchID:    req.DispatchID,
			CorrelationID: req.CorrelationID,
		}, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.PlanDispatchResult{}, factory.ErrNotRunning
	default:
		return factory.PlanDispatchResult{}, factory.ErrNotRunning
	}
}

// AcceptDispatchResult accepts or retires a correlated worker result through the
// root dispatch-plan contract. Nested IMP-RUN packets own durable outbox wiring.
func (f *factoryImpl) AcceptDispatchResult(
	_ context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return factory.AcceptDispatchResultResult{
			Outcome:       factory.DispatchPlanOutcomeRetired,
			DispatchID:    req.DispatchID,
			CorrelationID: req.CorrelationID,
		}, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return factory.AcceptDispatchResultResult{}, factory.ErrNotRunning
	default:
		return factory.AcceptDispatchResultResult{}, factory.ErrNotRunning
	}
}

// CaptureCheckpoint captures a versioned Runtime execution checkpoint through
// the root checkpoint contract. Nested IMP-RUN packets own durable codec wiring;
// this method maps lifecycle availability onto typed root errors for compile
// continuity and returns opaque strategy bytes without Petri/JS vocabulary.
func (f *factoryImpl) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		id := req.CheckpointID
		if id == "" {
			id = "checkpoint-stub"
		}
		return factory.CaptureCheckpointResult{
			Outcome: factory.CheckpointOutcomeCaptured,
			Checkpoint: factory.Checkpoint{
				CheckpointID:  id,
				SchemaVersion: 1,
				StrategyKind:  "runtime",
				Payload:       []byte(`{}`),
			},
		}, nil
	default:
		return factory.CaptureCheckpointResult{}, factory.ErrNotRunning
	}
}

// LoadCheckpoint loads or inspects checkpoint compatibility through the root
// checkpoint contract. Durable store wiring remains IMP-RUN; missing identity
// maps to ErrCheckpointNotFound until nested packets land.
func (f *factoryImpl) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		if req.CheckpointID == "" {
			return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
		}
		return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
	default:
		return factory.LoadCheckpointResult{}, factory.ErrNotRunning
	}
}

// RestoreCheckpoint restores a compatible opaque checkpoint through the root
// checkpoint contract. Nested IMP-RUN packets own durable restore wiring.
func (f *factoryImpl) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return factory.RestoreCheckpointResult{
			Outcome:      factory.CheckpointOutcomeRestored,
			CheckpointID: req.Checkpoint.CheckpointID,
		}, nil
	default:
		return factory.RestoreCheckpointResult{}, factory.ErrNotRunning
	}
}
