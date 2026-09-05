package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type asyncStartCompletion struct {
	result workersessions.StartResult
	err    error
}

// startNotAcceptedError keeps the public Start error classification stable
// without exposing an adapter's raw failure text to callers that only need to
// know that the admission barrier was not reached.
type startNotAcceptedError struct {
	cause error
}

func (e *startNotAcceptedError) Error() string {
	return workersessions.ErrStartNotAccepted.Error()
}

func (e *startNotAcceptedError) Unwrap() error {
	if e.cause == nil {
		return workersessions.ErrStartNotAccepted
	}
	return errors.Join(workersessions.ErrStartNotAccepted, e.cause)
}

func startNotAccepted(cause error) error {
	return &startNotAcceptedError{cause: cause}
}

func (r *registry) serverOwnedContext() context.Context {
	r.mu.RLock()
	ctx := r.lifecycleCtx
	r.mu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func awaitStartReplay(ctx context.Context, replay *startReplay) (workersessions.StartResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-replay.done:
		return cloneStartResult(replay.result), replay.err
	case <-ctx.Done():
		select {
		case <-replay.done:
			return cloneStartResult(replay.result), replay.err
		default:
		}
		return workersessions.StartResult{}, ctx.Err()
	}
}

func startReservationOutcome(err error) string {
	if errors.Is(err, workersessions.ErrStartRequestIDConflict) {
		return "idempotency_conflict"
	}
	if errors.Is(err, workersessions.ErrStartServerStopping) {
		return "server_stopping"
	}
	return "not_startable"
}

func (r *registry) finishStart() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeStarts == 0 {
		return
	}
	r.activeStarts--
	if r.activeStarts == 0 && r.startsDone != nil {
		close(r.startsDone)
	}
}

func (r *registry) isStopping() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stopping
}

func (r *registry) supervisionContext(supervision *supervision) context.Context {
	if supervision == nil {
		return context.Background()
	}
	supervision.mu.Lock()
	serverOwned := supervision.serverOwned
	supervision.mu.Unlock()
	if serverOwned {
		return r.serverOwnedContext()
	}
	return context.Background()
}

// Stop is the Worker Sessions-owned process lifecycle boundary. It closes
// asynchronous admission first, then routes every already-registered
// supervision through the existing Terminate/control path. The lifecycle
// context remains usable until those paths have committed their terminal
// records, so shutdown cannot turn a joined execution into an unobservable
// terminal state.
func (r *registry) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.stopOnce.Do(func() {
		r.stopErr = r.stopOwned(ctx)
		close(r.stopDone)
	})
	<-r.stopDone
	return r.stopErr
}

func (r *registry) stopOwned(ctx context.Context) error {
	r.mu.Lock()
	r.stopping = true
	startDone := r.startsDone
	ids := make([]string, 0, len(r.supervisions))
	for id, supervision := range r.supervisions {
		if supervision.serverOwned {
			ids = append(ids, id)
		}
	}
	lifecycleCancel := r.lifecycleCancel
	r.mu.Unlock()
	sort.Strings(ids)

	var stopErr error
	for _, id := range ids {
		if _, err := r.terminateForShutdown(ctx, id); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
		if err := r.waitForSupervisionDriver(ctx, id); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
	}
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			stopErr = errors.Join(stopErr, ctx.Err())
		}
	}
	if lifecycleCancel != nil {
		lifecycleCancel()
	}
	return stopErr
}

