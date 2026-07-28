package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// FactoryEngine is the signal-driven graph (colored petri net) executor. It blocks on a select over
// wake channels and only wakes when something happens: a worker result arrives,
// new work is submitted, or the context is cancelled.
type FactoryEngine struct {
	state                 *state.Net
	runtimeState          *RuntimeState
	subsystems            []subsystems.Subsystem // sorted by TickGroup
	logger                logging.Logger
	clock                 factory.Clock
	workRequestIDs        work.RequestIDGenerator
	resultCh              chan struct{}
	submitSignal          chan struct{}
	submissionHook        *queuedSubmissionHook
	submissionHooks       []factory.SubmissionHook
	submissionState       map[string]map[string]string
	workRequests          map[string]workdomain.WorkRequestSubmitResult
	projectionWaiters     map[string]chan struct{}
	runLoopActive         bool
	recordSubmission      func(work.FactorySubmissionRecord)
	recordWorkRequest     func(int, work.WorkRequestRecord)
	recordWorkInput       func(int, workdomain.SubmitRequest, factorytoken.Token)
	recordDispatch        func(interfaces.FactoryDispatchRecord)
	recordCompletion      func(interfaces.FactoryCompletionRecord)
	recordResponse        func(int, workerexecution.WorkResult, interfaces.CompletedDispatch)
	recordPetriMutations  func([]interfaces.TokenMutationRecord) error
	dispatchHandler       func(work.WorkDispatch)
	dispatchHook          factory.DispatchResultHook
	resultHandler         func() // called when a result event is processed (e.g. decrement in-flight counter)
	automaticTicksPaused  func() bool
	onResultBufferDrained func(drainedCount int)
	mu                    sync.Mutex
	transformer           *token_transformer.Transformer
	acceptingSubmits      bool
}

// NewFactoryEngine creates a new engine for the given net and marking.
// Subsystems are sorted by TickGroup on construction.
func NewFactoryEngine(
	n *state.Net,
	marking *petri.Marking,
	subs []subsystems.Subsystem,
	logger logging.Logger,
	clock factory.Clock,
	workRequestIDs work.RequestIDGenerator,
	dispatchHandler func(work.WorkDispatch),
	dispatchHook factory.DispatchResultHook,
	transformer *token_transformer.Transformer,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
	submissionHooks []factory.SubmissionHook,
	recordSubmission func(work.FactorySubmissionRecord),
	recordWorkRequest func(int, work.WorkRequestRecord),
	recordWorkInput func(int, workdomain.SubmitRequest, factorytoken.Token),
	recordDispatch func(interfaces.FactoryDispatchRecord),
	recordCompletion func(interfaces.FactoryCompletionRecord),
	recordResponse func(int, workerexecution.WorkResult, interfaces.CompletedDispatch),
	recordPetriMutations func([]interfaces.TokenMutationRecord) error,
	automaticTicksPaused func() bool,
	onResultBufferDrained func(int),
) (*FactoryEngine, error) {
	if clock == nil {
		return nil, fmt.Errorf("Factory Runtime engine clock is required")
	}
	if workRequestIDs == nil {
		return nil, fmt.Errorf("Work Request ID generator is required")
	}
	if transformer == nil {
		return nil, fmt.Errorf("Factory Runtime token transformer is required")
	}
	// Sort subsystems by TickGroup.
	sorted := make([]subsystems.Subsystem, len(subs))
	copy(sorted, subs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TickGroup() < sorted[j].TickGroup()
	})

	if resultBuffer == nil {
		resultBuffer = buffers.NewTypedBuffer[workerexecution.WorkResult](64)
	}
	e := &FactoryEngine{
		state: n,
		runtimeState: &RuntimeState{
			Marking:      marking,
			Dispatches:   make(map[string]*interfaces.DispatchEntry),
			ResultBuffer: resultBuffer,
		},
		subsystems:            sorted,
		logger:                logging.EnsureLogger(logger),
		clock:                 clock,
		workRequestIDs:        workRequestIDs,
		resultCh:              make(chan struct{}, 64),
		submitSignal:          make(chan struct{}, 1),
		submissionHook:        newQueuedSubmissionHook(),
		submissionHooks:       append([]factory.SubmissionHook(nil), submissionHooks...),
		submissionState:       make(map[string]map[string]string),
		workRequests:          make(map[string]workdomain.WorkRequestSubmitResult),
		projectionWaiters:     make(map[string]chan struct{}),
		recordSubmission:      recordSubmission,
		recordWorkRequest:     recordWorkRequest,
		recordWorkInput:       recordWorkInput,
		recordDispatch:        recordDispatch,
		recordCompletion:      recordCompletion,
		recordResponse:        recordResponse,
		recordPetriMutations:  recordPetriMutations,
		dispatchHandler:       dispatchHandler,
		dispatchHook:          dispatchHook,
		automaticTicksPaused:  automaticTicksPaused,
		onResultBufferDrained: onResultBufferDrained,
		transformer:           transformer,
		acceptingSubmits:      true,
	}
	e.submissionHooks = append([]factory.SubmissionHook{e.submissionHook}, e.submissionHooks...)
	e.submissionHooks = sortedSubmissionHooks(e.submissionHooks)
	return e, nil
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

	drained := 0
	for {
		result, ok := buffer.Read()
		if !ok {
			break
		}
		e.appendObservedResult(result)
		drained++
	}
	if drained > 0 && e.onResultBufferDrained != nil {
		e.onResultBufferDrained(drained)
	}
}

