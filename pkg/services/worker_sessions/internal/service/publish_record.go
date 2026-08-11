package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
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
	requestID        string
	verifyTopicReady bool
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

	if _, err := r.transitionToStarting(req.ID); err != nil {
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
	r.ensureObservation(
		req.ID,
		attemptID,
		req.Execution.Execution.Dispatch.Execution.RequestID,
		req.Execution.Execution.Dispatch.Execution.WorkIDs,
	)

	if err := r.publishOpeningRecord(ctx, req.ID, attemptID); err != nil {
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
	supervision.mu.Unlock()
	return invocationPreparation{supervision: supervision}, nil
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
	mu sync.Mutex
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

// appendDraft validates draft with the existing Workers draft rules, then
// appends it, detached, onto topic using identity as the complete Events
// idempotency tuple. Every SESSION or source-native Worker record this
// package commits -- the W3 opening record and a caller's published source
// observation alike -- funnels through this one helper, so "validate with
// workers.ValidateDraft, then events.Append with a caller-owned identity" is
// defined in exactly one place. A non-nil error means no record was
// committed; draft is never marshaled or appended once ValidateDraft
// rejects it.
func (r *registry) appendDraft(ctx context.Context, topic events.Topic, identity events.AppendIdentity, schemaID events.SchemaID, draft workers.Draft) (events.AppendResult, error) {
	if err := workers.ValidateDraft(draft); err != nil {
		return events.AppendResult{}, fmt.Errorf("worker sessions: invalid draft: %w", err)
	}
	// draft's fields are exhaustively strings and json.RawMessage, neither of
	// which json.Marshal can fail to encode, so the marshal error is
	// unreachable and intentionally discarded rather than defended against.
	envelope, _ := json.Marshal(draft)
	return r.events.Append(ctx, events.AppendRequest{
		Topic:          topic,
		SourceType:     identity.SourceType,
		SourceID:       identity.SourceID,
		SourceSequence: identity.SourceSequence,
		SourceEventID:  identity.SourceEventID,
		SchemaID:       schemaID,
		Payload:        envelope,
	}.Detached())
}

// Fixed identity Start uses to commit the one opening SESSION/STARTED record
// onto workersessions.Topic(id) before Workers invocation, and the one
// terminal SESSION record after. Both records are Worker-Sessions-owned
// lifecycle, not a source-native Workers observation, so their SourceID is
// the Worker Session's own stable identity and their SourceSequence/
// SourceEventID are fixed constants scoped by that SourceID: a genuine retry
// can never occur (transitionToStarting and commitTerminal each succeed at
// most once per identity), so no richer sequencing is required here.
const (
	lifecycleSourceType    events.SourceType     = "worker_session_lifecycle"
	openingSourceSequence  events.SourceSequence = 1
	openingSourceEventID   events.SourceEventID  = "started"
	terminalSourceSequence events.SourceSequence = 2
	terminalSourceEventID  events.SourceEventID  = "terminal"
	workerDraftSchemaID    events.SchemaID       = "workers.draft.v1"
)

// publishOpeningRecord commits the one opening KindSession/PhaseStarted
// workers.Draft onto workersessions.Topic(id), detached from any
// caller-owned backing array, before Start ever calls
// workers.WorkstationExecutionService.DispatchWorkstation. It runs under
// id's publication lock, the same lock PublishRecord and
// publishTerminalRecord use, and opens id's publication window only once
// the append itself has committed: no PublishRecord call can be accepted for
// id until this succeeds. A non-nil return means no record was committed and
// the window stays closed: Start must not proceed to Workers handoff.
func (r *registry) publishOpeningRecord(ctx context.Context, id, attemptID string) error {
	pub := r.publicationFor(id)
	pub.mu.Lock()
	defer pub.mu.Unlock()

	// workers.SessionPayload{Status: string} has one string field, so
	// json.Marshal cannot fail here; the error is intentionally discarded
	// rather than defended against.
	draftPayload, _ := json.Marshal(workers.SessionPayload{Status: string(workersessions.StateStarting)})
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseStarted,
		Payload:    draftPayload,
		DispatchID: attemptID,
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: openingSourceSequence,
		SourceEventID:  openingSourceEventID,
	}
	if _, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft); err != nil {
		return err
	}
	pub.open = true
	pub.lastSequence = make(map[sourceKey]events.SourceSequence)
	pub.accepted = make(map[events.AppendIdentity]struct{})
	return nil
}

