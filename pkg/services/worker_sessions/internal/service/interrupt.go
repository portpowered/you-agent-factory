package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type hostSupervisionDeadlineTimer struct{ timer *time.Timer }

func (timer hostSupervisionDeadlineTimer) C() <-chan time.Time { return timer.timer.C }
func (timer hostSupervisionDeadlineTimer) Stop() bool          { return timer.timer.Stop() }

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
	reservation, historyErr := r.beginControlHistory(
		callerCtx,
		req.SourceWorkerSessionID,
		workersessions.ControlActionInterrupt,
		req.RequestID,
	)
	if historyErr != nil && !errors.Is(historyErr, workersessions.ErrSessionNotFound) {
		result := r.interruptResultSnapshot(req, workersessions.InterruptPhaseValidation, false)
		return result, newInterruptError(workersessions.InterruptPhaseValidation, result, historyErr)
	}
	replay, owner, err := r.reserveInterrupt(req)
	if err != nil {
		r.finishInterruptControlHistory(reservation, req.SourceWorkerSessionID, workersessions.ControlOutcomeFailed, "")
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
		outcome := workersessions.ControlOutcomeFailed
		if interruptErr == nil || result.Source.State == workersessions.StateCanceled {
			outcome = workersessions.ControlOutcomeApplied
		}
		r.finishInterruptControlHistory(reservation, req.SourceWorkerSessionID, outcome, replay.plan.dispatchID)
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

const (
	controlSourceType         events.SourceType     = "worker_session_control"
	controlRequestSourceEvent events.SourceEventID  = "request"
	controlOutcomeSourceEvent events.SourceEventID  = "outcome"
	controlRequestSourceSeq   events.SourceSequence = 1
	controlOutcomeSourceSeq   events.SourceSequence = 2
)

// controlHistoryGate keeps one control bracket ahead of the terminal
// publication boundary. It is separate from publication.mu because a
// Workers callback may synchronously publish a terminal record while the
// control operation is still waiting for that callback to return.
type controlHistoryGate struct {
	mu      sync.Mutex
	pending bool
	closed  bool
	done    chan struct{}
}

func (gate *controlHistoryGate) acquire() bool {
	for {
		gate.mu.Lock()
		if gate.closed {
			gate.mu.Unlock()
			return false
		}
		if !gate.pending {
			gate.pending = true
			gate.done = make(chan struct{})
			gate.mu.Unlock()
			return true
		}
		wait := gate.done
		gate.mu.Unlock()
		<-wait
	}
}

func (gate *controlHistoryGate) close() {
	for {
		gate.mu.Lock()
		if !gate.pending {
			gate.closed = true
			gate.mu.Unlock()
			return
		}
		wait := gate.done
		gate.mu.Unlock()
		<-wait
	}
}

func (gate *controlHistoryGate) release() {
	gate.mu.Lock()
	if !gate.pending {
		gate.mu.Unlock()
		return
	}
	wait := gate.done
	gate.pending = false
	gate.done = nil
	gate.mu.Unlock()
	close(wait)
}

type controlHistoryReservation struct {
	pub         *publication
	sessionID   string
	action      workersessions.ControlAction
	requestID   string
	correlation string
	dispatchID  string
	turnID      string
	supervision *supervision
	finishOnce  sync.Once
}

// beginControlHistory commits the request half of a control bracket before
// the caller can invoke Workers. A control racing the in-process terminal
// transition may still be recorded while the terminal publication window is
// open; the terminal gate then orders the bracket before the terminal record.
func (r *registry) beginControlHistory(
	ctx context.Context,
	id string,
	action workersessions.ControlAction,
	requestID string,
) (*controlHistoryReservation, error) {
	session, supervision, err := r.controlTarget(id)
	if err != nil {
		return nil, err
	}
	pub := r.publicationFor(id)
	if pub == nil {
		// Small lifecycle unit registries intentionally omit Events and the
		// publication map. Preserve their control semantics; production
		// reservations always install a publication before exposing a session.
		return nil, nil
	}
	if !pub.control.acquire() {
		return nil, nil
	}

	pub.mu.Lock()
	open := pub.open
	dispatchID := ""
	turnID := pub.turnID
	if supervision != nil {
		supervision.mu.Lock()
		dispatchID = strings.TrimSpace(supervision.dispatchID)
		turnID = strings.TrimSpace(supervision.turnID)
		supervision.mu.Unlock()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = controlFallbackRequestID(action, id, dispatchID)
	}
	if _, exists := pub.completedControls[controlReplayKey(action, requestID)]; exists {
		pub.mu.Unlock()
		pub.control.release()
		return nil, nil
	}
	pub.mu.Unlock()
	if !open {
		pub.control.release()
		return nil, nil
	}

	reservation := &controlHistoryReservation{
		pub:         pub,
		sessionID:   id,
		action:      action,
		requestID:   requestID,
		correlation: controlCorrelation(action, id, requestID, dispatchID),
		dispatchID:  dispatchID,
		turnID:      turnID,
		supervision: supervision,
	}
	if err := r.appendControlRecord(controlContext(ctx), reservation, workersessions.ControlRecordTypeRequest, "", dispatchID, session.State); err != nil {
		pub.control.release()
		return nil, err
	}
	if supervision != nil {
		supervision.mu.Lock()
		if supervision.controlHistory == nil {
			supervision.controlHistory = reservation
			reservation.supervision = supervision
		}
		supervision.mu.Unlock()
	}
	return reservation, nil
}

func controlContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func controlFallbackRequestID(action workersessions.ControlAction, sessionID, dispatchID string) string {
	return strings.Join([]string{string(action), sessionID, dispatchID}, "/")
}

func controlReplayKey(action workersessions.ControlAction, requestID string) string {
	return string(action) + "\x00" + strings.TrimSpace(requestID)
}

func controlCorrelation(action workersessions.ControlAction, sessionID, requestID, dispatchID string) string {
	value := strings.Join([]string{string(action), sessionID, requestID, dispatchID}, "\x00")
	digest := sha256.Sum256([]byte(value))
	return "control-" + hex.EncodeToString(digest[:])
}

func (r *registry) finishControlHistory(
	reservation *controlHistoryReservation,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) {
	if reservation == nil {
		return
	}
	reservation.finishOnce.Do(func() {
		if strings.TrimSpace(dispatchID) == "" {
			dispatchID = reservation.dispatchID
		}
		ctx := r.serverOwnedContext()
		reservation.pub.mu.Lock()
		err := error(nil)
		if reservation.pub.open {
			err = r.appendControlRecordLocked(ctx, reservation, workersessions.ControlRecordTypeOutcome, outcome, dispatchID, state)
		} else {
			err = workersessions.ErrPublicationNotOpen
		}
		if reservation.pub.completedControls == nil {
			reservation.pub.completedControls = make(map[string]struct{})
		}
		reservation.pub.completedControls[controlReplayKey(reservation.action, reservation.requestID)] = struct{}{}
		reservation.pub.mu.Unlock()
		if err != nil && !errors.Is(err, workersessions.ErrPublicationNotOpen) {
			r.logger.Info(
				"worker session control history outcome publication failed",
				"sessionID", reservation.sessionID,
				"action", string(reservation.action),
				"outcome", string(outcome),
				"error", err.Error(),
			)
		}
		if reservation.supervision != nil {
			reservation.supervision.mu.Lock()
			if reservation.supervision.controlHistory == reservation {
				reservation.supervision.controlHistory = nil
			}
			reservation.supervision.mu.Unlock()
		}
		reservation.pub.control.release()
	})
}

func (r *registry) appendControlRecord(
	ctx context.Context,
	reservation *controlHistoryReservation,
	recordType workersessions.ControlRecordType,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) error {
	reservation.pub.mu.Lock()
	defer reservation.pub.mu.Unlock()
	if !reservation.pub.open {
		return workersessions.ErrPublicationNotOpen
	}
	return r.appendControlRecordLocked(ctx, reservation, recordType, outcome, dispatchID, state)
}

func (r *registry) appendControlRecordLocked(
	ctx context.Context,
	reservation *controlHistoryReservation,
	recordType workersessions.ControlRecordType,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) error {
	payload := workersessions.ControlRecordPayload{
		RecordType:      recordType,
		Action:          reservation.action,
		Outcome:         outcome,
		RequestID:       reservation.requestID,
		CorrelationID:   reservation.correlation,
		WorkerSessionID: reservation.sessionID,
		DispatchID:      strings.TrimSpace(dispatchID),
		AttemptID:       strings.TrimSpace(dispatchID),
		State:           state,
	}
	payloadJSON, _ := json.Marshal(payload)
	provenance := lifecycleProvenance("")
	provenance.NativeEventSubtype = "worker_session.control." + strings.ToLower(string(recordType))
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseUpdated,
		Provenance: provenance,
		Payload:    payloadJSON,
		DispatchID: strings.TrimSpace(dispatchID),
		TurnID:     reservation.turnID,
	}
	sequence := controlRequestSourceSeq
	eventID := controlRequestSourceEvent
	if recordType == workersessions.ControlRecordTypeOutcome {
		sequence = controlOutcomeSourceSeq
		eventID = controlOutcomeSourceEvent
	}
	identity := events.AppendIdentity{
		SourceType:     controlSourceType,
		SourceID:       events.SourceID(reservation.correlation),
		SourceSequence: sequence,
		SourceEventID:  eventID,
	}
	_, err := r.appendDraft(ctx, workersessions.Topic(reservation.sessionID), identity, workerDraftSchemaID, draft)
	return err
}