func (e *FactoryEngine) appendObservedResult(result workerexecution.WorkResult) {
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
func (e *FactoryEngine) retireCompletedDispatches(results []workerexecution.WorkResult, completed map[string]interfaces.CompletedDispatch) {
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
					ProviderSession: workerexecution.CloneProviderSessionMetadata(r.ProviderSession),
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

func completedDispatchReasonFromResult(result workerexecution.WorkResult) string {
	switch result.Outcome {
	case workerexecution.OutcomeFailed:
		return result.Error
	case workerexecution.OutcomeContinue:
		return result.Feedback
	case workerexecution.OutcomeRejected:
		return result.Feedback
	default:
		return ""
	}
}

func workResultForCompletedDispatch(result workerexecution.WorkResult, completed interfaces.CompletedDispatch) workerexecution.WorkResult {
	result.Outcome = completed.Outcome
	switch completed.Outcome {
	case workerexecution.OutcomeFailed:
		result.Error = completed.Reason
	case workerexecution.OutcomeContinue:
		result.Feedback = completed.Reason
	case workerexecution.OutcomeRejected:
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

func (e *FactoryEngine) wakeForOperatorControl() {
	select {
	case e.submitSignal <- struct{}{}:
	default:
	}
	if hook, ok := e.dispatchHook.(factory.DispatchResultHookWakeSignaler); ok && hook.HasBufferedResults() {
		hook.SignalBufferedResults()
	}
}

// WakeForPendingProcessing signals the engine loop when buffered submissions or
// worker results are waiting to be processed.
func (e *FactoryEngine) WakeForPendingProcessing() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.wakeForPendingProcessing()
}

func (e *FactoryEngine) wakeForPendingProcessing() {
	if !e.hasBufferedInputs() {
		return
	}
	select {
	case e.submitSignal <- struct{}{}:
	default:
	}
	if hook, ok := e.dispatchHook.(factory.DispatchResultHookWakeSignaler); ok && hook.HasBufferedResults() {
		hook.SignalBufferedResults()
	}
}

func (e *FactoryEngine) hasBufferedInputs() bool {
	if e.submissionHook != nil && len(e.submissionHook.batches) > 0 {
		return true
	}
	buffer := e.runtimeState.ResultBuffer
	if buffer != nil && buffer.HasData() {
		return true
	}
	if e.dispatchHook != nil && e.dispatchHook.HasPendingResults() {
		return true
	}
	return false
}

// SubmitWorkRequest validates and enqueues a canonical work request batch.
// Repeated request IDs are treated as idempotent no-ops.
func (e *FactoryEngine) SubmitWorkRequest(context context.Context, request workdomain.WorkRequest) (workdomain.WorkRequestSubmitResult, error) {
	e.mu.Lock()
	if existing, exists := e.workRequests[request.RequestID]; exists && request.RequestID != "" {
		e.mu.Unlock()
		existing.Accepted = false
		return existing, nil
	}
	e.mu.Unlock()

	normalized, err := workdomain.NormalizeWorkRequest(request, workdomain.WorkRequestNormalizeOptions{
		ValidWorkTypes:    e.validWorkTypes(),
		ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes),
		IDGenerator:       e.workRequestIDs,
	})
	if err != nil {
		return workdomain.WorkRequestSubmitResult{}, err
	}
	if request.RequestID == "" && len(normalized) > 0 {
		request.RequestID = normalized[0].RequestID
	}
	return e.submitNormalizedWorkRequest(context, request.RequestID, normalized)
}

func (e *FactoryEngine) submitNormalizedWorkRequest(context context.Context, requestID string, work []workdomain.SubmitRequest) (workdomain.WorkRequestSubmitResult, error) {
	select {
	case <-context.Done():
		return workdomain.WorkRequestSubmitResult{}, context.Err()
	default:
	}

	result := workdomain.WorkRequestSubmitResultFromNormalized(requestID, work, true)

	e.mu.Lock()
	if !e.acceptingSubmits {
		e.mu.Unlock()
		return workdomain.WorkRequestSubmitResult{}, fmt.Errorf("engine has terminated")
	}
	if existing, exists := e.workRequests[requestID]; exists {
		e.mu.Unlock()
		existing.Accepted = false
		return existing, nil
	}
	if e.conflictingMaterializedWorkID(work) != "" {
		e.mu.Unlock()
		return workdomain.WorkRequestSubmitResultFromNormalized(requestID, work, false), nil
	}
	e.workRequests[requestID] = result
	e.submissionHook.enqueue(work)
	awaitProjection := e.shouldAwaitObservableProjection()
	var projectionWait <-chan struct{}
	if awaitProjection {
		waitCh := make(chan struct{}, 1)
		e.projectionWaiters[requestID] = waitCh
		projectionWait = waitCh
	}
	e.mu.Unlock()

	select {
	case e.submitSignal <- struct{}{}:
	default:
	}

	if awaitProjection {
		if err := e.awaitObservableProjection(context, requestID, projectionWait); err != nil {
			return workdomain.WorkRequestSubmitResult{}, err
		}
	}

	return result, nil
}

func (e *FactoryEngine) conflictingMaterializedWorkID(work []workdomain.SubmitRequest) string {
	for _, req := range work {
		if req.WorkID == "" {
			continue
		}
		if e.visibleWorkIDAlreadyMaterialized(req.WorkID) {
			return req.WorkID
		}
	}
	return ""
}

func (e *FactoryEngine) visibleWorkIDAlreadyMaterialized(workID string) bool {
	if workID == "" {
		return false
	}
	if _, ok := findWorkTokenByID(e.runtimeState.Marking.Tokens, workID); ok {
		return true
	}
	return workIDInActiveDispatches(e.runtimeState.Dispatches, workID)
}

func (e *FactoryEngine) shouldAwaitObservableProjection() bool {
	if !e.runLoopActive {
		return false
	}
	if e.automaticTicksPaused != nil && e.automaticTicksPaused() {
		return false
	}
	return true
}

func (e *FactoryEngine) awaitObservableProjection(
	ctx context.Context,
	requestID string,
	waitCh <-chan struct{},
) error {
	select {
	case <-waitCh:
		return nil
	case <-ctx.Done():
		e.mu.Lock()
		delete(e.projectionWaiters, requestID)
		e.mu.Unlock()
		return ctx.Err()
	}
}

func (e *FactoryEngine) signalObservableProjection(requestID string) {
	if requestID == "" {
		return
	}
	waitCh, ok := e.projectionWaiters[requestID]
	if !ok {
		return
	}
	delete(e.projectionWaiters, requestID)
	select {
	case waitCh <- struct{}{}:
	default:
	}
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
func (e *FactoryEngine) Run(ctx context.Context) error {
	e.logger.Info("engine started")
	e.mu.Lock()
	e.runLoopActive = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.runLoopActive = false
		e.acceptingSubmits = false
		e.mu.Unlock()
	}()

	// Initial drain-and-tick pass: process any pending submissions or state
	// mutations from manual Tick() calls that happened before Run was called.
	// Without this, tokens left in intermediate states by pre-Run ticks would
	// never advance because the select loop waits for new channel events.
	terminated, err := e.runInitialTickPass(ctx)
	if err != nil {
		e.logger.Error("engine initial tick error", "error", err)
		return err
	}
	if terminated {
		e.logger.Info("engine terminated during initial tick pass")
		return nil
	}

	var dispatchWait <-chan struct{}
	if e.dispatchHook != nil {
		dispatchWait = e.dispatchHook.WaitCh()
	}
	for {
		if err := e.waitForEngineSignal(ctx, dispatchWait); err != nil {
			return err
		}
		terminated, err := e.runUntilQuiescent(ctx)
		if err != nil {
			e.logger.Error("engine tick error", "error", err)
			return err
		}
		if terminated {
			e.logger.Info("engine terminated")
			return nil
		}
	}
}

func (e *FactoryEngine) runInitialTickPass(ctx context.Context) (bool, error) {
	e.mu.Lock()
	e.drainChannels()
	e.mu.Unlock()
	return e.runUntilQuiescent(ctx)
}

func (e *FactoryEngine) waitForEngineSignal(ctx context.Context, dispatchWait <-chan struct{}) error {
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
	return nil
}

func (e *FactoryEngine) runUntilQuiescent(ctx context.Context) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		mutated, shouldTerminate, err := e.tickOnce(ctx)
		if err != nil {
			return false, err
		}
		if shouldTerminate {
			return e.finishTerminationDrain()
		}
		if !mutated {
			e.mu.Lock()
			stillBuffered := e.hasBufferedInputs()
			paused := e.automaticTicksPaused != nil && e.automaticTicksPaused()
			if stillBuffered && !paused {
				e.wakeForPendingProcessing()
			}
			e.mu.Unlock()
			if stillBuffered && !paused {
				continue
			}
			return false, nil
		}
	}
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

// tick runs a single tick cycle: execute subsystems in order, apply mutations
// atomically between each subsystem execution. Returns (mutated, shouldTerminate, error).
// mutated is true if any mutations were applied (another tick may be needed).
// shouldTerminate is true if the TerminationCheck subsystem signaled completion.
func (e *FactoryEngine) tick(ctx context.Context) (bool, bool, error) {
	if e.automaticTicksPaused != nil && e.automaticTicksPaused() {
		e.logger.Debug("engine: skipping automatic tick while factory is paused")
		return false, false, nil
	}

	rtSnapshot, mutated, keepAlive, err := e.beginTick(ctx)
	if err != nil {
		return false, false, err
	}
	shouldTerminate := false
	totalDispatches := 0
	completedDispatches := make(map[string]interfaces.CompletedDispatch)
	e.logger.Info("engine: [START] running engine tick", "tick", e.runtimeState.TickCount)
	for _, sub := range e.subsystems {
		rtSnapshot = e.refreshSnapshotBeforeSubsystem(sub, rtSnapshot)
		result, err := e.executeSubsystem(ctx, sub, &rtSnapshot)
		if err != nil {
			return false, false, fmt.Errorf("subsystem tick-group %d: %w", sub.TickGroup(), err)
		}
		if result == nil {
			continue
		}
		if err := e.recordCompletedPetriMutations(result.CompletedDispatches); err != nil {
			return false, false, err
		}

		if result.ShouldTerminate {
			shouldTerminate = true
		}

		rtSnapshot, mutated, err = e.applySubsystemResult(ctx, sub.TickGroup(), result, rtSnapshot, mutated)
		if err != nil {
			return false, false, err
		}
		dispatched, updatedSnapshot, err := e.forwardDispatches(ctx, result.Dispatches, rtSnapshot)
		if err != nil {
			return false, false, err
		}
		if dispatched {
			rtSnapshot = updatedSnapshot
			totalDispatches += len(result.Dispatches)
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
	shouldTerminate = e.finishTick(keepAlive, shouldTerminate, totalDispatches, completedDispatches, rtSnapshot, mutated)
	return mutated, shouldTerminate, nil
}

func (e *FactoryEngine) recordCompletedPetriMutations(completed []interfaces.CompletedDispatch) error {
	if e.recordPetriMutations == nil {
		return nil
	}
	for i := range completed {
		if len(completed[i].OutputMutations) == 0 {
			continue
		}
		if err := e.recordPetriMutations(completed[i].OutputMutations); err != nil {
			return fmt.Errorf("record completed dispatch %q Petri mutations: %w", completed[i].DispatchID, err)
		}
	}
	return nil
}

func (e *FactoryEngine) beginTick(ctx context.Context) (interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], bool, bool, error) {
	e.runtimeState.TickCount++
	e.runtimeState.Marking.TickCount = e.runtimeState.TickCount
	if logicalClock, ok := e.clock.(factory.LogicalClock); ok {
		logicalClock.SetTick(e.runtimeState.TickCount)
	}
	e.drainPendingResults()
	dispatchResults, err := e.invokeDispatchResultHook(ctx)
	if err != nil {
		return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}, false, false, err
	}
	hookSubmissions, keepAlive, err := e.invokeSubmissionHooks(ctx)
	if err != nil {
		return interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{}, false, false, err
	}
	mutated := hookSubmissions > 0 || dispatchResults > 0 || keepAlive
	return e.runtimeState.Snapshot(), mutated, keepAlive, nil
}

func (e *FactoryEngine) refreshSnapshotBeforeSubsystem(sub subsystems.Subsystem, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	if sub.TickGroup() > subsystems.Transitioner {
		return snapshot
	}
	e.drainPendingResults()
	if len(e.runtimeState.Results) > 0 && sub.TickGroup() > subsystems.Dispatcher {
		return e.runtimeState.Snapshot()
	}
	return snapshot
}

func (e *FactoryEngine) executeSubsystem(ctx context.Context, sub subsystems.Subsystem, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	e.logger.Debug("engine: executing subsystem", "subsystem", sub.TickGroup())
	return sub.Execute(ctx, snapshot)
}

func (e *FactoryEngine) applySubsystemResult(ctx context.Context, tickGroup subsystems.TickGroup, result *interfaces.TickResult, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], mutated bool) (interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], bool, error) {
	if len(result.Mutations) > 0 {
		if err := applyMutations(e.runtimeState.Marking, e.state.Places, result.Mutations, e.clock.Now()); err != nil {
			return snapshot, mutated, fmt.Errorf("applying mutations from tick-group %d: %w", tickGroup, err)
		}
		snapshot = e.runtimeState.Snapshot()
		mutated = true
	}
	if len(result.GeneratedBatches) > 0 {
		if _, err := e.processGeneratedSubmissionBatches(result.GeneratedBatches, "tick-result"); err != nil {
			return snapshot, mutated, fmt.Errorf("processing generated batches from tick-group %d: %w", tickGroup, err)
		}
		snapshot = e.runtimeState.Snapshot()
		mutated = true
	}
	return snapshot, mutated, nil
}

