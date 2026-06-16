package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

const (
	// ChildExecutorModeFake selects deterministic in-process child execution.
	ChildExecutorModeFake = workflowruntime.ChildExecutionModeFake
	// ChildExecutorModeLive selects provider-backed child execution.
	ChildExecutorModeLive = workflowruntime.ChildExecutionModeLive
)

// RuntimeOptions selects durable JavaScript runtime execution behavior without
// changing workflow source syntax.
type RuntimeOptions struct {
	ChildExecutorMode string
}

// ProviderChildExecutor routes one child agent.run through a real provider inference call.
type ProviderChildExecutor struct {
	sessionID string
	provider  workers.Provider
	records   workflowruntime.ChildRecordSink
}

// NewProviderChildExecutor constructs one provider-backed child executor.
func NewProviderChildExecutor(
	sessionID string,
	provider workers.Provider,
	records workflowruntime.ChildRecordSink,
) *ProviderChildExecutor {
	return &ProviderChildExecutor{
		sessionID: sessionID,
		provider:  provider,
		records:   records,
	}
}

// Execute records queued and running dispatch transitions, calls the provider, and
// appends terminal child dispatch records for shared session inspection.
func (e *ProviderChildExecutor) Execute(req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	if strings.HasPrefix(req.Prompt, "fail:") {
		return e.executeFailed(req)
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	providerName, providerSessionRef := "", ""
	artifactID := e.records.NextChildArtifactID()
	artifactRef := workflowresult.FormatArtifactURI(e.sessionID, artifactID)

	base := workflowruntime.ChildDispatchRecord{
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Label:           req.Label,
		PromptDigest:    workflowruntime.TextDigest(req.Prompt),
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    workflowruntime.SchemaDigest(req.OutputSchema),
		RunnerID:        strings.TrimSpace(req.Command),
		ExecutionMode:   ChildExecutorModeLive,
		ArtifactRef:     artifactRef,
	}

	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusRunning)

	inferReq := providerInferenceRequestFromChild(e.sessionID, dispatchID, req)
	resp, err := e.provider.Infer(context.Background(), inferReq)
	if err != nil {
		return e.failedChildResult(base, req, dispatchID, childIndex, providerName, providerSessionRef, err)
	}

	providerName, providerSessionRef = providerSessionFields(resp.ProviderSession)
	output := providerChildOutput(req, resp.Content)

	completed := base
	completed.Status = workflowruntime.ChildDispatchStatusCompleted
	completed.Provider = providerName
	completed.ProviderSessionRef = providerSessionRef
	completed.ArtifactRef = artifactRef
	e.records.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &completed,
	})

	return workflowruntime.ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowruntime.ChildDispatchStatusCompleted,
		ExecutionMode:      ChildExecutorModeLive,
		Output:             output,
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *ProviderChildExecutor) executeFailed(req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	dispatchID, childIndex := e.childDispatchIdentity(req)
	base := workflowruntime.ChildDispatchRecord{
		DispatchID:    dispatchID,
		ChildIndex:    childIndex,
		Label:         req.Label,
		PromptDigest:  workflowruntime.TextDigest(req.Prompt),
		Model:         req.Model,
		ExecutionMode: ChildExecutorModeLive,
	}
	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusRunning)

	inferReq := providerInferenceRequestFromChild(e.sessionID, dispatchID, req)
	_, err := e.provider.Infer(context.Background(), inferReq)
	if err == nil {
		err = fmt.Errorf("live child failed: %s", strings.TrimPrefix(req.Prompt, "fail:"))
	}
	return e.failedChildResult(base, req, dispatchID, childIndex, "", "", err)
}