func (r *registry) waitForSupervisionDriver(ctx context.Context, id string) error {
	r.mu.RLock()
	supervision := r.supervisions[id]
	r.mu.RUnlock()
	if supervision == nil || supervision.driverDone == nil {
		return nil
	}
	select {
	case <-supervision.driverDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// invocationPreparationOptions select the ownership and acceptance behavior
// around the shared invocation setup. The synchronous path intentionally
// keeps its historical EventsAppender-only dependency and caller-visible
// terminal return contract; asynchronous Start adds the retained/live topic
// readiness barrier and server-owned lifecycle admission around this same
// preparation and supervision state machine.
type invocationPreparationOptions struct {
	serverOwned      bool
	direct           bool
	continuation     bool
	runtimeOwned     bool
	requestID        string
	verifyTopicReady bool
	lineage          *workers.SessionLineage
}

type invocationPreparation struct {
	supervision  *supervision
	session      workersessions.Session
	terminal     bool
	preAdmission bool
	failure      error
}

// prepareInvocation owns the setup shared by synchronous InvokeSession and
// asynchronous Start: identity reservation, STARTING transition, observation
// capture, opening publication, and supervision registration. It is the only
// path allowed to move a resolved invocation from RESERVED to a Workers
// handoff. Terminal setup failures are committed and published here so the
// two entry points cannot drift in event ordering or terminal classification;
// callers choose only whether that deterministic failure is returned as an
// InvokeSession result or as Start's not-accepted error.
func (r *registry) prepareInvocation(
	ctx context.Context,
	req workersessions.InvokeSessionRequest,
	options invocationPreparationOptions,
) (invocationPreparation, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	r.reserveIfAbsent(req.ID)
	acceptedFields := []any{
		"sessionID", req.ID,
		"attemptID", attemptID,
		"outcome", "reserved",
		"state", string(workersessions.StateReserved),
	}
	if options.requestID != "" {
		acceptedFields = append(acceptedFields, "requestID", options.requestID)
	}
	r.logger.Info("worker session start accepted", acceptedFields...)

	if _, err := r.transitionToStartingWithExecution(req.ID, req.Execution.Execution); err != nil {
		if terminal, ok := r.preAdmissionControlTerminal(req.ID, attemptID); ok {
			r.publishTerminalSnapshot(ctx, req.ID, attemptID, terminal)
			return invocationPreparation{
				session:      terminal,
				terminal:     true,
				preAdmission: true,
				failure:      workersessions.ErrStartAdmissionFailed,
			}, nil
		}
		fields := []any{"sessionID", req.ID, "attemptID", attemptID, "outcome", "not_startable"}
		if options.requestID != "" {
			fields = append(fields, "requestID", options.requestID)
		}
		r.logger.Info("worker session start rejected", fields...)
		return invocationPreparation{}, err
	}
	startedAt := r.ensureObservationWithFactorySession(
		req.ID,
		attemptID,
		req.Execution.Execution.Dispatch.Execution.RequestID,
		req.Execution.Execution.Dispatch.Execution.WorkIDs,
		options.direct,
		req.Execution.Execution.FactorySessionID,
	)
	workerRecording, err := r.startWorkerRecording(ctx, req)
	if err != nil {
		r.logger.Info(
			"worker session recording opening rejected",
			"sessionID", req.ID,
			"attemptID", attemptID,
			"outcome", "failed",
			"error", err.Error(),
		)
		final := r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
		return invocationPreparation{
			session:  final,
			terminal: true,
			failure:  workersessions.ErrStartOpeningPublication,
		}, nil
	}

	if err := r.publishOpeningRecord(
		ctx,
		req.ID,
		attemptID,
		openingSessionPayload(req.ID, attemptID, startedAt, req.Execution.Execution, options.lineage),
		providerIdentityForExecution(req.Execution.Execution),
		workerRecording,
	); err != nil {
		final := r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
		return invocationPreparation{
			session:  final,
			terminal: true,
			failure:  workersessions.ErrStartOpeningPublication,
		}, nil
	}
	if options.verifyTopicReady {
		if err := r.ensureOpeningTopicReady(ctx, req.ID); err != nil {
			final := r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
			return invocationPreparation{
				session:  final,
				terminal: true,
				failure:  errors.Join(workersessions.ErrStartOpeningPublication, workersessions.ErrEventTopicUnavailable),
			}, nil
		}
	}
	if options.serverOwned && r.isStopping() {
		final := r.terminalizeInvocationBeforeAdmission(ctx, req.ID, attemptID)
		return invocationPreparation{
			session:  final,
			terminal: true,
			failure:  workersessions.ErrStartServerStopping,
		}, nil
	}
	if options.runtimeOwned {
		return invocationPreparation{}, nil
	}

	return r.registerInvocationSupervision(ctx, req, options)
}

func (r *registry) registerInvocationSupervision(
	ctx context.Context,
	req workersessions.InvokeSessionRequest,
	options invocationPreparationOptions,
) (invocationPreparation, error) {
	attemptID := req.Execution.Execution.Dispatch.DispatchID
	if options.verifyTopicReady {
		eventReadyFields := []any{"sessionID", req.ID, "attemptID", attemptID, "outcome", "event_ready", "state", string(workersessions.StateStarting)}
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
		req.Execution,
	)
	if !canStart {
		final, _ := r.Get(context.Background(), workersessions.GetRequest{ID: req.ID})
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

// startReserved runs the original Start state machine after reserveStart has
// atomically installed the request replay and RESERVED session records.
func (r *registry) startReserved(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	serverCtx := r.serverOwnedContext()
	prepared, err := r.prepareInvocation(
		serverCtx,
		workersessions.InvokeSessionRequest{ID: req.ID, Execution: req.Execution, Retry: req.Retry},
		invocationPreparationOptions{
			serverOwned:      true,
			direct:           true,
			requestID:        req.RequestID,
			verifyTopicReady: true,
		},
	)
	if err != nil {
		return workersessions.StartResult{}, err
	}
	if prepared.terminal {
		return workersessions.StartResult{Session: prepared.session}, startNotAccepted(prepared.failure)
	}
	invokeReq := workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	}
	go func() {
		r.driveRegisteredInvocation(serverCtx, invokeReq, prepared.supervision)
	}()

	select {
	case <-prepared.supervision.admitted:
		return r.startResult(req.ID), nil
	case <-prepared.supervision.done:
		select {
		case <-prepared.supervision.admitted:
			return r.startResult(req.ID), nil
		default:
			return r.startResult(req.ID), startNotAccepted(r.startAdmissionCause(prepared.supervision))
		}
	}
}

func (r *registry) startResult(id string) workersessions.StartResult {
	session, _ := r.Get(context.Background(), workersessions.GetRequest{ID: id})
	return workersessions.StartResult{Session: session}
}

func (r *registry) startAdmissionCause(supervision *supervision) error {
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	if supervision.err != nil {
		return errors.Join(workersessions.ErrStartAdmissionFailed, supervision.err)
	}
	return workersessions.ErrStartAdmissionFailed
}

func (r *registry) publishTerminalSnapshot(ctx context.Context, id, attemptID string, session workersessions.Session) {
	if !session.Terminal() {
		return
	}
	result := workersessions.TerminalResult{}
	if session.Result != nil {
		result = *session.Result
	}
	r.publishTerminalRecordOrLog(ctx, id, attemptID, session.State, result)
}

// ensureOpeningTopicReady proves both retained replay and live subscription
// can observe the opening record before Start hands the execution to Workers.
// The canceled cleanup call unregisters the short-lived readiness subscriber
// without relying on timing or a store-specific Close operation.
func (r *registry) ensureOpeningTopicReady(ctx context.Context, id string) error {
	if r.retainedReader == nil || r.eventReader == nil {
		return workersessions.ErrEventTopicUnavailable
	}
	topic := workersessions.Topic(id)
	readResult, err := r.retainedReader.Read(ctx, events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 1,
	})
	if err != nil || readResult.Validate() != nil || readResult.Outcome != events.ReadOutcomeProgress || len(readResult.Records) == 0 || !openingRecordMatches(readResult.Records[0], id) {
		return workersessions.ErrEventTopicUnavailable
	}

	subscription, err := r.eventReader.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 1,
	})
	if err != nil || subscription == nil {
		return workersessions.ErrEventTopicUnavailable
	}
	delivery := subscription.Next(ctx)
	cleanupCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = subscription.Next(cleanupCtx)
	if delivery.Validate() != nil || delivery.Kind != events.DeliveryRecord || !openingRecordMatches(delivery.Record, id) {
		return workersessions.ErrEventTopicUnavailable
	}
	return nil
}

