package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type archivedContinuationSource struct {
	snapshot continuationSourceSnapshot
	target   recordings.WorkerControlTarget
}

// Capability is a detached read, never a reservation or restored supervision.
// Refusals preserve history while removing any claim of execution authority.
func (r *registry) withContinuationCapability(ctx context.Context, projected workersessions.Observation) workersessions.Observation {
	projected.Revivable = false
	projected.ContinuationHeadWorkerSessionID = ""
	req, head, err := r.continuationObservationHead(ctx, projected)
	if err != nil {
		return projected
	}
	projected.ContinuationHeadWorkerSessionID = head.ID
	if !head.Terminal() {
		return projected
	}

	snapshot, _, err := r.continuationCapabilitySnapshot(ctx, req)
	if err != nil || r.inspectContinuationReference(ctx, snapshot.session.ProviderSessionAssociation.Reference, snapshot.session.ID, snapshot.execution.Execution.FactorySessionID) != nil {
		return projected
	}
	// A peer read runs without the registry lock. Never describe a source that
	// acquired a successor or changed its exact association during that read.
	_, currentHead, err := r.continuationObservationHead(ctx, projected)
	if err != nil || currentHead.ID != head.ID || currentHead.State != head.State {
		projected.ContinuationHeadWorkerSessionID = ""
		return projected
	}
	current, _, err := r.continuationCapabilitySnapshot(ctx, req)
	if err != nil || current.dispatchID != snapshot.dispatchID ||
		*current.session.ProviderSessionAssociation != *snapshot.session.ProviderSessionAssociation {
		projected.ContinuationHeadWorkerSessionID = ""
		return projected
	}
	projected.Revivable = true
	return projected
}

func (r *registry) continuationCapabilitySnapshot(ctx context.Context, req workersessions.ContinueRequest) (continuationSourceSnapshot, *archivedContinuationSource, error) {
	captured, err := r.readContinuationRecipeContext(ctx, req, true)
	if err != nil {
		return continuationSourceSnapshot{}, nil, err
	}
	archived, err := r.readArchivedContinuationSourceContext(ctx, req, true)
	if err != nil {
		return continuationSourceSnapshot{}, nil, err
	}
	r.mu.Lock()
	snapshot, err := r.continuationSnapshotLocked(req, archived)
	snapshot.session = snapshot.session.Clone()
	stopping := r.stopping
	r.mu.Unlock()
	if err != nil {
		return snapshot, archived, err
	}
	if stopping {
		return snapshot, archived, workersessions.ErrContinuationExecutionUnavailable
	}
	if captured != nil {
		if captured.Execution.Dispatch.DispatchID != snapshot.dispatchID {
			return snapshot, archived, workersessions.ErrContinuationExecutionUnavailable
		}
		snapshot.execution = *captured
	}
	// Validate the same recipe as admission without reserving a successor.
	if err := (workersessions.InvokeSessionRequest{ID: snapshot.session.ID, Execution: snapshot.execution}).Validate(); err != nil {
		return snapshot, archived, err
	}
	return snapshot, archived, r.validateContinuationSupport(ctx, snapshot.session.ProviderSessionAssociation.Reference)
}

func (r *registry) continuationObservationHead(ctx context.Context, projected workersessions.Observation) (workersessions.ContinueRequest, workersessions.Session, error) {
	req := workersessions.ContinueRequest{SourceWorkerSessionID: projected.WorkerSessionID, FactorySessionID: projected.FactorySessionID}
	seen := make(map[string]bool)
	previous := ""
	for {
		if err := observationContextError(ctx); err != nil {
			return req, workersessions.Session{}, err
		}
		if seen[req.SourceWorkerSessionID] {
			return req, workersessions.Session{}, workersessions.ErrContinuationExecutionUnavailable
		}
		seen[req.SourceWorkerSessionID] = true
		source, scope, err := r.continuationObservationSource(ctx, req)
		if err != nil || !source.State.Valid() || (previous != "" && source.PredecessorWorkerSessionID != previous) ||
			(!source.Terminal() && source.SuccessorWorkerSessionID != "") {
			return req, workersessions.Session{}, workersessions.ErrContinuationExecutionUnavailable
		}
		if source.SuccessorWorkerSessionID == "" {
			return req, source, nil
		}
		previous = source.ID
		req.SourceWorkerSessionID, req.FactorySessionID = source.SuccessorWorkerSessionID, scope
	}
}

