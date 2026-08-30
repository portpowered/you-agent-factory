package engine

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/jsonvalue"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type engineStateSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

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

func completedDispatchReasonFromResult(result workerexecution.WorkResult) string {
	switch result.Outcome {
	case workerexecution.OutcomeCanceled:
		if result.Cancellation != nil {
			return string(result.Cancellation.Reason)
		}
		return string(workerexecution.DispatchCancellationReasonCanceled)
	case workerexecution.OutcomeFailed:
		return result.Error
	case workerexecution.OutcomeContinue, workerexecution.OutcomeRejected:
		return result.Feedback
	default:
		return ""
	}
}

func workResultForCompletedDispatch(result workerexecution.WorkResult, completed interfaces.CompletedDispatch) workerexecution.WorkResult {
	result.Outcome = completed.Outcome
	result.Cancellation = completed.Cancellation.Clone()
	if completed.Outcome == workerexecution.OutcomeCanceled {
		result.Error = completed.Reason
		result.FailureDetail = nil
		result.FailureMetadata = nil
		result.RecordedOutputWork = nil
		result.Output = ""
		result.StructuredResult = nil
		result.StructuredResultPresent = false
	}
	result.SelectedClassificationLabel = completed.SelectedClassificationLabel
	if completed.FailureDetail != nil {
		result.FailureDetail = workerexecution.CloneFailureDetail(completed.FailureDetail)
	}
	switch completed.Outcome {
	case workerexecution.OutcomeFailed:
		result.Error = completed.Reason
	case workerexecution.OutcomeContinue, workerexecution.OutcomeRejected:
		result.Feedback = completed.Reason
	}
	return result
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
			cp.ExpectedArtifactContext = cloneExpectedArtifactTemplateContext(v.ExpectedArtifactContext)
			cp.ConsumedTokens = cloneWorkerTokens(v.ConsumedTokens)
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
	if d.IgnoredResult != nil {
		ignored := *d.IgnoredResult
		cp.IgnoredResult = &ignored
	}
	cp.ExpectedArtifactContext = cloneExpectedArtifactTemplateContext(d.ExpectedArtifactContext)
	cp.ArtifactVerification = d.ArtifactVerification.Clone()
	cp.Cancellation = d.Cancellation.Clone()
	cp.FailureDetail = workerexecution.CloneFailureDetail(d.FailureDetail)
	cp.ProviderSession = (d.ProviderSession).Clone()
	cp.ConsumedTokens = cloneWorkerTokens(d.ConsumedTokens)
	if d.OutputMutations != nil {
		cp.OutputMutations = make([]interfaces.TokenMutationRecord, len(d.OutputMutations))
		for i := range d.OutputMutations {
			cp.OutputMutations[i] = deepCopyTokenMutationRecord(d.OutputMutations[i])
		}
	}
	return cp
}

func cloneExpectedArtifactTemplateContext(
	context *work.ExpectedArtifactTemplateContext,
) *work.ExpectedArtifactTemplateContext {
	return context.Clone()
}

func deepCopyWorkResult(result workerexecution.WorkResult) workerexecution.WorkResult {
	cp := result
	cp.Cancellation = result.Cancellation.Clone()
	if result.Continuation != nil {
		continuation := result.Continuation.Clone()
		cp.Continuation = &continuation
	}
	cp.StructuredResult = jsonvalue.Clone(result.StructuredResult)
	cp.StructuredResultPresent = jsonvalue.Present(result.StructuredResult, result.StructuredResultPresent)
	cp.ArtifactVerification = result.ArtifactVerification.Clone()
	cp.FailureDetail = workerexecution.CloneFailureDetail(result.FailureDetail)
	return cp
}

func deepCopyTokenMutationRecord(m interfaces.TokenMutationRecord) interfaces.TokenMutationRecord {
	cp := m
	if m.Token != nil {
		runtimeToken := factorytoken.FromWorker(*m.Token)
		tokenCopy := factorytoken.ToWorker(runtimeToken)
		cp.Token = &tokenCopy
	}
	return cp
}

func (e *FactoryEngine) tickOnce(ctx context.Context) (bool, bool, error) {
	release, err := e.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		return false, false, err
	}
	defer release()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.tick(ctx)
}

func (e *FactoryEngine) finishTerminationDrain() bool {
	e.mu.Lock()
	e.acceptingSubmits = false
	drained := e.drainChannels()
	if drained {
		e.acceptingSubmits = true
	}
	e.mu.Unlock()
	if drained {
		return false
	}
	return true
}

// Tick executes a single tick synchronously. Drains all pending channel events
// first, then runs the full tick cycle. For deterministic testing.
func (e *FactoryEngine) Tick(ctx context.Context) error {
	release, err := e.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		return err
	}
	defer release()
	e.mu.Lock()
	defer e.mu.Unlock()

	e.drainChannels()
	_, _, err = e.tick(ctx)
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
	if e == nil || e.runtimeState == nil {
		return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}
	}
	// Published snapshots are canonical runtime boundaries, not a TTL cache:
	// while the next tick is busy, they define the consistent boundary readers
	// can observe without waiting for that tick to finish.
	// Preserve the current state when the engine is idle, including explicit
	// owner-local mutations made by synchronous controls. TryLock is deliberate:
	// a busy tick must fall through to the last complete published boundary.
	if e.mu.TryLock() {
		snapshot := e.runtimeState.Snapshot()
		e.storePublishedSnapshot(snapshot)
		e.mu.Unlock()
		return cloneEngineStateSnapshot(snapshot)
	}
	if published := e.publishedSnapshot.Load(); published != nil {
		return cloneEngineStateSnapshot(*published)
	}
	// NewFactoryEngine always publishes an initial snapshot. Keep a safe
	// fallback for package-local zero-value fixtures that bypass the constructor.
	e.mu.Lock()
	snapshot := e.runtimeState.Snapshot()
	e.storePublishedSnapshot(snapshot)
	e.mu.Unlock()
	return cloneEngineStateSnapshot(snapshot)
}