func openingRecordMatches(record events.Record, id string) bool {
	return record.ID.Topic == workersessions.Topic(id) &&
		record.SourceType == lifecycleSourceType &&
		record.SourceID == events.SourceID(id) &&
		record.SourceSequence == openingSourceSequence &&
		record.SourceEventID == openingSourceEventID &&
		record.SchemaID == workerDraftSchemaID
}

func (r *registry) terminalizeInvocationBeforeAdmission(ctx context.Context, id, attemptID string) workersessions.Session {
	terminal := failedTerminal(workersessions.FailureCauseEventPublicationFailure, safeDetail(workersessions.FailureCauseEventPublicationFailure, nil))
	final, committed := r.commitTerminal(id, workersessions.StateFailed, terminal)
	if committed {
		r.logTerminal(id, attemptID, final)
		r.publishTerminalRecordOrLog(ctx, id, attemptID, workersessions.StateFailed, *final.Result)
	} else {
		r.publishTerminalSnapshot(ctx, id, attemptID, final)
	}
	return final
}

// sourceKey narrows the four-part Events idempotency identity down to the
// (SourceType, SourceID) pair PublishRecord tracks ordering against: every
// record sharing one sourceKey is one source's own observation stream, and
// must commit in non-decreasing SourceSequence order within it.
type sourceKey struct {
	sourceType events.SourceType
	sourceID   events.SourceID
}

