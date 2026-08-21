package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// InvokeSession and its attempt loop live beside the controls they race with.

var _ interface {
	BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
} = (*registry)(nil)

// transitionToStarting atomically moves id from StateReserved to
// StateStarting. Only one caller can win this transition for a given id: a
// concurrent Start racing to claim the same newly reserved or already
// reserved identity, or an identity in any other state, sees
// ErrSessionNotStartable and makes no mutation and no Workers call.
func (r *registry) transitionToStarting(id string) (workersessions.Session, error) {
	return r.transitionToStartingWithExecution(id, workers.WorkstationExecutionRequest{})
}

func (r *registry) transitionToStartingWithExecution(id string, execution workers.WorkstationExecutionRequest) (workersessions.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateReserved {
		return workersessions.Session{}, workersessions.ErrSessionNotStartable
	}

	session.State = workersessions.StateStarting
	session.Result = nil
	session.Model = optionalExecutionFact(execution.Model)
	session.ReasoningEffort = optionalExecutionFact(execution.ReasoningEffort)
	r.sessions[id] = session
	return cloneSession(session), nil
}

func optionalExecutionFact(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneOptionalExecutionFact(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// BeginRuntimeAttempt opens the Worker Session observation and recording
// window, then returns control to Factory Runtime. It intentionally stops
// before registerInvocationSupervision or any Workers boundary call: Runtime
// has already admitted the detached attempt and remains responsible for its
// execution, cancellation, and terminal race.
func (r *registry) BeginRuntimeAttempt(
	ctx context.Context,
	req workersessions.RuntimeAttemptRequest,
) (workersessions.RuntimeAttempt, error) {
	if r == nil {
		return nil, workersessions.ErrStartAdmissionFailed
	}
	if err := (workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
	}).Validate(); err != nil {
		return nil, err
	}
	ctx = runtimeAttemptContext(ctx)
	logicalDispatchID, attemptID := runtimeAttemptIDs(req)
	if r.runtimeAttemptOwnedByOther(logicalDispatchID, req.ID, attemptID) {
		return nil, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}

	execution := req.Execution
	execution.Execution = workers.CloneWorkstationExecutionRequest(req.Execution.Execution)
	execution.Execution.Dispatch.DispatchID = attemptID
	prepared, err := r.prepareInvocation(
		context.WithoutCancel(ctx),
		workersessions.InvokeSessionRequest{ID: req.ID, Execution: execution},
		invocationPreparationOptions{runtimeOwned: true},
	)
	if err != nil {
		return nil, err
	}
	if preparationErr := runtimeAttemptPreparationError(prepared); preparationErr != nil {
		return nil, preparationErr
	}
	if !r.transitionToRunning(req.ID) {
		return nil, workersessions.ErrStartAdmissionFailed
	}
	if !r.claimRuntimeAttempt(logicalDispatchID, req.ID, attemptID) {
		r.terminalizeInvocationBeforeAdmission(context.WithoutCancel(ctx), req.ID, attemptID)
		return nil, workersessions.ErrProviderSessionAssociationAttemptMismatch
	}

	handle := &runtimeAttempt{
		registry:   r,
		workerID:   req.ID,
		dispatchID: logicalDispatchID,
		attemptID:  attemptID,
	}
	return workersessions.RuntimeAttempt(handle.Complete), nil
}

func runtimeAttemptPreparationError(prepared invocationPreparation) error {
	if !prepared.terminal {
		return nil
	}
	return prepared.failure
}

// InvokeSession supervises one resolved execution through the same preparation
// and attempt driver used by asynchronous Start. Workers owns the execution
// call and its admission callback; Worker Sessions owns the surrounding
// identity, control, and terminal publication state.
//
// req.Execution.WorkstationName routes into the runtime binding already
// assembled by Workers, allowing Petri and JavaScript children to share it.
func (r *registry) InvokeSession(ctx context.Context, req workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session start rejected", "sessionID", req.ID, "attemptID", attemptID, "outcome", "invalid")
		return workersessions.InvokeSessionResult{}, err
	}

	prepared, err := r.prepareInvocation(ctx, req, invocationPreparationOptions{})
	if err != nil {
		return workersessions.InvokeSessionResult{}, err
	}
	if prepared.terminal {
		if prepared.preAdmission {
			return workersessions.InvokeSessionResult{
				Session:     prepared.session,
				Dispatch:    canceledBeforeAdmissionResult(req.Execution),
				DispatchErr: workers.ErrWorkstationDispatchCanceled,
			}, nil
		}
		return workersessions.InvokeSessionResult{Session: prepared.session}, nil
	}
	return r.driveRegisteredInvocation(ctx, req, prepared.supervision)
}

