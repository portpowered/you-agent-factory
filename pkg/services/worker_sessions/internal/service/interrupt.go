package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type interruptTuple struct {
	sourceID    string
	successorID string
	message     string
}

type interruptPlan struct {
	request     workersessions.InterruptRequest
	execution   workers.WorkstationDispatchRequest
	reference   providers.SessionRef
	dispatchID  string
	supervision *supervision
}

type interruptReplay struct {
	tuple  interruptTuple
	plan   interruptPlan
	done   chan struct{}
	result workersessions.InterruptResult
	err    error
}

type interruptCompletion struct {
	result workersessions.InterruptResult
	err    error
}

// Interrupt claims the source before asking Workers to cancel it. Its
// server-owned operation remains replayable when the caller leaves, and the
// successor is not reserved until the source's callback has committed CANCELED.
func (r *registry) Interrupt(
	ctx context.Context,
	req workersessions.InterruptRequest,
) (workersessions.InterruptResult, error) {
	callerCtx := ctx
	if callerCtx == nil {
		callerCtx = context.Background()
	}
	if err := req.Validate(); err != nil {
		result := r.interruptResultSnapshot(req.Normalize(), workersessions.InterruptPhaseValidation, false)
		return result, newInterruptError(workersessions.InterruptPhaseValidation, result, err)
	}
	req = req.Normalize()
	replay, owner, err := r.reserveInterrupt(req)
	if err != nil {
		result := r.interruptResultSnapshot(req, workersessions.InterruptPhaseValidation, false)
		r.logInterruptRejected(req, workersessions.InterruptPhaseValidation, err)
		return result, newInterruptError(workersessions.InterruptPhaseValidation, result, err)
	}
	if !owner {
		result, replayErr := awaitInterruptReplay(callerCtx, replay)
		return result, replayErr
	}

	outcomes := make(chan interruptCompletion, 1)
	go func() {
		result, interruptErr := r.runInterrupt(replay.plan)
		r.finishInterruptReplay(replay, result, interruptErr)
		r.finishStart()
		outcomes <- interruptCompletion{result: result, err: interruptErr}
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
			"worker session interrupt wait canceled",
			"sourceWorkerSessionID", req.SourceWorkerSessionID,
			"successorWorkerSessionID", req.SuccessorWorkerSessionID,
			"requestID", req.RequestID,
			"outcome", "caller_canceled",
		)
		return workersessions.InterruptResult{}, callerCtx.Err()
	}
}