// publication is one Worker Session's own publication window: the
// mutual-exclusion boundary that makes "opening record, then every accepted
// source-native record in source order, then exactly one terminal record"
// hold even under concurrent PublishRecord callers and a racing terminal
// append. mu serializes every record this session ever commits through
// appendDraft -- publishOpeningRecord, PublishRecord, and
// publishTerminalRecord alike -- so at most one commit for this session is
// ever in flight at a time, and the two lifecycle boundaries (open, then
// closed) are observed by every publish attempt under the same lock that
// guards them. lastSequence and open are owned entirely by that lock; a
// publication is never read or written without holding mu.
type publication struct {
	mu                sync.Mutex
	control           controlHistoryGate
	completedControls map[string]struct{}
	// open is true only between a successfully committed opening record and
	// the start of the terminal-record commit attempt. PublishRecord rejects
	// every call observed while open is false, whether that is because the
	// session was only ever Reserved, its opening record has not yet
	// committed, or its terminal record has already started committing.
	open bool
	// lastSequence is the highest SourceSequence already accepted for each
	// (SourceType, SourceID) this session has published, used to reject a
	// record whose SourceSequence regresses behind one already committed.
	lastSequence map[sourceKey]events.SourceSequence
	// accepted holds every full Events idempotency identity this session has
	// already committed. It lets an exact retry of a previously accepted
	// identity reach Events (and resolve to the original record as a
	// duplicate) even after a later SourceSequence has advanced lastSequence
	// past it -- Events itself retains identities permanently for dedup, so a
	// retry must stay idempotent regardless of publication order since. Only
	// an identity never accepted before is subject to the out-of-order
	// rejection below.
	accepted map[events.AppendIdentity]struct{}
	// provider is the provider identity established by the opening record or
	// the first provider-bound lifecycle update. It is guarded by mu and is
	// intentionally separate from the optional Provider Session association.
	provider string
	// turnID is retained only to correlate a provider-bound SESSION/UPDATED
	// record when the opening did not yet know the provider.
	turnID string
	// recording is the Recordings-owned capture that must remain live until
	// the terminal Worker record has been durably consumed.
	recording recordings.WorkerSessionRecording
	// recordingID remains after recording is closed so a source-side
	// continuation lineage record can be appended to the same durable sidecar.
	recordingID string
}

// publicationFor returns the publication registered for id, or nil if id was
// never reserved. The returned pointer is stable for id's lifetime: callers
// serialize on its own mu rather than r.mu, so looking it up is a brief,
// independent critical section.
func (r *registry) publicationFor(id string) *publication {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.publications[id]
}