// driveInvocation begins execution supervision only after the opening Worker
// Session record committed, then runs attempts until one is terminal or the
// attempt budget is spent. Controls that win before boundary admission
// terminalize the session without sending a cancellation for unknown work.
//
// Every attempt after the first runs under the same session identity and the
// same already-open publication window, so a retried Worker stays one Worker:
// its attempts are successive records on one topic rather than a new Worker
// each time.
func (r *registry) driveInvocation(ctx context.Context, req workersessions.InvokeSessionRequest, attemptID string) (workersessions.InvokeSessionResult, error) {
	supervision, canStart := r.registerSupervision(req.ID, attemptID, req.Execution.Execution.Dispatch.Execution.RequestID, req.Execution)
	if !canStart {
		final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
		if final.Terminal() {
			r.publishTerminalRecordOrLog(ctx, req.ID, attemptID, final.State, workersessions.TerminalResult{})
		}
		return workersessions.InvokeSessionResult{Session: final}, nil
	}
	return r.driveRegisteredInvocation(ctx, req, supervision)
}

func (r *registry) driveRegisteredInvocation(ctx context.Context, req workersessions.InvokeSessionRequest, supervision *supervision) (workersessions.InvokeSessionResult, error) {
	defer supervision.signalDriverDone()
	supervision.mu.Lock()
	supervision.retryBudget = req.Retry.Attempts()
	supervision.mu.Unlock()

	handoff := workers.WorkstationDispatchRequest{
		WorkstationName: req.Execution.WorkstationName,
		Execution:       workers.CloneWorkstationExecutionRequest(req.Execution.Execution),
	}
	for {
		result, retry := r.publishRegisteredAttempt(
			ctx, req.ID, handoff, supervision, r.beginExecutionPublish(req.ID, supervision),
		)
		if !retry {
			result.Attempts = supervision.attemptCount()
			return result, nil
		}
		next, prepared := r.prepareRetryAttempt(req.ID, supervision)
		if !prepared {
			// The session left STARTING under the retry -- a control, or a
			// terminal committed elsewhere. Report what actually stands rather
			// than publishing an attempt for a session that has moved on.
			final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
			supervision.signalDone()
			return workersessions.InvokeSessionResult{
				Session:  final,
				Dispatch: supervision.lastResult(),
				Attempts: supervision.attemptCount(),
			}, nil
		}
		previousDispatchID := handoff.Execution.Dispatch.DispatchID
		if err := r.publishAttemptLineageRecord(
			context.WithoutCancel(ctx),
			req.ID,
			next,
			workers.AttemptReasonRetry,
			previousDispatchID,
			supervision.attemptCount()+1,
		); err != nil {
			final, committed := r.commitTerminal(req.ID, workersessions.StateFailed, classifyTerminal(err, workers.WorkstationDispatchResult{}))
			supervision.mu.Lock()
			supervision.err = err
			supervision.publishing = false
			supervision.mu.Unlock()
			supervision.signalPublished()
			supervision.signalDone()
			if committed {
				r.logTerminal(req.ID, next.Execution.Dispatch.DispatchID, final)
				r.publishTerminalRecordOrLog(ctx, req.ID, next.Execution.Dispatch.DispatchID, final.State, *final.Result)
			}
			return workersessions.InvokeSessionResult{Session: final, DispatchErr: err, Attempts: supervision.attemptCount()}, nil
		}
		handoff = next
		r.logger.Info(
			"worker session retry",
			"sessionID", req.ID,
			"attemptID", handoff.Execution.Dispatch.DispatchID,
			"outcome", "retrying",
			"attempt", supervision.attemptCount()+1,
			"budget", req.Retry.Attempts(),
		)
	}
}