func (r *registry) reserveInterrupt(
	req workersessions.InterruptRequest,
) (*interruptReplay, bool, error) {
	tuple := interruptTuple{
		sourceID:    req.SourceWorkerSessionID,
		successorID: req.SuccessorWorkerSessionID,
		message:     req.ReplacementMessage,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.interruptReplays == nil {
		r.interruptReplays = make(map[string]*interruptReplay)
	}
	if existing, ok := r.interruptReplays[req.RequestID]; ok {
		if existing.tuple != tuple {
			return nil, false, workersessions.ErrInterruptRequestIDConflict
		}
		return existing, false, nil
	}
	if r.stopping {
		return nil, false, workersessions.ErrInterruptServerStopping
	}
	plan, err := r.prepareInterruptPlanLocked(req)
	if err != nil {
		return nil, false, err
	}
	replay := r.storeInterruptReservationLocked(req, tuple, plan)
	return replay, true, nil
}

func (r *registry) prepareInterruptPlanLocked(
	req workersessions.InterruptRequest,
) (interruptPlan, error) {
	source, err := r.interruptSourceLocked(req)
	if err != nil {
		return interruptPlan{}, err
	}
	association, err := interruptSourceAssociation(source)
	if err != nil {
		return interruptPlan{}, err
	}
	return r.reserveInterruptSupervisionLocked(req, source, association)
}

func (r *registry) interruptSourceLocked(req workersessions.InterruptRequest) (workersessions.Session, error) {
	source, exists := r.sessions[req.SourceWorkerSessionID]
	if !exists {
		return workersessions.Session{}, workersessions.ErrInterruptSourceNotFound
	}
	if source.State != workersessions.StateRunning {
		return workersessions.Session{}, workersessions.ErrInterruptSourceNotActive
	}
	if source.SuccessorWorkerSessionID != "" {
		return workersessions.Session{}, workersessions.ErrInterruptSourceConflict
	}
	if _, exists := r.sessions[req.SuccessorWorkerSessionID]; exists {
		return workersessions.Session{}, workersessions.ErrInterruptSourceConflict
	}
	return source, nil
}

func (r *registry) reserveInterruptSupervisionLocked(
	req workersessions.InterruptRequest,
	source workersessions.Session,
	association workersessions.ProviderSessionAssociation,
) (interruptPlan, error) {
	supervision := r.supervisions[source.ID]
	if supervision == nil {
		return interruptPlan{}, workersessions.ErrInterruptExecutionUnavailable
	}
	supervision.mu.Lock()
	if !supervision.accepted || supervision.publishing || supervision.dispatchID == "" {
		supervision.mu.Unlock()
		return interruptPlan{}, workersessions.ErrInterruptSourceNotActive
	}
	if supervision.interrupting || supervision.controlAction != "" ||
		supervision.requestedAction != "" || supervision.controlActive ||
		supervision.continuing {
		supervision.mu.Unlock()
		return interruptPlan{}, workersessions.ErrInterruptSourceConflict
	}
	if association.DispatchID != supervision.dispatchID || association.AttemptID != supervision.dispatchID {
		supervision.mu.Unlock()
		return interruptPlan{}, workersessions.ErrInterruptProviderSessionInvalid
	}
	supervision.interrupting = true
	supervision.interruptRequestID = req.RequestID
	supervision.interruptDone = make(chan struct{})
	supervision.controlActive = true
	supervision.controlDone = make(chan struct{})
	supervision.requestedAction = workersessions.ControlActionCancel
	plan := interruptPlan{
		request:     req,
		execution:   cloneWorkstationDispatchRequest(supervision.execution),
		reference:   association.Reference.Clone(),
		dispatchID:  supervision.dispatchID,
		supervision: supervision,
	}
	supervision.mu.Unlock()
	return plan, nil
}

func (r *registry) storeInterruptReservationLocked(
	req workersessions.InterruptRequest,
	tuple interruptTuple,
	plan interruptPlan,
) *interruptReplay {
	if r.startsDone == nil {
		r.startsDone = make(chan struct{})
		close(r.startsDone)
	}
	if r.activeStarts == 0 {
		r.startsDone = make(chan struct{})
	}
	r.activeStarts++
	replay := &interruptReplay{tuple: tuple, plan: plan, done: make(chan struct{})}
	r.interruptReplays[req.RequestID] = replay
	r.logger.Info(
		"worker session interrupt",
		"sourceWorkerSessionID", req.SourceWorkerSessionID,
		"successorWorkerSessionID", req.SuccessorWorkerSessionID,
		"attemptID", plan.dispatchID,
		"requestID", req.RequestID,
		"outcome", "reserved",
	)
	return replay
}

func interruptSourceAssociation(source workersessions.Session) (workersessions.ProviderSessionAssociation, error) {
	if source.ProviderSessionAssociation == nil {
		return workersessions.ProviderSessionAssociation{}, workersessions.ErrInterruptProviderSessionMissing
	}
	association := source.ProviderSessionAssociation.Clone()
	if err := association.Validate(); err != nil {
		return workersessions.ProviderSessionAssociation{}, fmt.Errorf("%w: %w", workersessions.ErrInterruptProviderSessionInvalid, err)
	}
	if association.WorkerSessionID != source.ID {
		return workersessions.ProviderSessionAssociation{}, fmt.Errorf("%w: worker session identity mismatch", workersessions.ErrInterruptProviderSessionInvalid)
	}
	return association, nil
}

func (r *registry) runInterrupt(plan interruptPlan) (workersessions.InterruptResult, error) {
	defer finishInterruptOperation(plan.supervision)
	boundaryContext := context.WithoutCancel(r.serverOwnedContext())
	cancelResult, cancelErr := r.boundary.Cancel(boundaryContext, workers.WorkstationDispatchCancelRequest{
		DispatchID: plan.dispatchID,
	})
	canceled := cancelErr == nil && (cancelResult.Outcome == workers.WorkstationDispatchCancelOutcomeCanceled ||
		cancelResult.Outcome == workers.WorkstationDispatchCancelOutcomeAlreadyCanceled)
	finishInterruptBoundary(plan.supervision, canceled)
	if !canceled {
		cause := interruptCancellationCause(cancelResult, cancelErr)
		result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseSourceCancellation, false)
		return result, newInterruptError(workersessions.InterruptPhaseSourceCancellation, result, cause)
	}
	<-plan.supervision.done
	source, _ := r.Get(context.Background(), workersessions.GetRequest{ID: plan.request.SourceWorkerSessionID})
	if source.State != workersessions.StateCanceled {
		cause := fmt.Errorf("%w: authoritative source state is %s", workersessions.ErrInterruptSourceCancellationFailed, source.State)
		result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseSourceCancellation, false)
		return result, newInterruptError(workersessions.InterruptPhaseSourceCancellation, result, cause)
	}

	continued, continueErr := r.Continue(boundaryContext, workersessions.ContinueRequest{
		RequestID:                interruptContinuationRequestID(plan.request.RequestID),
		SourceWorkerSessionID:    plan.request.SourceWorkerSessionID,
		SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID,
		FollowUpInput:            plan.request.ReplacementMessage,
	})
	result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseSuccessorAdmission, continueErr == nil)
	if continueErr != nil {
		return result, newInterruptError(workersessions.InterruptPhaseSuccessorAdmission, result, errors.Join(workersessions.ErrInterruptSuccessorAdmissionFailed, continueErr))
	}
	result.Successor = continued.Session.Clone()
	if !interruptSuccessorMatches(result.Successor, plan.reference, plan.execution.Execution.Dispatch.DispatchID) {
		cause := fmt.Errorf("%w: successor Provider Session reference does not match source", workersessions.ErrInterruptSuccessorAdmissionFailed)
		result.Accepted = false
		return result, newInterruptError(workersessions.InterruptPhaseSuccessorAdmission, result, cause)
	}
	result.Accepted = true
	r.logger.Info(
		"worker session interrupt",
		"sourceWorkerSessionID", plan.request.SourceWorkerSessionID,
		"successorWorkerSessionID", plan.request.SuccessorWorkerSessionID,
		"attemptID", plan.dispatchID,
		"requestID", plan.request.RequestID,
		"outcome", "accepted",
	)
	return result, nil
}

