package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Pause stops only an exact admitted attempt that already retained a complete
// Provider Session association. The session becomes PAUSED only after the
// established Workers cancellation callback has committed; an unassociated
// execution remains truthfully unsupported rather than becoming a fabricated
// resumable session.
func (r *registry) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	reservation, err := r.beginControlHistory(ctx, req.ID, workersessions.ControlActionPause, req.RequestID)
	if err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	for {
		result, retry, iterationErr := r.pauseIteration(ctx, req)
		if !retry {
			r.finishControlHistory(reservation, controlResultOutcome(result, iterationErr), result.DispatchID, result.Session.State)
			return result, iterationErr
		}
	}
}

func (r *registry) pauseIteration(
	ctx context.Context,
	req workersessions.ControlRequest,
) (workersessions.ControlResult, bool, error) {
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, false, err
	}
	if session.Terminal() || session.State == workersessions.StatePaused {
		return r.controlNoop(req.ID, workersessions.ControlActionPause, session, supervision), false, nil
	}
	if session.State != workersessions.StateRunning || supervision == nil || session.ProviderSessionAssociation == nil {
		result, err := r.unsupportedControl(ctx, req, workersessions.ControlActionPause)
		return result, false, err
	}

	attempt := supervision.beginCancellation(workersessions.ControlActionPause)
	switch attempt.kind {
	case cancellationAttemptNoop:
		return r.controlNoop(req.ID, workersessions.ControlActionPause, session, supervision), false, nil
	case cancellationAttemptWait:
		<-attempt.wait
		return workersessions.ControlResult{}, true, nil
	case cancellationAttemptBeforeAdmission:
		result, err := r.unsupportedControl(ctx, req, workersessions.ControlActionPause)
		return result, false, err
	case cancellationAttemptBoundary:
		return r.pauseBoundary(ctx, req, supervision, attempt)
	default:
		return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, false, fmt.Errorf("unknown pause cancellation attempt")
	}
}

func (r *registry) pauseBoundary(
	ctx context.Context,
	req workersessions.ControlRequest,
	supervision *supervision,
	attempt cancellationAttempt,
) (workersessions.ControlResult, bool, error) {
	cancelResult, cancelErr := r.boundary.Cancel(
		context.WithoutCancel(ctx),
		workers.WorkstationDispatchCancelRequest{DispatchID: attempt.dispatchID},
	)
	alreadyTerminal := supervision.finishCancellation(
		workersessions.ControlActionPause,
		attempt.wait,
		cancelResult,
		cancelErr,
		sessionIsTerminal(r, req.ID),
	)
	if cancelErr != nil {
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := workersessions.ControlResult{Session: current, Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed, DispatchID: attempt.dispatchID}
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
		return result, false, cancelErr
	}
	if alreadyTerminal || cancelResult.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		<-supervision.done
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		return r.controlNoop(req.ID, workersessions.ControlActionPause, current, supervision), false, nil
	}
	select {
	case <-supervision.paused:
	case <-supervision.done:
	}
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result := workersessions.ControlResult{Session: current, Action: workersessions.ControlActionPause, DispatchID: attempt.dispatchID, Outcome: workersessions.ControlOutcomeNoop}
	if current.State == workersessions.StatePaused {
		result.Outcome = workersessions.ControlOutcomeApplied
	}
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, false, nil
}

func validateResumeAssociation(session workersessions.Session) error {
	association := session.ProviderSessionAssociation
	if association == nil {
		return workersessions.ErrProviderSessionAssociationMissing
	}
	if err := association.Validate(); err != nil {
		return fmt.Errorf("%w: %w", workersessions.ErrInvalidProviderSessionAssociation, err)
	}
	if association.WorkerSessionID != session.ID {
		return fmt.Errorf("%w: worker session identity mismatch", workersessions.ErrInvalidProviderSessionAssociation)
	}
	return nil
}

