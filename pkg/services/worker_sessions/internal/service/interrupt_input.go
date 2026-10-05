package service

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// This recipe preserves the accepted execution and reference, never execution
// handles or inherited credentials. A recipe is data, not admission authority.
type durableInterruptInput struct {
	Version            int                                `json:"version"`
	ReplacementMessage string                             `json:"replacementMessage"`
	Execution          workers.WorkstationDispatchRequest `json:"execution"`
	ProviderReference  interruptInputReference            `json:"providerReference"`
}

type interruptInputReference struct {
	Provider providers.ID `json:"provider"`
	Kind     string       `json:"kind"`
	ID       string       `json:"id"`
}

func encodeInterruptInput(plan interruptPlan) ([]byte, error) {
	execution := plan.execution.Execution
	// Explicit overrides have no durable configuration reference with which to
	// restore their values. Never silently redact them into a different recipe.
	if !interruptExecutionReplaySafe(execution) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	input := durableInterruptInput{
		Version: 1, ReplacementMessage: plan.request.ReplacementMessage,
		Execution:         plan.execution,
		ProviderReference: interruptInputReference{Provider: plan.reference.Provider, Kind: plan.reference.Kind, ID: plan.reference.ID},
	}
	if !validInterruptInput(input, plan.request, plan.dispatchID) {
		return nil, recordings.ErrWorkerRecordingPersistence
	}
	payload, err := json.Marshal(input)
	if err != nil || !interruptRecipeSafe(payload, execution.ProcessEnvironment) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	return payload, nil
}

func interruptPromptNeedsRedaction(redaction *workers.PromptRedaction) bool {
	return redaction != nil && (redaction.FailClosed || redaction.RedactSystemPrompt || redaction.RedactUserMessage)
}

func interruptExecutionReplaySafe(execution workers.WorkstationExecutionRequest) bool {
	return len(execution.EnvVars) == 0 && execution.WorkflowContext == nil &&
		!interruptPromptNeedsRedaction(execution.PromptRedaction)
}

func validInterruptInput(input durableInterruptInput, req workersessions.InterruptRequest, attemptID string) bool {
	reference := providers.SessionRef{Provider: input.ProviderReference.Provider, Kind: input.ProviderReference.Kind, ID: input.ProviderReference.ID}
	start := workersessions.StartRequest{RequestID: req.RequestID, ID: req.SourceWorkerSessionID, Execution: input.Execution}
	return input.Version == 1 && req.Validate() == nil && input.ReplacementMessage == req.ReplacementMessage &&
		input.Execution.Execution.Dispatch.DispatchID == attemptID && reference.Validate() == nil && start.Validate() == nil &&
		interruptExecutionReplaySafe(input.Execution.Execution)
}

// Inspect decoded string values, rather than encoded JSON, so escaping cannot
// hide a configured credential in a prompt, argument, token or reference.
func interruptRecipeSafe(payload []byte, environment []string) bool {
	var document any
	if json.Unmarshal(payload, &document) != nil {
		return false
	}
	for _, entry := range environment {
		name, value, found := strings.Cut(entry, "=")
		if found && value != "" && interruptSensitiveEnvironmentName(name) && interruptJSONContains(document, value) {
			return false
		}
	}
	return true
}

func interruptJSONContains(document any, value string) bool {
	switch item := document.(type) {
	case string:
		return strings.Contains(item, value)
	case []any:
		for _, child := range item {
			if interruptJSONContains(child, value) {
				return true
			}
		}
	case map[string]any:
		for key, child := range item {
			if strings.Contains(key, value) || interruptJSONContains(child, value) {
				return true
			}
		}
	}
	return false
}

func validateCapturedInterruptInput(stored, legacyPayload []byte, req workersessions.InterruptRequest, attemptID string) error {
	if !uniqueInterruptJSONFields(stored) {
		return recordings.ErrWorkerRecordingPersistence
	}
	// Earlier request-only artifacts remain read-only replay evidence. They do
	// not contain a recipe and cannot authorize pending replacement recovery.
	if bytes.Equal(stored, legacyPayload) {
		return nil
	}
	var legacy workersessions.InterruptRequest
	if json.Unmarshal(stored, &legacy) == nil && legacy.RequestID != "" {
		return workersessions.ErrInterruptRequestIDConflict
	}
	var input durableInterruptInput
	decoder := json.NewDecoder(bytes.NewReader(stored))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		return recordings.ErrWorkerRecordingPersistence
	}
	if !canonicalInterruptRecipeFields(stored, input) {
		return recordings.ErrWorkerRecordingPersistence
	}
	accepted := req
	accepted.ReplacementMessage = input.ReplacementMessage
	if !validInterruptInput(input, accepted, attemptID) {
		return recordings.ErrWorkerRecordingPersistence
	}
	if input.ReplacementMessage != req.ReplacementMessage {
		return workersessions.ErrInterruptRequestIDConflict
	}
	return nil
}

// Compare field names with the decoded contract's own encoding. Struct aliases
// disappear on encoding, while case-sensitive customer map keys survive. This
// prevents a later canonical field from concealing an earlier private alias.
func canonicalInterruptRecipeFields(payload []byte, input durableInterruptInput) bool {
	canonical, err := json.Marshal(input)
	if err != nil {
		return false
	}
	var supplied, contract any
	if json.Unmarshal(payload, &supplied) != nil || json.Unmarshal(canonical, &contract) != nil {
		return false
	}
	return interruptRecipeFieldsMatch(supplied, contract)
}

func interruptRecipeFieldsMatch(supplied, contract any) bool {
	switch fields := supplied.(type) {
	case map[string]any:
		allowed, ok := contract.(map[string]any)
		if !ok {
			return false
		}
		for name, value := range fields {
			canonical, exists := allowed[name]
			if !exists || !interruptRecipeFieldsMatch(value, canonical) {
				return false
			}
		}
	case []any:
		allowed, ok := contract.([]any)
		if !ok || len(fields) != len(allowed) {
			return false
		}
		for index, value := range fields {
			if !interruptRecipeFieldsMatch(value, allowed[index]) {
				return false
			}
		}
	}
	return true
}