func interruptSuccessorMatches(
	session workersessions.Session,
	reference providers.SessionRef,
	sourceDispatchID string,
) bool {
	return session.ID != "" && session.State == workersessions.StateRunning &&
		session.ProviderSessionAssociation != nil &&
		session.ProviderSessionAssociation.Reference == reference &&
		session.ProviderSessionAssociation.DispatchID == continuationDispatchID(sourceDispatchID, session.ID)
}

func finishInterruptBoundary(supervision *supervision, canceled bool) {
	supervision.mu.Lock()
	wait := supervision.controlDone
	supervision.controlDone = nil
	supervision.controlActive = false
	if canceled {
		supervision.controlAction = workersessions.ControlActionCancel
	} else if supervision.requestedAction == workersessions.ControlActionCancel {
		supervision.requestedAction = ""
	}
	supervision.mu.Unlock()
	if wait != nil {
		close(wait)
	}
}

func finishInterruptOperation(supervision *supervision) {
	supervision.mu.Lock()
	wait := supervision.interruptDone
	supervision.interruptDone = nil
	supervision.interrupting = false
	supervision.interruptRequestID = ""
	supervision.mu.Unlock()
	if wait != nil {
		close(wait)
	}
}

func (r *registry) finishInterruptReplay(
	replay *interruptReplay,
	result workersessions.InterruptResult,
	err error,
) {
	replay.result = result.Clone()
	replay.err = err
	close(replay.done)
}