// validateResumeAssociationForSupervision checks the registry-owned control
// and attempt facts immediately before a resume can reserve a Workers
// continuation. A stored Provider Session reference is not sufficient on its
// own: it must still belong to the admitted paused attempt that owns this
// supervision. This check deliberately happens before prepareContinuation and
// therefore before any Workers or Providers call.
func validateResumeAssociationForSupervision(session workersessions.Session, supervision *supervision) error {
	if err := validateResumeAssociation(session); err != nil {
		return err
	}
	if supervision == nil {
		return workersessions.ErrProviderSessionAssociationNotAvailable
	}

	association := session.ProviderSessionAssociation
	supervision.mu.Lock()
	dispatchID := strings.TrimSpace(supervision.dispatchID)
	turnID := strings.TrimSpace(supervision.turnID)
	accepted := supervision.accepted
	activeControl := supervision.controlActive || supervision.requestedAction != "" || supervision.controlAction != ""
	continuing := supervision.continuing
	publishing := supervision.publishing
	supervision.mu.Unlock()

	if dispatchID == "" {
		return workersessions.ErrProviderSessionAssociationNotAvailable
	}
	if association.DispatchID != dispatchID || association.AttemptID != dispatchID {
		return fmt.Errorf("%w: stored association does not belong to the active attempt", workersessions.ErrProviderSessionAssociationAttemptMismatch)
	}
	if strings.TrimSpace(association.TurnID) != turnID {
		return fmt.Errorf("%w: stored association does not belong to the active turn", workersessions.ErrProviderSessionAssociationAttemptMismatch)
	}
	if !accepted {
		return workersessions.ErrProviderSessionAssociationNotAvailable
	}
	if activeControl || continuing || publishing {
		return fmt.Errorf("%w: resume is owned by another active control", workersessions.ErrInvalidState)
	}
	return nil
}

func (r *registry) rejectedResume(
	session workersessions.Session,
	supervision *supervision,
	err error,
) (workersessions.ControlResult, error) {
	result := workersessions.ControlResult{
		Session: session,
		Action:  workersessions.ControlActionResume,
		Outcome: workersessions.ControlOutcomeFailed,
	}
	if supervision != nil {
		result.DispatchID = supervision.dispatchID
	}
	r.logger.Info(
		"worker session control",
		"sessionID", session.ID,
		"attemptID", result.DispatchID,
		"action", string(result.Action),
		"outcome", string(result.Outcome),
	)
	return result, err
}

func (s *supervision) resumeInFlight() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.continuing
}

func (s *supervision) continuationWasAdmitted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

func (r *registry) prepareContinuation(
	id string,
	supervision *supervision,
	reference providers.SessionRef,
) (workers.WorkstationDispatchRequest, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StatePaused || session.ProviderSessionAssociation == nil ||
		session.ProviderSessionAssociation.Reference != reference {
		return workers.WorkstationDispatchRequest{}, "", false
	}

	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	association := session.ProviderSessionAssociation
	if !supervision.accepted || supervision.dispatchID == "" ||
		association.DispatchID != supervision.dispatchID || association.AttemptID != supervision.dispatchID ||
		strings.TrimSpace(association.TurnID) != strings.TrimSpace(supervision.turnID) ||
		supervision.continuing || supervision.publishing {
		return workers.WorkstationDispatchRequest{}, "", false
	}
	previousDispatchID := supervision.dispatchID
	supervision.resumeCount++
	supervision.attemptsMade++
	continuation := cloneWorkstationDispatchRequest(supervision.execution)
	continuation.Execution.Dispatch.DispatchID = fmt.Sprintf("%s/resume/%d", previousDispatchID, supervision.resumeCount)
	continuationRef := reference.ContinuationRef()
	continuation.Execution.Continuation = &continuationRef
	supervision.dispatchID = continuation.Execution.Dispatch.DispatchID
	delete(r.dispatchOwners, previousDispatchID)
	r.dispatchOwners[supervision.dispatchID] = id
	supervision.publishing = true
	supervision.accepted = false
	supervision.continuing = true
	supervision.result = workers.WorkstationDispatchResult{}
	supervision.err = nil
	session.State = workersessions.StateStarting
	r.sessions[id] = session
	return continuation, previousDispatchID, true
}

func (r *registry) revertContinuation(id string, supervision *supervision, previousDispatchID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	supervision.mu.Lock()
	currentDispatchID := supervision.dispatchID
	supervision.dispatchID = previousDispatchID
	if supervision.attemptsMade > 0 {
		supervision.attemptsMade--
	}
	supervision.publishing = false
	supervision.continuing = false
	supervision.accepted = true
	supervision.mu.Unlock()

	delete(r.dispatchOwners, currentDispatchID)
	r.dispatchOwners[previousDispatchID] = id
	if session, exists := r.sessions[id]; exists && session.State == workersessions.StateStarting {
		session.State = workersessions.StatePaused
		r.sessions[id] = session
	}
}