// publishRegisteredAttempt runs exactly one attempt and reports whether the
// caller should run another. A true retry answer is only ever produced for an
// attempt that reached Workers, failed with a retryable classification, and
// left the session un-terminalized on purpose.
func (r *registry) publishRegisteredAttempt(
	ctx context.Context,
	sessionID string,
	handoff workers.WorkstationDispatchRequest,
	supervision *supervision,
	canPublish bool,
) (workersessions.InvokeSessionResult, bool) {
	attemptID := handoff.Execution.Dispatch.DispatchID
	if !canPublish {
		final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: sessionID})
		supervision.signalPublished()
		supervision.signalDone()
		if final.Terminal() {
			r.publishTerminalRecordOrLog(ctx, sessionID, attemptID, final.State, workersessions.TerminalResult{})
		}
		return workersessions.InvokeSessionResult{Session: final}, false
	}

	r.logger.Info("worker session start", "sessionID", sessionID, "attemptID", attemptID, "outcome", "handoff", "state", string(workersessions.StateStarting))
	attemptDone := supervision.beginAttempt()
	publishErr := r.publishExecution(
		context.WithoutCancel(ctx),
		sessionID,
		handoff,
		supervision,
	)
	if publishErr != nil {
		final, committed := r.commitTerminal(sessionID, workersessions.StateFailed, classifyTerminal(publishErr, workers.WorkstationDispatchResult{}))
		supervision.mu.Lock()
		supervision.err = publishErr
		supervision.mu.Unlock()
		supervision.signalPublished()
		supervision.finishAttempt()
		supervision.signalDone()
		if committed {
			r.logTerminal(sessionID, attemptID, final)
			r.publishTerminalRecordOrLog(ctx, sessionID, attemptID, final.State, *final.Result)
		}
		return workersessions.InvokeSessionResult{Session: final}, false
	}

	r.finishSupervisionPublication(supervision)
	<-attemptDone
	if supervision.retryDecided() {
		return workersessions.InvokeSessionResult{}, true
	}
	<-supervision.done

	final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: sessionID})
	supervision.mu.Lock()
	result, dispatchErr := supervision.result, supervision.err
	supervision.mu.Unlock()
	return workersessions.InvokeSessionResult{Session: final, Dispatch: result, DispatchErr: dispatchErr}, false
}

// prepareRetryAttempt moves the session back to STARTING and mints the next
// attempt's dispatch identity. The attempt-scoped suffix mirrors the resume
// suffix prepareContinuation already mints, so one session's successive Workers
// dispatches stay distinguishable in logs and in the dispatch-owner lookup
// without ever reusing an identity Workers has already seen.
func (r *registry) prepareRetryAttempt(id string, supervision *supervision) (workers.WorkstationDispatchRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	session, exists := r.sessions[id]
	if !exists || session.Terminal() {
		return workers.WorkstationDispatchRequest{}, false
	}

	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	if supervision.controlAction != "" || supervision.requestedAction != "" {
		return workers.WorkstationDispatchRequest{}, false
	}
	previousDispatchID := supervision.dispatchID
	next := cloneWorkstationDispatchRequest(supervision.execution)
	next.Execution.Dispatch.DispatchID = fmt.Sprintf("%s/attempt/%d", supervision.baseDispatchID(), supervision.attemptsMade+1)
	supervision.dispatchID = next.Execution.Dispatch.DispatchID
	delete(r.dispatchOwners, previousDispatchID)
	r.dispatchOwners[supervision.dispatchID] = id
	supervision.publishing = true
	supervision.accepted = false
	supervision.result = workers.WorkstationDispatchResult{}
	supervision.err = nil
	session.State = workersessions.StateStarting
	r.sessions[id] = session
	return next, true
}

