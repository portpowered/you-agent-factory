package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// InvokeSession and its attempt loop live beside the controls they race with.

var _ interface {
	BeginRuntimeAttempt(context.Context, workersessions.RuntimeAttemptRequest) (workersessions.RuntimeAttempt, error)
} = (*registry)(nil)

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
// and attempt driver used by asynchronous Start. The boundary is the sole
// mechanism that starts, cancels, and reports an attempt; the result callback
// remains authoritative for terminal Workers output, so control cannot
// fabricate a Factory Runtime result.
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

// driveInvocation begins boundary supervision only after the opening Worker
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
			ctx, req.ID, handoff, supervision, r.beginBoundaryPublish(req.ID, supervision),
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
	publishErr := r.boundary.PublishWithAdmission(
		context.WithoutCancel(ctx),
		handoff,
		func() { r.acceptSupervision(sessionID, supervision) },
		func(_ context.Context, _ workers.WorkstationDispatchRequest, result workers.WorkstationDispatchResult, dispatchErr error) {
			r.completeSupervision(sessionID, supervision, result, dispatchErr)
		},
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
	if payload.WorkerType = strings.TrimSpace(request.WorkerType); payload.WorkerType == "" {
		payload.WorkerType = strings.TrimSpace(dispatch.WorkerType)
	}
	if payload.ProjectID == "" {
		payload.ProjectID = strings.TrimSpace(dispatch.ProjectID)
	}
	selection := workers.SessionProviderSelection{
		RunnerID:         strings.TrimSpace(request.RunnerID),
		Source:           request.RunnerSelectionSource,
		ExecutorProvider: strings.TrimSpace(request.ExecutorProvider),
		ModelProvider:    strings.TrimSpace(request.ModelProvider),
	}
	if selection.RunnerID != "" || selection.Source != "" || selection.ExecutorProvider != "" || selection.ModelProvider != "" {
		payload.ProviderSelection = &selection
	}
	if request.ResumeSession != nil {
		continuation := workers.SessionContinuation{
			Provider: string(request.ResumeSession.Provider),
			Kind:     request.ResumeSession.Kind,
			ID:       request.ResumeSession.ID,
		}
		if continuation.Provider != "" || continuation.Kind != "" || continuation.ID != "" {
			payload.Continuation = &continuation
			payload.AttemptReason = workers.AttemptReasonResume
		}
	}
	if len(lineages) > 0 && lineages[0] != nil {
		lineage := lineages[0].Clone()
		payload.Lineage = &lineage
	}
	return payload
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
	if identity, err := workers.RunnerIdentityForWorker(request.ExecutorProvider, request.ModelProvider); err == nil && strings.TrimSpace(identity) != "" {
		return strings.TrimSpace(identity)
	}
	if request.ResumeSession != nil {
		return strings.TrimSpace(string(request.ResumeSession.Provider))
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

func (r *registry) ListObservations(ctx context.Context, req workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation list rejected", "outcome", "invalid")
		return workersessions.ListObservationsResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}

	r.mu.RLock()
	ids := make([]observationOrder, 0)
	for id, current := range r.observations {
		if containsString(current.workIDs, req.WorkID) {
			ids = append(ids, observationOrder{
				id:        id,
				startedAt: current.startedAt,
				attemptID: current.attemptID,
			})
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "not_found")
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	sortObservationOrder(ids)

	observations := make([]workersessions.Observation, 0, len(ids))
	for _, item := range ids {
		projected, err := r.projectObservation(ctx, item.id)
		if err != nil {
			return workersessions.ListObservationsResult{}, err
		}
		observations = append(observations, projected)
	}
	sortObservationAttempts(observations)
	r.logger.Info("worker session observation list", "workID", req.WorkID, "outcome", "success", "result_count", len(observations))
	return workersessions.ListObservationsResult{Observations: observations}, nil
}

func (r *registry) GetObservation(ctx context.Context, req workersessions.GetObservationRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation get rejected", "outcome", "invalid")
		return workersessions.Observation{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}

	r.mu.RLock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == req.ProviderSession {
			ids = append(ids, id)
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		r.logger.Info("worker session observation get", "outcome", "not_found")
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	// An exact Provider Session identity must be unique. If corrupted or
	// legacy state ever contains two matches, deterministic identity order
	// still makes the result stable without exposing both as one observation.
	sortStrings(ids)
	projected, err := r.projectObservation(ctx, ids[0])
	if err != nil {
		return workersessions.Observation{}, err
	}
	r.logger.Info("worker session observation get", "workerSessionID", projected.WorkerSessionID, "outcome", "success")
	return projected, nil
}

func (r *registry) GetObservationByWorkerSessionID(ctx context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation get by Worker Session rejected", "outcome", "invalid")
		return workersessions.Observation{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	// Worker-ID lookup is the provider-neutral history boundary. It must not
	// require transcript enrichment from Provider Sessions: a worker can emit
	// canonical lifecycle/output records without a readable provider transcript.
	projected, err := r.projectWorkerSessionIdentity(ctx, req.WorkerSessionID)
	if err != nil {
		r.logger.Info("worker session observation get by Worker Session", "workerSessionID", req.WorkerSessionID, "outcome", "not_found")
		return workersessions.Observation{}, err
	}
	r.logger.Info("worker session observation get by Worker Session", "workerSessionID", projected.WorkerSessionID, "outcome", "success")
	return projected, nil
}

func (r *registry) projectObservation(ctx context.Context, id string) (workersessions.Observation, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}
	projected, err := r.projectWorkerSessionIdentity(ctx, id)
	if err != nil {
		return workersessions.Observation{}, err
	}

	if !projected.ProviderSessionAvailable {
		return projected, nil
	}
	return r.enrichWithProviderSessionsProjection(ctx, projected)
}

// projectWorkerSessionIdentity projects the registry-owned Worker Session
// identity and lifecycle timing without consulting Provider Sessions. The
// canonical Worker-ID history stream uses this boundary so missing provider
// transcript storage cannot hide retained Worker Session records.
func (r *registry) projectWorkerSessionIdentity(ctx context.Context, id string) (workersessions.Observation, error) {
	if err := observationContextError(ctx); err != nil {
		return workersessions.Observation{}, err
	}

	session, metadata, ok := r.loadObservationState(id)
	if !ok {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}

	projected := baseObservation(id, session, metadata)
	applyObservationTiming(&projected, session, metadata, r.clock)
	if session.Result != nil && session.Result.Cause != nil {
		failure := *session.Result.Cause
		projected.Failure = &failure
	}
	return projected, nil
}

// loadObservationState returns detached snapshots of the registered session
// and observation metadata for id. ok is false when either is missing.
func (r *registry) loadObservationState(id string) (workersessions.Session, *observation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, exists := r.sessions[id]
	metadata := r.observations[id]
	if exists {
		session = cloneSession(session)
	}
	if metadata != nil {
		metadata = cloneObservation(metadata)
	}
	if !exists || metadata == nil {
		return workersessions.Session{}, nil, false
	}
	return session, metadata, true
}

// baseObservation projects the registry-owned identity, correlation, and
// lifecycle facts that never require the Provider Sessions root.
func baseObservation(id string, session workersessions.Session, metadata *observation) workersessions.Observation {
	projected := workersessions.Observation{
		WorkerSessionID:            id,
		PredecessorWorkerSessionID: session.PredecessorWorkerSessionID,
		SuccessorWorkerSessionID:   session.SuccessorWorkerSessionID,
		Direct:                     metadata.direct,
		WorkIDs:                    append([]string(nil), metadata.workIDs...),
		TurnID:                     metadata.turnID,
		AttemptID:                  metadata.attemptID,
		State:                      session.State,
		DurationBasis:              workersessions.DurationBasisUnavailable,
		Transcript:                 workersessions.TranscriptAvailabilityUnavailable,
	}
	if session.ProviderSessionAssociation != nil {
		projected.ProviderSession = session.ProviderSessionAssociation.Reference.Clone()
		projected.ProviderSessionAvailable = true
		projected.TurnID = session.ProviderSessionAssociation.TurnID
		projected.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	return projected
}

// applyObservationTiming fills projected's start/end/duration fields from
// metadata, using clock for an active (non-terminal) session's elapsed time.
func applyObservationTiming(projected *workersessions.Observation, session workersessions.Session, metadata *observation, clock platformclock.Source) {
	if metadata.startedAt.IsZero() {
		return
	}
	started := metadata.startedAt
	projected.StartedAt = &started
	switch {
	case metadata.endedAt != nil:
		ended := *metadata.endedAt
		projected.EndedAt = &ended
		projected.Duration = nonNegativeDuration(ended.Sub(started))
		projected.DurationBasis = workersessions.DurationBasisRecordedTimestamps
	case !session.Terminal():
		projected.Duration = nonNegativeDuration(clock.Now().Sub(started))
		projected.DurationBasis = workersessions.DurationBasisActiveClock
	}
}

func nonNegativeDuration(duration time.Duration) *time.Duration {
	if duration < 0 {
		duration = 0
	}
	return &duration
}

// enrichWithProviderSessionsProjection adds transcript availability, token
// usage, and parse diagnostics from the Provider Sessions root. It is only
// called when projected already carries an available Provider Session
// reference.
func (r *registry) enrichWithProviderSessionsProjection(ctx context.Context, projected workersessions.Observation) (workersessions.Observation, error) {
	if r.providerSessions == nil {
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	result, err := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: projected.ProviderSession.Clone(),
		Context: ctx,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, providersessions.ErrOperationCanceled) {
			return workersessions.Observation{}, workersessions.ErrObservationCanceled
		}
		return workersessions.Observation{}, workersessions.ErrObservationProjectionUnavailable
	}
	projected.Transcript = workersessions.TranscriptAvailabilityAvailable
	projected.TokenUsage = observationTokenUsage(result.Detail.Parse.TokenUsage)
	projected.Parse = observationParseDiagnostics(result.Detail.Parse)
	return projected, nil
}

func observationContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ErrObservationCanceled
	}
	return nil
}

func cloneObservation(value *observation) *observation {
	if value == nil {
		return nil
	}
	clone := *value
	clone.workIDs = append([]string(nil), value.workIDs...)
	if value.endedAt != nil {
		ended := *value.endedAt
		clone.endedAt = &ended
	}
	return &clone
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

type observationOrder struct {
	id        string
	startedAt time.Time
	attemptID string
}

func sortObservationOrder(values []observationOrder) {
	sort.SliceStable(values, func(i, j int) bool {
		left, right := values[i], values[j]
		switch {
		case !left.startedAt.Equal(right.startedAt):
			return left.startedAt.Before(right.startedAt)
		case left.attemptID != right.attemptID:
			return left.attemptID < right.attemptID
		default:
			return left.id < right.id
		}
	})
}

func sortObservationAttempts(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func observationTokenUsage(source *providersessions.TokenUsage) *workersessions.TokenUsage {
	if source == nil {
		return nil
	}
	return &workersessions.TokenUsage{
		CacheWriteTokens:      cloneInt(source.CacheWriteTokens),
		CachedInputTokens:     cloneInt(source.CachedInputTokens),
		InputTokens:           cloneInt(source.InputTokens),
		OutputTokens:          cloneInt(source.OutputTokens),
		ReasoningOutputTokens: cloneInt(source.ReasoningOutputTokens),
		TotalTokens:           cloneInt(source.TotalTokens),
	}
}

func observationParseDiagnostics(source providersessions.ParseSummary) workersessions.ParseDiagnostics {
	result := workersessions.ParseDiagnostics{
		EventCount:         source.EventCount,
		MalformedLineCount: source.MalformedLineCount,
		UnknownEventCount:  source.UnknownEventCount,
		Errors:             make([]workersessions.ParseDiagnostic, 0, len(source.ParseErrors)),
	}
	for _, item := range source.ParseErrors {
		result.Errors = append(result.Errors, workersessions.ParseDiagnostic{
			Code:       "provider_session_parse_error",
			LineNumber: item.LineNumber,
			Message:    safeDiagnosticMessage(item.Message),
		})
	}
	return result
}

func safeDiagnosticMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	if message == "" || strings.ContainsAny(message, `/\`) {
		return "provider session parse error"
	}
	lower := strings.ToLower(message)
	for _, sensitive := range []string{"password", "authorization", "bearer ", "secret", "prompt"} {
		if strings.Contains(lower, sensitive) {
			return "provider session parse error"
		}
	}
	if len(message) > 256 {
		message = message[:256]
	}
	return message
}

// observationSubscription adapts the canonical Events subscription to the
// Worker Sessions outcome vocabulary and closes itself immediately after the
// lifecycle terminal record.
type observationSubscription struct {
	source          events.Subscription
	replay          *replayObservationSubscription
	workerSessionID string

	mu             sync.Mutex
	closed         bool
	terminalReplay bool
	cursorProvided bool
	delivered      bool
	activeCancel   context.CancelFunc
}

func (s *observationSubscription) Next(ctx context.Context) workersessions.ObservationDelivery {
	if s.replay != nil {
		return s.replay.Next(ctx)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	nextContext, cancel := context.WithCancel(ctx)
	s.activeCancel = cancel
	s.mu.Unlock()
	delivery := s.source.Next(nextContext)
	cancel()

	s.mu.Lock()
	s.activeCancel = nil
	closed := s.closed
	s.mu.Unlock()
	if closed && delivery.Kind != events.DeliveryCanceled {
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
	}
	return s.projectSourceDelivery(delivery)
}

func (s *observationSubscription) projectSourceDelivery(delivery events.Delivery) workersessions.ObservationDelivery {
	switch delivery.Kind {
	case events.DeliveryRecord:
		event := projectObservationEvent(delivery.Record, s.workerSessionID)
		s.mu.Lock()
		s.delivered = true
		s.mu.Unlock()
		if isTerminalLifecycleRecord(delivery.Record) {
			s.closeSource()
			s.mu.Lock()
			terminalReplay := s.terminalReplay
			s.mu.Unlock()
			kind := workersessions.ObservationDeliveryTerminal
			if terminalReplay {
				kind = workersessions.ObservationDeliveryTerminalReplay
			}
			return workersessions.ObservationDelivery{Kind: kind, Event: event}
		}
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryRecord, Event: event}
	case events.DeliveryCanceled:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
	case events.DeliveryGap:
		s.closeSource()
		s.mu.Lock()
		cursorProvided, delivered := s.cursorProvided, s.delivered
		s.mu.Unlock()
		if cursorProvided && !delivered {
			return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationCursorStale}
		}
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceGap}
	case events.DeliveryBackpressure:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	case events.DeliveryClosed:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceClosed}
	default:
		s.closeSource()
		return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliverySourceFailure, Err: workersessions.ErrObservationSourceUnavailable}
	}
}

func (s *observationSubscription) Close() {
	if s.replay != nil {
		s.replay.Close()
		return
	}
	s.closeSource()
}

func (s *observationSubscription) closeSource() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	activeCancel := s.activeCancel
	s.mu.Unlock()
	if activeCancel != nil {
		activeCancel()
		return
	}
	// Events has no separate Close method. A canceled Next is its explicit
	// unregister operation, and it is non-blocking because the context is
	// already canceled.
	if s.source != nil {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		s.source.Next(cancelled)
	}
}

func projectObservationEvent(record events.Record, workerSessionIDArgs ...string) workersessions.ObservationEvent {
	workerSessionID := observationWorkerSessionIDFromTopic(record.ID.Topic)
	if len(workerSessionIDArgs) > 0 && strings.TrimSpace(workerSessionIDArgs[0]) != "" {
		workerSessionID = strings.TrimSpace(workerSessionIDArgs[0])
	}
	return workersessions.ObservationEvent{
		Position: uint64(record.ID.Position),
		Cursor: workersessions.ObservationCursor{
			WorkerSessionID: workerSessionID,
			Position:        uint64(record.ID.Position),
		},
		SourceType:     string(record.SourceType),
		SourceID:       string(record.SourceID),
		SourceSequence: uint64(record.SourceSequence),
		SourceEventID:  string(record.SourceEventID),
		SchemaID:       string(record.SchemaID),
		Payload:        append([]byte(nil), record.Payload...),
	}
}

func observationWorkerSessionIDFromTopic(topic events.Topic) string {
	value := strings.TrimSpace(string(topic))
	value = strings.TrimPrefix(value, "worker-session/")
	value = strings.TrimSuffix(value, "/events")
	return value
}
