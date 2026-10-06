package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ReadWorkerContinuationSource requires a committed terminal for the exact
// recipe attempt. Incomplete history and missing references never authorize
// a native continuation, including after this writer is reopened.
func (writer *FileWriter) ReadWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	execution, err := writer.ReadWorkerRestartRecipe(ctx, target)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	snapshot, err := writer.LoadWorkerRecording(ctx, target.RecordingID)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	for _, session := range snapshot.Sessions {
		if session.WorkerSessionID != target.WorkerSessionID {
			continue
		}
		return capturedContinuationSource(session, target, execution)
	}
	return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
}

func capturedContinuationSource(session recordings.WorkerSessionRecordingSnapshot, target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) (recordings.WorkerContinuationSource, error) {
	if session.RecordingGenerationID != target.RecordingGenerationID || session.OwnerEpoch != target.OwnerEpoch ||
		session.Status != recordings.WorkerRecordingStatusComplete || session.ExecutionTerminal == nil {
		return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingIncomplete
	}
	for _, record := range session.Records {
		if record.ID.Position != session.ExecutionTerminal.Position {
			continue
		}
		var draft workers.Draft
		var payload workers.SessionPayload
		if json.Unmarshal(record.Payload, &draft) != nil || draft.Kind != workers.KindSession ||
			draft.DispatchID != target.ExpectedAttemptID || json.Unmarshal(draft.Payload, &payload) != nil ||
			payload.Status != session.ExecutionTerminal.Status || payload.Continuation == nil {
			return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
		}
		reference := providers.SessionRef{Provider: providers.ID(payload.Continuation.Provider), Kind: payload.Continuation.Kind, ID: payload.Continuation.ID}
		if reference.Validate() != nil {
			return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
		}
		return recordings.WorkerContinuationSource{Execution: execution, Reference: reference, Terminal: *session.ExecutionTerminal, TurnID: draft.TurnID}, nil
	}
	return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
}

// ReadWorkerRestartRecipe resolves the immutable input using the selected
// store's identity. The captured epoch is checked as data, never upgraded to
// this host's epoch or used to restore execution authority.
func (writer *FileWriter) ReadWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	entry := writer.entry(target.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	key := recordings.WorkerControlOperationKey{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
		FactorySessionID: target.FactorySessionID, RequestID: "restart-recipe/" + target.ExpectedAttemptID,
	}
	identity, err := writer.controlInputIdentity(ctx, entry, key)
	if err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	session := entry.sessions[target.WorkerSessionID]
	if target.ExpectedAttemptID == "" || session.generation != target.RecordingGenerationID || session.ownerEpoch != target.OwnerEpoch {
		return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerControlConflict
	}
	path := writer.controlInputPath(controlInputRef(identity))
	if err := writer.checkControlInputPath(path); err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	data, err := writer.storage.ReadFile(path)
	if err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	input, err := decodeControlInput(data, identity)
	if err != nil {
		return workers.WorkstationDispatchRequest{}, err
	}
	var recipe workerRestartRecipe
	if json.Unmarshal(input, &recipe) != nil || recipe.Version != 1 || recipe.Target != target {
		return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerRecordingReplay
	}
	canonical, err := encodeWorkerRestartRecipe(target, recipe.Execution)
	// Canonical bytes also reject unknown fields and data excluded by the
	// detached execution contract rather than silently discarding them.
	if err != nil || !bytes.Equal(canonical, input) {
		return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerRecordingReplay
	}
	return recipe.Execution, nil
}

type workerRestartRecipe struct {
	Version   int                                `json:"version"`
	Target    recordings.WorkerControlTarget     `json:"target"`
	Execution workers.WorkstationDispatchRequest `json:"execution"`
}