func controlOutcomeFromDispatch(
	action workersessions.ControlAction,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) workersessions.ControlOutcome {
	if dispatchErr != nil && !dispatchCanceled(result, dispatchErr) {
		return workersessions.ControlOutcomeFailed
	}
	if dispatchCanceled(result, dispatchErr) {
		if action == workersessions.ControlActionPause || action == workersessions.ControlActionCancel || action == workersessions.ControlActionTerminate || action == workersessions.ControlActionInterrupt {
			return workersessions.ControlOutcomeApplied
		}
	}
	return workersessions.ControlOutcomeNoop
}

func controlResultOutcome(result workersessions.ControlResult, operationErr error) workersessions.ControlOutcome {
	if result.Outcome != "" {
		return result.Outcome
	}
	if operationErr != nil {
		return workersessions.ControlOutcomeFailed
	}
	return workersessions.ControlOutcomeNoop
}

func controlReservationFor(supervision *supervision) *controlHistoryReservation {
	if supervision == nil {
		return nil
	}
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	return supervision.controlHistory
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
	cancelResult, cancelErr := r.execution.CancelWorkstationDispatch(boundaryContext, workers.WorkstationDispatchCancelRequest{
		DispatchID: plan.dispatchID,
	})
	canceled := cancelErr == nil && (cancelResult.Outcome == workers.WorkstationDispatchCancelOutcomeCanceled ||
		cancelResult.Outcome == workers.WorkstationDispatchCancelOutcomeAlreadyCanceled)
	finishInterruptExecution(plan.supervision, canceled)
	if !canceled {
		cause := interruptCancellationCause(cancelResult, cancelErr)
		r.finishInterruptControlHistory(
			controlReservationFor(plan.supervision),
			plan.request.SourceWorkerSessionID,
			workersessions.ControlOutcomeFailed,
			plan.dispatchID,
		)
		result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseSourceCancellation, false)
		return result, newInterruptError(workersessions.InterruptPhaseSourceCancellation, result, cause)
	}
	<-plan.supervision.done
	source, _ := r.Get(context.Background(), workersessions.GetRequest{ID: plan.request.SourceWorkerSessionID})
	if source.State != workersessions.StateCanceled {
		cause := fmt.Errorf("%w: authoritative source state is %s", workersessions.ErrInterruptSourceCancellationFailed, source.State)
		r.finishInterruptControlHistory(
			controlReservationFor(plan.supervision),
			plan.request.SourceWorkerSessionID,
			workersessions.ControlOutcomeFailed,
			plan.dispatchID,
		)
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

func (r *registry) finishInterruptControlHistory(
	reservation *controlHistoryReservation,
	sourceID string,
	outcome workersessions.ControlOutcome,
	dispatchID string,
) {
	if reservation == nil {
		return
	}
	state := workersessions.StateReserved
	if session, err := r.Get(context.Background(), workersessions.GetRequest{ID: sourceID}); err == nil {
		state = session.State
	}
	r.finishControlHistory(reservation, outcome, dispatchID, state)
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

func finishInterruptExecution(supervision *supervision, canceled bool) {
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

func (r *registry) transitionToPaused(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateRunning || session.ProviderSessionAssociation == nil {
		return false
	}
	session.State = workersessions.StatePaused
	r.sessions[id] = session
	return true
}

// Resume control is kept with the interrupt/control publication machinery so
// lifecycle control admission remains split from terminal classification and
// the oversized lifecycle implementation stays below the package ratchet.
// Resume starts one next Workers attempt for the exact paused Worker Session.
// The request carries the registry-owned reference unchanged so the Workers
// provider runner must route only through Providers.Continue.
func (r *registry) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	reservation, err := r.beginControlHistory(ctx, req.ID, workersessions.ControlActionResume, req.RequestID)
	if err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	return r.resumeReserved(ctx, req, reservation)
}

func (r *registry) resumeReserved(
	ctx context.Context,
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
) (workersessions.ControlResult, error) {
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		result := workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}
		r.finishControlHistory(reservation, result.Outcome, "", workersessions.StateReserved)
		return result, err
	}
	if result, resumeErr, handled := r.resumeBeforeAdmission(ctx, req, session, supervision); handled {
		return r.finishResumeHistory(reservation, result, resumeErr)
	}
	if err := validateResumeAssociationForSupervision(session, supervision); err != nil {
		result, rejectedErr := r.rejectedResume(session, supervision, err)
		return r.finishResumeHistory(reservation, result, rejectedErr)
	}

	continuation, previousDispatchID, prepared := r.prepareContinuation(req.ID, supervision, session.ProviderSessionAssociation.Reference)
	if !prepared {
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := r.controlNoop(req.ID, workersessions.ControlActionResume, current, supervision)
		return r.finishResumeHistory(reservation, result, nil)
	}
	if err := r.publishAttemptLineageRecord(
		context.WithoutCancel(ctx),
		req.ID,
		continuation,
		workers.AttemptReasonResume,
		previousDispatchID,
		supervision.attemptCount(),
	); err != nil {
		return r.resumePublicationFailure(req, reservation, supervision, continuation, previousDispatchID, err)
	}
	if err := r.publishResumeDispatch(ctx, req, supervision, continuation); err != nil {
		return r.resumePublicationFailure(req, reservation, supervision, continuation, previousDispatchID, err)
	}
	return r.resumeAdmissionResult(req, reservation, supervision, continuation, previousDispatchID)
}

