package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	appendResult, err := r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	pub.mu.Unlock()
	if recording != nil {
		closeErr := r.closeWorkerRecording(
			context.WithoutCancel(ctx),
			recording,
			state,
			appendResult.Record.ID.Position,
		)
		if err == nil {
			err = closeErr
		} else if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	return err
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
