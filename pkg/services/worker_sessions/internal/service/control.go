package service

import (
	"context"
	"errors"
	"sync"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// supervision is the process-local, immutable dispatch association and
// synchronization state for one Worker Session attempt. It is deliberately
// not exposed in Session snapshots: callers address the stable session ID,
// while Worker Sessions alone owns the exact dispatch ID used for boundary
// cancellation.
type supervision struct {
	dispatchID string

	mu              sync.Mutex
	publishing      bool
	accepted        bool
	requestedAction workersessions.ControlAction
	controlAction   workersessions.ControlAction
	controlActive   bool
	controlDone     chan struct{}
	result          workers.WorkstationDispatchResult
	err             error

	published     chan struct{}
	publishedOnce sync.Once
	done          chan struct{}
	doneOnce      sync.Once
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

func newSupervision(dispatchID string) *supervision {
	return &supervision{
		dispatchID: dispatchID,
		published:  make(chan struct{}),
		done:       make(chan struct{}),
	}
}

func (s *supervision) signalPublished() { s.publishedOnce.Do(func() { close(s.published) }) }
func (s *supervision) signalDone()      { s.doneOnce.Do(func() { close(s.done) }) }

func (s *supervision) beginCancellation(action workersessions.ControlAction) cancellationAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.controlAction != "" {
		return cancellationAttempt{kind: cancellationAttemptNoop}
	}
	if s.controlActive {
		return cancellationAttempt{kind: cancellationAttemptWait, wait: s.controlDone}
	}
	if s.publishing && !s.accepted {
		return cancellationAttempt{kind: cancellationAttemptWait, wait: s.published}
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
		s.controlAction = action
	} else if !alreadyTerminal && !sessionTerminal {
		s.requestedAction = ""
	}
	s.mu.Unlock()
	return alreadyTerminal
}

// Start supervises one resolved dispatch through the injected workstation pool
// boundary. The boundary is the sole mechanism that starts, cancels, and
// reports the attempt; the result callback remains authoritative for terminal
// Workers output, so control cannot fabricate a Factory Runtime result.
func (r *registry) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "invalid")
		return workersessions.StartResult{}, err
	}

	r.reserveIfAbsent(req.ID)
	r.logger.Info("worker session start accepted", "sessionID", req.ID, "attemptID", attemptID, "outcome", "reserved", "state", string(workersessions.StateReserved))
	if _, err := r.transitionToStarting(req.ID); err != nil {
		if terminal, ok := r.preAdmissionControlTerminal(req.ID, attemptID); ok {
			return workersessions.StartResult{
				Session:     terminal,
				Dispatch:    canceledBeforeAdmissionResult(req.Execution),
				DispatchErr: workers.ErrWorkstationDispatchCanceled,
			}, nil
		}
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "not_startable")
		return workersessions.StartResult{}, err
	}

	if err := r.publishOpeningRecord(ctx, req.ID, attemptID); err != nil {
		terminal := failedTerminal(workersessions.FailureCauseEventPublicationFailure, safeDetail(workersessions.FailureCauseEventPublicationFailure, nil))
		final, committed := r.commitTerminal(req.ID, workersessions.StateFailed, terminal)
		if committed {
			r.logTerminal(req.ID, attemptID, final)
			r.publishTerminalRecordOrLog(ctx, req.ID, attemptID, workersessions.StateFailed, terminal)
		}
		return workersessions.StartResult{Session: final}, nil
	}
	return r.startPublishedAttempt(ctx, req, attemptID)
}

// startPublishedAttempt begins boundary supervision only after the opening
// Worker Session record committed. Controls that win before boundary admission
// terminalize the session without sending a cancellation for unknown work.
func (r *registry) startPublishedAttempt(ctx context.Context, req workersessions.StartRequest, attemptID string) (workersessions.StartResult, error) {
	supervision, canStart := r.registerSupervision(req.ID, attemptID)
	if !canStart {
		final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		if final.Terminal() {
			r.publishTerminalRecordOrLog(ctx, req.ID, attemptID, final.State, workersessions.TerminalResult{})
		}
		return workersessions.StartResult{Session: final}, nil
	}
	return r.publishRegisteredAttempt(ctx, req, attemptID, supervision, r.beginBoundaryPublish(req.ID, supervision))
}

