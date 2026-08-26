package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
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
	seededRestoredWorkIDs map[string]struct{}
	replayDispatchWorkIDs map[string]struct{}
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
	terminationResult     *interfaces.TerminationResult
	// admissionGate serializes a complete live-change admission transaction
	// with ticks that may acquire or release resource tokens.
	admissionGate       chan struct{}
	capacityWakePending bool
	capacityChanged     chan struct{}
	resourceLeases      map[string]resourceCapacityLease
	nextResourceLeaseID uint64
	factoryRevision     int

	// publishedSnapshot is the last complete runtime boundary. Readers use it
	// when the engine is busy in a tick so read-only observation never waits on
	// dispatch, submission hooks, or other work performed under mu.
	publishedSnapshot           atomic.Pointer[engineStateSnapshot]
	pendingProjectionRequestIDs map[string]struct{}
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
	seededRestoredWorkIDs map[string]struct{},
	seededReplayWorkIDsWithRecordedDispatch map[string]struct{},
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
		seededRestoredWorkIDs: cloneWorkIDSet(seededRestoredWorkIDs),
		replayDispatchWorkIDs: cloneWorkIDSet(seededReplayWorkIDsWithRecordedDispatch),
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
		admissionGate:         make(chan struct{}, 1),
		capacityChanged:       make(chan struct{}),
		resourceLeases:        make(map[string]resourceCapacityLease),

		pendingProjectionRequestIDs: make(map[string]struct{}),
	}
	e.admissionGate <- struct{}{}
	e.publishRuntimeSnapshotLocked()
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
					DispatchID:           entry.DispatchID,
					TransitionID:         entry.TransitionID,
					WorkstationName:      entry.WorkstationName,
					Outcome:              r.Outcome,
					Cancellation:         r.Cancellation.Clone(),
					Reason:               completedDispatchReasonFromResult(r),
					ArtifactVerification: r.ArtifactVerification.Clone(),
					FailureDetail:        workerexecution.CloneFailureDetail(r.FailureDetail),
					ProviderSession:      (r.Continuation).SessionMetadata(),
					StartTime:            entry.StartTime,
					EndTime:              now,
					Duration:             now.Sub(entry.StartTime),
					ConsumedTokens:       entry.ConsumedTokens,
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

// SubmitWorkRequest validates and enqueues a canonical work request batch.
// Repeated request IDs are treated as idempotent no-ops.
func (e *FactoryEngine) SubmitWorkRequest(context context.Context, request workdomain.WorkRequest) (workdomain.WorkRequestSubmitResult, error) {
	release, err := e.AcquireResourceCapacityAdmission(context)
	if err != nil {
		return workdomain.WorkRequestSubmitResult{}, err
	}

	e.mu.Lock()
	if existing, exists := e.workRequests[request.RequestID]; exists && request.RequestID != "" {
		e.mu.Unlock()
		release()
		existing.Accepted = false
		return existing, nil
	}
	normalized, err := workdomain.NormalizeWorkRequest(request, workdomain.WorkRequestNormalizeOptions{
		ValidWorkTypes:    e.validWorkTypes(),
		ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes),
		IDGenerator:       e.workRequestIDs,
		ExistingWorks:     e.existingWorksForAdmissionLocked(),
	})
	e.mu.Unlock()
	release()
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