// PublishRecord validates req, then appends req.Draft, detached, as a
// source-native Worker record onto workersessions.Topic(req.SessionID) using
// req's complete Events idempotency identity, through the same appendDraft
// helper publishOpeningRecord uses. PublishRecord requires an established
// publication window: req.SessionID must have committed its opening record
// and must not yet have started committing its terminal record, and it
// enforces that accepted records for one (SourceType, SourceID) commit in
// non-decreasing SourceSequence order, rejecting a call that would regress
// behind one already accepted -- unless the call's full identity was itself
// already accepted, in which case it always reaches Events and resolves to
// the original record as a duplicate. Beyond that ordering and window
// enforcement, PublishRecord relies on Events for aggregate order, duplicate resolution,
// cursors, reads, and subscriptions. An invalid Draft, an unopened or closed
// publication window, an out-of-order SourceSequence, a malformed Events
// identity, or a rejected Events append is returned unchanged, and no record
// is committed.
func (r *registry) PublishRecord(ctx context.Context, req workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "invalid")
		return workersessions.PublishRecordResult{}, err
	}

	pub := r.publicationFor(req.SessionID)
	if pub == nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "not_found")
		return workersessions.PublishRecordResult{}, workersessions.ErrSessionNotFound
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()

	if !pub.open {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "publication_not_open")
		return workersessions.PublishRecordResult{}, workersessions.ErrPublicationNotOpen
	}
	if err := r.ensurePublishRecordProvider(ctx, req, pub); err != nil {
		return workersessions.PublishRecordResult{}, err
	}

	key := sourceKey{sourceType: req.SourceType, sourceID: req.SourceID}
	identity := events.AppendIdentity{
		SourceType:     req.SourceType,
		SourceID:       req.SourceID,
		SourceSequence: req.SourceSequence,
		SourceEventID:  req.SourceEventID,
	}
	_, alreadyAccepted := pub.accepted[identity]
	if last := pub.lastSequence[key]; !alreadyAccepted && req.SourceSequence < last {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "out_of_order")
		return workersessions.PublishRecordResult{}, workersessions.ErrOutOfOrderPublication
	}

	appendResult, err := r.appendDraft(ctx, workersessions.Topic(req.SessionID), identity, req.SchemaID, req.Draft)
	if err != nil {
		r.logger.Info("worker session publish record rejected", "sessionID", req.SessionID, "outcome", "append_failed")
		return workersessions.PublishRecordResult{}, err
	}
	r.updateUsageProjection(req.SessionID, req.Draft)
	pub.accepted[identity] = struct{}{}
	if req.SourceSequence > pub.lastSequence[key] {
		pub.lastSequence[key] = req.SourceSequence
	}
	outcome := workersessions.PublishOutcomeAccepted
	if appendResult.Outcome == events.AppendOutcomeDuplicate {
		outcome = workersessions.PublishOutcomeDuplicate
	}
	r.logger.Info(
		"worker session publish record",
		"sessionID", req.SessionID,
		"outcome", publishOutcomeLabel(outcome),
		"aggregate_sequence", uint64(appendResult.Record.ID.Position),
	)
	return workersessions.PublishRecordResult{
		SessionID:         req.SessionID,
		AggregateSequence: appendResult.Record.ID.Position,
		Outcome:           outcome,
	}, nil
}

func publishOutcomeLabel(outcome workersessions.PublishOutcome) string {
	switch outcome {
	case workersessions.PublishOutcomeAccepted:
		return "accepted"
	case workersessions.PublishOutcomeDuplicate:
		return "duplicate"
	default:
		return "unspecified"
	}
}

func sameProviderIdentity(left, right string) bool {
	return strings.EqualFold(
		providers.ID(left).CanonicalSessionProvider(),
		providers.ID(right).CanonicalSessionProvider(),
	)
}

func (r *registry) ensurePublishRecordProvider(
	ctx context.Context,
	req workersessions.PublishRecordRequest,
	pub *publication,
) error {
	provider := strings.TrimSpace(req.Draft.Provenance.Provider)
	if provider == "" || strings.EqualFold(provider, "agent-run") {
		return nil
	}
	if pub.provider != "" {
		if !sameProviderIdentity(pub.provider, provider) {
			return workersessions.ErrProviderBindingConflict
		}
		return nil
	}

	dispatchID := strings.TrimSpace(req.Draft.DispatchID)
	if dispatchID == "" {
		return workersessions.ErrInvalidProviderBinding
	}
	r.mu.RLock()
	ownerID, exists := r.dispatchOwners[dispatchID]
	r.mu.RUnlock()
	if !exists || ownerID != req.SessionID {
		return workersessions.ErrProviderBindingAttemptMismatch
	}
	_, err := r.publishProviderBindingLocked(ctx, req.SessionID, dispatchID, provider, pub)
	return err
}