func (r *registry) finishContinuationPublication(supervision *supervision) {
	supervision.mu.Lock()
	supervision.publishing = false
	supervision.mu.Unlock()
}

func (r *registry) unsupportedControl(_ context.Context, req workersessions.ControlRequest, action workersessions.ControlAction) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	result := workersessions.ControlResult{Session: session, Action: action, Outcome: workersessions.ControlOutcomeUnsupported}
	if supervision != nil {
		result.DispatchID = supervision.dispatchID
	}
	if session.Terminal() {
		result.Outcome = workersessions.ControlOutcomeNoop
	}
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(action), "outcome", string(result.Outcome))
	return result, nil
}

func (r *registry) Cancel(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.cancelControl(ctx, req, workersessions.ControlActionCancel, true)
}

func (r *registry) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.cancelControl(ctx, req, workersessions.ControlActionTerminate, true)
}

func (r *registry) terminateForShutdown(ctx context.Context, id string) (workersessions.ControlResult, error) {
	return r.cancelControl(ctx, workersessions.ControlRequest{ID: id}, workersessions.ControlActionTerminate, false)
}

func (r *registry) cancelControl(ctx context.Context, req workersessions.ControlRequest, action workersessions.ControlAction, detachContext bool) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	reservation, err := r.beginControlHistory(ctx, req.ID, action, req.RequestID)
	if err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	for {
		result, retry, iterationErr := r.cancelControlIteration(ctx, req, action, detachContext)
		if !retry {
			r.finishControlHistory(reservation, controlResultOutcome(result, iterationErr), result.DispatchID, result.Session.State)
			return result, iterationErr
		}
	}
}

func (r *registry) cancelControlIteration(
	ctx context.Context,
	req workersessions.ControlRequest,
	action workersessions.ControlAction,
	detachContext bool,
) (workersessions.ControlResult, bool, error) {
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, false, err
	}
	if session.Terminal() {
		return r.controlNoop(req.ID, action, session, supervision), false, nil
	}
	if supervision == nil {
		final, _ := r.commitControlTerminal(req.ID, controlTerminalState(action))
		return r.controlApplied(req.ID, action, final, nil), false, nil
	}
	if session.State == workersessions.StatePaused {
		return r.terminalizePausedControl(req.ID, action, supervision), false, nil
	}

	attempt := supervision.beginCancellation(action)
	switch attempt.kind {
	case cancellationAttemptNoop:
		return r.controlNoop(req.ID, action, session, supervision), false, nil
	case cancellationAttemptWait:
		<-attempt.wait
		return workersessions.ControlResult{}, true, nil
	case cancellationAttemptBeforeAdmission:
		final, _ := r.commitControlTerminal(req.ID, controlTerminalState(action))
		r.finishControlHistory(controlReservationFor(supervision), workersessions.ControlOutcomeApplied, supervision.dispatchID, final.State)
		supervision.signalDone()
		return r.controlApplied(req.ID, action, final, supervision), false, nil
	case cancellationAttemptBoundary:
		return r.cancelBoundary(ctx, req, action, detachContext, supervision, attempt)
	default:
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, false, fmt.Errorf("unknown terminal cancellation attempt")
	}
}

func (r *registry) cancelBoundary(
	ctx context.Context,
	req workersessions.ControlRequest,
	action workersessions.ControlAction,
	detachContext bool,
	supervision *supervision,
	attempt cancellationAttempt,
) (workersessions.ControlResult, bool, error) {
	boundaryContext := ctx
	if detachContext {
		boundaryContext = context.WithoutCancel(ctx)
	}
	cancelResult, cancelErr := r.boundary.Cancel(boundaryContext, workers.WorkstationDispatchCancelRequest{DispatchID: attempt.dispatchID})
	alreadyTerminal := supervision.finishCancellation(action, attempt.wait, cancelResult, cancelErr, sessionIsTerminal(r, req.ID))

	// Every terminal control promises to return only after the authoritative
	// dispatch callback has committed. Workers can report an already-canceled
	// dispatch while that callback is still in flight, so the callback channel
	// remains the canonical join point for both Cancel and Terminate.
	if alreadyTerminal || cancelAlreadyCanceled(cancelResult, cancelErr) {
		<-supervision.done
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := workersessions.ControlResult{Session: current, Action: action, Outcome: workersessions.ControlOutcomeNoop, DispatchID: attempt.dispatchID}
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(action), "outcome", string(result.Outcome))
		return result, false, nil
	}

	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result := workersessions.ControlResult{Session: current, Action: action, DispatchID: attempt.dispatchID}
	if cancelErr != nil {
		result.Outcome = workersessions.ControlOutcomeFailed
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(action), "outcome", string(result.Outcome))
		return result, false, cancelErr
	}
	if cancelResult.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		result.Outcome = workersessions.ControlOutcomeNoop
		return result, false, nil
	}
	<-supervision.done
	result.Session, _ = r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result.Outcome = workersessions.ControlOutcomeApplied
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(action), "outcome", string(result.Outcome))
	return result, false, nil
}

