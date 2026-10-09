package service

import (
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

// Historical input is detached data. Read it outside the registry lock and
// select this host's execution services only when reserving a new successor.
func (r *registry) readArchivedContinuationSource(req workersessions.ContinueRequest) (*archivedContinuationSource, error) {
	r.mu.RLock()
	_, exists := r.sessions[r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)]
	_, replay := r.continueReplays[req.RequestID]
	r.mu.RUnlock()
	if exists || replay || r.logs == nil {
		return nil, nil
	}
	ctx := r.serverOwnedContext()
	page, err := r.logs.reader.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{
		WorkerSessionID: req.SourceWorkerSessionID, Limit: 1,
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, workersessions.ErrContinuationSourceNotFound
	}
	if err != nil || page.Catalog.WorkerSessionID != req.SourceWorkerSessionID ||
		page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	if req.FactorySessionID != "" && page.Catalog.FactorySessionID != req.FactorySessionID {
		return nil, workersessions.ErrContinuationSourceNotFound
	}
	target, err := archivedContinuationTarget(page, req.SourceWorkerSessionID)
	if err != nil {
		return nil, err
	}
	captured, err := r.restart.ReadWorkerContinuationSource(ctx, target)
	if err != nil || !directRestartRecipeSafe(captured.Execution) ||
		captured.Execution.Execution.Dispatch.DispatchID != target.ExpectedAttemptID ||
		captured.Execution.Execution.FactorySessionID != target.FactorySessionID || captured.Terminal != *page.Terminal {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	return archivedContinuationSnapshot(page, target, captured)
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
	source := workersessions.Session{
		ID: target.WorkerSessionID, State: workersessions.State(captured.Terminal.Status),
		SuccessorWorkerSessionID: page.SuccessorWorkerSessionID,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: target.WorkerSessionID, DispatchID: target.ExpectedAttemptID,
			AttemptID: target.ExpectedAttemptID, TurnID: captured.TurnID, Reference: captured.Reference,
		},
	}
	if !source.Terminal() || validateContinuationSourceAssociation(source) != nil {
		return nil, workersessions.ErrContinuationProviderSessionInvalid
	}
	// Only direct admission saves this recipe. Factory correlation in the
	// opening is retained scope, rather than evidence of Runtime ownership.
	return &archivedContinuationSource{target: target, snapshot: continuationSourceSnapshot{
		session: source, execution: captured.Execution, dispatchID: target.ExpectedAttemptID,
		turnID: captured.TurnID, direct: true, archived: true,
	}}, nil
}

// The caller holds r.mu. Recheck live reservations after the detached read;
// never install a historical supervision handle or grant source controls.
func (r *registry) continuationSnapshotLocked(req workersessions.ContinueRequest, archived *archivedContinuationSource) (continuationSourceSnapshot, error) {
	address := r.workerAddressLocked(req.SourceWorkerSessionID, req.FactorySessionID)
	if _, exists := r.sessions[address]; exists || archived == nil {
		return r.snapshotContinuationSourceLocked(req)
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