// EnsureProviderBinding publishes the first provider-bound lifecycle record
// for the dispatch when the opening record did not already resolve a
// provider. The publication lock is the same lock used by source-native
// observations and the terminal boundary, so a provider output can never
// overtake its required SESSION/UPDATED binding.
func (r *registry) EnsureProviderBinding(
	ctx context.Context,
	req workersessions.ProviderBindingRequest,
) (workersessions.ProviderBindingResult, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session provider binding rejected", "attemptID", req.DispatchID, "outcome", "invalid")
		return workersessions.ProviderBindingResult{}, err
	}

	r.mu.RLock()
	ownerID, exists := r.dispatchOwners[strings.TrimSpace(req.DispatchID)]
	r.mu.RUnlock()
	if !exists {
		r.logger.Info("worker session provider binding rejected", "attemptID", req.DispatchID, "outcome", "unknown_dispatch")
		return workersessions.ProviderBindingResult{}, workersessions.ErrProviderBindingAttemptMismatch
	}

	pub := r.publicationFor(ownerID)
	if pub == nil {
		return workersessions.ProviderBindingResult{}, workersessions.ErrSessionNotFound
	}
	provider := providers.ID(req.Provider).CanonicalSessionProvider()
	pub.mu.Lock()
	defer pub.mu.Unlock()
	return r.publishProviderBindingLocked(ctx, ownerID, strings.TrimSpace(req.DispatchID), provider, pub)
}

func (r *registry) publishProviderBindingLocked(
	ctx context.Context,
	ownerID string,
	dispatchID string,
	provider string,
	pub *publication,
) (workersessions.ProviderBindingResult, error) {
	provider = providers.ID(provider).CanonicalSessionProvider()
	if !pub.open {
		r.logger.Info("worker session provider binding rejected", "sessionID", ownerID, "attemptID", dispatchID, "outcome", "publication_not_open")
		return workersessions.ProviderBindingResult{}, workersessions.ErrPublicationNotOpen
	}
	if pub.provider != "" {
		if !sameProviderIdentity(pub.provider, provider) {
			return workersessions.ProviderBindingResult{}, workersessions.ErrProviderBindingConflict
		}
		return workersessions.ProviderBindingResult{
			WorkerSessionID: ownerID,
			DispatchID:      dispatchID,
			Provider:        pub.provider,
			Outcome:         workersessions.ProviderBindingOutcomeDuplicate,
		}, nil
	}

	selection := workers.SessionProviderSelection{RunnerID: provider}
	payload := workers.SessionPayload{
		Status:            string(workersessions.StateStarting),
		WorkerSessionID:   ownerID,
		DispatchID:        dispatchID,
		TurnID:            pub.turnID,
		AttemptID:         dispatchID,
		ProviderSelection: &selection,
	}
	payloadJSON, _ := json.Marshal(payload)
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseUpdated,
		Provenance: lifecycleProvenance(provider),
		Payload:    payloadJSON,
		DispatchID: dispatchID,
		TurnID:     pub.turnID,
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       providerBindingSourceID(ownerID),
		SourceSequence: providerBindingSourceSequence,
		SourceEventID:  providerBindingSourceEventID,
	}
	appendResult, err := r.appendDraft(ctx, workersessions.Topic(ownerID), identity, workerDraftSchemaID, draft)
	if err != nil {
		r.logger.Info("worker session provider binding rejected", "sessionID", ownerID, "attemptID", dispatchID, "outcome", "append_failed")
		return workersessions.ProviderBindingResult{}, err
	}
	pub.provider = provider
	outcome := workersessions.ProviderBindingOutcomeAccepted
	if appendResult.Outcome == events.AppendOutcomeDuplicate {
		outcome = workersessions.ProviderBindingOutcomeDuplicate
	}
	r.logger.Info("worker session provider binding", "sessionID", ownerID, "attemptID", dispatchID, "provider", provider, "outcome", string(outcome))
	return workersessions.ProviderBindingResult{
		WorkerSessionID: ownerID,
		DispatchID:      dispatchID,
		Provider:        provider,
		Outcome:         outcome,
	}, nil
}

