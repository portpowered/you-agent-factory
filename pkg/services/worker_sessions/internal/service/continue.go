package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

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
	lineage   *workers.SessionLineage
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
	if requestID := r.continuationSources[source.ID]; requestID != "" && requestID != req.RequestID {
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
	r.sessions[source.ID] = source
	r.sessions[req.SuccessorWorkerSessionID] = workersessions.Session{
		ID:                         req.SuccessorWorkerSessionID,
		State:                      workersessions.StateReserved,
		ProviderSessionAssociation: continuationAssociation(req, continuation, snapshot.turnID, source.ProviderSessionAssociation.Reference),
	}
	if r.continuationSources == nil {
		r.continuationSources = make(map[string]string)
	}
	r.continuationSources[source.ID] = req.RequestID
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
			lineage:          plan.lineage,
		},
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
	if r.continuationSources[plan.request.SourceWorkerSessionID] == plan.request.RequestID {
		delete(r.continuationSources, plan.request.SourceWorkerSessionID)
	}
	r.mu.Unlock()
}

// commitContinuationLineage publishes the source-side relationship only after
// the successor's admission barrier has opened. The source execution is
// already terminal, so its live capture handle has closed; the optional
// Recordings writer is therefore fed the exact accepted Events record
// directly, preserving a durable prefix or an explicit loss classification.
func (r *registry) commitContinuationLineage(plan continuePlan) {
	source, err := r.Get(context.Background(), workersessions.GetRequest{ID: plan.request.SourceWorkerSessionID})
	if err != nil || source.ProviderSessionAssociation == nil {
		r.releaseContinuationReservation(plan)
		return
	}
	association := source.ProviderSessionAssociation.Clone()
	reference := association.Reference.Clone()
	continuation := workers.SessionContinuation{
		Provider: string(reference.Provider),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
	lineage := workers.SessionLineage{SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID}
	payload := workers.SessionPayload{
		Status:          string(source.State),
		WorkerSessionID: source.ID,
		TurnID:          association.TurnID,
		DispatchID:      association.DispatchID,
		AttemptID:       association.AttemptID,
		Continuation:    &continuation,
		Lineage:         &lineage,
	}
	identity := events.AppendIdentity{
		SourceType:     continuationLineageSourceType,
		SourceID:       events.SourceID(source.ID + "/successor/" + plan.request.SuccessorWorkerSessionID),
		SourceSequence: continuationLineageSourceSequence,
		SourceEventID:  continuationLineageSourceEventID,
	}
	if err := r.publishSessionLineageRecord(context.Background(), source.ID, identity, payload, true); err != nil {
		r.logger.Info(
			"worker session continuation lineage publication failed",
			"sourceWorkerSessionID", source.ID,
			"successorWorkerSessionID", plan.request.SuccessorWorkerSessionID,
			"outcome", "append_failed",
		)
	}

	r.mu.Lock()
	if current, exists := r.sessions[source.ID]; exists {
		if current.SuccessorWorkerSessionID == "" || current.SuccessorWorkerSessionID == plan.request.SuccessorWorkerSessionID {
			current.SuccessorWorkerSessionID = plan.request.SuccessorWorkerSessionID
			r.sessions[source.ID] = current
		}
	}
	if current, exists := r.sessions[plan.request.SuccessorWorkerSessionID]; exists {
		if current.PredecessorWorkerSessionID == "" || current.PredecessorWorkerSessionID == source.ID {
			current.PredecessorWorkerSessionID = source.ID
			r.sessions[plan.request.SuccessorWorkerSessionID] = current
		}
	}
	if r.continuationSources[source.ID] == plan.request.RequestID {
		delete(r.continuationSources, source.ID)
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
	appendResult, err := r.appendDraft(ctx, workersessions.Topic(sessionID), identity, workerDraftSchemaID, draft)
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
			"sessionID", sessionID,
			"outcome", "unavailable",
		)
		return
	}
	err := writer.PersistWorkerRecord(context.WithoutCancel(ctx), recordings.WorkerRecordingRecord{
		RecordingID:     recordingID,
		WorkerSessionID: sessionID,
		Record:          record.Detached(),
	})
	if err == nil {
		return
	}
	r.logger.Info(
		"worker session continuation lineage recording failed",
		"sessionID", sessionID,
		"outcome", "degraded",
	)
	if failureWriter, ok := r.recording.(recordings.WorkerRecordingFailureWriter); ok && failureWriter != nil {
		_ = failureWriter.PersistWorkerRecordingFailure(context.WithoutCancel(ctx), recordings.WorkerRecordingFailure{
			RecordingID:     recordingID,
			WorkerSessionID: sessionID,
			Topic:           workersessions.Topic(sessionID),
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
	lineage := workers.SessionLineage{
		PreviousDispatchID: previousDispatchID,
		PreviousAttemptID:  previousDispatchID,
	}
	payload := openingSessionPayload(
		sessionID,
		currentDispatchID,
		r.clock.Now(),
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

func (r *registry) ListWorkerSessionObservations(
	ctx context.Context,
	req workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	query, err := r.parseObservationListQuery(ctx, req)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	ids := r.observationListIDs(query.cursor, query.scope, req.States)
	pageIDs := observationListPage(ids, query.limit)
	observations, err := r.projectObservationList(ctx, pageIDs)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	nextToken := observationListNextToken(ids, pageIDs)
	r.logger.Info("worker session top-level observation list", "scope", string(query.scope), "state_count", len(req.States), "result_count", len(observations), "has_next", nextToken != "")
	return workersessions.ListWorkerSessionObservationsResult{
		Observations: observations,
		MaxResults:   query.limit,
		NextToken:    nextToken,
	}, nil
}

type observationListQuery struct {
	limit  int
	cursor string
	scope  workersessions.ObservationScope
}

func (r *registry) parseObservationListQuery(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) (observationListQuery, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session top-level observation list rejected", "outcome", "invalid")
		return observationListQuery{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return observationListQuery{}, err
	}
	limit := req.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	cursor, err := decodeObservationListCursor(req.NextToken)
	if err != nil {
		return observationListQuery{}, err
	}
	return observationListQuery{limit: limit, cursor: cursor, scope: req.Scope.Normalized()}, nil
}

func decodeObservationListCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", workersessions.ErrInvalidObservationPagination
	}
	return string(decoded), nil
}

func (r *registry) observationListIDs(cursor string, scope workersessions.ObservationScope, states []workersessions.State) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.observations))
	for id, metadata := range r.observations {
		if metadata == nil || id <= cursor || !observationScopeMatches(metadata.direct, scope) {
			continue
		}
		session, exists := r.sessions[id]
		if !exists || !observationStateMatches(session.State, states) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func observationListPage(ids []string, limit int) []string {
	if len(ids) <= limit {
		return ids
	}
	return ids[:limit]
}

func (r *registry) projectObservationList(ctx context.Context, ids []string) ([]workersessions.Observation, error) {
	observations := make([]workersessions.Observation, 0, len(ids))
	for _, id := range ids {
		projected, err := r.projectObservation(ctx, id)
		if err != nil {
			return nil, err
		}
		observations = append(observations, projected)
	}
	return observations, nil
}

func observationListNextToken(allIDs, pageIDs []string) string {
	if len(allIDs) <= len(pageIDs) || len(pageIDs) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(pageIDs[len(pageIDs)-1]))
}

func observationScopeMatches(direct bool, scope workersessions.ObservationScope) bool {
	switch scope.Normalized() {
	case workersessions.ObservationScopeDirect:
		return direct
	case workersessions.ObservationScopeFactory:
		return !direct
	case workersessions.ObservationScopeAll:
		return true
	default:
		return false
	}
}

func observationStateMatches(state workersessions.State, states []workersessions.State) bool {
	if len(states) == 0 {
		return true
	}
	return slices.Contains(states, state)
}

// ReadTranscriptByWorkerSessionID resolves the registry-owned association
// before projecting a direct or Factory Worker Session transcript. The caller
// cannot substitute a provider tuple or cause a provider-session discovery.
func (r *registry) ReadTranscriptByWorkerSessionID(
	ctx context.Context,
	req workersessions.ReadTranscriptByWorkerSessionIDRequest,
) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session identity transcript read rejected", "outcome", "invalid")
		return workersessions.ReadTranscriptResult{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}
	session, metadata, ok := r.loadObservationState(req.WorkerSessionID)
	if !ok {
		r.logger.Info("worker session identity transcript read", "workerSessionID", req.WorkerSessionID, "outcome", "not_found")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationSessionNotFound
	}
	providerSession := providers.SessionRef{}
	if session.ProviderSessionAssociation != nil {
		providerSession = session.ProviderSessionAssociation.Reference
	}
	return r.projectTranscript(ctx, session, metadata, providerSession)
}

