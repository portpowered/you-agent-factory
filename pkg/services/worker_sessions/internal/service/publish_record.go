package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
		workers.CanonicalProviderSessionProvider(left),
		workers.CanonicalProviderSessionProvider(right),
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
	provider := workers.CanonicalProviderSessionProvider(req.Provider)
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
	provider = workers.CanonicalProviderSessionProvider(provider)
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

// Fixed identities Worker Sessions uses to commit its opening, optional
// provider-binding, and terminal lifecycle records. The binding has its own
// SourceID so the opening/terminal source keeps its historical sequence
// numbers while the publication lock still determines aggregate order.
const (
	lifecycleSourceType           events.SourceType     = "worker_session_lifecycle"
	openingSourceSequence         events.SourceSequence = 1
	openingSourceEventID          events.SourceEventID  = "started"
	providerBindingSourceSequence events.SourceSequence = 1
	providerBindingSourceEventID  events.SourceEventID  = "provider-bound"
	terminalSourceSequence        events.SourceSequence = 2
	terminalSourceEventID         events.SourceEventID  = "terminal"
	workerDraftSchemaID           events.SchemaID       = "workers.draft.v1"
)

func providerBindingSourceID(id string) events.SourceID {
	return events.SourceID(id + "/provider-binding")
}

func lifecycleProvenance(provider string) workers.Provenance {
	return workers.Provenance{
		Delivery:        workers.DeliverySynthesized,
		Fidelity:        workers.FidelityLifecycleOnly,
		NativeEventType: string(lifecycleSourceType),
		Provider:        workers.CanonicalProviderSessionProvider(provider),
		Representation:  workers.RepresentationNotification,
	}
}

func isTerminalLifecycleRecord(record events.Record) bool {
	return record.SourceType == lifecycleSourceType &&
		record.SourceSequence >= terminalSourceSequence &&
		record.SourceEventID == terminalSourceEventID
}

// publishOpeningRecord commits the one opening KindSession/PhaseStarted
// workers.Draft onto workersessions.Topic(id), detached from any
// caller-owned backing array, before Start ever calls
// workers.WorkstationExecutionService.DispatchWorkstation. It runs under
// id's publication lock, the same lock PublishRecord and
// publishTerminalRecord use, and opens id's publication window only once
// the append itself has committed: no PublishRecord call can be accepted for
// id until this succeeds. A non-nil return means no record was committed and
// the window stays closed: Start must not proceed to Workers handoff.
func (r *registry) publishOpeningRecord(
	ctx context.Context,
	id,
	attemptID string,
	payload workers.SessionPayload,
	provider string,
	recordingsForSession ...recordings.WorkerSessionRecording,
) error {
	pub := r.publicationFor(id)
	pub.mu.Lock()
	recording := firstWorkerRecording(recordingsForSession)

	// SessionPayload contains only JSON value fields, so json.Marshal cannot
	// fail here; the error is intentionally discarded rather than defended
	// against.
	draftPayload, _ := json.Marshal(payload)
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseStarted,
		Provenance: lifecycleProvenance(provider),
		Payload:    draftPayload,
		DispatchID: attemptID,
		TurnID:     payload.TurnID,
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: openingSourceSequence,
		SourceEventID:  openingSourceEventID,
	}
	if _, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft); err != nil {
		openingErr := fmt.Errorf("%w: %v", recordings.ErrWorkerRecordingOpening, err)
		if recording != nil {
			if abortErr := recording.Abort(context.WithoutCancel(ctx), openingErr); abortErr != nil {
				r.logger.Info(
					"worker session recording opening cleanup failed",
					"sessionID", id,
					"attemptID", attemptID,
					"outcome", "cleanup_failed",
				)
			}
		}
		pub.mu.Unlock()
		return err
	}
	if recording != nil {
		if err := recording.AwaitOpening(ctx); err != nil {
			if abortErr := recording.Abort(context.WithoutCancel(ctx), err); abortErr != nil {
				r.logger.Info(
					"worker session recording opening cleanup failed",
					"sessionID", id,
					"attemptID", attemptID,
					"outcome", "cleanup_failed",
				)
			}
			pub.mu.Unlock()
			return err
		}
	}
	pub.open = true
	pub.recording = recording
	pub.provider = workers.CanonicalProviderSessionProvider(provider)
	pub.turnID = strings.TrimSpace(payload.TurnID)
	pub.lastSequence = make(map[sourceKey]events.SourceSequence)
	pub.accepted = make(map[events.AppendIdentity]struct{})
	pub.mu.Unlock()
	return nil
}

func firstWorkerRecording(recordingsForSession []recordings.WorkerSessionRecording) recordings.WorkerSessionRecording {
	if len(recordingsForSession) == 0 {
		return nil
	}
	return recordingsForSession[0]
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
		Provenance: lifecycleProvenance(""),
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
	if pub == nil {
		return workersessions.ErrSessionNotFound
	}
	pub.mu.Lock()
	if !pub.open {
		pub.mu.Unlock()
		return workersessions.ErrPublicationNotOpen
	}
	pub.open = false
	recording := pub.recording
	pub.recording = nil
	draft.Provenance = lifecycleProvenance(pub.provider)

	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: terminalSourceSequence,
		SourceEventID:  terminalSourceEventID,
	}
	_, err = r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	pub.mu.Unlock()
	if recording != nil {
		if closeErr := recording.Close(context.WithoutCancel(ctx)); err == nil {
			err = closeErr
		}
	}
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
