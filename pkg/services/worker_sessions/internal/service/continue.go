package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	continuationLineageSourceType     events.SourceType     = "worker_session_lineage"
	continuationLineageSourceSequence events.SourceSequence = 1
	continuationLineageSourceEventID  events.SourceEventID  = "successor"
	attemptLineageSourceType          events.SourceType     = "worker_session_attempt"
	attemptLineageSourceSequence      events.SourceSequence = 1
	resumeAttemptSourceEventID        events.SourceEventID  = "resume"
	retryAttemptSourceEventID         events.SourceEventID  = "retry"
)

func (r *registry) reconcileOverdueAttempt(
	id string, supervision *supervision, attemptID string,
	attemptDone chan struct{}, deadlineAt time.Time,
) {
	if !supervision.deadlineAttemptActive(attemptID, attemptDone) {
		return
	}
	_ = attemptID
	supervision.markDeadlineExceeded()
	canceled, err := supervision.cancelExecution()
	if !canceled {
		supervision.clearDeadlineExceeded()
		err = workers.ErrUnknownWorkstationDispatch
	}
	if err == nil || errors.Is(err, workers.ErrWorkstationDispatchAlreadyTerminal) || errors.Is(err, workers.ErrWorkstationDispatchCanceled) {
		if err != nil {
			supervision.clearDeadlineExceeded()
		}
		return
	}
	supervision.clearDeadlineExceeded()
	r.logger.Info(
		"worker session reconciliation failed",
		"sessionID", publicWorkerID(id),
		"attemptID", attemptID,
		"dispatchID", attemptID,
		"reason", string(workers.WorkstationDispatchReconciliationReasonTimeout),
		"prior_state", string(workersessions.StateRunning),
		"deadline", deadlineAt.UTC().Format(time.RFC3339Nano),
		"outcome", "rejected",
	)
}

func timeoutDispatchResult(result workers.WorkstationDispatchResult) workers.WorkstationDispatchResult {
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	result.ReconciliationReason = workers.WorkstationDispatchReconciliationReasonTimeout
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.Error = workers.ErrWorkstationDispatchTimeout.Error()
	result.Result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyRetryable,
		Type:   workers.WorkFailureTypeTimeout,
	}
	return result
}

// continueTuple is the immutable caller-owned identity of one continuation
// request. The exact Provider Session and resolved execution are captured in
// the replay plan at reservation time, but are deliberately not caller input.
type continueTuple struct {
	sourceID    string
	successorID string
	input       string
	resolveHead bool
}

type continuePlan struct {
	sourceAddress     string
	addressedSourceID string
	executor          workers.Service
	clock             platformclock.Source
	scheduler         platformclock.TimerSource
	request           workersessions.ContinueRequest
	execution         workers.WorkstationDispatchRequest
	direct            bool
	lineage           *workers.SessionLineage
	archived          bool
	interrupt         bool
}

type continuationSourceSnapshot struct {
	address    string
	executor   workers.Service
	clock      platformclock.Source
	scheduler  platformclock.TimerSource
	session    workersessions.Session
	execution  workers.WorkstationDispatchRequest
	dispatchID string
	turnID     string
	direct     bool
	archived   bool
}

type continueReplay struct {
	tuple  continueTuple
	plan   continuePlan
	done   chan struct{}
	result workersessions.ContinueResult
	err    error
}

type continueCompletion struct {
	result workersessions.ContinueResult
	err    error
}

// Continue reserves one successor and returns at the same server-owned
// Workers admission barrier as Start. The caller's context only controls how
// long this method waits; an admitted continuation remains owned by the
// process after caller cancellation and is replayable by RequestID.
func (r *registry) Continue(
	ctx context.Context,
	req workersessions.ContinueRequest,
) (workersessions.ContinueResult, error) {
	callerCtx := ctx
	if callerCtx == nil {
		callerCtx = context.Background()
	}
	if err := req.Validate(); err != nil {
		r.logContinuationRejected(req, "invalid")
		return workersessions.ContinueResult{}, err
	}
	req = req.Normalize()
	r.mu.RLock()
	_, addressErr := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	r.mu.RUnlock()
	if addressErr != nil {
		return workersessions.ContinueResult{}, addressErr
	}
	replay, owner, err := r.reserveContinuation(req, callerCtx)
	if err != nil {
		r.logContinuationRejected(req, continuationReservationOutcome(err))
		return workersessions.ContinueResult{}, err
	}
	if !owner {
		result, replayErr := awaitContinueReplay(callerCtx, replay)
		r.logger.Info(
			"worker session continuation replay",
			"sourceWorkerSessionID", req.SourceWorkerSessionID,
			"successorWorkerSessionID", replay.plan.request.SuccessorWorkerSessionID,
			"requestID", req.RequestID,
			"outcome", continuationReplayOutcome(replayErr),
		)
		return result, replayErr
	}

	outcomes := make(chan continueCompletion, 1)
	go func() {
		result, continueErr := r.continueReserved(replay.plan)
		r.finishContinueReplay(replay, result, continueErr)
		r.finishStart()
		outcomes <- continueCompletion{result: result, err: continueErr}
	}()
	select {
	case outcome := <-outcomes:
		return outcome.result, outcome.err
	case <-callerCtx.Done():
		select {
		case outcome := <-outcomes:
			return outcome.result, outcome.err
		default:
		}
		r.logger.Info(
			"worker session continuation wait canceled",
			"sourceWorkerSessionID", req.SourceWorkerSessionID,
			"successorWorkerSessionID", req.SuccessorWorkerSessionID,
			"requestID", req.RequestID,
			"outcome", "caller_canceled",
		)
		return workersessions.ContinueResult{}, callerCtx.Err()
	}
}