// terminalizePausedControl consumes a PAUSED session without asking Workers to
// cancel its already-canceled original dispatch. The original Start call is
// intentionally waiting for either continuation or this terminal decision, so
// signalDone is the authoritative release after the terminal record commits.
func (r *registry) terminalizePausedControl(id string, action workersessions.ControlAction, supervision *supervision) workersessions.ControlResult {
	state := controlTerminalState(action)
	final, committed := r.commitControlTerminal(id, state)
	if committed {
		r.finishControlHistory(controlReservationFor(supervision), workersessions.ControlOutcomeApplied, supervision.dispatchID, final.State)
		r.logTerminal(id, supervision.dispatchID, final)
		r.publishTerminalRecordOrLog(r.supervisionContext(supervision), id, supervision.dispatchID, state, workersessions.TerminalResult{})
		supervision.signalDone()
	}
	return r.controlApplied(id, action, final, supervision)
}

func (r *registry) preAdmissionControlTerminal(id, attemptID string) (workersessions.Session, bool) {
	session, err := r.Get(context.Background(), workersessions.GetRequest{ID: id})
	if err != nil || (session.State != workersessions.StateCanceled && session.State != workersessions.StateTerminated) {
		return workersessions.Session{}, false
	}
	r.logger.Info("worker session start skipped", "sessionID", id, "attemptID", attemptID, "outcome", string(session.State), "state", string(session.State))
	return session, true
}

func canceledBeforeAdmissionResult(request workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		WorkstationName: request.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCanceled,
		Result: workers.WorkResult{
			DispatchID:   request.Execution.Dispatch.DispatchID,
			TransitionID: request.Execution.Dispatch.TransitionID,
			Outcome:      workers.OutcomeFailed,
			Error:        workers.ErrWorkstationDispatchCanceled.Error(),
		},
	}
}

func cancelAlreadyTerminal(result workers.WorkstationDispatchCancelResult, err error) bool {
	return result.Outcome == workers.WorkstationDispatchCancelOutcomeAlreadyTerminal &&
		(err == nil || errors.Is(err, workers.ErrWorkstationDispatchAlreadyTerminal))
}

func cancelAlreadyCanceled(result workers.WorkstationDispatchCancelResult, err error) bool {
	return result.Outcome == workers.WorkstationDispatchCancelOutcomeAlreadyCanceled && err == nil
}

func (r *registry) controlTarget(id string) (workersessions.Session, *supervision, error) {
	r.mu.RLock()
	session, exists := r.sessions[id]
	supervision := r.supervisions[id]
	r.mu.RUnlock()
	if !exists {
		return workersessions.Session{}, nil, workersessions.ErrSessionNotFound
	}
	return cloneSession(session), supervision, nil
}

func (r *registry) controlNoop(id string, action workersessions.ControlAction, session workersessions.Session, supervision *supervision) workersessions.ControlResult {
	// Every terminal control promises the terminal callback's snapshot,
	// including a no-op caused by an earlier terminal control.
	if supervision != nil && (action == workersessions.ControlActionCancel || action == workersessions.ControlActionTerminate) {
		<-supervision.done
		if current, err := r.Get(context.Background(), workersessions.GetRequest{ID: id}); err == nil {
			session = current
		}
	}
	result := workersessions.ControlResult{Session: session, Action: action, Outcome: workersessions.ControlOutcomeNoop}
	if supervision != nil {
		result.DispatchID = supervision.dispatchID
	}
	r.logger.Info("worker session control", "sessionID", id, "attemptID", result.DispatchID, "action", string(action), "outcome", string(result.Outcome))
	return result
}

func (r *registry) controlApplied(id string, action workersessions.ControlAction, session workersessions.Session, supervision *supervision) workersessions.ControlResult {
	result := workersessions.ControlResult{Session: session, Action: action, Outcome: workersessions.ControlOutcomeApplied}
	if supervision != nil {
		result.DispatchID = supervision.dispatchID
	}
	r.logger.Info("worker session control", "sessionID", id, "attemptID", result.DispatchID, "action", string(action), "outcome", string(result.Outcome))
	return result
}

