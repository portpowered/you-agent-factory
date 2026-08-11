package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// startTuple is the detached, normalized value compared for one asynchronous
// caller request ID. Retry zero and one are the same effective policy, and the
// resolved Workers execution is cloned before it enters this tuple so caller
// mutation cannot change a replay decision or an admitted execution.
type startTuple struct {
	SessionID   string
	Execution   workers.WorkstationDispatchRequest
	MaxAttempts int
}

// startReplay is the one process-local record for an asynchronous caller
// request ID. The owner closes done after the original acceptance or
// deterministic pre-admission failure is complete; every replay returns that
// stored outcome instead of entering the start state machine again.
type startReplay struct {
	tuple     startTuple
	sessionID string
	done      chan struct{}
	result    workersessions.StartResult
	err       error
}

func normalizeStartRequest(req workersessions.StartRequest) workersessions.StartRequest {
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.Execution = cloneWorkstationDispatchRequest(req.Execution)
	req.Retry.MaxAttempts = req.Retry.Attempts()
	return req
}

func startTupleFor(req workersessions.StartRequest) startTuple {
	return startTuple{
		SessionID:   req.ID,
		Execution:   cloneWorkstationDispatchRequest(req.Execution),
		MaxAttempts: req.Retry.Attempts(),
	}
}

func awaitStartReplay(replay *startReplay) (workersessions.StartResult, error) {
	<-replay.done
	return cloneStartResult(replay.result), replay.err
}

func (r *registry) finishStartReplay(replay *startReplay, result workersessions.StartResult, err error) {
	replay.result = cloneStartResult(result)
	replay.err = err
	close(replay.done)
}

func cloneStartResult(result workersessions.StartResult) workersessions.StartResult {
	result.Session = cloneSession(result.Session)
	return result
}

func startReservationOutcome(err error) string {
	if errors.Is(err, workersessions.ErrStartRequestIDConflict) {
		return "idempotency_conflict"
	}
	return "not_startable"
}

func startReplayOutcome(err error) string {
	if err == nil {
		return "accepted"
	}
	return "rejected"
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

	published     chan struct{}
	publishedOnce sync.Once
	paused        chan struct{}
	pausedOnce    sync.Once
	admitted      chan struct{}
	admittedOnce  sync.Once
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

func (s *supervision) beginCancellation(action workersessions.ControlAction) cancellationAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	supervision.mu.Lock()
	supervision.accepted = running
	serverOwned := supervision.serverOwned
	preAdmissionControl := supervision.preAdmissionAction != ""
	supervision.publishing = false
	supervision.mu.Unlock()
	supervision.signalPublished()
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if session, exists := r.sessions[id]; !exists || session.State != workersessions.StateStarting {
		return nil, false
	}
	supervision := newSupervision(dispatchID, turnID, executions...)
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

func (r *registry) completeSupervision(id string, supervision *supervision, result workers.WorkstationDispatchResult, dispatchErr error) {
	supervision.mu.Lock()
	action := supervision.requestedAction
	continuing := supervision.continuing
	dispatchID := supervision.dispatchID
	supervision.mu.Unlock()
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
	final, committed := r.commitTerminal(id, state, terminal)
	if committed {
		r.logTerminal(id, dispatchID, final)
		r.publishTerminalRecordOrLog(context.Background(), id, dispatchID, state, *final.Result)
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
	metadata := result.Result.ProviderSession
	if metadata == nil {
		return false
	}
	reference := providers.SessionRef{
		Provider: providers.ID(metadata.Provider),
		Kind:     metadata.Kind,
		ID:       metadata.ID,
	}
	if err := reference.Validate(); err != nil {
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

// associateProviderSessionFromResult preserves the Provider Session reference
// returned by Workers before the terminal lifecycle record can close the
// session's publication window. A malformed or conflicting Worker result is
// visible in structured operation logs and never replaces an accepted exact
// reference; it also never invents a replacement from runner or model state.
func (r *registry) associateProviderSessionFromResult(
	id, dispatchID string,
	result workers.WorkstationDispatchResult,
) {
	metadata := result.Result.ProviderSession
	if metadata == nil {
		return
	}
	_, err := r.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: id,
		DispatchID:      dispatchID,
		Reference: providers.SessionRef{
			Provider: providers.ID(metadata.Provider),
			Kind:     metadata.Kind,
			ID:       metadata.ID,
		},
	})
	if err != nil {
		r.logger.Info("worker session provider session association from result rejected", "sessionID", id, "attemptID", dispatchID, "outcome", "rejected")
	}
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

// Pause stops only an exact admitted attempt that already retained a complete
// Provider Session association. The session becomes PAUSED only after the
// established Workers cancellation callback has committed; an unassociated
// execution remains truthfully unsupported rather than becoming a fabricated
// resumable session.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (r *registry) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	for {
		session, supervision, err := r.controlTarget(req.ID)
		if err != nil {
			return workersessions.ControlResult{Action: workersessions.ControlActionPause, Outcome: workersessions.ControlOutcomeFailed}, err
		}
		if session.Terminal() || session.State == workersessions.StatePaused {
			return r.controlNoop(req.ID, workersessions.ControlActionPause, session, supervision), nil
		}
		if session.State != workersessions.StateRunning || supervision == nil || session.ProviderSessionAssociation == nil {
			return r.unsupportedControl(ctx, req, workersessions.ControlActionPause)
		}

		attempt := supervision.beginCancellation(workersessions.ControlActionPause)
		switch attempt.kind {
		case cancellationAttemptNoop:
			return r.controlNoop(req.ID, workersessions.ControlActionPause, session, supervision), nil
		case cancellationAttemptWait:
			<-attempt.wait
			continue
		case cancellationAttemptBeforeAdmission:
			return r.unsupportedControl(ctx, req, workersessions.ControlActionPause)
		case cancellationAttemptBoundary:
		}

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
			return result, cancelErr
		}
		if alreadyTerminal || cancelResult.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
			<-supervision.done
			current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
			return r.controlNoop(req.ID, workersessions.ControlActionPause, current, supervision), nil
		}
		select {
		case <-supervision.paused:
		case <-supervision.done:
		}
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := workersessions.ControlResult{Session: current, Action: workersessions.ControlActionPause, DispatchID: attempt.dispatchID}
		if current.State == workersessions.StatePaused {
			result.Outcome = workersessions.ControlOutcomeApplied
		} else {
			result.Outcome = workersessions.ControlOutcomeNoop
		}
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", attempt.dispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
		return result, nil
	}
}