func (r *registry) continuationObservationSource(ctx context.Context, req workersessions.ContinueRequest) (workersessions.Session, string, error) {
	r.mu.RLock()
	address, err := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	source, exists := r.sessions[address]
	source = source.Clone()
	scope := req.FactorySessionID
	if metadata := r.observations[address]; metadata != nil {
		scope = metadata.factorySessionID
	}
	r.mu.RUnlock()
	if err != nil || exists {
		return source, scope, err
	}
	if r.logs == nil {
		return workersessions.Session{}, "", workersessions.ErrContinuationSourceNotFound
	}
	row, err := r.logs.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: req.SourceWorkerSessionID, FactorySessionID: req.FactorySessionID,
	})
	if err != nil || row.RecordingHealth != recordings.WorkerRecordingStatusComplete ||
		row.ConfirmationState != workersessions.ConfirmationStateConfirmed {
		return workersessions.Session{}, "", workersessions.ErrContinuationExecutionUnavailable
	}
	source = workersessions.Session{ID: row.WorkerSessionID, State: row.State,
		PredecessorWorkerSessionID: row.PredecessorWorkerSessionID, SuccessorWorkerSessionID: row.SuccessorWorkerSessionID}
	if row.ProviderSessionAvailable {
		source.ProviderSessionAssociation = &workersessions.ProviderSessionAssociation{WorkerSessionID: row.WorkerSessionID,
			DispatchID: row.AttemptID, AttemptID: row.AttemptID, TurnID: row.TurnID, Reference: row.ProviderSession}
	}
	return source, row.FactorySessionID, nil
}

// Historical input is detached data. Read it outside the registry lock and
// select this host's execution services only when reserving a new successor.
func (r *registry) readArchivedContinuationSource(req workersessions.ContinueRequest) (*archivedContinuationSource, error) {
	return r.readArchivedContinuationSourceContext(r.serverOwnedContext(), req)
}

func (r *registry) continuationNeedsCapturedSource(req workersessions.ContinueRequest) bool {
	r.mu.RLock()
	address := r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	source, exists := r.sessions[address]
	liveFactory := exists && source.Terminal() && r.supervisions[address] == nil
	_, replay := r.continueReplays[req.RequestID]
	r.mu.RUnlock()
	return (!exists || liveFactory) && !replay && r.logs != nil
}

