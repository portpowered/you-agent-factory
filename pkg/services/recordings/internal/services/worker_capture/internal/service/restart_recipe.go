package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type workerRestartRecipe struct {
	Version   int                                `json:"version"`
	Target    recordings.WorkerControlTarget     `json:"target"`
	Execution workers.WorkstationDispatchRequest `json:"execution"`
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
	if execution.WorkstationName == "" || request.Dispatch.WorkstationName != execution.WorkstationName || target.ExpectedAttemptID == "" || request.Dispatch.DispatchID != target.ExpectedAttemptID {
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