// WorkerSessionIDForDispatch resolves the current Workers attempt identity
// to the stable Worker Session that owns its topic. Provider progress names
// dispatches, while lifecycle records are keyed by the stable session.
func (r *registry) WorkerSessionIDForDispatch(_ context.Context, dispatchID string) (string, error) {
	if strings.TrimSpace(dispatchID) == "" {
		return "", workersessions.ErrInvalidProviderBinding
	}
	r.mu.RLock()
	ownerID, exists := r.dispatchOwners[strings.TrimSpace(dispatchID)]
	r.mu.RUnlock()
	if !exists {
		return "", workersessions.ErrProviderBindingAttemptMismatch
	}
	return ownerID, nil
}
func (r *registry) StreamObservations(ctx context.Context, req workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation stream rejected", "outcome", "invalid")
		return workersessions.ObservationSubscription{}, err
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	workerSessionID, alreadyTerminal, workerSessionState, err := r.observationStreamSession(req.ProviderSession)
	if err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	return r.streamObservationTopic(ctx, workerSessionID, workerSessionState, alreadyTerminal, req.Limit, req.ReplayOnly, req.Cursor)
}

func (r *registry) StreamObservationsByWorkerSessionID(ctx context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	if err := req.Validate(); err != nil {
		r.logger.Info("worker session observation stream by Worker Session rejected", "outcome", "invalid")
		return workersessions.ObservationSubscription{}, err
	}
	req.WorkerSessionID = strings.TrimSpace(req.WorkerSessionID)
	if err := observationContextError(ctx); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	// A restarted process has no in-memory session or Events topic. The
	// Recordings-owned v2 artifact is the authoritative Worker-ID replay
	// source in that case, and also avoids making replay depend on a provider
	// session store.
	session, _, exists := r.loadObservationState(req.WorkerSessionID)
	preferDurable := req.ReplayOnly || !exists || session.State.Terminal()
	if preferDurable {
		if projection, found, err := r.durableWorkerProjection(ctx, req.WorkerSessionID); err != nil {
			return workersessions.ObservationSubscription{}, err
		} else if found {
			return durableObservationStream(ctx, projection, req.Limit, req.Cursor)
		}
	}
	workerSessionID, alreadyTerminal, workerSessionState, err := r.observationStreamSessionByID(req.WorkerSessionID)
	if err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	return r.streamObservationTopic(ctx, workerSessionID, workerSessionState, alreadyTerminal, req.Limit, req.ReplayOnly, req.Cursor)
}

func (r *registry) streamObservationTopic(
	ctx context.Context,
	workerSessionID string,
	workerSessionState workersessions.State,
	alreadyTerminal bool,
	limit int,
	replayOnly bool,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	if err := validateObservationCursorWorkerSessionID(cursor, workerSessionID); err != nil {
		return workersessions.ObservationSubscription{}, err
	}
	if !replayOnly && r.eventReader == nil {
		r.logger.Info("worker session observation stream", "outcome", "source_unavailable")
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	if limit == 0 {
		limit = workersessions.DefaultObservationStreamLimit
	}
	topic := workersessions.Topic(workerSessionID)
	if replayOnly {
		return r.replayObservationStream(ctx, topic, workerSessionState, limit, cursor)
	}
	return r.liveObservationStream(ctx, topic, limit, alreadyTerminal, cursor)
}

func validateObservationCursorWorkerSessionID(
	cursor *workersessions.ObservationCursor,
	workerSessionID string,
) error {
	if cursor == nil || strings.TrimSpace(cursor.WorkerSessionID) == "" {
		return nil
	}
	if strings.TrimSpace(cursor.WorkerSessionID) != strings.TrimSpace(workerSessionID) {
		return workersessions.ErrObservationCursorForeign
	}
	return nil
}

func (r *registry) observationStreamSession(ref providers.SessionRef) (string, bool, workersessions.State, error) {
	r.mu.RLock()
	workerSessionID := ""
	alreadyTerminal := false
	workerSessionState := workersessions.StateReserved
	for id, session := range r.sessions {
		if session.ProviderSessionAssociation != nil &&
			session.ProviderSessionAssociation.Reference == ref {
			workerSessionID = id
			alreadyTerminal = session.Terminal()
			workerSessionState = session.State
			break
		}
	}
	r.mu.RUnlock()
	if workerSessionID == "" {
		r.logger.Info("worker session observation stream", "outcome", "not_found")
		return "", false, workersessions.StateReserved, workersessions.ErrObservationSessionNotFound
	}
	return workerSessionID, alreadyTerminal, workerSessionState, nil
}

func (r *registry) observationStreamSessionByID(id string) (string, bool, workersessions.State, error) {
	r.mu.RLock()
	session, exists := r.sessions[id]
	r.mu.RUnlock()
	if !exists {
		r.logger.Info("worker session observation stream by Worker Session", "workerSessionID", id, "outcome", "not_found")
		return "", false, workersessions.StateReserved, workersessions.ErrObservationSessionNotFound
	}
	return id, session.Terminal(), session.State, nil
}

func (r *registry) replayObservationStream(
	ctx context.Context,
	topic events.Topic,
	state workersessions.State,
	limit int,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	replay, err := newReplayObservationSubscription(ctx, r.retainedReader, topic, state, limit, cursor)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, workersessions.ErrObservationCanceled) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCanceled
		}
		return workersessions.ObservationSubscription{}, err
	}
	wrapped := &observationSubscription{replay: replay, workerSessionID: observationWorkerSessionIDFromTopic(topic)}
	return workersessions.ObservationSubscription{NextFunc: wrapped.Next, CloseFunc: wrapped.Close}, nil
}