func controlTerminalState(action workersessions.ControlAction) workersessions.State {
	if action == workersessions.ControlActionTerminate {
		return workersessions.StateTerminated
	}
	return workersessions.StateCanceled
}

func sessionIsTerminal(r *registry, id string) bool {
	session, err := r.Get(context.Background(), workersessions.GetRequest{ID: id})
	return err == nil && session.Terminal()
}

// supervision is the process-local, immutable dispatch association and
// synchronization state for one Worker Session attempt. Session snapshots can
// expose only a detached accepted Provider Session association; this mutable
// supervision remains Worker Sessions-owned so callers address controls by
// stable session ID while Worker Sessions retains the exact dispatch ID.
type supervision struct {
	dispatchID string
	turnID     string
	execution  workers.WorkstationDispatchRequest
	startedAt  time.Time
	deadlineAt time.Time

	mu                 sync.Mutex
	publishing         bool
	accepted           bool
	serverOwned        bool
	continuing         bool
	resumeCount        uint
	preAdmissionAction workersessions.ControlAction
	requestedAction    workersessions.ControlAction
	controlAction      workersessions.ControlAction
	controlActive      bool
	controlDone        chan struct{}
	controlHistory     *controlHistoryReservation
	interrupting       bool
	interruptRequestID string
	interruptDone      chan struct{}
	result             workers.WorkstationDispatchResult
	err                error

	// retryBudget is the total attempt allowance for this supervision and
	// attemptsMade counts the attempts actually published. retryPending records
	// one completed attempt's decision to run another; attemptDone is the
	// per-attempt release the invocation driver waits on, recreated before each
	// publish. They are deliberately separate from done: controls wait on done
	// for the session's final terminal outcome, which a retried attempt has not
	// reached yet.
	retryBudget  int
	attemptsMade int
	retryPending bool
	attemptDone  chan struct{}

	published      chan struct{}
	publishedOnce  sync.Once
	paused         chan struct{}
	pausedOnce     sync.Once
	admitted       chan struct{}
	admittedOnce   sync.Once
	done           chan struct{}
	doneOnce       sync.Once
	driverDone     chan struct{}
	driverDoneOnce sync.Once
}

type cancellationAttemptKind uint8

const (
	cancellationAttemptNoop cancellationAttemptKind = iota
	cancellationAttemptWait
	cancellationAttemptBeforeAdmission
	cancellationAttemptBoundary
)

type cancellationAttempt struct {
	kind       cancellationAttemptKind
	wait       chan struct{}
	dispatchID string
}

func newSupervision(dispatchID, turnID string, executions ...workers.WorkstationDispatchRequest) *supervision {
	var execution workers.WorkstationDispatchRequest
	if len(executions) > 0 {
		execution = executions[0]
	}
	return &supervision{
		dispatchID:  dispatchID,
		turnID:      turnID,
		execution:   cloneWorkstationDispatchRequest(execution),
		retryBudget: 1,
		published:   make(chan struct{}),
		paused:      make(chan struct{}),
		admitted:    make(chan struct{}),
		done:        make(chan struct{}),
		driverDone:  make(chan struct{}),
	}
}

// beginAttempt installs the release channel for the attempt about to publish
// and clears the previous attempt's retry decision. The channel is recreated
// per attempt because each attempt needs its own one-shot release; done stays
// reserved for the session's single terminal outcome.
func (s *supervision) beginAttempt() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attemptsMade++
	s.retryPending = false
	s.attemptDone = make(chan struct{})
	s.deadlineAt = time.Time{}
	return s.attemptDone
}

// finishAttempt releases whichever attempt channel is currently installed.
// A supervision whose attempt never began (a control that won before any
// publish) has none, which is not an error.
func (s *supervision) finishAttempt() {
	s.mu.Lock()
	attemptDone := s.attemptDone
	s.attemptDone = nil
	s.mu.Unlock()
	if attemptDone != nil {
		close(attemptDone)
	}
}

// retryDecided reports the attempt's retry decision to the driver.
func (s *supervision) retryDecided() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retryPending
}

func cloneWorkstationDispatchRequest(request workers.WorkstationDispatchRequest) workers.WorkstationDispatchRequest {
	return workers.WorkstationDispatchRequest{
		WorkstationName: request.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(request.Execution),
	}
}