func awaitInterruptReplay(
	ctx context.Context,
	replay *interruptReplay,
) (workersessions.InterruptResult, error) {
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
		return workersessions.InterruptResult{}, ctx.Err()
	}
}

func interruptResult(
	req workersessions.InterruptRequest,
	phase workersessions.InterruptPhase,
	accepted bool,
) workersessions.InterruptResult {
	result := workersessions.InterruptResult{
		RequestID:                req.RequestID,
		SourceWorkerSessionID:    req.SourceWorkerSessionID,
		SuccessorWorkerSessionID: req.SuccessorWorkerSessionID,
		Phase:                    phase,
		Accepted:                 accepted,
	}
	return result
}

func (r *registry) interruptResultSnapshot(
	req workersessions.InterruptRequest,
	phase workersessions.InterruptPhase,
	accepted bool,
) workersessions.InterruptResult {
	result := interruptResult(req, phase, accepted)
	r.mu.RLock()
	if source, ok := r.sessions[req.SourceWorkerSessionID]; ok {
		result.Source = cloneSession(source)
	}
	if successor, ok := r.sessions[req.SuccessorWorkerSessionID]; ok {
		result.Successor = cloneSession(successor)
	}
	r.mu.RUnlock()
	return result
}

func newInterruptError(
	phase workersessions.InterruptPhase,
	result workersessions.InterruptResult,
	cause error,
) error {
	phaseCause := interruptPhaseCause(phase)
	return &workersessions.InterruptError{
		Phase:  phase,
		Result: result.Clone(),
		Cause:  errors.Join(phaseCause, cause),
	}
}

func interruptPhaseCause(phase workersessions.InterruptPhase) error {
	switch phase {
	case workersessions.InterruptPhaseValidation:
		return workersessions.ErrInterruptValidation
	case workersessions.InterruptPhaseSourceCancellation:
		return workersessions.ErrInterruptSourceCancellation
	case workersessions.InterruptPhaseSuccessorAdmission:
		return workersessions.ErrInterruptSuccessorAdmission
	default:
		return errors.New("worker session: unknown interrupt phase")
	}
}

func interruptCancellationCause(
	result workers.WorkstationDispatchCancelResult,
	err error,
) error {
	if err != nil {
		return errors.Join(workersessions.ErrInterruptSourceCancellationFailed, err)
	}
	if result.Outcome == workers.WorkstationDispatchCancelOutcomeAlreadyTerminal {
		return errors.Join(workersessions.ErrInterruptSourceCancellationFailed, workers.ErrWorkstationDispatchAlreadyTerminal)
	}
	return fmt.Errorf("%w: Workers returned cancellation outcome %s", workersessions.ErrInterruptSourceCancellationFailed, result.Outcome)
}

func interruptContinuationRequestID(requestID string) string {
	return "interrupt/" + requestID
}

func (r *registry) logInterruptRejected(
	req workersessions.InterruptRequest,
	phase workersessions.InterruptPhase,
	err error,
) {
	r.logger.Info(
		"worker session interrupt rejected",
		"sourceWorkerSessionID", req.SourceWorkerSessionID,
		"successorWorkerSessionID", req.SuccessorWorkerSessionID,
		"requestID", req.RequestID,
		"phase", string(phase),
		"error", err.Error(),
	)
}