func (r *registry) publishRegisteredAttempt(
	ctx context.Context,
	req workersessions.StartRequest,
	attemptID string,
	supervision *supervision,
	canPublish bool,
) (workersessions.StartResult, error) {
	if !canPublish {
		final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		supervision.signalPublished()
		supervision.signalDone()
		if final.Terminal() {
			r.publishTerminalRecordOrLog(ctx, req.ID, attemptID, final.State, workersessions.TerminalResult{})
		}
		return workersessions.StartResult{Session: final}, nil
	}

	r.logger.Info("worker session start", "sessionID", req.ID, "attemptID", attemptID, "outcome", "handoff", "state", string(workersessions.StateStarting))
	handoff := workers.WorkstationDispatchRequest{
		WorkstationName: req.Execution.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(req.Execution.Execution),
	}
	publishErr := r.boundary.PublishWithAdmission(
		context.WithoutCancel(ctx),
		handoff,
		func() { r.acceptSupervision(req.ID, supervision) },
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, dispatchErr error) {
			r.completeSupervision(req.ID, supervision, result, dispatchErr)
		},
	)
	if publishErr != nil {
		final, committed := r.commitTerminal(req.ID, workersessions.StateFailed, classifyTerminal(publishErr, workers.WorkstationDispatchResult{}))
		supervision.mu.Lock()
		supervision.err = publishErr
		supervision.mu.Unlock()
		supervision.signalPublished()
		supervision.signalDone()
		if committed {
			r.logTerminal(req.ID, attemptID, final)
			r.publishTerminalRecordOrLog(ctx, req.ID, attemptID, final.State, *final.Result)
		}
		return workersessions.StartResult{Session: final}, nil
	}

	r.finishSupervisionPublication(supervision)
	<-supervision.done

	final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	supervision.mu.Lock()
	result, dispatchErr := supervision.result, supervision.err
	supervision.mu.Unlock()
	return workersessions.StartResult{Session: final, Dispatch: result, DispatchErr: dispatchErr}, nil
}

// acceptSupervision records Workers' exact cancellable-admission point. It
// deliberately runs from the pool boundary callback rather than after Publish
// returns: synchronous Publish waits for terminal completion, but its admitted
// dispatch must remain controllable throughout that wait.
func (r *registry) acceptSupervision(id string, supervision *supervision) {
	supervision.mu.Lock()
	supervision.accepted = true
	supervision.publishing = false
	supervision.mu.Unlock()
	supervision.signalPublished()
	r.transitionToRunning(id)
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

func (r *registry) registerSupervision(id, dispatchID string) (*supervision, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if session, exists := r.sessions[id]; !exists || session.State != workersessions.StateStarting {
		return nil, false
	}
	supervision := newSupervision(dispatchID)
	r.supervisions[id] = supervision
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

func (r *registry) transitionToRunning(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateStarting {
		return
	}
	session.State = workersessions.StateRunning
	r.sessions[id] = session
}

func (r *registry) completeSupervision(id string, supervision *supervision, result workers.WorkstationDispatchResult, dispatchErr error) {
	supervision.mu.Lock()
	supervision.result = result
	supervision.err = dispatchErr
	action := supervision.requestedAction
	supervision.mu.Unlock()

	state, terminal := dispatchedTerminal(action, result, dispatchErr)
	final, committed := r.commitTerminal(id, state, terminal)
	if committed {
		r.logTerminal(id, supervision.dispatchID, final)
		r.publishTerminalRecordOrLog(context.Background(), id, supervision.dispatchID, state, terminal)
	}
	supervision.signalDone()
}

func dispatchedTerminal(action workersessions.ControlAction, result workers.WorkstationDispatchResult, dispatchErr error) (workersessions.State, workersessions.TerminalResult) {
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled || errors.Is(dispatchErr, workers.ErrWorkstationDispatchCanceled) {
		if action == workersessions.ControlActionTerminate {
			return workersessions.StateTerminated, workersessions.TerminalResult{}
		}
		return workersessions.StateCanceled, workersessions.TerminalResult{}
	}
	terminal := classifyTerminal(dispatchErr, result)
	if terminal.Outcome == workersessions.TerminalOutcomeCompleted {
		return workersessions.StateCompleted, terminal
	}
	return workersessions.StateFailed, terminal
}

func (r *registry) logTerminal(id, attemptID string, session workersessions.Session) {
	cause := ""
	if session.Result != nil {
		cause = causeKindString(session.Result.Cause)
	}
	r.logger.Info("worker session start terminal", "sessionID", id, "attemptID", attemptID, "outcome", string(session.State), "state", string(session.State), "cause", cause)
}

// Pause never changes lifecycle state until Worker Sessions owns a truthful
// resumable execution capability. Returning UNSUPPORTED is deliberate: it
// prevents a fabricated PAUSED state that a later resume could not honor.
func (r *registry) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.unsupportedControl(ctx, req, workersessions.ControlActionPause)
}

// Resume is unsupported until the exact paused provider-session association is
// implemented. Terminal sessions retain their idempotent NOOP behavior.
func (r *registry) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.unsupportedControl(ctx, req, workersessions.ControlActionResume)
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
	return r.cancelControl(ctx, req, workersessions.ControlActionCancel)
}

