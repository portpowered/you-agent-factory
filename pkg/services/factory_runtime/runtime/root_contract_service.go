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
	if !validObservationScope(req.Scope) {
		return factory.ObserveResult{}, factory.ErrInvalidObservationScope
	}
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

// PlanDispatch validates the published boundary but does not report false
// success before the nested IMP-RUN packet connects it to the canonical outbox.
func (f *factoryImpl) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		if req.DispatchID == "" || req.CorrelationID == "" {
			return factory.PlanDispatchResult{}, factory.ErrInvalidDispatchResultBoundary
		}
		return factory.PlanDispatchResult{}, factory.ErrCapabilityUnavailable
	default:
		return factory.PlanDispatchResult{}, factory.ErrNotRunning
	}
}

// AcceptDispatchResult validates the published boundary but does not report
// false retirement before canonical correlation/result ingress is connected.
func (f *factoryImpl) AcceptDispatchResult(
	_ context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		if !validDispatchResultOutcome(req.ResultOutcome) {
			return factory.AcceptDispatchResultResult{}, factory.ErrInvalidDispatchResultBoundary
		}
		if req.DispatchID == "" || req.CorrelationID == "" {
			return factory.AcceptDispatchResultResult{}, factory.ErrUnknownDispatchCorrelation
		}
		return factory.AcceptDispatchResultResult{}, factory.ErrCapabilityUnavailable
	default:
		return factory.AcceptDispatchResultResult{}, factory.ErrNotRunning
	}
}

// CaptureCheckpoint does not report false capture before the nested IMP-RUN
// packet connects an execution-state codec and canonical checkpoint store.
func (f *factoryImpl) CaptureCheckpoint(
	_ context.Context,
	req factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return factory.CaptureCheckpointResult{}, factory.ErrCapabilityUnavailable
	default:
		return factory.CaptureCheckpointResult{}, factory.ErrNotRunning
	}
}

// LoadCheckpoint rejects missing identity and otherwise reports that the
// canonical checkpoint store has not landed yet.
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
		return factory.LoadCheckpointResult{}, factory.ErrCapabilityUnavailable
	default:
		return factory.LoadCheckpointResult{}, factory.ErrNotRunning
	}
}

// RestoreCheckpoint validates the published envelope but does not report false
// restoration before canonical mutable-state restore wiring lands.
func (f *factoryImpl) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
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
		return factory.RestoreCheckpointResult{}, factory.ErrCapabilityUnavailable
	default:
		return factory.RestoreCheckpointResult{}, factory.ErrNotRunning
	}
}

func validObservationScope(scope factory.ObservationScope) bool {
	switch scope {
	case "", factory.ObservationScopeFull, factory.ObservationScopeStatus, factory.ObservationScopeProgress,
		factory.ObservationScopeDispatches, factory.ObservationScopeResults, factory.ObservationScopeResources,
		factory.ObservationScopeHealth:
		return true
	default:
		return false
	}
}

func validDispatchResultOutcome(outcome factory.DispatchResultOutcome) bool {
	switch outcome {
	case "", factory.DispatchResultOutcomeSuccess, factory.DispatchResultOutcomeFailure, factory.DispatchResultOutcomeCancelled:
		return true
	default:
		return false
	}
}
