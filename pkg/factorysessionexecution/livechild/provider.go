package livechild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/provider"
)

// ProviderChildExecutor routes one child agent.run through a real provider inference call.
type ProviderChildExecutor struct {
	sessionID  string
	provider   workers.Provider
	records    workflowruntime.ChildRecordSink
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

const liveChildInitialRetryBackoff = 100 * time.Millisecond

// NewProviderChildExecutor constructs one provider-backed child executor.
func NewProviderChildExecutor(
	sessionID string,
	provider workers.Provider,
	records workflowruntime.ChildRecordSink,
) *ProviderChildExecutor {
	return NewRetryingProviderChildExecutor(sessionID, provider, records, 0)
}

// NewRetryingProviderChildExecutor constructs a provider-backed child executor
// with the effective workflow policy's bounded retry limit.
func NewRetryingProviderChildExecutor(
	sessionID string,
	provider workers.Provider,
	records workflowruntime.ChildRecordSink,
	maxRetries int,
) *ProviderChildExecutor {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &ProviderChildExecutor{
		sessionID:  sessionID,
		provider:   provider,
		records:    records,
		maxRetries: maxRetries,
		sleep:      sleepWithContext,
	}
}

// Execute records queued and running dispatch transitions, calls the provider, and
// appends terminal child dispatch records for shared session inspection.
func (e *ProviderChildExecutor) Execute(ctx context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workflowruntime.ChildExecutionResult{}, err
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	providerName, providerSessionRef := "", ""
	artifactID := e.records.NextChildArtifactID()
	artifactRef := workflowresult.FormatArtifactURI(e.sessionID, artifactID)

	base := workflowruntime.ChildDispatchRecord{
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Attempt:         1,
		Label:           req.Label,
		PromptDigest:    workflowruntime.TextDigest(req.Prompt),
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    workflowruntime.SchemaDigest(req.OutputSchema),
		RunnerID:        strings.TrimSpace(req.Command),
		ExecutionMode:   workflowruntime.ChildExecutionModeLive,
		ArtifactRef:     artifactRef,
	}

	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusQueued)

	inferReq := providerInferenceRequestFromChild(e.sessionID, dispatchID, req)
	resp, attempt, err := e.inferWithRetry(ctx, inferReq, base)
	base.Attempt = attempt
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
	completed.Output = workflowruntime.CloneOutputMap(output)
	e.records.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &completed,
	})

	return workflowruntime.ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowruntime.ChildDispatchStatusCompleted,
		ExecutionMode:      workflowruntime.ChildExecutionModeLive,
		Output:             output,
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *ProviderChildExecutor) inferWithRetry(
	ctx context.Context,
	req interfaces.ProviderInferenceRequest,
	base workflowruntime.ChildDispatchRecord,
) (interfaces.InferenceResponse, int, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return interfaces.InferenceResponse{}, attempt, err
		}
		running := base
		running.Attempt = attempt
		e.records.AppendChildDispatch(running, workflowruntime.ChildDispatchStatusRunning)

		resp, err := e.provider.Infer(ctx, req)
		if err == nil {
			return resp, attempt, nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return interfaces.InferenceResponse{}, attempt, contextErr
		}
		providerErr := provider.NormalizeProviderExecutionError(err)
		if providerErr == nil {
			return interfaces.InferenceResponse{}, attempt, err
		}
		decision := provider.WorkFailureDecisionFromProviderError(providerErr)
		if !decision.Retryable || attempt > e.maxRetries {
			return interfaces.InferenceResponse{}, attempt, providerErr
		}
		if err := e.sleep(ctx, liveChildInitialRetryBackoff<<uint(attempt-1)); err != nil {
			return interfaces.InferenceResponse{}, attempt, err
		}
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	failed.FailureDetail = childExecutionFailureDetail(err)
	if providerErr := providerErrorFrom(err); providerErr != nil {
		decision := provider.WorkFailureDecisionFromProviderError(providerErr)
		failed.Retryable = &decision.Retryable
		failed.FailureClassification = providerErr.Type
	}
	e.records.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	diagnostic := err.Error()
	return workflowruntime.ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowruntime.ChildDispatchStatusFailed,
		ExecutionMode:      workflowruntime.ChildExecutionModeLive,
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

func childExecutionFailureDetail(err error) *interfaces.FailureDetail {
	detail := &interfaces.FailureDetail{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: err.Error(),
	}
	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) {
		return provider.SafeProviderFailureDetail(providerErr)
	}
	return detail
}

func providerErrorFrom(err error) *provider.ProviderError {
	var providerErr *provider.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr
	}
	return nil
}
