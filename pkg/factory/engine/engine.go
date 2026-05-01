package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/portpowered/agent-factory/pkg/buffers"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/factory/subsystems"
	"github.com/portpowered/agent-factory/pkg/factory/token_transformer"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// FactoryEngine is the signal-driven graph (colored petri net) executor. It blocks on a select over
// wake channels and only wakes when something happens: a worker result arrives,
// new work is submitted, or the context is cancelled.
type FactoryEngine struct {
	state             *state.Net
	runtimeState      *RuntimeState
	subsystems        []subsystems.Subsystem // sorted by TickGroup
	logger            logging.Logger
	clock             factory.Clock
	resultCh          chan struct{}
	submitSignal      chan struct{}
	submissionHook    *queuedSubmissionHook
	submissionHooks   []factory.SubmissionHook
	submissionState   map[string]map[string]string
	workRequests      map[string]interfaces.WorkRequestSubmitResult
	recordSubmission  func(interfaces.FactorySubmissionRecord)
	recordWorkRequest func(int, interfaces.WorkRequestRecord)
	recordWorkInput   func(int, interfaces.SubmitRequest, interfaces.Token)
	recordDispatch    func(interfaces.FactoryDispatchRecord)
	recordCompletion  func(interfaces.FactoryCompletionRecord)
	recordResponse    func(int, interfaces.WorkResult, interfaces.CompletedDispatch)
	dispatchHandler   func(interfaces.WorkDispatch)
	dispatchHook      factory.DispatchResultHook
	resultHandler     func() // called when a result event is processed (e.g. decrement in-flight counter)
	mu                sync.Mutex
	transformer       *token_transformer.Transformer
	acceptingSubmits  bool
}

// NewFactoryEngine creates a new engine for the given net and marking.
// Subsystems are sorted by TickGroup on construction.
func NewFactoryEngine(
	n *state.Net,
	marking *petri.Marking,
	subs []subsystems.Subsystem,
	opts ...Option,
) *FactoryEngine {
	// Sort subsystems by TickGroup.
	sorted := make([]subsystems.Subsystem, len(subs))
	copy(sorted, subs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TickGroup() < sorted[j].TickGroup()
	})

	e := &FactoryEngine{
		state: n,
		runtimeState: &RuntimeState{
			Marking:      marking,
			Dispatches:   make(map[string]*interfaces.DispatchEntry),
			ResultBuffer: buffers.NewTypedBuffer[interfaces.WorkResult](64),
		},
		subsystems:       sorted,
		logger:           logging.NoopLogger{},
		clock:            factory.RealClock{},
		resultCh:         make(chan struct{}, 64),
		submitSignal:     make(chan struct{}, 1),
		submissionHook:   newQueuedSubmissionHook(),
		submissionState:  make(map[string]map[string]string),
		workRequests:     make(map[string]interfaces.WorkRequestSubmitResult),
		acceptingSubmits: true,
	}
	for _, opt := range opts {
		opt(e)
	}
	e.submissionHooks = append([]factory.SubmissionHook{e.submissionHook}, e.submissionHooks...)
	e.submissionHooks = sortedSubmissionHooks(e.submissionHooks)
	if e.transformer == nil {
		e.transformer = token_transformer.New(n.Places, n.WorkTypes)
	}
	e.clock = factory.EnsureClock(e.clock)
	return e
}

// drainPendingResults moves any buffered results into runtimeState.Results.
// Dispatch entries are NOT removed here — they remain visible to subsystems
// (especially TerminationCheck) until end-of-tick cleanup. This prevents
// false deadlock detection when async results arrive mid-tick.
// Must be called while holding engine.mu.
func (e *FactoryEngine) drainPendingResults() {
	buffer := e.runtimeState.ResultBuffer
	if buffer == nil || !buffer.HasData() {
		return
	}

	for {
		result, ok := buffer.Read()
		if !ok {
			return
		}
		e.appendObservedResult(result)
	}
}

