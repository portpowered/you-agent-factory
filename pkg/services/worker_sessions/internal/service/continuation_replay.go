package service

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Recovery returns detached data only. Neither a saved input nor an opening
// authorizes another execution; the committed source admission link and exact
// completed successor must both agree with the immutable request.
func (r *registry) readCompletedContinuationReplay(req workersessions.ContinueRequest, source *archivedContinuationSource) (*continueReplay, error) {
	target := source.target
	ctx := r.serverOwnedContext()
	stored, err := r.restart.ReadWorkerContinuationInput(ctx, recordings.WorkerControlOperationKey{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
		FactorySessionID: target.FactorySessionID, RequestID: "continue/" + req.RequestID,
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, recordings.ErrWorkerRecordingPersistence
	}
	input, err := decodeContinuationInput(stored, req, target)
	if err != nil {
		return nil, err
	}
	if source.snapshot.session.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	reference := source.snapshot.session.ProviderSessionAssociation.Reference
	if input.ProviderReference != (interruptInputReference{Provider: reference.Provider, Kind: reference.Kind, ID: reference.ID}) {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	page, err := r.logs.reader.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{
		WorkerSessionID: req.SuccessorWorkerSessionID, Limit: 1,
	})
	if err != nil || !completedContinuationMatches(page, input) {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	session := workersessions.Session{
		ID: req.SuccessorWorkerSessionID, State: workersessions.StateCompleted,
		Result:                     &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted},
		PredecessorWorkerSessionID: req.SourceWorkerSessionID,
		SuccessorWorkerSessionID:   page.SuccessorWorkerSessionID,
		ProviderSessionAssociation: continuationAssociation(req, input.Execution, source.snapshot.turnID, reference),
	}
	replay := &continueReplay{
		tuple: continueTuple{sourceID: req.SourceWorkerSessionID, successorID: req.SuccessorWorkerSessionID, input: req.FollowUpInput},
		plan:  continuePlan{request: req}, done: make(chan struct{}),
		result: workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: session},
	}
	close(replay.done)
	return replay, nil
}

func completedContinuationMatches(page recordings.WorkerCapturedActivityPage, input durableContinuationInput) bool {
	if page.Catalog.WorkerSessionID != input.SuccessorWorkerSessionID || page.Catalog.FactorySessionID != input.Target.FactorySessionID ||
		page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil || page.Terminal.Status != string(workersessions.StateCompleted) {
		return false
	}
	opening, ok := completedContinuationOpening(page.Opening.Payload)
	if !ok {
		return false
	}
	execution := input.Execution.Execution
	return completedContinuationIdentityMatches(opening, input) &&
		opening.WorkingDirectory == execution.WorkingDirectory && opening.Model == execution.Model && opening.ReasoningEffort == execution.ReasoningEffort
}

func completedContinuationOpening(payload json.RawMessage) (workers.SessionPayload, bool) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		json.Unmarshal(draft.Payload, &opening) != nil || opening.ValidateLineage() != nil || opening.Lineage == nil || opening.Continuation == nil {
		return workers.SessionPayload{}, false
	}
	return opening, true
}

func completedContinuationIdentityMatches(opening workers.SessionPayload, input durableContinuationInput) bool {
	execution := input.Execution.Execution
	return opening.WorkerSessionID == input.SuccessorWorkerSessionID && opening.FactorySessionID == input.Target.FactorySessionID &&
		opening.DispatchID == execution.Dispatch.DispatchID && opening.AttemptID == execution.Dispatch.DispatchID &&
		opening.Lineage.PredecessorWorkerSessionID == input.Target.WorkerSessionID &&
		opening.Lineage.PreviousAttemptID == input.Target.ExpectedAttemptID && opening.Lineage.PreviousDispatchID == input.Target.ExpectedAttemptID &&
		opening.Continuation.Provider == string(input.ProviderReference.Provider) && opening.Continuation.Kind == input.ProviderReference.Kind &&
		opening.Continuation.ID == input.ProviderReference.ID
}