// claimRetryAttempt decides, exactly once per completed attempt, whether
// another attempt runs. Every disqualifying condition is checked before the
// budget so a canceled or control-owned session can never consume one.
//
// The retryable predicate is Workers' own classification of the failure it
// produced. Worker Sessions deliberately does not carry a second opinion: a
// caller-supplied predicate would let two orchestrators disagree about what
// "retryable" means for the identical provider failure.
func (r *registry) claimRetryAttempt(
	supervision *supervision,
	action workersessions.ControlAction,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) bool {
	if action != "" || dispatchCanceled(result, dispatchErr) {
		return false
	}
	if !retryableDispatchResult(result) {
		return false
	}
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	if supervision.controlAction != "" || supervision.requestedAction != "" {
		return false
	}
	// A continuation is a resumed provider session, not a fresh attempt;
	// retrying one would re-enter Providers.Continue against a reference the
	// failed attempt may already have consumed.
	if supervision.continuing {
		return false
	}
	if supervision.attemptsMade >= supervision.retryBudget {
		return false
	}
	supervision.retryPending = true
	return true
}

func retryableDispatchResult(result workers.WorkstationDispatchResult) bool {
	metadata := result.Result.FailureMetadata
	if metadata == nil {
		return false
	}
	return workers.FailureDecisionFromMetadata(metadata).Retryable
}

// observation is the registry-owned timing and Work correlation captured at
// the Worker Sessions lifecycle boundary. Provider Session association remains
// its own exact resumability fact; resolved provider identity is carried by
// the lifecycle record's provenance instead.
type observation struct {
	workIDs   []string
	turnID    string
	attemptID string
	direct    bool
	startedAt time.Time
	endedAt   *time.Time
}

func (r *registry) ensureObservation(id, attemptID, turnID string, workIDs []string, direct ...bool) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, exists := r.observations[id]; exists {
		return current.startedAt
	}
	startedAt := r.clock.Now()
	r.observations[id] = &observation{
		workIDs:   append([]string(nil), workIDs...),
		turnID:    turnID,
		attemptID: attemptID,
		direct:    len(direct) > 0 && direct[0],
		startedAt: startedAt,
	}
	return startedAt
}

func openingSessionPayload(
	id string,
	attemptID string,
	startedAt time.Time,
	request workers.WorkstationExecutionRequest,
	lineages ...*workers.SessionLineage,
) workers.SessionPayload {
	dispatch := request.Dispatch
	payload := workers.SessionPayload{
		Status:           string(workersessions.StateStarting),
		StartedAt:        timeValue(startedAt),
		WorkerSessionID:  id,
		FactorySessionID: strings.TrimSpace(request.FactorySessionID),
		RecordingID:      strings.TrimSpace(request.RecordingID),
		ProjectID:        strings.TrimSpace(request.ProjectID),
		DispatchID:       attemptID,
		TransitionID:     strings.TrimSpace(dispatch.TransitionID),
		WorkstationName:  strings.TrimSpace(dispatch.WorkstationName),
		TurnID:           strings.TrimSpace(dispatch.Execution.RequestID),
		TraceID:          strings.TrimSpace(dispatch.Execution.TraceID),
		ReplayKey:        strings.TrimSpace(dispatch.Execution.ReplayKey),
		WorkIDs:          append([]string(nil), dispatch.Execution.WorkIDs...),
		AttemptID:        attemptID,
		Attempt:          1,
		AttemptReason:    workers.AttemptReasonInitial,
		Model:            strings.TrimSpace(request.Model),
		ReasoningEffort:  strings.TrimSpace(request.ReasoningEffort),
		WorkingDirectory: strings.TrimSpace(request.WorkingDirectory),
		Capabilities:     cloneCapabilities(request.Capabilities),
	}
	payload.WorkerType = firstNonEmpty(strings.TrimSpace(request.WorkerType), strings.TrimSpace(dispatch.WorkerType))
	payload.ProjectID = firstNonEmpty(payload.ProjectID, strings.TrimSpace(dispatch.ProjectID))
	payload.ProviderSelection = openingProviderSelection(request)
	if continuation := openingSessionContinuation(request); continuation != nil {
		payload.Continuation = continuation
		payload.AttemptReason = workers.AttemptReasonResume
	}
	if len(lineages) > 0 && lineages[0] != nil {
		lineage := lineages[0].Clone()
		payload.Lineage = &lineage
	}
	return payload
}

