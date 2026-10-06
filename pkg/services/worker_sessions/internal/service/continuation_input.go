package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

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
	if err == nil {
		_, err = decodeContinuationInput(stored, plan.request, target)
	}
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
	if _, err := decodeContinuationInput(payload, plan.request, target); err != nil {
		return nil, err
	}
	return payload, nil
}

// A retained tuple must describe one exact source and successor execution.
// Decoding proves input integrity only; successor admission needs separate
// committed capture evidence before recovery may return an accepted result.
func decodeContinuationInput(payload []byte, req workersessions.ContinueRequest, target recordings.WorkerControlTarget) (durableContinuationInput, error) {
	var input durableContinuationInput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if !uniqueInterruptJSONFields(payload) || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF ||
		!canonicalInterruptJSONFields(payload, input) {
		return durableContinuationInput{}, recordings.ErrWorkerRecordingPersistence
	}
	if !validContinuationInput(input) || input.Target != target {
		return durableContinuationInput{}, recordings.ErrWorkerRecordingPersistence
	}
	accepted := workersessions.ContinueRequest{
		RequestID: input.RequestID, SourceWorkerSessionID: input.Target.WorkerSessionID,
		SuccessorWorkerSessionID: input.SuccessorWorkerSessionID, FollowUpInput: input.FollowUpInput,
	}
	if req.Validate() != nil || accepted != req.Normalize() {
		return durableContinuationInput{}, workersessions.ErrContinuationRequestIDConflict
	}
	// Workers deliberately excludes Continuation from execution JSON. Restore
	// only the separately captured opaque reference, never a serialized handle.
	reference := providers.SessionRef{Provider: input.ProviderReference.Provider, Kind: input.ProviderReference.Kind, ID: input.ProviderReference.ID}
	continued := reference.ContinuationRef()
	input.Execution.Execution.Continuation = &continued
	return input, nil
}

func validContinuationInput(input durableContinuationInput) bool {
	req := workersessions.ContinueRequest{
		RequestID: input.RequestID, SourceWorkerSessionID: input.Target.WorkerSessionID,
		SuccessorWorkerSessionID: input.SuccessorWorkerSessionID, FollowUpInput: input.FollowUpInput,
	}
	target := input.Target
	reference := providers.SessionRef{Provider: input.ProviderReference.Provider, Kind: input.ProviderReference.Kind, ID: input.ProviderReference.ID}
	execution := input.Execution.Execution
	if input.Version != 1 || req.Validate() != nil || req != req.Normalize() || reference.Validate() != nil ||
		strings.TrimSpace(target.RecordingID) == "" || strings.TrimSpace(target.RecordingGenerationID) == "" ||
		strings.TrimSpace(target.OwnerEpoch) == "" || strings.TrimSpace(target.ExpectedAttemptID) == "" {
		return false
	}
	return continuationInputExecutionMatches(input, execution) && directRestartRecipeSafe(input.Execution) &&
		(workersessions.InvokeSessionRequest{ID: input.SuccessorWorkerSessionID, Execution: input.Execution}).Validate() == nil
}

func continuationInputExecutionMatches(input durableContinuationInput, execution workers.WorkstationExecutionRequest) bool {
	return execution.FactorySessionID == input.Target.FactorySessionID && execution.Dispatch.InputTokens != nil &&
		execution.Dispatch.DispatchID == continuationDispatchID(input.Target.ExpectedAttemptID, input.SuccessorWorkerSessionID) &&
		execution.UserMessage == input.FollowUpInput
}