func (e *FactoryEngine) appendObservedResult(result interfaces.WorkResult) {
	index := len(e.runtimeState.Results)
	e.runtimeState.Results = append(e.runtimeState.Results, result)
	if e.recordCompletion != nil {
		e.recordCompletion(interfaces.FactoryCompletionRecord{
			CompletionID: completionRecordID(e.runtimeState.TickCount, result.DispatchID, index),
			DispatchID:   result.DispatchID,
			ObservedTick: e.runtimeState.TickCount,
			Result:       result,
		})
	}
}

// retireCompletedDispatches removes dispatch entries for results that were
// processed during this tick and records them in DispatchHistory. Transitioner-
// supplied completion records take precedence; missing records fall back to a
// minimal timing summary so dispatch bookkeeping still completes.
func (e *FactoryEngine) retireCompletedDispatches(results []interfaces.WorkResult, completed map[string]interfaces.CompletedDispatch) {
	for _, r := range results {
		if entry, ok := e.runtimeState.Dispatches[r.DispatchID]; ok {
			completedDispatch, hasCompletedRecord := completed[r.DispatchID]
			if !hasCompletedRecord {
				now := e.clock.Now()
				completedDispatch = interfaces.CompletedDispatch{
					DispatchID:      entry.DispatchID,
					TransitionID:    entry.TransitionID,
					WorkstationName: entry.WorkstationName,
					Outcome:         r.Outcome,
					Reason:          completedDispatchReasonFromResult(r),
					ProviderSession: cloneProviderSession(r.ProviderSession),
					StartTime:       entry.StartTime,
					EndTime:         now,
					Duration:        now.Sub(entry.StartTime),
					ConsumedTokens:  entry.ConsumedTokens,
				}
			}
			e.runtimeState.DispatchHistory = append(e.runtimeState.DispatchHistory, completedDispatch)
			if e.recordResponse != nil {
				e.recordResponse(e.runtimeState.TickCount, workResultForCompletedDispatch(r, completedDispatch), completedDispatch)
			}
			delete(e.runtimeState.Dispatches, r.DispatchID)
			if e.runtimeState.InFlightCount > 0 {
				e.runtimeState.InFlightCount--
			}
		}
	}
}

func completedDispatchReasonFromResult(result interfaces.WorkResult) string {
	switch result.Outcome {
	case interfaces.OutcomeFailed:
		return result.Error
	case interfaces.OutcomeContinue:
		return result.Feedback
	case interfaces.OutcomeRejected:
		return result.Feedback
	default:
		return ""
	}
}

func workResultForCompletedDispatch(result interfaces.WorkResult, completed interfaces.CompletedDispatch) interfaces.WorkResult {
	result.Outcome = completed.Outcome
	switch completed.Outcome {
	case interfaces.OutcomeFailed:
		result.Error = completed.Reason
	case interfaces.OutcomeContinue:
		result.Feedback = completed.Reason
	case interfaces.OutcomeRejected:
		result.Feedback = completed.Reason
	}
	return result
}

// NotifyResult wakes the engine after a WorkResult is enqueued so the engine
// ticks and routes the result. Non-blocking: drops if the buffer is full.
func (e *FactoryEngine) NotifyResult() {
	select {
	case e.resultCh <- struct{}{}:
	default:
	}
}

// SubmitWorkRequest validates and enqueues a canonical work request batch.
// Repeated request IDs are treated as idempotent no-ops.
func (e *FactoryEngine) SubmitWorkRequest(context context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	e.mu.Lock()
	if existing, exists := e.workRequests[request.RequestID]; exists && request.RequestID != "" {
		e.mu.Unlock()
		existing.Accepted = false
		return existing, nil
	}
	e.mu.Unlock()

	normalized, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes:    e.validWorkTypes(),
		ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes),
	})
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	if request.RequestID == "" && len(normalized) > 0 {
		request.RequestID = normalized[0].RequestID
	}
	return e.submitNormalizedWorkRequest(context, request.RequestID, normalized)
}