// reserveContinuation validates the terminal source and atomically captures
// the source's exact Provider Session association, the resolved execution,
// idempotency tuple, and successor lineage before any opening event or
// Workers/provider effect can occur.
func (r *registry) reserveContinuation(
	req workersessions.ContinueRequest,
	callers ...context.Context,
) (*continueReplay, bool, error) {
	r.mu.RLock()
	address, err := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	existing := r.continueReplays[req.RequestID]
	r.mu.RUnlock()
	if err != nil {
		return nil, false, err
	}
	tuple := continueTuple{sourceID: firstNonEmpty(address, req.SourceWorkerSessionID), successorID: req.SuccessorWorkerSessionID,
		input: req.FollowUpInput, resolveHead: req.ResolveHead}
	if existing != nil {
		if existing.tuple != tuple {
			return nil, false, workersessions.ErrContinuationRequestIDConflict
		}
		return existing, false, nil
	}
	addressed := req
	if req.ResolveHead {
		var replay *continueReplay
		req, replay, err = r.resolveContinuationHead(req)
		if err != nil || replay != nil {
			return replay, false, err
		}
	}
	return r.reserveResolvedContinuation(req, addressed, tuple, callers...)
}

func (r *registry) reserveResolvedContinuation(
	req, addressed workersessions.ContinueRequest, tuple continueTuple, callers ...context.Context,
) (*continueReplay, bool, error) {
	captured, err := r.readContinuationRecipe(req, callers...)
	if err != nil {
		return nil, false, err
	}
	archived, err := r.readArchivedContinuationSource(req)
	if err != nil {
		return nil, false, err
	}
	if archived != nil {
		if replay, err := r.readTerminalContinuationReplay(addressed, archived); replay != nil || err != nil {
			return replay, false, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	address, err := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	if err != nil {
		return nil, false, err
	}
	if address == "" {
		address = req.SourceWorkerSessionID
	}

	if replay, err := r.existingContinuationReservationLocked(req.RequestID, tuple); replay != nil || err != nil {
		return replay, false, err
	}
	snapshot, err := r.continuationSnapshotLocked(req, archived)
	if err != nil {
		return nil, false, err
	}
	if captured != nil {
		if captured.Execution.Dispatch.DispatchID != snapshot.dispatchID {
			return nil, false, workersessions.ErrContinuationExecutionUnavailable
		}
		// Credentials belong to this host's live execution context and are
		// deliberately absent from the detached persisted recipe.
		captured.Execution.ProcessEnvironment = append([]string(nil), snapshot.execution.Execution.ProcessEnvironment...)
		snapshot.execution = *captured
	}
	continuation, err := r.buildContinuationExecutionLocked(req, snapshot)
	if err != nil {
		return nil, false, err
	}
	if err := r.validateContinuationSupportLocked(snapshot.session.ProviderSessionAssociation.Reference); err != nil {
		return nil, false, err
	}
	return r.reserveAvailableContinuationLocked(req, addressed, tuple, snapshot, continuation, archived)
}

// Inspection is a peer read, never a registry-lock or admission owner. Recheck
// the exact source and request after it returns before acquiring a reservation.
// The caller holds r.mu and retains responsibility for its final unlock.
func (r *registry) reserveAvailableContinuationLocked(
	req, addressed workersessions.ContinueRequest, tuple continueTuple,
	snapshot continuationSourceSnapshot, continuation workers.WorkstationDispatchRequest,
	archived *archivedContinuationSource,
) (*continueReplay, bool, error) {
	snapshot.session = snapshot.session.Clone()
	reference := snapshot.session.ProviderSessionAssociation.Reference
	r.mu.Unlock()
	err := r.inspectContinuationReference(r.serverOwnedContext(), reference)
	r.mu.Lock()
	if replay, replayErr := r.existingContinuationReservationLocked(req.RequestID, tuple); replay != nil || replayErr != nil {
		return replay, false, replayErr
	}
	if err != nil {
		return nil, false, err
	}
	current, err := r.continuationSnapshotLocked(req, archived)
	if err != nil {
		return nil, false, err
	}
	if current.dispatchID != snapshot.dispatchID || *current.session.ProviderSessionAssociation != *snapshot.session.ProviderSessionAssociation {
		return nil, false, workersessions.ErrContinuationExecutionUnavailable
	}
	if _, err := r.buildContinuationExecutionLocked(req, current); err != nil {
		return nil, false, err
	}
	replay := r.storeContinuationReservationLocked(req, tuple, snapshot, continuation)
	replay.plan.addressedSourceID = addressed.SourceWorkerSessionID
	if archived != nil {
		r.publications[snapshot.address] = &publication{capture: archived.target}
	}
	return replay, true, nil
}

func (r *registry) existingContinuationReservationLocked(requestID string, tuple continueTuple) (*continueReplay, error) {
	if existing := r.continueReplays[requestID]; existing != nil {
		if existing.tuple != tuple {
			return nil, workersessions.ErrContinuationRequestIDConflict
		}
		return existing, nil
	}
	if r.stopping {
		return nil, workersessions.ErrContinuationServerStopping
	}
	return nil, nil
}

func (r *registry) inspectContinuationReference(ctx context.Context, reference providers.SessionRef) error {
	if r.inspection == nil || (reflect.ValueOf(r.inspection).Kind() == reflect.Pointer && reflect.ValueOf(r.inspection).IsNil()) {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	inspected, err := r.inspection.Inspect(providersessions.InspectRequest{Context: ctx, Session: reference.Clone()})
	if err != nil {
		return fmt.Errorf("%w: %w", workersessions.ErrContinuationExecutionUnavailable, err)
	}
	if inspected.Session != reference {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	return nil
}

// Replays return their original result before this query. A new reservation
// must use current policy and already negotiated facts before recording input
// or opening a successor, including when the source came from durable capture.
func (r *registry) validateContinuationSupportLocked(reference providers.SessionRef) error {
	if r.continuationSupport == nil {
		return nil
	}
	supported, err := r.continuationSupport.SupportsContinuation(controlContext(r.lifecycleCtx), reference)
	if err != nil {
		return fmt.Errorf("%w: %w", workersessions.ErrContinuationProviderSessionInvalid, err)
	}
	if !supported {
		return workersessions.ErrContinuationProviderSessionInvalid
	}
	return nil
}

func (r *registry) snapshotContinuationSourceLocked(
	req workersessions.ContinueRequest,
) (continuationSourceSnapshot, error) {
	address, err := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	if err != nil {
		return continuationSourceSnapshot{}, err
	}
	source, exists := r.sessions[address]
	if !exists {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceNotFound
	}
	if !source.Terminal() {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceActive
	}
	if source.SuccessorWorkerSessionID != "" {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	if requestID := r.continuationSources[address]; requestID != "" && requestID != req.RequestID {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	if err := validateContinuationSourceAssociation(source); err != nil {
		return continuationSourceSnapshot{}, err
	}
	supervision := r.supervisions[address]
	if supervision == nil {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationExecutionUnavailable
	}
	supervision.mu.Lock()
	if supervision.forceJournalPending != 0 || supervision.controlPersistenceLost {
		supervision.mu.Unlock()
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	interruptContinuation := interruptContinuationRequestID(supervision.interruptRequestID)
	if supervision.interrupting && (supervision.interruptRequestID == "" || req.RequestID != interruptContinuation) {
		supervision.mu.Unlock()
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	execution := cloneWorkstationDispatchRequest(supervision.execution)
	dispatchID := strings.TrimSpace(supervision.dispatchID)
	turnID := supervision.turnID
	supervision.mu.Unlock()
	if dispatchID == "" {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationExecutionUnavailable
	}
	if association := source.ProviderSessionAssociation; association.DispatchID != dispatchID || association.AttemptID != dispatchID {
		return continuationSourceSnapshot{}, fmt.Errorf("%w: source attempt identity mismatch", workersessions.ErrContinuationProviderSessionInvalid)
	}
	direct := false
	if metadata := r.observations[address]; metadata != nil {
		direct = metadata.direct
	}
	return continuationSourceSnapshot{
		address: address, executor: supervision.executor, clock: supervision.clock, scheduler: supervision.scheduler,
		session:    source,
		execution:  execution,
		dispatchID: dispatchID,
		turnID:     turnID,
		direct:     direct,
	}, nil
}

func (r *registry) buildContinuationExecutionLocked(
	req workersessions.ContinueRequest,
	snapshot continuationSourceSnapshot,
) (workers.WorkstationDispatchRequest, error) {
	if _, exists := r.sessions[req.SuccessorWorkerSessionID]; exists {
		return workers.WorkstationDispatchRequest{}, workersessions.ErrContinuationSuccessorConflict
	}
	continuation := continuationExecution(
		snapshot.execution,
		continuationDispatchID(snapshot.dispatchID, req.SuccessorWorkerSessionID),
		req.FollowUpInput,
		snapshot.session.ProviderSessionAssociation.Reference,
	)
	// A continuation is an independent direct attempt. Retain Factory Session
	// scope for addressing, while dropping live Runtime dispatch ownership.
	continuation.Execution.RuntimeID = ""
	if _, exists := r.dispatchOwners[continuation.Execution.Dispatch.DispatchID]; exists {
		return workers.WorkstationDispatchRequest{}, workersessions.ErrContinuationSuccessorConflict
	}
	if err := (workersessions.InvokeSessionRequest{
		ID:        req.SuccessorWorkerSessionID,
		Execution: continuation,
	}).Validate(); err != nil {
		return workers.WorkstationDispatchRequest{}, fmt.Errorf("%w: %w", workersessions.ErrContinuationExecutionUnavailable, err)
	}
	return continuation, nil
}

func (r *registry) storeContinuationReservationLocked(
	req workersessions.ContinueRequest,
	tuple continueTuple,
	snapshot continuationSourceSnapshot,
	continuation workers.WorkstationDispatchRequest,
) *continueReplay {
	source := snapshot.session
	address := firstNonEmpty(snapshot.address, source.ID)
	if !snapshot.archived {
		r.sessions[address] = source
	}
	r.sessions[req.SuccessorWorkerSessionID] = workersessions.Session{
		ID:                         req.SuccessorWorkerSessionID,
		State:                      workersessions.StateReserved,
		ProviderSessionAssociation: interruptContinuationAssociation(req, continuation, snapshot),
	}
	if r.continuationSources == nil {
		r.continuationSources = make(map[string]string)
	}
	if r.continueReplays == nil {
		r.continueReplays = make(map[string]*continueReplay)
	}
	r.continuationSources[address] = req.RequestID
	r.publications[req.SuccessorWorkerSessionID] = &publication{}
	if r.startsDone == nil {
		r.startsDone = make(chan struct{})
		close(r.startsDone)
	}
	if r.activeStarts == 0 {
		r.startsDone = make(chan struct{})
	}
	r.activeStarts++
	replay := &continueReplay{
		tuple: tuple,
		plan: continuePlan{
			sourceAddress: address, executor: snapshot.executor, clock: snapshot.clock, scheduler: snapshot.scheduler,
			request:   req,
			execution: continuation,
			direct:    true,
			archived:  snapshot.archived,
			lineage: &workers.SessionLineage{
				PredecessorWorkerSessionID: req.SourceWorkerSessionID,
				PreviousDispatchID:         snapshot.dispatchID,
				PreviousAttemptID:          snapshot.dispatchID,
			},
		},
		done: make(chan struct{}),
	}
	r.continueReplays[req.RequestID] = replay
	r.logger.Info(
		"worker session continuation",
		"addressedSourceWorkerSessionID", publicWorkerID(tuple.sourceID),
		"resolveHead", tuple.resolveHead,
		"sourceWorkerSessionID", req.SourceWorkerSessionID,
		"successorWorkerSessionID", req.SuccessorWorkerSessionID,
		"attemptID", continuation.Execution.Dispatch.DispatchID,
		"requestID", req.RequestID,
		"outcome", "reserved",
		"state", string(workersessions.StateReserved),
	)
	return replay
}

func validateContinuationSourceAssociation(session workersessions.Session) error {
	association := session.ProviderSessionAssociation
	if association == nil {
		return workersessions.ErrContinuationProviderSessionMissing
	}
	if err := association.Validate(); err != nil {
		return fmt.Errorf("%w: %w", workersessions.ErrContinuationProviderSessionInvalid, err)
	}
	if association.WorkerSessionID != session.ID {
		return fmt.Errorf("%w: worker session identity mismatch", workersessions.ErrContinuationProviderSessionInvalid)
	}
	return nil
}

func continuationExecution(
	base workers.WorkstationDispatchRequest,
	dispatchID string,
	followUpInput string,
	reference providers.SessionRef,
) workers.WorkstationDispatchRequest {
	continuation := cloneWorkstationDispatchRequest(base)
	continuation.Execution.Dispatch.DispatchID = dispatchID
	continuation.Execution.UserMessage = followUpInput
	continuedReference := reference.Clone()
	continuationRef := continuedReference.ContinuationRef()
	continuation.Execution.Continuation = &continuationRef
	return continuation
}

func continuationAssociation(
	req workersessions.ContinueRequest,
	execution workers.WorkstationDispatchRequest,
	turnID string,
	reference providers.SessionRef,
) *workersessions.ProviderSessionAssociation {
	return &workersessions.ProviderSessionAssociation{
		WorkerSessionID: req.SuccessorWorkerSessionID,
		TurnID:          turnID,
		DispatchID:      execution.Execution.Dispatch.DispatchID,
		AttemptID:       execution.Execution.Dispatch.DispatchID,
		Reference:       reference.Clone(),
	}
}

func continuationDispatchID(sourceDispatchID, successorID string) string {
	return sourceDispatchID + "/continue/" + successorID
}

func (r *registry) continueReserved(plan continuePlan) (workersessions.ContinueResult, error) {
	if err := r.persistContinuationInput(plan); err != nil {
		r.releaseContinuationReservation(plan)
		return r.continuationResult(plan), continuationNotAccepted(err)
	}
	runtimeID := strings.TrimSpace(plan.execution.Execution.RuntimeID)
	if runtimeID != "" {
		if !r.beginRuntimeOpening(runtimeID) {
			r.releaseContinuationReservation(plan)
			return r.continuationResult(plan), continuationNotAccepted(workersessions.ErrContinuationServerStopping)
		}
		defer r.finishRuntimeOpening(runtimeID)
	}
	serverCtx := r.serverOwnedContext()
	invoke := workersessions.InvokeSessionRequest{
		ID:        plan.request.SuccessorWorkerSessionID,
		Execution: plan.execution,
	}
	prepared, err := r.prepareInvocation(
		serverCtx,
		invoke,
		invocationPreparationOptions{
			serverOwned:      true,
			direct:           plan.direct,
			continuation:     plan.execution.Execution.Continuation != nil,
			requestID:        plan.request.RequestID,
			verifyTopicReady: true,
			lineage:          plan.lineage,
		},
		plan.executor,
		plan.clock,
		plan.scheduler,
	)
	if err != nil {
		r.releaseContinuationReservation(plan)
		return r.continuationResult(plan), continuationNotAccepted(err)
	}
	if prepared.terminal {
		r.releaseContinuationReservation(plan)
		return workersessions.ContinueResult{
			RequestID:                plan.request.RequestID,
			SourceWorkerSessionID:    plan.request.SourceWorkerSessionID,
			SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID,
			Session:                  prepared.session,
		}, continuationNotAccepted(prepared.failure)
	}
	go r.driveRegisteredInvocation(serverCtx, invoke, prepared.supervision)
	select {
	case <-prepared.supervision.admitted:
		r.commitContinuationLineage(plan)
		return r.continuationResult(plan), nil
	case <-prepared.supervision.done:
		select {
		case <-prepared.supervision.admitted:
			r.commitContinuationLineage(plan)
			return r.continuationResult(plan), nil
		default:
			r.releaseContinuationReservation(plan)
			return r.continuationResult(plan), continuationNotAccepted(r.startAdmissionCause(prepared.supervision))
		}
	}
}

// releaseContinuationReservation clears the source claim when the successor
// never reaches Workers admission. A reserved-but-unadmitted successor is not
// lineage evidence, so neither side is mutated into a durable relationship.
func (r *registry) releaseContinuationReservation(plan continuePlan) {
	r.mu.Lock()
	if r.continuationSources[plan.sourceAddressOrID()] == plan.request.RequestID {
		delete(r.continuationSources, plan.sourceAddressOrID())
	}
	r.mu.Unlock()
}

// commitContinuationLineage publishes the source-side relationship only after
// the successor's admission barrier has opened. The source execution is
// already terminal, so its live capture handle has closed; the optional
// Recordings writer is therefore fed the exact accepted Events record
// directly, preserving a durable prefix or an explicit loss classification.
func (r *registry) commitContinuationLineage(plan continuePlan) {
	if plan.archived {
		// The admitted successor opening carries durable predecessor evidence.
		// An archived source has no live Events topic to append or supervise.
		r.commitContinuationSessionLinks(plan, plan.sourceAddressOrID())
		return
	}
	r.mu.RLock()
	source, exists := r.sessions[plan.sourceAddressOrID()]
	source = cloneSession(source)
	r.mu.RUnlock()
	if !exists {
		r.releaseContinuationReservation(plan)
		return
	}
	payload := workers.SessionPayload{
		Status: string(source.State), WorkerSessionID: source.ID,
		Lineage: &workers.SessionLineage{SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID},
	}
	if association := source.ProviderSessionAssociation; association != nil {
		payload.TurnID = association.TurnID
		payload.DispatchID, payload.AttemptID = association.DispatchID, association.AttemptID
		payload.Continuation = &workers.SessionContinuation{
			Provider: string(association.Reference.Provider), Kind: association.Reference.Kind, ID: association.Reference.ID,
		}
	}
	if source.ProviderSessionAssociation == nil && plan.lineage != nil {
		payload.DispatchID, payload.AttemptID = plan.lineage.PreviousDispatchID, plan.lineage.PreviousAttemptID
	}
	identity := events.AppendIdentity{
		SourceType:     continuationLineageSourceType,
		SourceID:       events.SourceID(source.ID + "/successor/" + plan.request.SuccessorWorkerSessionID),
		SourceSequence: continuationLineageSourceSequence,
		SourceEventID:  continuationLineageSourceEventID,
	}
	if err := r.publishSessionLineageRecord(context.Background(), plan.sourceAddressOrID(), identity, payload, true); err != nil {
		r.logger.Info(
			"worker session continuation lineage publication failed",
			"sourceWorkerSessionID", source.ID,
			"successorWorkerSessionID", plan.request.SuccessorWorkerSessionID,
			"outcome", "append_failed",
		)
	}

	r.commitContinuationSessionLinks(plan, plan.sourceAddressOrID())
}

func (r *registry) commitContinuationSessionLinks(plan continuePlan, sourceID string) {
	r.mu.Lock()
	if current, exists := r.sessions[sourceID]; exists {
		if current.SuccessorWorkerSessionID == "" || current.SuccessorWorkerSessionID == plan.request.SuccessorWorkerSessionID {
			current.SuccessorWorkerSessionID = plan.request.SuccessorWorkerSessionID
			r.sessions[sourceID] = current
		}
	}
	if current, exists := r.sessions[plan.request.SuccessorWorkerSessionID]; exists {
		if current.PredecessorWorkerSessionID == "" || current.PredecessorWorkerSessionID == publicWorkerID(sourceID) {
			current.PredecessorWorkerSessionID = publicWorkerID(sourceID)
			r.sessions[plan.request.SuccessorWorkerSessionID] = current
		}
	}
	if r.continuationSources[sourceID] == plan.request.RequestID {
		delete(r.continuationSources, sourceID)
	}
	r.mu.Unlock()
}

// publishSessionLineageRecord appends a lifecycle-shaped Session/UPDATED
// record under the same per-session publication lock as normal Worker output.
// allowClosed is used only for the source-side successor link, whose execution
// terminal necessarily precedes this fact.
func (r *registry) publishSessionLineageRecord(
	ctx context.Context,
	sessionID string,
	identity events.AppendIdentity,
	payload workers.SessionPayload,
	allowClosed bool,
) error {
	pub := r.publicationFor(sessionID)
	if pub == nil {
		return workersessions.ErrSessionNotFound
	}
	payloadJSON, _ := json.Marshal(payload)
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseUpdated,
		Provenance: lifecycleProvenance(""),
		Payload:    payloadJSON,
		DispatchID: payload.DispatchID,
		TurnID:     payload.TurnID,
	}
	pub.mu.Lock()
	if !pub.open && !allowClosed {
		pub.mu.Unlock()
		return workersessions.ErrPublicationNotOpen
	}
	if pub.accepted == nil {
		pub.accepted = make(map[events.AppendIdentity]struct{})
	}
	if pub.lastSequence == nil {
		pub.lastSequence = make(map[sourceKey]events.SourceSequence)
	}
	key := sourceKey{sourceType: identity.SourceType, sourceID: identity.SourceID}
	_, alreadyAccepted := pub.accepted[identity]
	if last := pub.lastSequence[key]; !alreadyAccepted && identity.SourceSequence < last {
		pub.mu.Unlock()
		return workersessions.ErrOutOfOrderPublication
	}
	appendResult, err := r.appendDraft(ctx, r.observationTopic(sessionID), identity, workerDraftSchemaID, draft)
	if err != nil {
		pub.mu.Unlock()
		return err
	}
	pub.accepted[identity] = struct{}{}
	if identity.SourceSequence > pub.lastSequence[key] {
		pub.lastSequence[key] = identity.SourceSequence
	}
	recordingID := pub.recordingID
	liveRecording := pub.recording != nil
	pub.mu.Unlock()

	if allowClosed && !liveRecording && recordingID != "" {
		r.persistClosedLineageRecord(ctx, recordingID, sessionID, appendResult.Record)
	}
	return nil
}

func (r *registry) persistClosedLineageRecord(ctx context.Context, recordingID, sessionID string, record events.Record) {
	writer, ok := r.recording.(recordings.WorkerRecordingWriter)
	if !ok || writer == nil {
		r.logger.Info(
			"worker session continuation lineage recording unavailable",
			"sessionID", publicWorkerID(sessionID),
			"outcome", "unavailable",
		)
		return
	}
	err := writer.PersistWorkerRecord(context.WithoutCancel(ctx), recordings.WorkerRecordingRecord{
		RecordingID:     recordingID,
		WorkerSessionID: publicWorkerID(sessionID),
		Record:          record.Detached(),
	})
	if err == nil {
		return
	}
	r.logger.Info(
		"worker session continuation lineage recording failed",
		"sessionID", publicWorkerID(sessionID),
		"outcome", "degraded",
	)
	if failureWriter, ok := r.recording.(recordings.WorkerRecordingFailureWriter); ok && failureWriter != nil {
		_ = failureWriter.PersistWorkerRecordingFailure(context.WithoutCancel(ctx), recordings.WorkerRecordingFailure{
			RecordingID:     recordingID,
			WorkerSessionID: publicWorkerID(sessionID),
			Topic:           r.observationTopic(sessionID),
			Code:            "CONTINUATION_LINEAGE_PERSISTENCE_FAILED",
		})
	}
}

// publishAttemptLineageRecord records a retry or same-session resume before
// its next Workers handoff. The event is an explicit successor-attempt fact;
// callers therefore never need to infer chronology from dispatch suffixes.
func (r *registry) publishAttemptLineageRecord(
	ctx context.Context,
	sessionID string,
	attempt workers.WorkstationDispatchRequest,
	reason workers.AttemptReason,
	previousDispatchID string,
	attemptNumber int,
) error {
	currentDispatchID := attempt.Execution.Dispatch.DispatchID
	r.mu.RLock()
	clock := r.observationClockLocked(sessionID)
	r.mu.RUnlock()
	lineage := workers.SessionLineage{
		PreviousDispatchID: previousDispatchID,
		PreviousAttemptID:  previousDispatchID,
	}
	payload := openingSessionPayload(
		sessionID,
		currentDispatchID,
		clock.Now(),
		attempt.Execution,
		&lineage,
	)
	payload.Attempt = attemptNumber
	payload.AttemptReason = reason
	if reason == workers.AttemptReasonRetry {
		payload.Continuation = nil
	}
	eventID := retryAttemptSourceEventID
	if reason == workers.AttemptReasonResume {
		eventID = resumeAttemptSourceEventID
	}
	identity := events.AppendIdentity{
		SourceType:     attemptLineageSourceType,
		SourceID:       events.SourceID(sessionID + "/attempt/" + currentDispatchID),
		SourceSequence: attemptLineageSourceSequence,
		SourceEventID:  eventID,
	}
	return r.publishSessionLineageRecord(ctx, sessionID, identity, payload, false)
}

func (r *registry) continuationResult(plan continuePlan) workersessions.ContinueResult {
	session, _ := r.Get(context.Background(), workersessions.GetRequest{ID: plan.request.SuccessorWorkerSessionID})
	return workersessions.ContinueResult{
		RequestID:                plan.request.RequestID,
		SourceWorkerSessionID:    plan.request.SourceWorkerSessionID,
		SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID,
		Session:                  session,
	}
}

func (r *registry) finishContinueReplay(replay *continueReplay, result workersessions.ContinueResult, err error) {
	replay.result = result.Clone()
	replay.err = err
	close(replay.done)
}

func awaitContinueReplay(ctx context.Context, replay *continueReplay) (workersessions.ContinueResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-replay.done:
		return replay.result.Clone(), replay.err
	case <-ctx.Done():
		select {
		case <-replay.done:
			return replay.result.Clone(), replay.err
		default:
		}
		return workersessions.ContinueResult{}, ctx.Err()
	}
}

type continuationNotAcceptedError struct {
	cause error
}

func (e *continuationNotAcceptedError) Error() string {
	return workersessions.ErrContinuationNotAccepted.Error()
}

func (e *continuationNotAcceptedError) Unwrap() error {
	if e.cause == nil {
		return workersessions.ErrContinuationNotAccepted
	}
	return errors.Join(workersessions.ErrContinuationNotAccepted, e.cause)
}

func continuationNotAccepted(cause error) error {
	return &continuationNotAcceptedError{cause: cause}
}

func continuationReservationOutcome(err error) string {
	switch {
	case errors.Is(err, workersessions.ErrContinuationRequestIDConflict):
		return "idempotency_conflict"
	case errors.Is(err, workersessions.ErrContinuationServerStopping):
		return "server_stopping"
	case errors.Is(err, workersessions.ErrContinuationSourceNotFound):
		return "source_not_found"
	case errors.Is(err, workersessions.ErrContinuationSourceActive):
		return "source_active"
	default:
		return "rejected"
	}
}

func continuationReplayOutcome(err error) string {
	if err == nil {
		return "accepted"
	}
	return "rejected"
}

func (r *registry) logContinuationRejected(req workersessions.ContinueRequest, outcome string) {
	r.logger.Info(
		"worker session continuation rejected",
		"sourceWorkerSessionID", req.SourceWorkerSessionID,
		"successorWorkerSessionID", req.SuccessorWorkerSessionID,
		"requestID", req.RequestID,
		"outcome", outcome,
	)
}

// interruptSuccessorAdmittedState reports whether a successor snapshot taken
// after Workers admission is consistent with a successful admission. The
// snapshot is read after the admission barrier opens, so a fast successor may
// already have progressed past RUNNING to COMPLETED; that is still an accepted
// admission, not a mismatch.
func interruptSuccessorAdmittedState(state workersessions.State) bool {
	switch state {
	case workersessions.StateStarting, workersessions.StateRunning, workersessions.StateCompleted:
		return true
	default:
		return false
	}
}

func (r *registry) associateProviderSession(
	req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req.WorkerSessionID = r.workerAddressLocked(req.WorkerSessionID, req.FactorySessionID)
	return r.associateProviderSessionLocked(req)
}

func (r *registry) providerBindingOwner(req workersessions.ProviderBindingRequest) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	req.WorkerSessionID = r.workerAddressLocked(req.WorkerSessionID, req.FactorySessionID)
	dispatchID := strings.TrimSpace(req.DispatchID)
	if req.WorkerSessionID != "" {
		if attempt := r.runtimeAttemptControls[req.WorkerSessionID]; attempt != nil {
			return req.WorkerSessionID, attempt.attemptID == dispatchID && r.runtimeAttemptOwners[attempt.key] == req.WorkerSessionID
		}
		if supervision := r.supervisions[req.WorkerSessionID]; supervision != nil && supervision.runtimeKey.RuntimeID != "" {
			supervision.mu.Lock()
			owned := supervision.dispatchID == dispatchID && r.runtimeAttemptOwners[supervision.runtimeKey] == req.WorkerSessionID
			supervision.mu.Unlock()
			return req.WorkerSessionID, owned
		}
		return req.WorkerSessionID, r.dispatchOwners[dispatchID] == req.WorkerSessionID
	}
	ownerID, exists := r.dispatchOwners[dispatchID]
	return ownerID, exists
}

func (r *registry) associateProviderSessionLocked(
	req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	session, exists := r.sessions[req.WorkerSessionID]
	if !exists {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrSessionNotFound
	}
	supervision := r.supervisions[req.WorkerSessionID]
	attempt := r.runtimeAttemptControls[req.WorkerSessionID]
	if !r.providerAssociationOwnedLocked(req) {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}

	turnID := ""
	dispatchID := req.DispatchID
	attemptID := dispatchID
	if supervision != nil {
		supervision.mu.Lock()
		turnID = supervision.turnID
		dispatchID = supervision.dispatchID
		attemptID = dispatchID
		if supervision.runtimeKey.RuntimeID != "" {
			dispatchID = supervision.runtimeKey.DispatchID
		}
		supervision.mu.Unlock()
	} else if attempt != nil {
		// Runtime owns execution, but its immutable handle still identifies the
		// physical attempt. A logical dispatch may survive several attempts.
		dispatchID = attempt.dispatchID
		attemptID = attempt.attemptID
		if observation := r.observations[req.WorkerSessionID]; observation != nil {
			turnID = observation.turnID
		}
	}
	association := workersessions.ProviderSessionAssociation{
		WorkerSessionID: publicWorkerID(req.WorkerSessionID),
		TurnID:          turnID,
		DispatchID:      dispatchID,
		AttemptID:       attemptID,
		Reference:       req.Reference.Clone(),
	}
	if existing := session.ProviderSessionAssociation; existing != nil {
		if existing.Reference == association.Reference {
			return workersessions.ProviderSessionAssociationResult{
				Association: existing.Clone(),
				Outcome:     workersessions.ProviderSessionAssociationOutcomeDuplicate,
			}, nil
		}
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationConflict
	}
	if session.Terminal() {
		return workersessions.ProviderSessionAssociationResult{}, workersessions.ErrProviderSessionAssociationNotAvailable
	}

	session.ProviderSessionAssociation = &association
	r.sessions[req.WorkerSessionID] = session
	return workersessions.ProviderSessionAssociationResult{
		Association: association.Clone(),
		Outcome:     workersessions.ProviderSessionAssociationOutcomeAccepted,
	}, nil
}

type runtimeAdmission struct {
	closed   bool
	openings int
	drained  chan struct{}
}

func (r *registry) closeRuntimeAdmission(ctx context.Context, runtimeID string) error {
	r.mu.Lock()
	admission := r.runtimeAdmissionLocked(runtimeID)
	admission.closed = true
	drained := admission.drained
	r.mu.Unlock()
	r.logger.Info("runtime Worker admission closed", "runtimeID", runtimeID, "outcome", "closed")
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CloseRuntimeAttempts seals this runtime's admission, joins opening effects,
// then joins its admitted attempts without discarding retained observations.
// An opening that loses the final admission race terminalizes its own capture;
// the closed runtime ID cannot admit another attempt. Peer scopes stay open.
func (r *registry) CloseRuntimeAttempts(ctx context.Context, runtimeID string) error {
	ctx = runtimeAttemptContext(ctx)
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		return workersessions.ErrProviderSessionAssociationAttemptMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.closeRuntimeAdmission(ctx, runtimeID); err != nil {
		return err
	}
	r.mu.RLock()
	var ids []string
	for _, attempt := range r.runtimeAttemptControls {
		if attempt.key.RuntimeID == runtimeID {
			ids = append(ids, attempt.workerID)
		}
	}
	for id, supervision := range r.supervisions {
		supervision.mu.Lock()
		owned := supervision.runtimeKey.RuntimeID == runtimeID || (supervision.serverOwned && strings.TrimSpace(supervision.execution.Execution.RuntimeID) == runtimeID)
		supervision.mu.Unlock()
		if owned {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()
	results := make(chan error, len(ids))
	for _, id := range ids {
		go func() {
			_, err := r.Cancel(ctx, workersessions.ControlRequest{ID: id})
			if err == nil {
				err = r.waitForSupervisionDriver(ctx, id)
			}
			results <- err
		}()
	}
	var closeErr error
	for range ids {
		select {
		case err := <-results:
			closeErr = errors.Join(closeErr, err)
		case <-ctx.Done():
			return errors.Join(closeErr, ctx.Err())
		}
	}
	return closeErr
}

func (r *registry) registerInvocationSupervision(
	ctx context.Context,
	req workersessions.InvokeSessionRequest,
	options invocationPreparationOptions,
	executor workers.Service,
	clock platformclock.Source,
	scheduler platformclock.TimerSource,
) (invocationPreparation, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	if options.verifyTopicReady {
		eventReadyFields := []any{"sessionID", publicWorkerID(req.ID), "attemptID", attemptID, "outcome", "event_ready", "state", string(workersessions.StateStarting)}
		if options.requestID != "" {
			eventReadyFields = append(eventReadyFields, "requestID", options.requestID)
		}
		r.logger.Info("worker session start", eventReadyFields...)
	}
	supervision, canStart := r.registerSupervisionOwned(
		options.serverOwned,
		req.ID,
		attemptID,
		req.Execution.Execution.Dispatch.Execution.RequestID,
		executor,
		clock,
		scheduler,
		options.runtimeKey,
		req.Execution,
	)
	if !canStart {
		if options.runtimeKey.RuntimeID != "" {
			final := r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
			return invocationPreparation{session: final, terminal: true, preAdmission: true, failure: workersessions.ErrStartAdmissionFailed}, nil
		}
		final, _ := r.Get(context.WithoutCancel(ctx), workersessions.GetRequest{ID: req.ID})
		if options.serverOwned && r.runtimeAdmissionClosed(req.Execution.Execution.RuntimeID) && !final.Terminal() {
			final = r.cancelInvocationBeforeAdmission(ctx, req.ID, attemptID)
			return invocationPreparation{session: final, terminal: true, failure: workersessions.ErrStartServerStopping}, nil
		}
		if options.serverOwned && r.isStopping() && !final.Terminal() {
			final = r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
			return invocationPreparation{
				session:  final,
				terminal: true,
				failure:  workersessions.ErrStartServerStopping,
			}, nil
		}
		if final.Terminal() {
			r.publishTerminalSnapshot(ctx, req.ID, attemptID, final)
			return invocationPreparation{
				session:  final,
				terminal: true,
				failure:  workersessions.ErrStartAdmissionFailed,
			}, nil
		}
		return invocationPreparation{}, startNotAccepted(workersessions.ErrStartAdmissionFailed)
	}
	supervision.mu.Lock()
	supervision.retryBudget = req.Retry.Attempts()
	supervision.continuing = options.continuation
	supervision.mu.Unlock()
	return invocationPreparation{supervision: supervision}, nil
}

// A reservation retains its selected owner across retries and terminal publication.
func (plan continuePlan) sourceAddressOrID() string {
	return firstNonEmpty(plan.sourceAddress, plan.request.SourceWorkerSessionID)
}
