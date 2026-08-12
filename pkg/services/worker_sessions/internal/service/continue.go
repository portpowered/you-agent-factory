package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// continueTuple is the immutable caller-owned identity of one continuation
// request. The exact Provider Session and resolved execution are captured in
// the replay plan at reservation time, but are deliberately not caller input.
type continueTuple struct {
	sourceID    string
	successorID string
	input       string
}

type continuePlan struct {
	request   workersessions.ContinueRequest
	execution workers.WorkstationDispatchRequest
	direct    bool
}

type continuationSourceSnapshot struct {
	session    workersessions.Session
	execution  workers.WorkstationDispatchRequest
	dispatchID string
	turnID     string
	direct     bool
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
	replay, owner, err := r.reserveContinuation(req)
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
) (*continueReplay, bool, error) {
	tuple := continueTuple{
		sourceID:    req.SourceWorkerSessionID,
		successorID: req.SuccessorWorkerSessionID,
		input:       req.FollowUpInput,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.continueReplays == nil {
		r.continueReplays = make(map[string]*continueReplay)
	}
	if existing, ok := r.continueReplays[req.RequestID]; ok {
		if existing.tuple != tuple {
			return nil, false, workersessions.ErrContinuationRequestIDConflict
		}
		return existing, false, nil
	}
	if r.stopping {
		return nil, false, workersessions.ErrContinuationServerStopping
	}
	snapshot, err := r.snapshotContinuationSourceLocked(req)
	if err != nil {
		return nil, false, err
	}
	continuation, err := r.buildContinuationExecutionLocked(req, snapshot)
	if err != nil {
		return nil, false, err
	}
	return r.storeContinuationReservationLocked(req, tuple, snapshot, continuation), true, nil
}

func (r *registry) snapshotContinuationSourceLocked(
	req workersessions.ContinueRequest,
) (continuationSourceSnapshot, error) {
	source, exists := r.sessions[req.SourceWorkerSessionID]
	if !exists {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceNotFound
	}
	if !source.Terminal() {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceActive
	}
	if source.SuccessorWorkerSessionID != "" {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	if err := validateContinuationSourceAssociation(source); err != nil {
		return continuationSourceSnapshot{}, err
	}
	supervision := r.supervisions[source.ID]
	if supervision == nil {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationExecutionUnavailable
	}
	supervision.mu.Lock()
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
	if metadata := r.observations[source.ID]; metadata != nil {
		direct = metadata.direct
	}
	return continuationSourceSnapshot{
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
	source.SuccessorWorkerSessionID = req.SuccessorWorkerSessionID
	r.sessions[source.ID] = source
	r.sessions[req.SuccessorWorkerSessionID] = workersessions.Session{
		ID:                         req.SuccessorWorkerSessionID,
		State:                      workersessions.StateReserved,
		ProviderSessionAssociation: continuationAssociation(req, continuation, snapshot.turnID, source.ProviderSessionAssociation.Reference),
		PredecessorWorkerSessionID: req.SourceWorkerSessionID,
	}
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
			request:   req,
			execution: continuation,
			direct:    snapshot.direct,
		},
		done: make(chan struct{}),
	}
	r.continueReplays[req.RequestID] = replay
	r.logger.Info(
		"worker session continuation",
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
	continuation.Execution.ResumeSession = &continuedReference
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
			continuation:     true,
			requestID:        plan.request.RequestID,
			verifyTopicReady: true,
		},
	)
	if err != nil {
		return r.continuationResult(plan), continuationNotAccepted(err)
	}
	if prepared.terminal {
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
		return r.continuationResult(plan), nil
	case <-prepared.supervision.done:
		select {
		case <-prepared.supervision.admitted:
			return r.continuationResult(plan), nil
		default:
			return r.continuationResult(plan), continuationNotAccepted(r.startAdmissionCause(prepared.supervision))
		}
	}
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
