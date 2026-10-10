package service

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// LookupPreparedWorkerContinuationSource requires a committed terminal for the exact
// recipe attempt. Incomplete history and missing references never authorize
// a native continuation, including after this writer is reopened.
func (writer *FileWriter) LookupPreparedWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	summary, err := writer.LookupWorkerSessionSummary(ctx, target.WorkerSessionID)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	capture := summary.Capture
	if capture.Catalog.RecordingID != target.RecordingID || capture.Catalog.FactorySessionID != target.FactorySessionID {
		return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
	}
	recipe, err := writer.readWorkerRestartRecipe(ctx, target, true)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	source, err := capturedContinuationSource(recordings.WorkerSessionRecordingSnapshot{
		RecordingGenerationID: capture.Catalog.RecordingGenerationID, OwnerEpoch: capture.Catalog.OwnerEpoch,
		Status: capture.Health, ExecutionTerminal: capture.Terminal, Records: capture.MetadataRecords,
	}, target, recipe.Execution)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	source.SessionMetadata = bytes.Clone(recipe.SessionMetadata)
	return source, nil
}

// ValidateWorkerContinuationSource keeps immutable input validation with its
// owner. Prepared history alone never authorizes execution after artifact loss
// or replacement, even when the replacement is another canonical recipe.
func (writer *FileWriter) ValidateWorkerContinuationSource(ctx context.Context, target recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	source, err := writer.LookupPreparedWorkerContinuationSource(ctx, target)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	persisted, err := writer.readWorkerRestartRecipe(ctx, target, false)
	if err != nil {
		return recordings.WorkerContinuationSource{}, err
	}
	if !reflect.DeepEqual(persisted.Execution, source.Execution) || !bytes.Equal(persisted.SessionMetadata, source.SessionMetadata) {
		return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingReplay
	}
	return source, nil
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
	recipe, err := writer.readWorkerRestartRecipe(ctx, target, false)
	return recipe.Execution, err
}