func (e *FactoryEngine) forwardDispatches(ctx context.Context, records []interfaces.DispatchRecord, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (bool, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if len(records) == 0 || (e.dispatchHandler == nil && e.dispatchHook == nil) {
		return false, snapshot, nil
	}
	for _, rec := range records {
		if err := e.forwardDispatchRecord(ctx, rec); err != nil {
			return false, snapshot, err
		}
	}
	e.drainPendingResults()
	return true, e.runtimeState.Snapshot(), nil
}

func (e *FactoryEngine) forwardDispatchRecord(ctx context.Context, rec interfaces.DispatchRecord) error {
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
			return fmt.Errorf("dispatch/result hook submit dispatch %q: %w", rec.Dispatch.DispatchID, err)
		}
	}
	if e.dispatchHandler != nil {
		e.dispatchHandler(rec.Dispatch)
	}
	return nil
}

func (e *FactoryEngine) finishTick(keepAlive bool, shouldTerminate bool, totalDispatches int, completedDispatches map[string]interfaces.CompletedDispatch, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], mutated bool) bool {
	e.retireCompletedDispatches(e.runtimeState.Results, completedDispatches)
	e.runtimeState.Results = nil
	if keepAlive {
		shouldTerminate = false
	}
	e.logger.Info("engine: [END] tick complete",
		"tick", e.runtimeState.TickCount,
		"mutations", mutated,
		"dispatches", totalDispatches,
		"shouldTerminate", shouldTerminate,
		"tokens", len(snapshot.Marking.Tokens))
	return shouldTerminate
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

	result, err := e.dispatchHook.OnTick(ctx, e.runtimeState.Snapshot())
	if err != nil {
		return 0, fmt.Errorf("dispatch/result hook: %w", err)
	}
	for _, workResult := range result {
		e.appendObservedResult(workResult)
	}
	return len(result), nil
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
			generated := make([]work.GeneratedSubmissionBatch, len(result.GeneratedBatches))
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
		if len(result.MarkingMutations) > 0 {
			if err := e.applyHookMarkingMutations(result.MarkingMutations); err != nil {
				return 0, false, fmt.Errorf("submission hook %q marking mutations: %w", hookName, err)
			}
			snapshot = e.runtimeState.Snapshot()
		}
	}
	return totalSubmissions, keepAlive, nil
}