func (r *registry) readArchivedContinuationSourceContext(ctx context.Context, req workersessions.ContinueRequest, observationOnly ...bool) (*archivedContinuationSource, error) {
	if !r.continuationNeedsCapturedSource(req) {
		return nil, nil
	}
	r.mu.RLock()
	address := r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	_, published := r.continuationPublicationLocked(address)
	r.mu.RUnlock()
	if err := waitContinuationPublication(ctx, published); err != nil {
		return nil, err
	}
	reader, supported := r.logs.reader.(recordings.WorkerCapturedSummaryReader)
	if !supported {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, req.SourceWorkerSessionID)
	page, err := archivedContinuationPage(req, summary, err)
	if err != nil {
		return nil, err
	}
	target, err := archivedContinuationTarget(page, req.SourceWorkerSessionID)
	if err != nil {
		return nil, err
	}
	if r.restart == nil {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	captured, err := r.lookupContinuationSource(ctx, target, observationOnly...)
	if err != nil || !directRestartRecipeSafe(captured.Execution) ||
		captured.Execution.Execution.Dispatch.DispatchID != target.ExpectedAttemptID ||
		captured.Execution.Execution.FactorySessionID != target.FactorySessionID || captured.Terminal != *page.Terminal {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	return archivedContinuationSnapshot(page, target, captured)
}

func archivedContinuationPage(req workersessions.ContinueRequest, summary recordings.WorkerCapturedSummary, lookupErr error) (recordings.WorkerCapturedActivityPage, error) {
	capture := summary.Capture
	page := recordings.WorkerCapturedActivityPage{Catalog: capture.Catalog, Opening: capture.Opening, Terminal: capture.Terminal,
		Health: capture.Health, SuccessorWorkerSessionID: capture.SuccessorWorkerSessionID}
	if errors.Is(lookupErr, os.ErrNotExist) {
		return page, workersessions.ErrContinuationSourceNotFound
	}
	if lookupErr != nil || page.Catalog.WorkerSessionID != req.SourceWorkerSessionID || page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil {
		return page, workersessions.ErrContinuationExecutionUnavailable
	}
	if req.FactorySessionID != "" && page.Catalog.FactorySessionID != req.FactorySessionID {
		return page, workersessions.ErrContinuationSourceNotFound
	}
	return page, nil
}

func archivedContinuationTarget(page recordings.WorkerCapturedActivityPage, sourceID string) (recordings.WorkerControlTarget, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || opening.WorkerSessionID != sourceID ||
		opening.FactorySessionID != page.Catalog.FactorySessionID || opening.AttemptID == "" {
		return recordings.WorkerControlTarget{}, workersessions.ErrContinuationExecutionUnavailable
	}
	target := recordings.WorkerControlTarget{
		RecordingID: page.Catalog.RecordingID, WorkerSessionID: page.Catalog.WorkerSessionID,
		FactorySessionID: page.Catalog.FactorySessionID, RecordingGenerationID: page.Catalog.RecordingGenerationID,
		OwnerEpoch: page.Catalog.OwnerEpoch, ExpectedAttemptID: opening.AttemptID,
	}
	return target, nil
}

func archivedContinuationSnapshot(page recordings.WorkerCapturedActivityPage, target recordings.WorkerControlTarget, captured recordings.WorkerContinuationSource) (*archivedContinuationSource, error) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(page.Opening.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil || opening.ValidateLineage() != nil {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	source := workersessions.Session{
		ID: target.WorkerSessionID, State: workersessions.State(captured.Terminal.Status),
		SuccessorWorkerSessionID: page.SuccessorWorkerSessionID,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: target.WorkerSessionID, DispatchID: target.ExpectedAttemptID,
			AttemptID: target.ExpectedAttemptID, TurnID: captured.TurnID, Reference: captured.Reference,
		},
	}
	if opening.Lineage != nil {
		source.PredecessorWorkerSessionID = opening.Lineage.PredecessorWorkerSessionID
	}
	if !source.Terminal() || validateContinuationSourceAssociation(source) != nil {
		return nil, workersessions.ErrContinuationProviderSessionInvalid
	}
	// Historical Factory correlation is retained scope, never restored Runtime
	// ownership. Every revived source is admitted by the direct Workers path.
	return &archivedContinuationSource{target: target, snapshot: continuationSourceSnapshot{
		session: source, execution: captured.Execution, dispatchID: target.ExpectedAttemptID,
		turnID: captured.TurnID, direct: true, archived: true,
	}}, nil
}

// Resolve only committed forward links with matching reverse lineage in the
// same owner scope. The subsequent reservation rechecks the selected source;
// this read never acquires execution or cancellation authority.
func (r *registry) resolveContinuationHead(req workersessions.ContinueRequest) (workersessions.ContinueRequest, *continueReplay, error) {
	addressed := req
	seen := make(map[string]bool)
	previous := ""
	for {
		if seen[req.SourceWorkerSessionID] {
			return req, nil, workersessions.ErrContinuationExecutionUnavailable
		}
		seen[req.SourceWorkerSessionID] = true
		source, scope, archived, err := r.continuationHeadSource(req)
		if err != nil {
			return req, nil, err
		}
		if previous != "" && source.PredecessorWorkerSessionID != previous {
			return req, nil, workersessions.ErrContinuationExecutionUnavailable
		}
		if archived != nil {
			if replay, err := r.readTerminalContinuationReplay(addressed, archived); replay != nil || err != nil {
				return req, replay, err
			}
		}
		if source.SuccessorWorkerSessionID == "" {
			return req, nil, nil
		}
		previous = source.ID
		req.SourceWorkerSessionID = source.SuccessorWorkerSessionID
		req.FactorySessionID = scope
	}
}

func (r *registry) continuationHeadSource(req workersessions.ContinueRequest) (workersessions.Session, string, *archivedContinuationSource, error) {
	r.mu.RLock()
	address, err := r.resolveWorkerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	source, exists := r.sessions[address]
	scope := req.FactorySessionID
	if metadata := r.observations[address]; metadata != nil {
		scope = metadata.factorySessionID
	}
	r.mu.RUnlock()
	if err != nil {
		return workersessions.Session{}, "", nil, err
	}
	if exists {
		if !source.Terminal() {
			return workersessions.Session{}, "", nil, workersessions.ErrContinuationSourceActive
		}
		return source, scope, nil, nil
	}
	archived, err := r.readArchivedContinuationSource(req)
	if err != nil {
		return workersessions.Session{}, "", nil, err
	}
	if archived == nil {
		return workersessions.Session{}, "", nil, workersessions.ErrContinuationSourceNotFound
	}
	return archived.snapshot.session, archived.target.FactorySessionID, archived, nil
}

// The caller holds r.mu. Recheck live reservations after the detached read;
// never install a historical supervision handle or grant source controls.
func (r *registry) continuationSnapshotLocked(req workersessions.ContinueRequest, archived *archivedContinuationSource) (continuationSourceSnapshot, error) {
	address := r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	if _, exists := r.sessions[address]; (exists && r.supervisions[address] != nil) || archived == nil {
		return r.snapshotContinuationSourceLocked(req)
	}
	if source, exists := r.sessions[address]; exists {
		if !source.Terminal() {
			return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceActive
		}
		if source.SuccessorWorkerSessionID != "" {
			return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
		}
		if validateContinuationSourceAssociation(source) != nil || source.State != archived.snapshot.session.State ||
			*source.ProviderSessionAssociation != *archived.snapshot.session.ProviderSessionAssociation {
			return continuationSourceSnapshot{}, workersessions.ErrContinuationProviderSessionInvalid
		}
		if attempt := r.runtimeAttemptControls[address]; attempt != nil {
			attempt.mu.Lock()
			unsafe := attempt.controlPending || attempt.forceJournalPending != 0 || attempt.controlPersistenceLost
			attempt.mu.Unlock()
			if unsafe {
				return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
			}
		}
	}
	address = firstNonEmpty(address, req.SourceWorkerSessionID)
	if archived.snapshot.session.SuccessorWorkerSessionID != "" ||
		(r.continuationSources[address] != "" && r.continuationSources[address] != req.RequestID) {
		return continuationSourceSnapshot{}, workersessions.ErrContinuationSourceConflict
	}
	snapshot := archived.snapshot
	snapshot.address = firstNonEmpty(address, req.SourceWorkerSessionID)
	snapshot.executor, snapshot.clock, snapshot.scheduler = r.execution, r.clock, r.scheduler
	return snapshot, nil
}
