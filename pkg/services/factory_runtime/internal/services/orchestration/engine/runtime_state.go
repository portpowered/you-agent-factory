package engine

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// RuntimeState is the unified mutable state container for the engine loop.
// All per-tick state lives here so it can be snapshotted atomically.
type RuntimeState struct {
	Marking              *petri.Marking                                   `json:"marking"`
	Dispatches           map[string]*interfaces.DispatchEntry             `json:"dispatches"`
	InFlightCount        int                                              `json:"in_flight_count"` // accurate count even when Dispatches map has key collisions
	Results              []workerexecution.WorkResult                     `json:"results"`
	ResultBuffer         *buffers.TypedBuffer[workerexecution.WorkResult] `json:"-"`
	DispatchHistory      []interfaces.CompletedDispatch                   `json:"dispatch_history"`
	ActiveThrottlePauses []interfaces.ActiveThrottlePause                 `json:"active_throttle_pauses,omitempty"`
	TickCount            int                                              `json:"tick_count"`
}

// Snapshot produces an immutable deep copy of the RuntimeState.
func (rs *RuntimeState) Snapshot() interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		TickCount:     rs.TickCount,
		InFlightCount: rs.InFlightCount,
	}

	// Deep copy marking via its own Snapshot method.
	if rs.Marking != nil {
		snap.Marking = rs.Marking.Snapshot()
	}

	// Deep copy dispatches.
	if rs.Dispatches != nil {
		snap.Dispatches = make(map[string]*interfaces.DispatchEntry, len(rs.Dispatches))
		for k, v := range rs.Dispatches {
			cp := *v
			cp.ConsumedTokens = factorytoken.CloneSlice(v.ConsumedTokens)
			if v.HeldMutations != nil {
				cp.HeldMutations = make([]interfaces.MarkingMutation, len(v.HeldMutations))
				copy(cp.HeldMutations, v.HeldMutations)
			}
			snap.Dispatches[k] = &cp
		}
	}

	// Deep copy results.
	if rs.Results != nil {
		snap.Results = make([]workerexecution.WorkResult, len(rs.Results))
		for i := range rs.Results {
			snap.Results[i] = deepCopyWorkResult(rs.Results[i])
		}
	}

	// Deep copy dispatch history.
	if rs.DispatchHistory != nil {
		snap.DispatchHistory = make([]interfaces.CompletedDispatch, len(rs.DispatchHistory))
		for i := range rs.DispatchHistory {
			snap.DispatchHistory[i] = deepCopyCompletedDispatch(rs.DispatchHistory[i])
		}
	}

	if rs.ActiveThrottlePauses != nil {
		snap.ActiveThrottlePauses = make([]interfaces.ActiveThrottlePause, len(rs.ActiveThrottlePauses))
		copy(snap.ActiveThrottlePauses, rs.ActiveThrottlePauses)
	}

	return snap
}

func deepCopyCompletedDispatch(d interfaces.CompletedDispatch) interfaces.CompletedDispatch {
	cp := d
	cp.ProviderSession = workerexecution.CloneProviderSessionMetadata(d.ProviderSession)
	cp.ConsumedTokens = factorytoken.CloneSlice(d.ConsumedTokens)
	if d.OutputMutations != nil {
		cp.OutputMutations = make([]interfaces.TokenMutationRecord, len(d.OutputMutations))
		for i := range d.OutputMutations {
			cp.OutputMutations[i] = deepCopyTokenMutationRecord(d.OutputMutations[i])
		}
	}
	return cp
}

func deepCopyWorkResult(result workerexecution.WorkResult) workerexecution.WorkResult {
	cp := result
	cp.ProviderSession = workerexecution.CloneProviderSessionMetadata(result.ProviderSession)
	return cp
}

func deepCopyTokenMutationRecord(m interfaces.TokenMutationRecord) interfaces.TokenMutationRecord {
	cp := m
	if m.Token != nil {
		tokenCopy := factorytoken.Clone(*m.Token)
		cp.Token = &tokenCopy
	}
	return cp
}

func (e *FactoryEngine) tickOnce(ctx context.Context) (bool, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick(ctx)
}

func (e *FactoryEngine) finishTerminationDrain() (bool, error) {
	e.mu.Lock()
	e.acceptingSubmits = false
	drained := e.drainChannels()
	if drained {
		e.acceptingSubmits = true
	}
	e.mu.Unlock()
	if drained {
		return false, nil
	}
	return true, nil
}

// Tick executes a single tick synchronously. Drains all pending channel events
// first, then runs the full tick cycle. For deterministic testing.
func (e *FactoryEngine) Tick(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.drainChannels()
	_, _, err := e.tick(ctx)
	return err
}

// TickN executes n ticks sequentially. For testing.
func (e *FactoryEngine) TickN(ctx context.Context, n int) error {
	for i := range n {
		if err := e.Tick(ctx); err != nil {
			return fmt.Errorf("tick %d: %w", i, err)
		}
	}
	return nil
}

// TickUntil ticks until the predicate returns true or maxTicks is exceeded.
func (e *FactoryEngine) TickUntil(ctx context.Context, pred func(*petri.MarkingSnapshot) bool, maxTicks int) error {
	for range maxTicks {
		if err := e.Tick(ctx); err != nil {
			return err
		}
		snap := e.runtimeState.Marking.Snapshot()
		if pred(&snap) {
			return nil
		}
	}
	return fmt.Errorf("predicate not satisfied after %d ticks", maxTicks)
}

// GetMarking returns a snapshot of the current marking.
func (e *FactoryEngine) GetMarking() petri.MarkingSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runtimeState.Marking.Snapshot()
}

// GetRuntimeStateSnapshot returns a full snapshot of the engine's runtime state.
func (e *FactoryEngine) GetRuntimeStateSnapshot() interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runtimeState.Snapshot()
}

// GetResultBuffer returns the runtime-owned work result buffer used to hand
// completed worker results back to the engine.
func (e *FactoryEngine) GetResultBuffer() *buffers.TypedBuffer[workerexecution.WorkResult] {
	return e.runtimeState.ResultBuffer
}

// drainChannels non-blocking drains all pending wake signals from resultCh and queued submissions.
// Returns true when at least one signal was drained.
func (e *FactoryEngine) drainChannels() bool {
	drained := false
	for {
		select {
		case <-e.resultCh:
			e.handleResult()
			drained = true
		default:
			select {
			case <-e.submitSignal:
				drained = true
				continue
			default:
				return drained
			}
		}
	}
}

// handleResult processes a single worker-result wake signal.
func (e *FactoryEngine) handleResult() {
	// Dispatch entries are NOT removed here. They are retired at end-of-tick
	// by retireCompletedDispatches, after all subsystems (including
	// TerminationCheck) have observed them. The actual WorkResult is in
	// pendingResults and will be drained at the start of the next tick.
	if e.resultHandler != nil {
		e.resultHandler()
	}
}

// RunningDispatches returns a copy of the current running dispatches mapping.
// Each entry maps a dispatch ID to the marking mutations consumed to fire it.
func (e *FactoryEngine) RunningDispatches() map[string][]interfaces.MarkingMutation {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make(map[string][]interfaces.MarkingMutation, len(e.runtimeState.Dispatches))
	for k, v := range e.runtimeState.Dispatches {
		muts := make([]interfaces.MarkingMutation, len(v.HeldMutations))
		copy(muts, v.HeldMutations)
		result[k] = muts
	}
	return result
}