func (r *registry) projectTranscript(
	ctx context.Context,
	session workersessions.Session,
	metadata *observation,
	providerSession providers.SessionRef,
) (workersessions.ReadTranscriptResult, error) {
	if !session.State.Terminal() {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "active")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptActive
	}
	if session.ProviderSessionAssociation == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
	}
	if r.providerSessions == nil {
		r.logger.Info("worker session transcript read", "workerSessionID", session.ID, "outcome", "projection_unavailable")
		return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptProjectionUnavailable
	}

	projected, projectErr := r.providerSessions.Project(providersessions.ProjectRequest{
		Session: providerSession.Clone(),
		Context: ctx,
	})
	if projectErr != nil {
		if errors.Is(projectErr, context.Canceled) || errors.Is(projectErr, providersessions.ErrOperationCanceled) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationCanceled
		}
		if transcriptSourceUnavailable(projectErr) {
			return workersessions.ReadTranscriptResult{}, workersessions.ErrObservationTranscriptUnavailable
		}
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("%w: %v", workersessions.ErrObservationTranscriptProjectionUnavailable, projectErr)
	}

	result := workersessions.ReadTranscriptResult{
		WorkerSessionID: session.ID,
		ProviderSession: providerSession.Clone(),
		WorkIDs:         append([]string(nil), metadata.workIDs...),
		AttemptID:       metadata.attemptID,
		State:           session.State,
		Entries:         transcriptEntries(projected.Detail.Transcript),
	}
	if session.ProviderSessionAssociation != nil {
		result.TurnID = session.ProviderSessionAssociation.TurnID
		result.AttemptID = session.ProviderSessionAssociation.AttemptID
	}
	if err := result.Validate(); err != nil {
		return workersessions.ReadTranscriptResult{}, fmt.Errorf("validate Worker Session transcript: %w", err)
	}
	r.logger.Info("worker session transcript read", "workerSessionID", result.WorkerSessionID, "outcome", "success", "result_count", len(result.Entries))
	return result, nil
}