func (writer *FileWriter) readWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget, preparedOnly bool) (workerRestartRecipe, error) {
	entry := writer.entry(target.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return writer.readWorkerRestartRecipeLocked(ctx, entry, target, preparedOnly)
}

func (writer *FileWriter) readWorkerRestartRecipeLocked(ctx context.Context, entry *recordingEntry, target recordings.WorkerControlTarget, preparedOnly bool) (workerRestartRecipe, error) {
	key := recordings.WorkerControlOperationKey{
		RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID,
		FactorySessionID: target.FactorySessionID, RequestID: "restart-recipe/" + target.ExpectedAttemptID,
	}
	identity, err := writer.restartInputIdentity(ctx, entry, key, preparedOnly)
	if err != nil {
		return workerRestartRecipe{}, err
	}
	session := entry.sessions[target.WorkerSessionID]
	if target.ExpectedAttemptID == "" || session.generation != target.RecordingGenerationID || session.ownerEpoch != target.OwnerEpoch {
		return workerRestartRecipe{}, recordings.ErrWorkerControlConflict
	}
	var input []byte
	if preparedOnly {
		input = bytes.Clone(session.restartRecipes[target.ExpectedAttemptID])
		if len(input) == 0 {
			return workerRestartRecipe{}, recordings.ErrWorkerRecordingReplay
		}
	} else {
		path := writer.controlInputPath(controlInputRef(identity))
		if err := writer.checkControlInputPath(path); err != nil {
			return workerRestartRecipe{}, err
		}
		data, err := writer.storage.ReadFile(path)
		if err != nil {
			return workerRestartRecipe{}, err
		}
		input, err = decodeControlInput(data, identity)
		if err != nil {
			return workerRestartRecipe{}, err
		}
	}
	var recipe workerRestartRecipe
	decoder := json.NewDecoder(bytes.NewReader(input))
	// Factory input tokens can contain integer facts beyond float64 precision.
	// Preserve their JSON numbers through canonical validation and continuation.
	decoder.UseNumber()
	if decoder.Decode(&recipe) != nil || recipe.Version != 1 || recipe.Target != target {
		return workerRestartRecipe{}, recordings.ErrWorkerRecordingReplay
	}
	canonical, err := encodeWorkerRestartRecipe(target, recipe.Execution, recipe.SessionMetadata)
	// Canonical bytes also reject unknown fields and data excluded by the
	// detached execution contract rather than silently discarding them.
	if err != nil || !bytes.Equal(canonical, input) {
		return workerRestartRecipe{}, recordings.ErrWorkerRecordingReplay
	}
	return recipe, nil
}

type workerRestartRecipe struct {
	SessionMetadata json.RawMessage                    `json:"sessionMetadata,omitempty"`
	Version         int                                `json:"version"`
	Target          recordings.WorkerControlTarget     `json:"target"`
	Execution       workers.WorkstationDispatchRequest `json:"execution"`
}

// ValidateWorkerRestartRecipe checks the same serializer used by admission,
// without opening capture or persisting input. New capture generations are
// SHA-256 hex strings; their contents do not change the serialized size.
func (writer *FileWriter) ValidateWorkerRestartRecipe(ctx context.Context, workerID string, execution workers.WorkstationDispatchRequest, metadata ...json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := recordings.WorkerControlTarget{
		RecordingID: execution.Execution.RecordingID, WorkerSessionID: workerID,
		FactorySessionID:      execution.Execution.FactorySessionID,
		RecordingGenerationID: strings.Repeat("0", 64), OwnerEpoch: writer.captureOwnerEpoch(),
		ExpectedAttemptID: execution.Execution.Dispatch.DispatchID,
	}
	_, err := encodeWorkerRestartRecipe(target, execution, metadata...)
	return err
}

// SaveWorkerRestartRecipe shares the journal's sync acknowledgement and keeps
// capture validation locked through persistence. No caller-selected path or
// independent ledger is introduced. The attempt is part of the immutable key.
func (writer *FileWriter) SaveWorkerRestartRecipe(ctx context.Context, target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest, metadata ...json.RawMessage) error {
	input, err := encodeWorkerRestartRecipe(target, execution, metadata...)
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
	if err == nil {
		if session.restartRecipes == nil {
			session.restartRecipes = make(map[string][]byte)
		}
		session.restartRecipes[target.ExpectedAttemptID] = bytes.Clone(input)
	}
	return err
}

func encodeWorkerRestartRecipe(target recordings.WorkerControlTarget, execution workers.WorkstationDispatchRequest, metadata ...json.RawMessage) ([]byte, error) {
	var sessionMetadata json.RawMessage
	if len(metadata) > 1 {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	if len(metadata) == 1 {
		sessionMetadata = metadata[0]
	}
	if len(sessionMetadata) != 0 && (!json.Valid(sessionMetadata) || bytes.Equal(bytes.TrimSpace(sessionMetadata), []byte("null"))) {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
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
	// Dynamic dispatch inputs may be structs in a live Factory attempt and JSON
	// objects after recovery. Normalize their representation before persistence
	// so strict canonical readback remains independent of Go struct field order.
	detached, err := json.Marshal(execution)
	if err != nil {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	decoder := json.NewDecoder(bytes.NewReader(detached))
	decoder.UseNumber()
	var normalized workers.WorkstationDispatchRequest
	if decoder.Decode(&normalized) != nil {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	input, err := json.Marshal(workerRestartRecipe{Version: 1, Target: target, Execution: normalized, SessionMetadata: sessionMetadata})
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

func (writer *FileWriter) restartInputIdentity(ctx context.Context, entry *recordingEntry, key recordings.WorkerControlOperationKey, preparedOnly bool) (controlInputArtifact, error) {
	if !preparedOnly {
		return writer.controlInputIdentity(ctx, entry, key)
	}
	if err := ctx.Err(); err != nil {
		return controlInputArtifact{}, err
	}
	// Ordinary admission must never hydrate a journal, even on a cold read.
	session := entry.sessions[key.WorkerSessionID]
	if !entry.loaded || entry.damaged || session == nil || len(session.records) == 0 {
		return controlInputArtifact{}, recordings.ErrWorkerRecordingReplay
	}
	if writer.catalogEntry(session).FactorySessionID != key.FactorySessionID {
		return controlInputArtifact{}, recordings.ErrWorkerControlConflict
	}
	return controlInputArtifact{Key: key, Generation: session.generation}, nil
}

// Activation prepares only the immutable recipe for each captured current attempt.
// Observation uses these detached bytes; admission revalidates the artifact.
func (writer *FileWriter) prepareRestartRecipe(ctx context.Context, entry *recordingEntry, session *recordingSession, catalog recordings.WorkerSessionCatalogEntry) {
	var draft workers.Draft
	var opening workers.SessionPayload
	if json.Unmarshal(session.records[0].Payload, &draft) != nil || json.Unmarshal(draft.Payload, &opening) != nil {
		return
	}
	if opening.AttemptID == "" && session.projection.ExecutionTerminal != nil {
		for _, record := range session.records {
			if record.ID.Position == session.projection.ExecutionTerminal.Position && json.Unmarshal(record.Payload, &draft) == nil {
				opening.AttemptID = draft.DispatchID
				break
			}
		}
	}
	target := recordings.WorkerControlTarget{RecordingID: catalog.RecordingID, WorkerSessionID: catalog.WorkerSessionID,
		FactorySessionID: catalog.FactorySessionID, RecordingGenerationID: session.generation, OwnerEpoch: session.ownerEpoch, ExpectedAttemptID: opening.AttemptID}
	recipe, err := writer.readWorkerRestartRecipeLocked(ctx, entry, target, false)
	if err != nil {
		return
	}
	input, err := encodeWorkerRestartRecipe(target, recipe.Execution, recipe.SessionMetadata)
	if err != nil {
		return
	}
	if session.restartRecipes == nil {
		session.restartRecipes = make(map[string][]byte)
	}
	session.restartRecipes[target.ExpectedAttemptID] = input
}