func (e *FactoryEngine) submitNormalizedWorkRequest(context context.Context, requestID string, work []interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	select {
	case <-context.Done():
		return interfaces.WorkRequestSubmitResult{}, context.Err()
	default:
	}

	traceID := ""
	if len(work) > 0 {
		traceID = work[0].TraceID
	}
	result := interfaces.WorkRequestSubmitResult{
		RequestID: requestID,
		TraceID:   traceID,
		Accepted:  true,
	}

	e.mu.Lock()
	if !e.acceptingSubmits {
		e.mu.Unlock()
		return interfaces.WorkRequestSubmitResult{}, fmt.Errorf("engine has terminated")
	}
	if existing, exists := e.workRequests[requestID]; exists {
		e.mu.Unlock()
		existing.Accepted = false
		return existing, nil
	}
	e.workRequests[requestID] = result
	e.submissionHook.enqueue(work)
	e.mu.Unlock()

	select {
	case e.submitSignal <- struct{}{}:
	default:
	}

	return result, nil
}

func (e *FactoryEngine) validWorkTypes() map[string]bool {
	valid := make(map[string]bool, len(e.state.WorkTypes))
	for workTypeID := range e.state.WorkTypes {
		valid[workTypeID] = true
	}
	return valid
}

// Run is the main execution loop. Blocks on a select over wake channels until
// ctx is cancelled or the marking has no more actionable tokens.
// portos:func-length-exception owner=agent-factory reason=legacy-engine-run-loop review=2026-07-18 removal=split-initial-drain-and-wait-loop-before-next-engine-loop-change
func (e *FactoryEngine) Run(ctx context.Context) error {
	e.logger.Info("engine started")
	defer func() {
		e.mu.Lock()
		e.acceptingSubmits = false
		e.mu.Unlock()
	}()

	// Initial drain-and-tick pass: process any pending submissions or state
	// mutations from manual Tick() calls that happened before Run was called.
	// Without this, tokens left in intermediate states by pre-Run ticks would
	// never advance because the select loop waits for new channel events.
	e.mu.Lock()
	e.drainChannels()
	e.mu.Unlock()
	for {
		e.mu.Lock()
		mutated, shouldTerminate, err := e.tick(ctx)
		e.mu.Unlock()
		if err != nil {
			e.logger.Error("engine initial tick error", "error", err)
			return err
		}
		if shouldTerminate {
			e.mu.Lock()
			e.acceptingSubmits = false
			drained := e.drainChannels()
			if drained {
				e.acceptingSubmits = true
			}
			e.mu.Unlock()
			if drained {
				continue
			}
			e.logger.Info("engine terminated during initial tick pass")
			return nil
		}
		if !mutated {
			break
		}
	}

	var dispatchWait <-chan struct{}
	if e.dispatchHook != nil {
		dispatchWait = e.dispatchHook.WaitCh()
	}
	for {
		select {
		case <-e.resultCh:
			e.mu.Lock()
			e.logger.Info("engine: result signal received")
			e.handleResult()
			e.mu.Unlock()
		case <-e.submitSignal:
			e.logger.Info("engine: submission hook wake-up received")
		case <-dispatchWait:
			e.logger.Info("engine: dispatch/result hook wake-up received")
		case <-ctx.Done():
			e.logger.Info("engine stopped", "reason", ctx.Err())
			return ctx.Err()
		}

		// Continue ticking until no more mutations are produced (quiescent)
		// or termination is signaled.
		for {
			e.mu.Lock()
			mutated, shouldTerminate, err := e.tick(ctx)
			e.mu.Unlock()
			if err != nil {
				e.logger.Error("engine tick error", "error", err)
				return err
			}
			if shouldTerminate {
				e.mu.Lock()
				e.acceptingSubmits = false
				drained := e.drainChannels()
				if drained {
					e.acceptingSubmits = true
				}
				e.mu.Unlock()
				if drained {
					continue
				}
				e.logger.Info("engine terminated")
				return nil
			}
			if !mutated {
				break
			}
		}
	}
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
func (e *FactoryEngine) GetResultBuffer() *buffers.TypedBuffer[interfaces.WorkResult] {
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

// tick runs a single tick cycle: execute subsystems in order, apply mutations
// atomically between each subsystem execution. Returns (mutated, shouldTerminate, error).
// mutated is true if any mutations were applied (another tick may be needed).
// shouldTerminate is true if the TerminationCheck subsystem signaled completion.
// portos:func-length-exception owner=agent-factory reason=legacy-engine-tick-orchestration review=2026-07-18 removal=split-hook-dispatch-and-retirement-phases-before-next-tick-pipeline-change
func (e *FactoryEngine) tick(ctx context.Context) (bool, bool, error) {
	e.runtimeState.TickCount++
	e.runtimeState.Marking.TickCount = e.runtimeState.TickCount
	if logicalClock, ok := e.clock.(factory.LogicalClock); ok {
		logicalClock.SetTick(e.runtimeState.TickCount)
	}

	// Drain any results enqueued since the last tick (from async pool bridge
	// or previous sync dispatch handlers).
	e.drainPendingResults()
	dispatchResults, err := e.invokeDispatchResultHook(ctx)
	if err != nil {
		return false, false, err
	}
	hookSubmissions, keepAlive, err := e.invokeSubmissionHooks(ctx)
	if err != nil {
		return false, false, err
	}

	rtSnapshot := e.runtimeState.Snapshot()

	mutated := false
	shouldTerminate := false
	totalDispatches := 0
	completedDispatches := make(map[string]interfaces.CompletedDispatch)
	if hookSubmissions > 0 || dispatchResults > 0 || keepAlive {
		mutated = true
	}

	e.logger.Info("engine: [START] running engine tick", "tick", e.runtimeState.TickCount)
	for _, sub := range e.subsystems {
		// Drain pending results before subsystems that can process them
		// (TickGroup <= Transitioner). This picks up results written by sync
		// dispatchers into the shared runtime result buffer in time
		// for the collector pipeline. We intentionally skip draining for
		// subsystems after the Transitioner to prevent async pool results from
		// being drained late, added to runtimeState.Results, and then cleared
		// at end of tick without ever being processed.
		if sub.TickGroup() <= subsystems.Transitioner {
			e.drainPendingResults()
			if len(e.runtimeState.Results) > 0 && sub.TickGroup() > subsystems.Dispatcher {
				rtSnapshot = e.runtimeState.Snapshot()
			}
		}

		e.logger.Debug("engine: executing subsystem", "subsystem", sub.TickGroup())
		result, err := sub.Execute(ctx, &rtSnapshot)
		if err != nil {
			return false, false, fmt.Errorf("subsystem tick-group %d: %w", sub.TickGroup(), err)
		}
		if result == nil {
			continue
		}

		if result.ShouldTerminate {
			shouldTerminate = true
		}

		// Apply mutations atomically.
		if len(result.Mutations) > 0 {
			if err := applyMutations(e.runtimeState.Marking, e.state.Places, result.Mutations); err != nil {
				return false, false, fmt.Errorf("applying mutations from tick-group %d: %w", sub.TickGroup(), err)
			}
			// Re-snapshot after mutations for subsequent subsystems.
			rtSnapshot = e.runtimeState.Snapshot()
			mutated = true
		}
		if len(result.GeneratedBatches) > 0 {
			if _, err := e.processGeneratedSubmissionBatches(result.GeneratedBatches, "tick-result"); err != nil {
				return false, false, fmt.Errorf("processing generated batches from tick-group %d: %w", sub.TickGroup(), err)
			}
			rtSnapshot = e.runtimeState.Snapshot()
			mutated = true
		}

		if e.dispatchHandler != nil || e.dispatchHook != nil {
			dispatched := false
			for _, rec := range result.Dispatches {
				now := e.clock.Now()
				rec.Dispatch.Execution.DispatchCreatedTick = e.runtimeState.TickCount
				rec.Dispatch.Execution.CurrentTick = e.runtimeState.TickCount
				e.runtimeState.Dispatches[rec.Dispatch.DispatchID] = &interfaces.DispatchEntry{
					DispatchID:      rec.Dispatch.DispatchID,
					TransitionID:    rec.Dispatch.TransitionID,
					WorkstationName: rec.Dispatch.WorkstationName,
					StartTime:       now,
					ConsumedTokens:  workers.WorkDispatchInputTokens(rec.Dispatch),
					HeldMutations:   rec.Mutations,
				}
				e.runtimeState.InFlightCount++
				if e.recordDispatch != nil {
					e.recordDispatch(interfaces.FactoryDispatchRecord{
						DispatchID:     rec.Dispatch.DispatchID,
						CreatedTick:    e.runtimeState.TickCount,
						Dispatch:       rec.Dispatch,
						HeldMutations:  rec.Mutations,
						ConsumedTokens: consumedTokenIDs(workers.WorkDispatchInputTokens(rec.Dispatch)),
					})
				}
				if e.dispatchHook != nil {
					if err := e.dispatchHook.SubmitDispatch(ctx, rec.Dispatch); err != nil {
						return false, false, fmt.Errorf("dispatch/result hook submit dispatch %q: %w", rec.Dispatch.DispatchID, err)
					}
				}
				if e.dispatchHandler != nil {
					e.dispatchHandler(rec.Dispatch)
				}
				totalDispatches++
				dispatched = true
			}

			// Re-snapshot after dispatches: dispatch entries were added to
			// runtimeState.Dispatches, and sync handlers may have enqueued
			// results. Subsequent subsystems (especially TerminationCheck)
			// must see in-flight dispatches to avoid false deadlock detection.
			if dispatched {
				e.drainPendingResults()
				rtSnapshot = e.runtimeState.Snapshot()
			}
		}

		for _, completedDispatch := range result.CompletedDispatches {
			completedDispatches[completedDispatch.DispatchID] = completedDispatch
		}
		if result.ThrottlePausesObserved {
			e.runtimeState.ActiveThrottlePauses = cloneActiveThrottlePauses(result.ActiveThrottlePauses)
			rtSnapshot = e.runtimeState.Snapshot()
		}
	}

	// Retire dispatch entries for results processed in this tick. This must
	// happen AFTER all subsystems (including TerminationCheck) have run, so
	// dispatch entries remain visible throughout the tick — preventing false
	// deadlock detection when async results arrive mid-tick.
	e.retireCompletedDispatches(e.runtimeState.Results, completedDispatches)

	// Clear processed results at end of tick so they don't carry over to the
	// next tick cycle.
	e.runtimeState.Results = nil

	if keepAlive {
		shouldTerminate = false
	}

	e.logger.Info("engine: [END] tick complete",
		"tick", e.runtimeState.TickCount,
		"mutations", mutated,
		"dispatches", totalDispatches,
		"shouldTerminate", shouldTerminate,
		"tokens", len(rtSnapshot.Marking.Tokens))

	return mutated, shouldTerminate, nil
}

func cloneActiveThrottlePauses(pauses []interfaces.ActiveThrottlePause) []interfaces.ActiveThrottlePause {
	if pauses == nil {
		return nil
	}
	clone := make([]interfaces.ActiveThrottlePause, len(pauses))
	copy(clone, pauses)
	return clone
}

func (e *FactoryEngine) invokeDispatchResultHook(ctx context.Context) (int, error) {
	if e.dispatchHook == nil {
		return 0, nil
	}

	result, err := e.dispatchHook.OnTick(ctx, interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: e.runtimeState.Snapshot(),
	})
	if err != nil {
		return 0, fmt.Errorf("dispatch/result hook: %w", err)
	}
	for _, workResult := range result.Results {
		e.appendObservedResult(workResult)
	}
	return len(result.Results), nil
}

func (e *FactoryEngine) invokeSubmissionHooks(ctx context.Context) (int, bool, error) {
	if len(e.submissionHooks) == 0 {
		return 0, false, nil
	}

	snapshot := e.runtimeState.Snapshot()
	totalSubmissions := 0
	keepAlive := false
	for _, hook := range e.submissionHooks {
		hookName := hook.Name()
		result, err := hook.OnTick(ctx, interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
			Snapshot:          snapshot,
			ContinuationState: copyHookState(e.submissionState[hookName]),
		})
		if err != nil {
			return 0, false, fmt.Errorf("submission hook %q: %w", hookName, err)
		}
		e.submissionState[hookName] = copyHookState(result.ContinuationState)
		if result.KeepAlive {
			keepAlive = true
		}

		if len(result.Results) > 0 {
			e.runtimeState.Results = append(e.runtimeState.Results, result.Results...)
		}
		if len(result.GeneratedBatches) > 0 {
			generated := make([]interfaces.GeneratedSubmissionBatch, len(result.GeneratedBatches))
			copy(generated, result.GeneratedBatches)
			for i := range generated {
				if generated[i].Metadata.Source == "" {
					generated[i].Metadata.Source = hookName
				}
			}
			count, err := e.processGeneratedSubmissionBatches(generated, hookName)
			if err != nil {
				return 0, false, fmt.Errorf("submission hook %q generated batches: %w", hookName, err)
			}
			totalSubmissions += count
			snapshot = e.runtimeState.Snapshot()
		}
	}
	return totalSubmissions, keepAlive, nil
}