func (s *supervision) signalPublished() { s.publishedOnce.Do(func() { close(s.published) }) }
func (s *supervision) signalPaused()    { s.pausedOnce.Do(func() { close(s.paused) }) }
func (s *supervision) signalAdmitted()  { s.admittedOnce.Do(func() { close(s.admitted) }) }

// signalDone releases the session's terminal waiters, and releases any attempt
// still in flight first. Every terminal path -- including the controls that
// terminalize before Workers ever admitted the dispatch -- reaches the session
// through signalDone, so binding the two here is what keeps the invocation
// driver from waiting on an attempt that will never report.
func (s *supervision) signalDone() {
	s.finishAttempt()
	s.doneOnce.Do(func() { close(s.done) })
}

func (s *supervision) signalDriverDone() {
	s.driverDoneOnce.Do(func() { close(s.driverDone) })
}

func (s *supervision) beginCancellation(action workersessions.ControlAction) cancellationAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interrupting {
		return cancellationAttempt{kind: cancellationAttemptWait, wait: s.interruptDone}
	}
	if s.controlAction != "" {
		return cancellationAttempt{kind: cancellationAttemptNoop}
	}
	if s.requestedAction != "" {
		return cancellationAttempt{kind: cancellationAttemptNoop}
	}
	if s.controlActive {
		return cancellationAttempt{kind: cancellationAttemptWait, wait: s.controlDone}
	}
	if s.publishing && !s.accepted {
		if s.preAdmissionAction == "" {
			s.preAdmissionAction = action
		}
		return cancellationAttempt{kind: cancellationAttemptWait, wait: s.published}
	}
	if s.preAdmissionAction != "" {
		action = s.preAdmissionAction
		s.preAdmissionAction = ""
		s.controlActive = true
		s.controlDone = make(chan struct{})
		s.requestedAction = action
		return cancellationAttempt{kind: cancellationAttemptBoundary, wait: s.controlDone, dispatchID: s.dispatchID}
	}
	if !s.accepted {
		s.controlAction = action
		return cancellationAttempt{kind: cancellationAttemptBeforeAdmission}
	}
	s.controlActive = true
	s.controlDone = make(chan struct{})
	s.requestedAction = action
	return cancellationAttempt{kind: cancellationAttemptBoundary, wait: s.controlDone, dispatchID: s.dispatchID}
}

func (s *supervision) finishCancellation(
	action workersessions.ControlAction,
	wait chan struct{},
	cancelResult workers.WorkstationDispatchCancelResult,
	cancelErr error,
	sessionTerminal bool,
) bool {
	alreadyTerminal := cancelAlreadyTerminal(cancelResult, cancelErr)
	s.mu.Lock()
	s.controlActive = false
	close(wait)
	if cancelErr == nil && cancelResult.Outcome == workers.WorkstationDispatchCancelOutcomeCanceled {
		if action != workersessions.ControlActionPause {
			s.controlAction = action
		}
	} else if !alreadyTerminal && !sessionTerminal {
		s.requestedAction = ""
	}
	s.mu.Unlock()
	return alreadyTerminal
}

// baseDispatchID returns the identity every attempt suffix is derived from, so
// attempt 3 is ".../attempt/3" rather than ".../attempt/2/attempt/3".
func (s *supervision) baseDispatchID() string {
	return s.execution.Execution.Dispatch.DispatchID
}

func (s *supervision) attemptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attemptsMade
}

func (s *supervision) lastResult() workers.WorkstationDispatchResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

// acceptSupervision records Workers' exact cancellable-admission point. It
// deliberately runs from the pool boundary callback rather than after Publish
// returns: synchronous Publish waits for terminal completion, but its admitted
// dispatch must remain controllable throughout that wait.
func (r *registry) acceptSupervision(id string, supervision *supervision) {
	running := r.transitionToRunning(id)
	acceptedAt := time.Time{}
	if running {
		acceptedAt = r.clock.Now()
	}
	reservation := controlReservationFor(supervision)
	if reservation != nil && reservation.action == workersessions.ControlActionResume {
		outcome := workersessions.ControlOutcomeApplied
		state := workersessions.StateRunning
		if !running {
			outcome = workersessions.ControlOutcomeFailed
			if current, err := r.Get(context.Background(), workersessions.GetRequest{ID: id}); err == nil {
				state = current.State
			}
		}
		r.finishControlHistory(reservation, outcome, supervision.dispatchID, state)
	}
	supervision.mu.Lock()
	supervision.accepted = running
	if running {
		supervision.startedAt = acceptedAt
		supervision.deadlineAt = time.Time{}
	}
	serverOwned := supervision.serverOwned
	preAdmissionControl := supervision.preAdmissionAction != ""
	supervision.publishing = false
	supervision.mu.Unlock()
	supervision.signalPublished()
	if running {
		r.startDeadlineWatcher(id, supervision, acceptedAt)
	}
	if running && serverOwned && !preAdmissionControl {
		supervision.signalAdmitted()
		r.logger.Info("worker session start", "sessionID", id, "attemptID", supervision.dispatchID, "outcome", "admitted", "state", string(workersessions.StateRunning))
	}
}