// ReadTranscript returns the final normalized transcript for one Worker
// Session. The canonical Worker Session identity is preferred; the exact
// Provider Session reference remains a compatibility lookup. The lifecycle
// check happens before the provider projection so an active session has a
// stable active outcome even when its provider source is not yet readable.
func (r *registry) ReadTranscript(ctx context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session transcript read rejected", "outcome", "invalid")
		return workersessions.ReadTranscriptResult{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ReadTranscriptResult{}, err
	}

	session, metadata, err := r.transcriptSession(req)
	if err != nil {
		r.logger.Info("worker session transcript read", "outcome", "not_found")
		return workersessions.ReadTranscriptResult{}, err
	}
	providerSession := req.ProviderSession
	if req.WorkerSessionID != "" && session.ProviderSessionAssociation != nil {
		providerSession = session.ProviderSessionAssociation.Reference
	}
	return r.projectTranscript(ctx, session, metadata, providerSession)
}

func (r *registry) transcriptSession(req workersessions.ReadTranscriptRequest) (workersessions.Session, *observation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, 1)
	for id, session := range r.sessions {
		matches := id == req.WorkerSessionID
		if req.WorkerSessionID == "" {
			matches = session.ProviderSessionAssociation != nil && session.ProviderSessionAssociation.Reference == req.ProviderSession
		}
		if matches {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	sortStrings(ids)
	session := cloneSession(r.sessions[ids[0]])
	metadata := cloneObservation(r.observations[ids[0]])
	if metadata == nil {
		return workersessions.Session{}, nil, workersessions.ErrObservationSessionNotFound
	}
	return session, metadata, nil
}