// publishRuntimeSnapshotLocked records one detached, internally consistent
// boundary. The caller must hold e.mu; the published value is never returned
// directly because callers receive a detached clone.
func (e *FactoryEngine) publishRuntimeSnapshotLocked() {
	if e == nil || e.runtimeState == nil {
		return
	}
	e.storePublishedSnapshot(e.runtimeState.Snapshot())
}

func (e *FactoryEngine) storePublishedSnapshot(snapshot engineStateSnapshot) {
	e.publishedSnapshot.Store(&snapshot)
}

func cloneEngineStateSnapshot(snapshot engineStateSnapshot) engineStateSnapshot {
	clone := snapshot
	clone.Marking = cloneMarkingSnapshot(snapshot.Marking)
	clone.Dispatches = cloneDispatchEntries(snapshot.Dispatches)
	if snapshot.Results != nil {
		clone.Results = make([]workerexecution.WorkResult, len(snapshot.Results))
		for index := range snapshot.Results {
			clone.Results[index] = deepCopyWorkResult(snapshot.Results[index])
		}
	}
	if snapshot.DispatchHistory != nil {
		clone.DispatchHistory = make([]interfaces.CompletedDispatch, len(snapshot.DispatchHistory))
		for index := range snapshot.DispatchHistory {
			clone.DispatchHistory[index] = deepCopyCompletedDispatch(snapshot.DispatchHistory[index])
		}
	}
	if snapshot.ActiveThrottlePauses != nil {
		clone.ActiveThrottlePauses = append([]interfaces.ActiveThrottlePause(nil), snapshot.ActiveThrottlePauses...)
	}
	if snapshot.EnabledTransitions != nil {
		clone.EnabledTransitions = append([]interfaces.EnabledTransition(nil), snapshot.EnabledTransitions...)
	}
	return clone
}

func cloneMarkingSnapshot(snapshot petri.MarkingSnapshot) petri.MarkingSnapshot {
	clone := snapshot
	if snapshot.Tokens != nil {
		clone.Tokens = make(map[string]*factorytoken.Token, len(snapshot.Tokens))
		for id, token := range snapshot.Tokens {
			if token == nil {
				clone.Tokens[id] = nil
				continue
			}
			value := factorytoken.Clone(*token)
			clone.Tokens[id] = &value
		}
	}
	if snapshot.PlaceTokens != nil {
		clone.PlaceTokens = make(map[string][]string, len(snapshot.PlaceTokens))
		for placeID, tokenIDs := range snapshot.PlaceTokens {
			clone.PlaceTokens[placeID] = append([]string(nil), tokenIDs...)
		}
	}
	if snapshot.ParentChildRegistrations != nil {
		clone.ParentChildRegistrations = make(petri.ParentChildRegistrationProjection, len(snapshot.ParentChildRegistrations))
		for parentID, registration := range snapshot.ParentChildRegistrations {
			children := factorytoken.CloneSlice(registration.Children)
			clone.ParentChildRegistrations[parentID] = petri.ParentChildRegistrationSet{
				Children: children,
				Complete: registration.Complete,
			}
		}
	}
	if snapshot.TraceContext != nil {
		clone.TraceContext = make(map[string]string, len(snapshot.TraceContext))
		for key, value := range snapshot.TraceContext {
			clone.TraceContext[key] = value
		}
	}
	return clone
}

func cloneDispatchEntries(entries map[string]*interfaces.DispatchEntry) map[string]*interfaces.DispatchEntry {
	if entries == nil {
		return nil
	}
	clone := make(map[string]*interfaces.DispatchEntry, len(entries))
	for id, entry := range entries {
		if entry == nil {
			clone[id] = nil
			continue
		}
		copyEntry := *entry
		copyEntry.ExpectedArtifactContext = cloneExpectedArtifactTemplateContext(entry.ExpectedArtifactContext)
		copyEntry.ConsumedTokens = cloneWorkerTokens(entry.ConsumedTokens)
		if entry.HeldMutations != nil {
			copyEntry.HeldMutations = append([]interfaces.MarkingMutation(nil), entry.HeldMutations...)
		}
		clone[id] = &copyEntry
	}
	return clone
}

func cloneWorkerTokens(values []workerexecution.Token) []workerexecution.Token {
	return factorytoken.ToWorkerSlice(factorytoken.FromWorkerSlice(values))
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
	var dispatchWait <-chan struct{}
	if e.dispatchHook != nil {
		dispatchWait = e.dispatchHook.WaitCh()
	}
	for {
		select {
		case <-e.resultCh:
			e.handleResult()
			drained = true
		case <-dispatchWait:
			// A dispatch-result hook wake-up is itself a reason to rerun the
			// termination check. The hook may have drained activity without
			// leaving a result in the engine-owned result channel yet.
			return true
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