// finishSupervisionPublication releases any control waiting for a publish
// that reached a terminal result before Workers admitted it. An admitted
// attempt has already been released by acceptSupervision.
func (r *registry) finishSupervisionPublication(supervision *supervision) {
	supervision.mu.Lock()
	supervision.publishing = false
	supervision.mu.Unlock()
	supervision.signalPublished()
}

func (r *registry) registerSupervision(
	id, dispatchID, turnID string,
	executions ...workers.WorkstationDispatchRequest,
) (*supervision, bool) {
	return r.registerSupervisionOwned(false, id, dispatchID, turnID, executions...)
}

func (r *registry) registerServerOwnedSupervision(
	id, dispatchID, turnID string,
	executions ...workers.WorkstationDispatchRequest,
) (*supervision, bool) {
	return r.registerSupervisionOwned(true, id, dispatchID, turnID, executions...)
}

func (r *registry) registerSupervisionOwned(
	serverOwned bool,
	id, dispatchID, turnID string,
	executions ...workers.WorkstationDispatchRequest,
) (*supervision, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return nil, false
	}
	if session, exists := r.sessions[id]; !exists || session.State != workersessions.StateStarting {
		return nil, false
	}
	supervision := newSupervision(dispatchID, turnID, executions...)
	supervision.serverOwned = serverOwned
	supervision.startedAt = r.clock.Now()
	r.supervisions[id] = supervision
	r.dispatchOwners[dispatchID] = id
	return supervision, true
}

func (r *registry) beginBoundaryPublish(id string, supervision *supervision) bool {
	r.mu.RLock()
	session := r.sessions[id]
	r.mu.RUnlock()
	if session.State != workersessions.StateStarting {
		return false
	}
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	if supervision.controlAction != "" || supervision.requestedAction != "" {
		return false
	}
	supervision.publishing = true
	return true
}

func (r *registry) transitionToRunning(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateStarting {
		return false
	}
	session.State = workersessions.StateRunning
	r.sessions[id] = session
	return true
}

func (r *registry) logReconciliation(
	id, attemptID string,
	result workers.WorkstationDispatchResult,
	priorState, resultingState workersessions.State,
	startedAt time.Time,
	deadlineAt time.Time,
) {
	elapsedMS := int64(0)
	if !startedAt.IsZero() {
		elapsedMS = r.clock.Now().Sub(startedAt).Milliseconds()
		if elapsedMS < 0 {
			elapsedMS = 0
		}
	}
	deadline := "not_applicable"
	configuredTimeoutMS := int64(0)
	if !deadlineAt.IsZero() {
		deadline = deadlineAt.UTC().Format(time.RFC3339Nano)
		configuredTimeoutMS = deadlineAt.Sub(startedAt).Milliseconds()
	}
	fields := []any{
		"sessionID", id,
		"attemptID", attemptID,
		"dispatchID", result.DispatchID,
		"reason", string(result.ReconciliationReason),
		"prior_state", string(priorState),
		"resulting_state", string(resultingState),
		"result", string(result.TerminalOutcome),
		"elapsed_ms", elapsedMS,
		"deadline", deadline,
	}
	if !deadlineAt.IsZero() {
		fields = append(fields, "configured_timeout_ms", configuredTimeoutMS)
	}
	r.logger.Info("worker session reconciliation", fields...)
}