// existingWorksForAdmissionLocked returns the current board identities used
// by live relation admission. Marking tokens cover queued, terminal, and
// failed Work; consumed dispatch tokens cover Work that is currently active
// and therefore temporarily absent from the marking.
//
// The caller must hold e.mu and the admission gate must prevent a tick from
// changing the board while this snapshot is consumed by normalization.
func (e *FactoryEngine) existingWorksForAdmissionLocked() []workdomain.ExistingWork {
	byID := make(map[string]workdomain.ExistingWork)
	add := func(color factorytoken.Color) {
		if color.DataType == factorytoken.DataTypeResource || color.WorkID == "" {
			return
		}
		candidate := workdomain.ExistingWork{
			WorkID:     color.WorkID,
			Name:       color.Name,
			WorkTypeID: color.WorkTypeID,
		}
		if current, exists := byID[candidate.WorkID]; exists {
			if current.Name == "" {
				current.Name = candidate.Name
			}
			if current.WorkTypeID == "" {
				current.WorkTypeID = candidate.WorkTypeID
			}
			byID[candidate.WorkID] = current
			return
		}
		byID[candidate.WorkID] = candidate
	}

	if e.runtimeState != nil && e.runtimeState.Marking != nil {
		for _, token := range e.runtimeState.Marking.Tokens {
			if token != nil {
				add(token.Color)
			}
		}
	}
	if e.runtimeState != nil {
		for _, dispatch := range e.runtimeState.Dispatches {
			if dispatch == nil {
				continue
			}
			for _, token := range dispatch.ConsumedTokens {
				add(token.Color)
			}
		}
	}

	works := make([]workdomain.ExistingWork, 0, len(byID))
	for _, candidate := range byID {
		works = append(works, candidate)
	}
	sort.Slice(works, func(i, j int) bool { return works[i].WorkID < works[j].WorkID })
	return works
}

// Run is the main execution loop. Blocks on a select over wake channels until
// ctx is cancelled or the marking has no more actionable tokens.
func (e *FactoryEngine) Run(ctx context.Context) error {
	e.logger.Info("engine started")
	e.mu.Lock()
	e.runLoopActive = true
	e.terminationResult = nil
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
		return e.terminationError()
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
			return e.terminationError()
		}
	}
}