func (r *registry) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.cancelControl(ctx, req, workersessions.ControlActionTerminate)
}

func (r *registry) cancelControl(ctx context.Context, req workersessions.ControlRequest, action workersessions.ControlAction) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	for {
		session, supervision, err := r.controlTarget(req.ID)
		if err != nil {
			return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed}, err
		}
		if session.Terminal() {
			return r.controlNoop(req.ID, action, session, supervision), nil
		}
		if supervision == nil {
			final, _ := r.commitControlTerminal(req.ID, controlTerminalState(action))
			return r.controlApplied(req.ID, action, final, nil), nil
		}

		attempt := supervision.beginCancellation(action)
		switch attempt.kind {
		case cancellationAttemptNoop:
			return r.controlNoop(req.ID, action, session, supervision), nil
		case cancellationAttemptWait:
			<-attempt.wait
			continue
		case cancellationAttemptBeforeAdmission:
			final, _ := r.commitControlTerminal(req.ID, controlTerminalState(action))
			supervision.signalDone()
			return r.controlApplied(req.ID, action, final, supervision), nil
		case cancellationAttemptBoundary:
			// The exact dispatch is now accepted and can only be stopped through
			// the Workers-owned boundary below.
		}
		wait := attempt.wait
		dispatchID := attempt.dispatchID

		cancelResult, cancelErr := r.boundary.Cancel(context.WithoutCancel(ctx), workers.WorkstationDispatchCancelRequest{DispatchID: dispatchID})
		alreadyTerminal := supervision.finishCancellation(action, wait, cancelResult, cancelErr, sessionIsTerminal(r, req.ID))

		// Terminate promises to return only after the authoritative dispatch
		// callback has committed. Workers reports an already-canceled dispatch
		// without an error, but that callback can still be in flight; join it
		// before returning the idempotent snapshot. Cancel intentionally keeps
		// its non-joining already-canceled behavior.
		if alreadyTerminal || (action == workersessions.ControlActionTerminate && cancelAlreadyCanceled(cancelResult, cancelErr)) {
			<-supervision.done
			current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
			result := workersessions.ControlResult{
				Session: current, Action: action, Outcome: workersessions.ControlOutcomeNoop, DispatchID: dispatchID,
			}
			r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", dispatchID, "action", string(action), "outcome", string(result.Outcome))
			return result, nil
		}

		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := workersessions.ControlResult{Session: current, Action: action, DispatchID: dispatchID}
		if cancelErr != nil {
			result.Outcome = workersessions.ControlOutcomeFailed
			r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", dispatchID, "action", string(action), "outcome", string(result.Outcome))
			return result, cancelErr
		}
		if cancelResult.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
			result.Outcome = workersessions.ControlOutcomeNoop
			return result, nil
		}
		if action == workersessions.ControlActionTerminate {
			<-supervision.done
			result.Session, _ = r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		}
		result.Outcome = workersessions.ControlOutcomeApplied
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", dispatchID, "action", string(action), "outcome", string(result.Outcome))
		return result, nil
	}
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