func (e *ProviderChildExecutor) failedChildResult(
	base workflowruntime.ChildDispatchRecord,
	req workflowruntime.ChildExecutionRequest,
	dispatchID string,
	childIndex int,
	providerName string,
	providerSessionRef string,
	err error,
) (workflowruntime.ChildExecutionResult, error) {
	failed := base
	failed.Status = workflowruntime.ChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	reason, message, errorClass := childExecutionFailureFields(err)
	failed.FailureReason = reason
	failed.FailureMessage = message
	failed.FailureErrorClass = errorClass
	e.records.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	diagnostic := err.Error()
	return workflowruntime.ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowruntime.ChildDispatchStatusFailed,
		ExecutionMode:      ChildExecutorModeLive,
		Diagnostic:         diagnostic,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, fmt.Errorf("%s", diagnostic)
}

func (e *ProviderChildExecutor) childDispatchIdentity(req workflowruntime.ChildExecutionRequest) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}

func providerInferenceRequestFromChild(
	sessionID string,
	dispatchID string,
	req workflowruntime.ChildExecutionRequest,
) interfaces.ProviderInferenceRequest {
	outputSchema := ""
	if req.OutputSchema != nil {
		if encoded, err := json.Marshal(req.OutputSchema); err == nil {
			outputSchema = string(encoded)
		}
	}
	inferReq := interfaces.ProviderInferenceRequest{
		Dispatch: interfaces.WorkDispatch{
			DispatchID: dispatchID,
		},
		UserMessage:  req.Prompt,
		Model:        req.Model,
		OutputSchema: outputSchema,
		SessionID:    sessionID,
		RunnerID:     strings.TrimSpace(req.Command),
	}
	if req.Model != "" {
		inferReq.ModelProvider = "workflow-child"
	}
	return inferReq
}

func providerChildOutput(req workflowruntime.ChildExecutionRequest, content string) map[string]any {
	trimmed := strings.TrimSpace(content)
	if req.OutputSchema != nil && trimmed != "" {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]any{
		"text":            trimmed,
		"subject":         req.ArgsSubject,
		"schemaValidated": req.OutputSchema != nil,
	}
}

func providerSessionFields(session *interfaces.ProviderSessionMetadata) (providerName, sessionRef string) {
	if session == nil {
		return "", ""
	}
	providerName = interfaces.CanonicalProviderSessionProvider(session.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(session.Provider)
	}
	sessionRef = strings.TrimSpace(session.ID)
	return providerName, sessionRef
}

func normalizeChildExecutorMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", ChildExecutorModeFake:
		return ChildExecutorModeFake
	case ChildExecutorModeLive:
		return ChildExecutorModeLive
	default:
		return strings.TrimSpace(mode)
	}
}

func resolveChildExecutorMode(configMode string, req StartRequest) string {
	if req.Runtime != nil && strings.TrimSpace(req.Runtime.ChildExecutorMode) != "" {
		return normalizeChildExecutorMode(req.Runtime.ChildExecutorMode)
	}
	return normalizeChildExecutorMode(configMode)
}

func childExecutionFailureFields(err error) (reason, message, errorClass string) {
	reason = workflowruntime.ChildExecutionFailureReason
	message = err.Error()
	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Type != "" {
			reason = string(providerErr.Type)
		}
		if providerErr.Message != "" {
			message = providerErr.Message
		}
		if providerErr.Family != "" {
			errorClass = string(providerErr.Family)
		}
	}
	return reason, message, errorClass
}

func (s *JavaScriptRuntimeService) childExecutorHooks(mode string) workflowruntime.Hooks {
	if mode != ChildExecutorModeLive {
		return workflowruntime.Hooks{}
	}
	provider := s.provider
	return workflowruntime.Hooks{
		NewChildExecutor: func(sessionID string, records workflowruntime.ChildRecordSink) workflowruntime.ChildExecutor {
			return NewProviderChildExecutor(sessionID, provider, records)
		},
	}
}

func validateLiveChildExecutorConfig(mode string, provider workers.Provider) error {
	if mode != ChildExecutorModeLive {
		return nil
	}
	if provider == nil {
		return NewValidationError("runtime.childExecutorMode", "provider is required for live-provider child execution")
	}
	return nil
}