func (e *FactoryEngine) applyHookMarkingMutations(mutations []interfaces.MarkingMutation) error {
	for _, mutation := range mutations {
		if mutation.Type != interfaces.MutationMove {
			continue
		}
		token, ok := e.runtimeState.Marking.Tokens[mutation.TokenID]
		if !ok || token == nil {
			continue
		}
		fromState := stateValueForPlace(e.state, mutation.FromPlace)
		if leavingFailedPlace(e.state, token.Color.WorkTypeID, fromState) {
			factorytoken.ClearGuardBlockingFields(&token.History)
		}
	}
	return applyMutations(e.runtimeState.Marking, e.state.Places, mutations, e.clock.Now())
}

func (e *FactoryEngine) processGeneratedSubmissionBatches(batches []work.GeneratedSubmissionBatch, defaultSource string) (int, error) {
	total := 0
	for i := range batches {
		batch := batches[i]
		source := generatedSubmissionSource(batch, defaultSource)
		normalized, requestID, err := e.normalizeGeneratedSubmissionBatch(batch)
		if err != nil {
			return total, err
		}
		if e.skipGeneratedSubmissionRequest(requestID, source) {
			continue
		}
		tokens, err := e.tokensFromGeneratedSubmissions(normalized)
		if err != nil {
			return total, err
		}
		e.recordGeneratedSubmissionRequest(requestID, source, batch, normalized)
		e.recordGeneratedSubmissionTokens(source, normalized, tokens)
		if source == externalSubmissionHookName {
			e.signalObservableProjection(requestID)
		}
		total += len(tokens)
	}
	return total, nil
}