// Resume starts one next Workers attempt for the exact paused Worker Session.
// The request carries the registry-owned reference unchanged so the Workers
// provider runner must route only through Providers.Continue.
func (r *registry) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	if err := req.Validate(); err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	session, supervision, err := r.controlTarget(req.ID)
	if err != nil {
		return workersessions.ControlResult{Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed}, err
	}
	if session.Terminal() {
		return r.controlNoop(req.ID, workersessions.ControlActionResume, session, supervision), nil
	}
	if supervision != nil && supervision.resumeInFlight() {
		return r.controlNoop(req.ID, workersessions.ControlActionResume, session, supervision), nil
	}
	if session.State == workersessions.StatePaused && session.ProviderSessionAssociation == nil {
		return r.rejectedResume(session, supervision, workersessions.ErrProviderSessionAssociationMissing)
	}
	if session.State != workersessions.StatePaused || supervision == nil {
		return r.unsupportedControl(ctx, req, workersessions.ControlActionResume)
	}
	if err := validateResumeAssociation(session); err != nil {
		return r.rejectedResume(session, supervision, err)
	}

	continuation, previousDispatchID, prepared := r.prepareContinuation(req.ID, supervision, session.ProviderSessionAssociation.Reference)
	if !prepared {
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		return r.controlNoop(req.ID, workersessions.ControlActionResume, current, supervision), nil
	}
	publishErr := r.boundary.PublishWithAdmission(
		context.WithoutCancel(ctx),
		continuation,
		func() { r.acceptSupervision(req.ID, supervision) },
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, dispatchErr error) {
			r.completeSupervision(req.ID, supervision, result, dispatchErr)
		},
	)
	if publishErr != nil {
		r.revertContinuation(req.ID, supervision, previousDispatchID)
		current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		result := workersessions.ControlResult{Session: current, Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeFailed, DispatchID: continuation.Execution.Dispatch.DispatchID}
		r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
		return result, publishErr
	}
	r.finishContinuationPublication(supervision)
	current, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
	result := workersessions.ControlResult{Session: current, Action: workersessions.ControlActionResume, Outcome: workersessions.ControlOutcomeApplied, DispatchID: continuation.Execution.Dispatch.DispatchID}
	r.logger.Info("worker session control", "sessionID", req.ID, "attemptID", result.DispatchID, "action", string(result.Action), "outcome", string(result.Outcome))
	return result, nil
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
	return s.continuing && (s.publishing || s.accepted)
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
	if supervision.continuing || supervision.publishing {
		return workers.WorkstationDispatchRequest{}, "", false
	}
	previousDispatchID := supervision.dispatchID
	supervision.resumeCount++
	continuation := cloneWorkstationDispatchRequest(supervision.execution)
	continuation.Execution.Dispatch.DispatchID = fmt.Sprintf("%s/resume/%d", previousDispatchID, supervision.resumeCount)
	continuedReference := reference.Clone()
	continuation.Execution.ResumeSession = &continuedReference
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
	return r.cancelControl(ctx, req, workersessions.ControlActionCancel)
}

func (r *registry) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return r.cancelControl(ctx, req, workersessions.ControlActionTerminate)
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
		if session.State == workersessions.StatePaused {
			return r.terminalizePausedControl(req.ID, action, supervision), nil
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

// terminalizePausedControl consumes a PAUSED session without asking Workers to
// cancel its already-canceled original dispatch. The original Start call is
// intentionally waiting for either continuation or this terminal decision, so
// signalDone is the authoritative release after the terminal record commits.
func (r *registry) terminalizePausedControl(id string, action workersessions.ControlAction, supervision *supervision) workersessions.ControlResult {
	state := controlTerminalState(action)
	final, committed := r.commitControlTerminal(id, state)
	if committed {
		r.logTerminal(id, supervision.dispatchID, final)
		r.publishTerminalRecordOrLog(context.Background(), id, supervision.dispatchID, state, workersessions.TerminalResult{})
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
	// Every successful Terminate result promises the terminal callback's
	// snapshot, including a no-op caused by an earlier Cancel or Terminate.
	// The prior control closes controlDone before that callback can complete,
	// so the callback channel is the only authoritative join point here.
	if action == workersessions.ControlActionTerminate && supervision != nil {
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