func (r *registry) resumeBeforeAdmission(
	ctx context.Context,
	req workersessions.ControlRequest,
	session workersessions.Session,
	supervision *supervision,
) (workersessions.ControlResult, error, bool) {
	if session.Terminal() || supervision != nil && supervision.resumeInFlight() {
		return r.controlNoop(req.ID, workersessions.ControlActionResume, session, supervision), nil, true
	}
	if session.State == workersessions.StatePaused && session.ProviderSessionAssociation == nil {
		result, err := r.rejectedResume(session, supervision, workersessions.ErrProviderSessionAssociationMissing)
		return result, err, true
	}
	if session.State != workersessions.StatePaused || supervision == nil {
		result, err := r.unsupportedControl(ctx, req, workersessions.ControlActionResume)
		return result, err, true
	}
	return workersessions.ControlResult{}, nil, false
}

func (r *registry) publishResumeDispatch(
	ctx context.Context,
	req workersessions.ControlRequest,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
) error {
	return r.publishExecution(context.WithoutCancel(ctx), req.ID, continuation, supervision)
}

func (r *registry) resumePublicationFailure(
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
	previousDispatchID string,
	publicationErr error,
) (workersessions.ControlResult, error) {
	r.revertContinuation(req.ID, supervision, previousDispatchID)
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result := workersessions.ControlResult{
		Session:    current,
		Action:     workersessions.ControlActionResume,
		Outcome:    workersessions.ControlOutcomeFailed,
		DispatchID: continuation.Execution.Dispatch.DispatchID,
	}
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, publicationErr
}

