package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The immutable tuple precedes successor opening and provider admission.
// Its existence alone never proves admission or authorizes a retry.
type durableContinuationInput struct {
	Version                  int                                `json:"version"`
	Target                   recordings.WorkerControlTarget     `json:"target"`
	RequestID                string                             `json:"requestId"`
	SuccessorWorkerSessionID string                             `json:"successorWorkerSessionId"`
	FollowUpInput            string                             `json:"followUpInput"`
	ProviderReference        interruptInputReference            `json:"providerReference"`
	Execution                workers.WorkstationDispatchRequest `json:"execution"`
}

func (r *registry) persistContinuationInput(plan continuePlan) error {
	if !plan.direct || r.logs == nil {
		return nil
	}
	pub := r.publicationFor(plan.request.SourceWorkerSessionID)
	if pub == nil || plan.lineage == nil {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	pub.mu.Lock()
	capture := pub.capture
	pub.mu.Unlock()
	ctx := r.serverOwnedContext()
	catalog, err := r.logs.reader.LookupWorkerSessionCapture(ctx, plan.request.SourceWorkerSessionID)
	if err != nil || catalog.WorkerSessionID != plan.request.SourceWorkerSessionID ||
		catalog.FactorySessionID != plan.execution.Execution.FactorySessionID {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	target := recordings.WorkerControlTarget{
		RecordingID: catalog.RecordingID, WorkerSessionID: catalog.WorkerSessionID,
		FactorySessionID: catalog.FactorySessionID, RecordingGenerationID: catalog.RecordingGenerationID,
		OwnerEpoch: catalog.OwnerEpoch, ExpectedAttemptID: plan.lineage.PreviousAttemptID,
	}
	capture.ExpectedAttemptID = plan.lineage.PreviousAttemptID
	if target != capture {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	input, err := encodeContinuationInput(plan, target)
	if err != nil {
		return workersessions.ErrContinuationExecutionUnavailable
	}
	key := recordings.WorkerControlOperationKey{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
		FactorySessionID: target.FactorySessionID, RequestID: "continue/" + plan.request.RequestID,
	}
	_, err = r.operations.PersistWorkerControlInput(context.WithoutCancel(ctx), key, input)
	if errors.Is(err, recordings.ErrWorkerControlConflict) {
		return workersessions.ErrContinuationRequestIDConflict
	}
	if err != nil {
		r.logger.Info("worker session continuation input unavailable", "sourceWorkerSessionID", target.WorkerSessionID,
			"requestID", plan.request.RequestID, "outcome", "persistence_failed")
		return recordings.ErrWorkerRecordingPersistence
	}
	stored, err := r.restart.ReadWorkerContinuationInput(ctx, key)
	if err != nil || !bytes.Equal(stored, input) {
		r.logger.Info("worker session continuation input unavailable", "sourceWorkerSessionID", target.WorkerSessionID,
			"requestID", plan.request.RequestID, "outcome", "read_failed")
		return recordings.ErrWorkerRecordingPersistence
	}
	return nil
}

func encodeContinuationInput(plan continuePlan, target recordings.WorkerControlTarget) ([]byte, error) {
	if plan.request.Validate() != nil || target.ExpectedAttemptID == "" || plan.execution.Execution.Continuation == nil ||
		plan.execution.Execution.UserMessage != plan.request.FollowUpInput || !directRestartRecipeSafe(plan.execution) {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	reference := plan.execution.Execution.Continuation
	execution := cloneWorkstationDispatchRequest(plan.execution)
	if execution.Execution.Dispatch.InputTokens == nil {
		execution.Execution.Dispatch.InputTokens = []any{}
	}
	input := durableContinuationInput{
		Version: 1, Target: target, RequestID: plan.request.RequestID,
		SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID, FollowUpInput: plan.request.FollowUpInput,
		ProviderReference: interruptInputReference{Provider: providers.ID(reference.Provider), Kind: reference.Kind, ID: reference.ProviderSessionID},
		Execution:         execution,
	}
	payload, err := json.Marshal(input)
	if err != nil || !interruptRecipeSafe(payload, execution.Execution.ProcessEnvironment) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	return payload, nil
}