func (e *FactoryEngine) terminationError() error {
	e.mu.Lock()
	var termination interfaces.TerminationResult
	if e.terminationResult != nil {
		termination = *e.terminationResult
	}
	e.mu.Unlock()

	if termination.Classification != interfaces.TerminationClassificationIncomplete {
		return nil
	}
	return &factory.IncompleteDrainError{
		NonTerminalWorkCount: termination.NonTerminalWorkCount,
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
			if e.finishTerminationDrain() {
				return true, nil
			}
			// A wake-up arrived while preparing to terminate. The drain
			// consumed it, so immediately rerun the canonical tick.
			continue
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

// tick runs a single tick cycle: execute subsystems in order, apply mutations
// atomically between each subsystem execution. Returns (mutated, shouldTerminate, error).
// mutated is true if any mutations were applied (another tick may be needed).
// shouldTerminate is true if the TerminationCheck subsystem signaled completion.
func (e *FactoryEngine) tick(ctx context.Context) (bool, bool, error) {
	if e.automaticTicksPaused != nil && e.automaticTicksPaused() {
		e.logger.Debug("engine: skipping automatic tick while factory is paused")
		return false, false, nil
	}
	e.capacityWakePending = false
	e.terminationResult = nil

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
			if result.Termination == nil {
				e.terminationResult = nil
			} else {
				termination := *result.Termination
				e.terminationResult = &termination
			}
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
			if isNonFatalPetriMutationPersistenceError(err) {
				e.logger.Error(
					"engine: durable Petri mutation snapshot rejected; continuing runtime loop",
					"dispatch_id", completed[i].DispatchID,
					"error", err,
				)
				continue
			}
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
		if _, err := e.processGeneratedSubmissionBatches(result.GeneratedBatches, "tick-result", false); err != nil {
			return snapshot, mutated, fmt.Errorf("processing generated batches from tick-group %d: %w", tickGroup, err)
		}
		snapshot = e.runtimeState.Snapshot()
		mutated = true
	}
	return snapshot, mutated, nil
}

func (e *FactoryEngine) forwardDispatches(ctx context.Context, records []interfaces.DispatchRecord, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (bool, interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	if len(records) == 0 {
		return false, snapshot, nil
	}
	if e.dispatchHandler == nil && e.dispatchHook == nil && !containsHumanApprovalDispatch(e.state, records) {
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
	humanApproval := isHumanApprovalDispatch(e.state, rec.Dispatch)
	rec.Dispatch.Execution.DispatchCreatedTick = e.runtimeState.TickCount
	rec.Dispatch.Execution.CurrentTick = e.runtimeState.TickCount
	e.runtimeState.Dispatches[rec.Dispatch.DispatchID] = &interfaces.DispatchEntry{
		DispatchID:              rec.Dispatch.DispatchID,
		TransitionID:            rec.Dispatch.TransitionID,
		WorkstationName:         rec.Dispatch.WorkstationName,
		ExpectedArtifactContext: cloneExpectedArtifactTemplateContext(rec.Dispatch.ExpectedArtifactContext),
		StartTime:               now,
		ConsumedTokens:          workers.WorkDispatchInputTokens(rec.Dispatch),
		HeldMutations:           rec.Mutations,
	}
	e.runtimeState.InFlightCount++
	if e.recordDispatch != nil {
		e.recordDispatch(interfaces.FactoryDispatchRecord{
			DispatchID:     rec.Dispatch.DispatchID,
			CreatedTick:    e.runtimeState.TickCount,
			Dispatch:       rec.Dispatch,
			HeldMutations:  rec.Mutations,
			ConsumedTokens: consumedTokenIDs(factorytoken.FromWorkerSlice(workers.WorkDispatchInputTokens(rec.Dispatch))),
			HumanApproval:  humanApproval,
		})
	}
	// A HUMAN_APPROVAL dispatch remains reserved in the in-flight table until a
	// later resolution lane supplies an explicit result. It never enters the
	// worker/provider/model/script or capacity execution boundary.
	if humanApproval {
		return nil
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

func isHumanApprovalDispatch(net *state.Net, dispatch work.WorkDispatch) bool {
	if net == nil {
		return false
	}
	transition := net.Transitions[dispatch.TransitionID]
	return transition != nil && transition.Type == petri.TransitionHumanApproval
}

func containsHumanApprovalDispatch(net *state.Net, records []interfaces.DispatchRecord) bool {
	for _, record := range records {
		if isHumanApprovalDispatch(net, record.Dispatch) {
			return true
		}
	}
	return false
}

func (e *FactoryEngine) finishTick(keepAlive bool, shouldTerminate bool, totalDispatches int, completedDispatches map[string]interfaces.CompletedDispatch, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], mutated bool) bool {
	e.retireCompletedDispatches(e.runtimeState.Results, completedDispatches)
	e.runtimeState.Results = nil
	e.publishRuntimeSnapshotLocked()
	e.signalPendingObservableProjections()
	if keepAlive {
		shouldTerminate = false
		e.terminationResult = nil
	}
	e.logger.Info("engine: [END] tick complete",
		"tick", e.runtimeState.TickCount,
		"mutations", mutated,
		"dispatches", totalDispatches,
		"shouldTerminate", shouldTerminate,
		"tokens", len(snapshot.Marking.Tokens))
	return shouldTerminate
}

func (e *FactoryEngine) signalPendingObservableProjections() {
	for requestID := range e.pendingProjectionRequestIDs {
		e.signalObservableProjection(requestID)
		delete(e.pendingProjectionRequestIDs, requestID)
	}
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
			count, err := e.processGeneratedSubmissionBatches(
				generated,
				hookName,
				hookName == factory.ReplaySubmissionHookName,
			)
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

// injectTokens creates tokens from submit requests and places them in INITIAL places.
func (e *FactoryEngine) injectTokens(requests []workdomain.SubmitRequest) {
	e.logger.Info("engine: injecting tokens", "count", len(requests))
	parentIDs := make(map[string]struct{})
	for _, req := range requests {
		token, err := e.transformer.InitialTokenFromSubmit(req, e.clock.Now())
		if err != nil {
			e.logger.Error("engine: failed to convert submit request to token", "work_type_id", req.WorkTypeID, "error", err)
			continue
		}
		e.runtimeState.Marking.RecordParentChildRegistration(token)
		e.runtimeState.Marking.AddToken(token)
		if token.Color.ParentID != "" {
			parentIDs[token.Color.ParentID] = struct{}{}
		}
		if e.recordWorkInput != nil {
			e.recordWorkInput(e.runtimeState.TickCount, req, *token)
		}
	}
	for parentID := range parentIDs {
		e.runtimeState.Marking.CompleteParentChildRegistration(parentID)
	}
}