// ValidateWorkerRestartRecipe checks the same serializer used by admission,
// without opening capture or persisting input. New capture generations are
// SHA-256 hex strings; their contents do not change the serialized size.
func (writer *FileWriter) ValidateWorkerRestartRecipe(ctx context.Context, workerID string, execution workers.WorkstationDispatchRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := recordings.WorkerControlTarget{
		RecordingID: execution.Execution.RecordingID, WorkerSessionID: workerID,
		FactorySessionID:      execution.Execution.FactorySessionID,
		RecordingGenerationID: strings.Repeat("0", 64), OwnerEpoch: writer.captureOwnerEpoch(),
		ExpectedAttemptID: execution.Execution.Dispatch.DispatchID,
	}
	_, err := encodeWorkerRestartRecipe(target, execution)
	return err
}

// SaveWorkerRestartRecipe shares the journal's sync acknowledgement and keeps
// capture validation locked through persistence. No caller-selected path or
// independent ledger is introduced. The attempt is part of the immutable key.
func (writer *FileWriter) SaveWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) error {
	input, err := encodeWorkerRestartRecipe(target, execution)
	if err != nil {
		return err
	}
	entry := writer.entry(target.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	key := recordings.WorkerControlOperationKey{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
		FactorySessionID: target.FactorySessionID, RequestID: "restart-recipe/" + target.ExpectedAttemptID,
	}
	if _, err := writer.controlInputIdentity(ctx, entry, key); err != nil {
		return err
	}
	session := entry.sessions[target.WorkerSessionID]
	if session.generation != target.RecordingGenerationID || session.ownerEpoch != target.OwnerEpoch {
		return recordings.ErrWorkerControlConflict
	}
	_, err = writer.persistWorkerControlInputLocked(ctx, entry, key, input)
	return err
}

func encodeWorkerRestartRecipe(target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) ([]byte, error) {
	request := execution.Execution
	redaction := request.PromptRedaction
	if len(request.EnvVars) != 0 || request.WorkflowContext != nil ||
		(redaction != nil && (redaction.FailClosed || redaction.RedactSystemPrompt || redaction.RedactUserMessage)) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	if !restartRecipeIdentityMatches(target, execution) {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	// Empty input is a JSON array in the versioned recipe contract. Assigning
	// the value copy leaves the caller's dispatch unchanged.
	if execution.Execution.Dispatch.InputTokens == nil {
		execution.Execution.Dispatch.InputTokens = []any{}
	}
	input, err := json.Marshal(workerRestartRecipe{Version: 1, Target: target, Execution: execution})
	if err != nil {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	if len(input) > controlInputLimit || !restartRecipeCredentialsSafe(input, request.ProcessEnvironment) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	return input, nil
}

func restartRecipeIdentityMatches(target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest) bool {
	request := execution.Execution
	return execution.WorkstationName != "" && request.Dispatch.WorkstationName == execution.WorkstationName &&
		target.ExpectedAttemptID != "" && request.Dispatch.DispatchID == target.ExpectedAttemptID &&
		request.FactorySessionID == target.FactorySessionID
}

// Inspect decoded values so JSON escaping cannot conceal an inherited secret
// copied into an argument, prompt or input token. The environment itself is
// excluded by the detached request's JSON contract.
func restartRecipeCredentialsSafe(input []byte, environment []string) bool {
	var document any
	if json.Unmarshal(input, &document) != nil {
		return false
	}
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if !found || value == "" {
			continue
		}
		projection := workers.ProjectCommandEnvForDiagnostics([]string{name + "="})
		if projection.Values[name] == workers.RedactedCommandEnvValue && restartRecipeContains(document, value) {
			return false
		}
	}
	return true
}

func restartRecipeContains(document any, secret string) bool {
	switch value := document.(type) {
	case string:
		return strings.Contains(value, secret)
	case []any:
		for _, child := range value {
			if restartRecipeContains(child, secret) {
				return true
			}
		}
	case map[string]any:
		for key, child := range value {
			if strings.Contains(key, secret) || restartRecipeContains(child, secret) {
				return true
			}
		}
	}
	return false
}