func generatedSubmissionSource(batch work.GeneratedSubmissionBatch, defaultSource string) string {
	if batch.Metadata.Source != "" {
		return batch.Metadata.Source
	}
	if defaultSource != "" {
		return defaultSource
	}
	return "generated-batch"
}

func (e *FactoryEngine) normalizeGeneratedSubmissionBatch(batch work.GeneratedSubmissionBatch) ([]workdomain.SubmitRequest, string, error) {
	normalized, err := workdomain.NormalizeGeneratedSubmissionBatch(batch, workdomain.WorkRequestNormalizeOptions{
		ValidWorkTypes:    e.validWorkTypes(),
		ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes),
		IDGenerator:       e.workRequestIDs,
	})
	if err != nil {
		return nil, "", err
	}
	requestID := ""
	if len(normalized) > 0 {
		requestID = normalized[0].RequestID
	}
	return normalized, requestID, nil
}

func (e *FactoryEngine) skipGeneratedSubmissionRequest(requestID string, source string) bool {
	if requestID == "" || source == externalSubmissionHookName {
		return false
	}
	_, exists := e.workRequests[requestID]
	return exists
}

func (e *FactoryEngine) tokensFromGeneratedSubmissions(normalized []workdomain.SubmitRequest) ([]*factorytoken.Token, error) {
	now := e.clock.Now()
	tokens := make([]*factorytoken.Token, 0, len(normalized))
	for _, req := range normalized {
		token, err := e.transformer.InitialTokenFromSubmit(req, now)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (e *FactoryEngine) recordGeneratedSubmissionRequest(
	requestID string,
	source string,
	batch work.GeneratedSubmissionBatch,
	normalized []workdomain.SubmitRequest,
) {
	e.workRequests[requestID] = workdomain.WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
	if e.recordWorkRequest == nil {
		return
	}
	record := workdomain.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
	record.ParentLineage = append([]string(nil), batch.Metadata.ParentLineage...)
	e.recordWorkRequest(e.runtimeState.TickCount, record)
}

func (e *FactoryEngine) recordGeneratedSubmissionTokens(
	source string,
	normalized []workdomain.SubmitRequest,
	tokens []*factorytoken.Token,
) {
	for index, token := range tokens {
		if e.recordSubmission != nil {
			e.recordSubmission(work.FactorySubmissionRecord{
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
}

// injectTokens creates tokens from submit requests and places them in INITIAL places.
func (e *FactoryEngine) injectTokens(requests []workdomain.SubmitRequest) {
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