func openingProviderSelection(request workers.WorkstationExecutionRequest) *workers.SessionProviderSelection {
	selection := workers.SessionProviderSelection{
		RunnerID:         strings.TrimSpace(request.RunnerID),
		Source:           request.RunnerSelectionSource,
		ExecutorProvider: strings.TrimSpace(request.ExecutorProvider),
		ModelProvider:    strings.TrimSpace(request.ModelProvider),
	}
	if selection.RunnerID == "" && selection.Source == "" && selection.ExecutorProvider == "" && selection.ModelProvider == "" {
		return nil
	}
	return &selection
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func openingSessionContinuation(request workers.WorkstationExecutionRequest) *workers.SessionContinuation {
	if request.Continuation != nil {
		continuationID := firstNonEmpty(request.Continuation.ProviderSessionID, request.Continuation.ExternalRef)
		continuationKind := request.Continuation.Kind
		if strings.TrimSpace(continuationKind) == "" {
			continuationKind = request.Continuation.Normalize().Kind
		}
		continuation := workers.SessionContinuation{
			Provider: request.Continuation.Provider,
			Kind:     continuationKind,
			ID:       continuationID,
		}
		if continuation.Provider != "" || continuation.Kind != "" || continuation.ID != "" {
			return &continuation
		}
		return nil
	}
	return nil
}

// providerIdentityForExecution returns only a provider identity already
// resolved by the Workers execution request. It deliberately does not choose
// a default runner: an empty result means the provider can be learned from a
// later provider-authored record and must then be bound before that output is
// committed.
func providerIdentityForExecution(request workers.WorkstationExecutionRequest) string {
	runner := strings.TrimSpace(request.RunnerID)
	if runner != "" && !strings.EqualFold(runner, workers.ExecutorProviderACP) && !strings.EqualFold(runner, "SCRIPT_WRAP") {
		return runner
	}
	executorProvider := strings.TrimSpace(request.ExecutorProvider)
	if executorProvider != "" &&
		!strings.EqualFold(executorProvider, workers.ExecutorProviderACP) &&
		!strings.EqualFold(executorProvider, "SCRIPT_WRAP") {
		return executorProvider
	}
	if modelProvider := strings.TrimSpace(request.ModelProvider); modelProvider != "" {
		return modelProvider
	}
	if request.Continuation != nil {
		return strings.TrimSpace(request.Continuation.Provider)
	}
	return ""
}

func timeValue(value time.Time) *time.Time {
	return &value
}

func cloneCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func (r *registry) finishObservationLocked(id string, endedAt time.Time) {
	current := r.observations[id]
	if current == nil || current.endedAt != nil {
		return
	}
	endedAt = endedAt.UTC()
	current.endedAt = &endedAt
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

func startReplayOutcome(err error) string {
	if err == nil {
		return "accepted"
	}
	return "rejected"
}