func (r *registry) closeWorkerRecording(
	ctx context.Context,
	recording recordings.WorkerSessionRecording,
	state workersessions.State,
	position events.AggregateSequence,
) error {
	finalizer, ok := recording.(recordings.WorkerSessionRecordingFinalizer)
	if !ok {
		return recording.Close(ctx)
	}
	phase, err := terminalPhase(state)
	if err != nil {
		return errors.Join(err, recording.Close(ctx))
	}
	return finalizer.CloseWithTerminal(ctx, recordings.WorkerRecordingTerminal{
		Position: position,
		Phase:    phase,
		Status:   string(state),
	})
}

func (r *registry) logTerminal(id, attemptID string, session workersessions.Session) {
	cause := ""
	if session.Result != nil {
		cause = causeKindString(session.Result.Cause)
	}
	r.logger.Info("worker session start terminal", "sessionID", id, "attemptID", attemptID, "outcome", string(session.State), "state", string(session.State), "cause", cause)
}

func (r *registry) sessionState(id string) workersessions.State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if session, ok := r.sessions[id]; ok {
		return session.State
	}
	return ""
}

func (r *registry) logReconciliationIfNeeded(
	id, attemptID string,
	result workers.WorkstationDispatchResult,
	priorState, resultingState workersessions.State,
	startedAt, deadlineAt time.Time,
) {
	if result.ReconciliationReason == "" {
		return
	}
	r.logReconciliation(id, attemptID, result, priorState, resultingState, startedAt, deadlineAt)
}

func (r *registry) liveObservationStream(
	ctx context.Context,
	topic events.Topic,
	limit int,
	terminalReplay bool,
	cursor *workersessions.ObservationCursor,
) (workersessions.ObservationSubscription, error) {
	workerSessionID := observationWorkerSessionIDFromTopic(topic)
	streamGenerationID := ""
	if r.workerRecordingHistoryReader() != nil {
		streamGenerationID = durableWorkerStreamGenerationForIdentity(workerSessionID)
	}
	from := events.Cursor{Topic: topic}
	if cursor != nil {
		from.Position = events.AggregateSequence(cursor.Position)
		if cursor.StreamGenerationID != "" && cursor.StreamGenerationID != streamGenerationID {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCursorUnavailable
		}
	}
	subscription, err := r.eventReader.Subscribe(ctx, events.SubscribeRequest{
		Topic: topic,
		From:  from,
		Limit: limit,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCanceled
		}
		if errors.Is(err, events.ErrUnresolvableCursor) {
			return workersessions.ObservationSubscription{}, workersessions.ErrObservationCursorFuture
		}
		return workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	wrapped := &observationSubscription{
		source: subscription, workerSessionID: workerSessionID, streamGenerationID: streamGenerationID, terminalReplay: terminalReplay,
		cursorProvided: cursor != nil,
	}
	return workersessions.ObservationSubscription{NextFunc: wrapped.Next, CloseFunc: wrapped.Close}, nil
}
