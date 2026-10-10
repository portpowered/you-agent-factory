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

// Recovery returns detached data only. Neither a saved input nor an opening
// authorizes another execution; the committed source lineage and exact terminal
// successor must both agree with the immutable request. The opening-derived
// lineage alone cannot prove Workers admission.
func (r *registry) readTerminalContinuationReplay(req workersessions.ContinueRequest, source *archivedContinuationSource) (*continueReplay, error) {
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
	result, err := r.readContinuationTerminalResult(ctx, page, input)
	if err != nil {
		return nil, err
	}
	addressed := req
	req.SourceWorkerSessionID = target.WorkerSessionID
	session := workersessions.Session{
		ID: req.SuccessorWorkerSessionID, State: workersessions.State(page.Terminal.Status),
		Result:                     result,
		PredecessorWorkerSessionID: req.SourceWorkerSessionID,
		SuccessorWorkerSessionID:   page.SuccessorWorkerSessionID,
		ProviderSessionAssociation: continuationAssociation(req, input.Execution, source.snapshot.turnID, reference),
	}
	replay := &continueReplay{
		tuple: continueTuple{sourceID: addressed.SourceWorkerSessionID, successorID: req.SuccessorWorkerSessionID, input: req.FollowUpInput, resolveHead: addressed.ResolveHead},
		plan:  continuePlan{request: req}, done: make(chan struct{}),
		result: workersessions.ContinueResult{RequestID: req.RequestID, SourceWorkerSessionID: req.SourceWorkerSessionID,
			SuccessorWorkerSessionID: req.SuccessorWorkerSessionID, Session: session},
	}
	close(replay.done)
	return replay, nil
}

func completedContinuationMatches(page recordings.WorkerCapturedActivityPage, input durableContinuationInput) bool {
	if page.Catalog.WorkerSessionID != input.SuccessorWorkerSessionID || page.Catalog.FactorySessionID != input.Target.FactorySessionID ||
		page.Health != recordings.WorkerRecordingStatusComplete || page.Terminal == nil || !workersessions.State(page.Terminal.Status).Terminal() {
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

// Drain the committed capture so replay preserves the original failure rather
// than synthesizing success from admission lineage. This creates no live owner.
func (r *registry) readContinuationTerminalResult(ctx context.Context, page recordings.WorkerCapturedActivityPage, input durableContinuationInput) (*workersessions.TerminalResult, error) {
	stream := &continuationCaptureStream{reader: r.logs.reader,
		request: recordings.WorkerCapturedActivityRequest{WorkerSessionID: input.SuccessorWorkerSessionID, Limit: 1},
		page:    page, catalog: page.Catalog, terminal: uint64(page.Terminal.Position), state: page.Terminal.Status,
		head: uint64(page.Terminal.Position), health: recordings.WorkerRecordingStatusComplete, topic: page.Opening.ID.Topic}
	defer stream.Close()
	for {
		delivery := stream.Next(ctx)
		if delivery.Kind == workersessions.ObservationDeliveryRecord {
			continue
		}
		if delivery.Kind != workersessions.ObservationDeliveryTerminalReplay || delivery.Event.Position != uint64(page.Terminal.Position) {
			return nil, workersessions.ErrContinuationExecutionUnavailable
		}
		return continuationTerminalResult(delivery.Event.Payload, page.Terminal.Status, input.Execution.Execution.Dispatch.DispatchID)
	}
}

func continuationTerminalResult(payload json.RawMessage, status, attemptID string) (*workersessions.TerminalResult, error) {
	var draft workers.Draft
	var terminal terminalSessionPayload
	state := workersessions.State(status)
	phase, err := terminalPhase(state)
	if err != nil || readPendingInterruptOpeningJSON(payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != phase ||
		draft.DispatchID != attemptID || readPendingInterruptOpeningJSON(draft.Payload, &terminal) != nil || terminal.Status != status {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	if state == workersessions.StateCanceled || state == workersessions.StateTerminated {
		return nil, nil
	}
	result := &workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}
	if state == workersessions.StateFailed {
		// Opening/recipe failures terminalize a reserved successor before Workers
		// admission, yet can leave a complete capture and reverse opening link.
		// Publication failure therefore cannot prove accepted admission on replay.
		if terminal.FailureCause == string(workersessions.FailureCauseEventPublicationFailure) {
			return nil, workersessions.ErrContinuationExecutionUnavailable
		}
		result.Outcome = workersessions.TerminalOutcomeFailed
		result.Cause = &workersessions.FailureCause{Kind: workersessions.FailureCauseKind(terminal.FailureCause), Detail: terminal.FailureDetail, AgentRunFailureClass: terminal.AgentRunFailureClass}
	}
	if result.Validate() != nil {
		return nil, workersessions.ErrContinuationExecutionUnavailable
	}
	return result, nil
}