func (r *registry) completeSupervision(id string, supervision *supervision, result workers.WorkstationDispatchResult, dispatchErr error) {
	supervision.mu.Lock()
	action := supervision.requestedAction
	continuing := supervision.continuing
	dispatchID := supervision.dispatchID
	serverOwned := supervision.serverOwned
	startedAt := supervision.startedAt
	deadlineAt := supervision.deadlineAt
	supervision.mu.Unlock()
	priorState := r.sessionState(id)
	if continuing && !r.continuationResultMatchesAssociation(id, result) {
		result = invalidContinuationResult(result)
		r.logger.Info("worker session continuation result rejected", "sessionID", id, "attemptID", dispatchID, "outcome", "reference_mismatch")
	}
	supervision.mu.Lock()
	supervision.result = result
	supervision.err = dispatchErr
	supervision.mu.Unlock()
	if !continuing {
		r.associateProviderSessionFromResult(id, dispatchID, result)
	}

	if action == workersessions.ControlActionPause && dispatchCanceled(result, dispatchErr) {
		if r.transitionToPaused(id) {
			r.finishControlHistory(controlReservationFor(supervision), workersessions.ControlOutcomeApplied, dispatchID, workersessions.StatePaused)
			supervision.mu.Lock()
			supervision.requestedAction = ""
			supervision.mu.Unlock()
			r.logger.Info("worker session control", "sessionID", id, "attemptID", dispatchID, "action", string(action), "outcome", string(workersessions.ControlOutcomeApplied))
			supervision.signalPaused()
			return
		}
	}

	// A retryable failure with budget left is deliberately not terminal: the
	// session stays open, its publication window stays open, and the driver
	// publishes the next attempt. Releasing only the attempt -- never done --
	// is what keeps controls waiting for the session's real terminal outcome
	// rather than being satisfied by an attempt that is about to be retried.
	if r.claimRetryAttempt(supervision, action, result, dispatchErr) {
		r.logReconciliationIfNeeded(id, dispatchID, result, priorState, workersessions.StateRunning, startedAt, deadlineAt)
		r.logger.Info(
			"worker session attempt",
			"sessionID", id,
			"attemptID", dispatchID,
			"outcome", "retryable_failure",
		)
		supervision.finishAttempt()
		return
	}

	state, terminal := dispatchedTerminal(action, result, dispatchErr)
	controlOutcome := controlOutcomeFromDispatch(action, result, dispatchErr)
	if action == workersessions.ControlActionPause && state != workersessions.StatePaused {
		controlOutcome = workersessions.ControlOutcomeNoop
	}
	if reservation := controlReservationFor(supervision); reservation != nil {
		if reservation.action == workersessions.ControlActionResume && action == "" {
			controlOutcome = workersessions.ControlOutcomeFailed
		}
		r.finishControlHistory(reservation, controlOutcome, dispatchID, state)
	}
	final, committed := r.commitTerminal(id, state, terminal)
	if committed {
		r.logTerminal(id, dispatchID, final)
		r.logReconciliationIfNeeded(id, dispatchID, result, priorState, state, startedAt, deadlineAt)
		terminalContext := context.Background()
		if serverOwned {
			terminalContext = r.serverOwnedContext()
		}
		r.publishTerminalRecordOrLog(terminalContext, id, dispatchID, state, *final.Result)
	}
	supervision.finishAttempt()
	supervision.signalDone()
}

// continuationResultMatchesAssociation keeps every Workers execution path
// accountable to the exact reference that admitted the continuation. Agent
// runners reject a provider mismatch before progress is published, while this
// final check prevents another Workers adapter from committing a plausible
// success under a stale or foreign retained association.
func (r *registry) continuationResultMatchesAssociation(id string, result workers.WorkstationDispatchResult) bool {
	if result.Result.Outcome != workers.OutcomeAccepted && result.Result.Outcome != workers.OutcomeContinue {
		return true
	}
	continuation := result.Result.Continuation
	if continuation == nil {
		return false
	}
	reference, err := continuation.ToSessionRef()
	if err != nil {
		return false
	}
	r.mu.RLock()
	session, exists := r.sessions[id]
	r.mu.RUnlock()
	return exists && session.ProviderSessionAssociation != nil &&
		session.ProviderSessionAssociation.Reference == reference
}

func invalidContinuationResult(result workers.WorkstationDispatchResult) workers.WorkstationDispatchResult {
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypePermanentBadRequest,
	}
	result.Result.ProviderContinuationFailureKind = providers.ContinuationFailureKindInvalid
	result.Result.ProviderContinuationOutcome = ""
	return result
}

func dispatchCanceled(result workers.WorkstationDispatchResult, dispatchErr error) bool {
	return result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled ||
		errors.Is(dispatchErr, workers.ErrWorkstationDispatchCanceled)
}