// terminalSessionPayload is the SESSION KindSession payload committed for
// the W3 terminal record. It always decodes as a valid workers.SessionPayload
// (Status is the one field that type declares), so validation and any reader
// that only knows about workers.SessionPayload continue to work unchanged;
// FailureCause/FailureDetail are additive fields present only on a FAILED
// terminal record, carrying the already-computed, already-safe
// FailureCause.Kind/Detail worker_sessions itself derives rather than any
// new free-form text.
type terminalSessionPayload struct {
	Status        string `json:"status,omitempty"`
	FailureCause  string `json:"failureCause,omitempty"`
	FailureDetail string `json:"failureDetail,omitempty"`
}

// terminalPhase is the pure mapping from a committed Worker Session State to
// its terminal projection Phase: COMPLETED and FAILED map one-to-one, and
// the W1 CANCELED/TERMINATED states -- neither reachable through Start until
// W6 adds controls -- share the existing PhaseCanceled pair per the W3 scope
// note that no new phase is introduced. Any other state has no terminal
// projection.
func terminalPhase(state workersessions.State) (workers.Phase, error) {
	switch state {
	case workersessions.StateCompleted:
		return workers.PhaseCompleted, nil
	case workersessions.StateFailed:
		return workers.PhaseFailed, nil
	case workersessions.StateCanceled, workersessions.StateTerminated:
		return workers.PhaseCanceled, nil
	default:
		return "", fmt.Errorf("worker sessions: state %q has no terminal projection phase", state)
	}
}

// terminalDraft is the pure mapping from a committed Worker Session
// State+TerminalResult to the one KindSession terminal workers.Draft W3
// projects after prior output. terminalDraft is side-effect free so every
// state, including the CANCELED/TERMINATED cases Start cannot yet produce, is
// directly unit-testable without a live session or Events call.
func terminalDraft(state workersessions.State, result workersessions.TerminalResult, attemptID string) (workers.Draft, error) {
	phase, err := terminalPhase(state)
	if err != nil {
		return workers.Draft{}, err
	}
	if state == workersessions.StateCompleted || state == workersessions.StateFailed {
		if err := result.Validate(); err != nil {
			return workers.Draft{}, err
		}
	}
	payload := terminalSessionPayload{Status: string(state)}
	if result.Cause != nil {
		payload.FailureCause = string(result.Cause.Kind)
		payload.FailureDetail = result.Cause.Detail
	}
	// terminalSessionPayload has only string fields, so json.Marshal cannot
	// fail to encode; the error is intentionally discarded rather than
	// defended against.
	payloadJSON, _ := json.Marshal(payload)
	return workers.Draft{
		Kind:       workers.KindSession,
		Phase:      phase,
		Payload:    payloadJSON,
		DispatchID: attemptID,
	}, nil
}

// publishTerminalRecord commits the one terminal KindSession workers.Draft
// onto workersessions.Topic(id), derived from state+result, through the same
// appendDraft helper publishOpeningRecord and PublishRecord already share.
// It runs under id's publication lock and closes id's publication window
// before attempting the append: once this call starts, no concurrent or
// later PublishRecord call can be accepted for id, and any PublishRecord call
// already holding the lock has fully committed or failed by the time this
// one acquires it. A non-nil return means no record was committed; the
// caller must not rewrite the already-committed canonical W2 terminal
// Session on this failure -- see publishTerminalRecordOrLog.
func (r *registry) publishTerminalRecord(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) error {
	draft, err := terminalDraft(state, result, attemptID)
	if err != nil {
		return err
	}

	pub := r.publicationFor(id)
	pub.mu.Lock()
	defer pub.mu.Unlock()
	pub.open = false

	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: terminalSourceSequence,
		SourceEventID:  terminalSourceEventID,
	}
	_, err = r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	return err
}

// publishTerminalRecordOrLog calls publishTerminalRecord and, on failure,
// logs it explicitly rather than propagating it: the caller already holds
// the real committed terminal Session (commitTerminal's return value) and
// must return that value unchanged. A terminal-record publication failure is
// never reported as success and never silently replaced with fabricated
// history.
func (r *registry) publishTerminalRecordOrLog(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) {
	if err := r.publishTerminalRecord(ctx, id, attemptID, state, result); err != nil {
		r.logger.Info(
			"worker session terminal record publication failed",
			"sessionID", id,
			"attemptID", attemptID,
			"state", string(state),
			"outcome", "publish_failed",
		)
	}
}
