package runtime

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/rootobservation"
)

// Terminate publishes the root terminate/stop control entrypoint. Durable stop
// wiring remains an IMP-RUN concern; this maps lifecycle states onto the plain
// root typed errors and accepted outcome without mutating run-loop internals.
func (f *factoryImpl) Terminate(_ context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	f.mu.Lock()
	state := f.state
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStateCompleted
		cancel := f.runCancel
		f.mu.Unlock()
		f.recordStateChange(state, interfaces.FactoryStateCompleted, req.Reason)
		if cancel != nil {
			cancel()
		} else {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
	default:
		f.mu.Unlock()
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
	return factory.ObserveResult{Observation: rootobservation.Project(snap, req.Scope)}, nil
}

// PlanDispatch publishes a stable dispatch intent through the root dispatch-plan
// contract. Nested IMP-RUN packets own durable outbox wiring; this method maps
// lifecycle availability onto typed root errors for compile continuity.
func (f *factoryImpl) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		if existing, ok := f.dispatchIntents[req.CorrelationID]; ok {
			if samePlanDispatchRequest(existing.request, req) {
				return factory.PlanDispatchResult{Outcome: factory.DispatchPlanOutcomeDuplicateIdempotent, DispatchID: req.DispatchID, CorrelationID: req.CorrelationID}, nil
			}
			return factory.PlanDispatchResult{}, factory.ErrDuplicateDispatchIntent
		}
		if req.DispatchID == "" || req.CorrelationID == "" {
			return factory.PlanDispatchResult{}, factory.ErrInvalidDispatchResultBoundary
		}
		f.dispatchIntents[req.CorrelationID] = plannedDispatch{request: req}
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
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		if !validDispatchResultOutcome(req.ResultOutcome) {
			return factory.AcceptDispatchResultResult{}, factory.ErrInvalidDispatchResultBoundary
		}
		if retired, ok := f.retiredDispatches[req.CorrelationID]; ok {
			if retired == req {
				return factory.AcceptDispatchResultResult{Outcome: factory.DispatchPlanOutcomeDuplicateIdempotent, DispatchID: req.DispatchID, CorrelationID: req.CorrelationID}, nil
			}
			return factory.AcceptDispatchResultResult{}, factory.ErrInvalidDispatchResultBoundary
		}
		intent, ok := f.dispatchIntents[req.CorrelationID]
		if !ok || intent.request.DispatchID != req.DispatchID {
			return factory.AcceptDispatchResultResult{}, factory.ErrUnknownDispatchCorrelation
		}
		delete(f.dispatchIntents, req.CorrelationID)
		f.retiredDispatches[req.CorrelationID] = req
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
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		id := req.CheckpointID
		if id == "" {
			id = "checkpoint"
		}
		checkpoint := factory.Checkpoint{CheckpointID: id, SchemaVersion: 1, StrategyKind: "runtime", Payload: []byte(`{}`)}
		f.checkpoints[id] = checkpoint
		return factory.CaptureCheckpointResult{
			Outcome:    factory.CheckpointOutcomeCaptured,
			Checkpoint: checkpoint,
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
	defer f.mu.RUnlock()
	state := f.state
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		checkpoint, ok := f.checkpoints[req.CheckpointID]
		if !ok {
			return factory.LoadCheckpointResult{}, factory.ErrCheckpointNotFound
		}
		if len(checkpoint.Payload) == 0 || checkpoint.SchemaVersion <= 0 {
			return factory.LoadCheckpointResult{}, factory.ErrCorruptCheckpoint
		}
		compatible := req.ExpectedSchemaVersion == 0 || req.ExpectedSchemaVersion == checkpoint.SchemaVersion
		if !compatible {
			return factory.LoadCheckpointResult{}, factory.ErrIncompatibleCheckpoint
		}
		return factory.LoadCheckpointResult{Outcome: factory.CheckpointOutcomeLoaded, Checkpoint: cloneCheckpoint(checkpoint), Compatible: compatible}, nil
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
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		if req.Checkpoint.CheckpointID == "" {
			return factory.RestoreCheckpointResult{}, factory.ErrCheckpointNotFound
		}
		if req.Checkpoint.SchemaVersion <= 0 || len(req.Checkpoint.Payload) == 0 {
			return factory.RestoreCheckpointResult{}, factory.ErrCorruptCheckpoint
		}
		if req.Checkpoint.SchemaVersion != 1 {
			return factory.RestoreCheckpointResult{}, factory.ErrIncompatibleCheckpoint
		}
		f.checkpoints[req.Checkpoint.CheckpointID] = cloneCheckpoint(req.Checkpoint)
		return factory.RestoreCheckpointResult{
			Outcome:      factory.CheckpointOutcomeRestored,
			CheckpointID: req.Checkpoint.CheckpointID,
		}, nil
	default:
		return factory.RestoreCheckpointResult{}, factory.ErrNotRunning
	}
}

func cloneCheckpoint(checkpoint factory.Checkpoint) factory.Checkpoint {
	checkpoint.Payload = append([]byte(nil), checkpoint.Payload...)
	return checkpoint
}

func samePlanDispatchRequest(left, right factory.PlanDispatchRequest) bool {
	if left.DispatchID != right.DispatchID || left.CorrelationID != right.CorrelationID ||
		left.WorkstationName != right.WorkstationName || left.WorkerType != right.WorkerType || left.ReplayKey != right.ReplayKey ||
		len(left.WorkIDs) != len(right.WorkIDs) {
		return false
	}
	for index := range left.WorkIDs {
		if left.WorkIDs[index] != right.WorkIDs[index] {
			return false
		}
	}
	return true
}

func validDispatchResultOutcome(outcome factory.DispatchResultOutcome) bool {
	switch outcome {
	case "", factory.DispatchResultOutcomeSuccess, factory.DispatchResultOutcomeFailure, factory.DispatchResultOutcomeCancelled:
		return true
	default:
		return false
	}
}