func (r *registry) resumeAdmissionResult(
	req workersessions.ControlRequest,
	reservation *controlHistoryReservation,
	supervision *supervision,
	continuation workers.WorkstationDispatchRequest,
	previousDispatchID string,
) (workersessions.ControlResult, error) {
	r.finishContinuationPublication(supervision)
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	if !supervision.continuationWasAdmitted() {
		// A Workers boundary may report a terminal callback before its admission
		// callback. Preserve that terminal history; if neither callback arrived,
		// restore the exact paused reservation instead of leaving a phantom
		// STARTING continuation behind.
		if !current.Terminal() && current.State == workersessions.StateStarting {
			r.revertContinuation(req.ID, supervision, previousDispatchID)
			current, _ = r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		}
		result := workersessions.ControlResult{
			Session:    current,
			Action:     workersessions.ControlActionResume,
			Outcome:    workersessions.ControlOutcomeFailed,
			DispatchID: continuation.Execution.Dispatch.DispatchID,
		}
		r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", continuation.Execution.Dispatch.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
		return result, workersessions.ErrStartAdmissionFailed
	}
	result := workersessions.ControlResult{
		Session:    current,
		Action:     workersessions.ControlActionResume,
		Outcome:    workersessions.ControlOutcomeApplied,
		DispatchID: continuation.Execution.Dispatch.DispatchID,
	}
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, nil
}

func (r *registry) finishResumeHistory(
	reservation *controlHistoryReservation,
	result workersessions.ControlResult,
	resumeErr error,
) (workersessions.ControlResult, error) {
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	return result, resumeErr
}