func (e *FactoryEngine) processGeneratedSubmissionBatches(batches []interfaces.GeneratedSubmissionBatch, defaultSource string) (int, error) {
	total := 0
	for i := range batches {
		batch := batches[i]
		source := batch.Metadata.Source
		if source == "" {
			source = defaultSource
		}
		if source == "" {
			source = "generated-batch"
		}
		normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
			ValidWorkTypes:    e.validWorkTypes(),
			ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes),
		})
		if err != nil {
			return total, err
		}

		requestID := ""
		if len(normalized) > 0 {
			requestID = normalized[0].RequestID
		}
		if _, exists := e.workRequests[requestID]; exists && requestID != "" && source != externalSubmissionHookName {
			continue
		}

		now := e.clock.Now()
		tokens := make([]*interfaces.Token, 0, len(normalized))
		for _, req := range normalized {
			token, err := e.transformer.InitialTokenFromSubmit(req, now)
			if err != nil {
				return total, err
			}
			tokens = append(tokens, token)
		}

		traceID := ""
		if len(normalized) > 0 {
			traceID = normalized[0].TraceID
		}
		e.workRequests[requestID] = interfaces.WorkRequestSubmitResult{
			RequestID: requestID,
			TraceID:   traceID,
			Accepted:  true,
		}

		if e.recordWorkRequest != nil {
			record := factory.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
			record.RelationContext = append([]interfaces.WorkRelation(nil), batch.Metadata.RelationContext...)
			record.ParentLineage = append([]string(nil), batch.Metadata.ParentLineage...)
			e.recordWorkRequest(
				e.runtimeState.TickCount,
				record,
			)
		}
		for index, token := range tokens {
			if e.recordSubmission != nil {
				e.recordSubmission(interfaces.FactorySubmissionRecord{
					SubmissionID: submissionRecordID(e.runtimeState.TickCount, source, index),
					ObservedTick: e.runtimeState.TickCount,
					Request:      normalized[index],
					Source:       source,
				})
			}
			e.runtimeState.Marking.AddToken(token)
			if e.recordWorkInput != nil {
				e.recordWorkInput(e.runtimeState.TickCount, normalized[index], *token)
			}
		}
		total += len(tokens)
	}
	return total, nil
}

// injectTokens creates tokens from submit requests and places them in INITIAL places.
func (e *FactoryEngine) injectTokens(requests []interfaces.SubmitRequest) {
	e.logger.Info("engine: injecting tokens", "count", len(requests))
	for _, req := range requests {
		token, err := e.transformer.InitialTokenFromSubmit(req, e.clock.Now())
		if err != nil {
			e.logger.Error("engine: failed to convert submit request to token", "work_type_id", req.WorkTypeID, "error", err)
			continue
		}
		e.runtimeState.Marking.AddToken(token)
		if e.recordWorkInput != nil {
			e.recordWorkInput(e.runtimeState.TickCount, req, *token)
		}
	}
}
