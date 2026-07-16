package livechild

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

// ProviderChildExecutor routes one child agent.run through a real provider inference call.
type ProviderChildExecutor struct {
	sessionID  string
	executor   providerexecution.Executor
	records    workflowruntime.ChildRecordSink
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

const liveChildInitialRetryBackoff = 100 * time.Millisecond

// NewProviderChildExecutor constructs one provider-backed child executor.
func NewProviderChildExecutor(
	sessionID string,
	executor providerexecution.Executor,
	records workflowruntime.ChildRecordSink,
) *ProviderChildExecutor {
	return NewRetryingProviderChildExecutor(sessionID, executor, records, 0)
}

// NewRetryingProviderChildExecutor constructs a provider-backed child executor
// with the effective workflow policy's bounded retry limit.
func NewRetryingProviderChildExecutor(
	sessionID string,
	executor providerexecution.Executor,
	records workflowruntime.ChildRecordSink,
	maxRetries int,
) *ProviderChildExecutor {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &ProviderChildExecutor{
		sessionID:  sessionID,
		executor:   executor,
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
		Preset:          req.Preset,
		ModelProvider:   req.ModelProvider,
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
	execution, err := e.executeWithRetry(ctx, inferReq, base)
	base.Attempt = execution.Attempt
	if err != nil {
		providerName, providerSessionRef = providerSessionFields(execution.ProviderSession)
		return e.failedChildResult(base, req, dispatchID, childIndex, providerName, providerSessionRef, execution.FailureDetail, execution.FailureDecision)
	}

	providerName, providerSessionRef = providerSessionFields(execution.ProviderSession)
	output := providerChildOutput(req, execution.Response.Content)

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

func (e *ProviderChildExecutor) executeWithRetry(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	base workflowruntime.ChildDispatchRecord,
) (providerexecution.ExecutionResult, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return providerexecution.ExecutionResult{Attempt: attempt}, err
		}
		running := base
		running.Attempt = attempt
		e.records.AppendChildDispatch(running, workflowruntime.ChildDispatchStatusRunning)

		result, err := e.executor.Execute(ctx, providerexecution.ExecutionInput{Request: req, Attempt: attempt})
		if err == nil {
			return result, nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return result, contextErr
		}
		if result.FailureDecision == nil || !result.FailureDecision.Retryable || attempt > e.maxRetries {
			return result, err
		}
		if err := e.sleep(ctx, liveChildInitialRetryBackoff<<uint(attempt-1)); err != nil {
			return e.executor.Execute(ctx, providerexecution.ExecutionInput{Request: req, Attempt: attempt + 1})
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
	failureDetail *workerexecution.FailureDetail,
	failureDecision *workerexecution.WorkFailureDecision,
) (workflowruntime.ChildExecutionResult, error) {
	failed := base
	failed.Status = workflowruntime.ChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	failed.FailureDetail = failureDetail
	if failureDetail != nil {
		failed.FailureClassification = failureDetail.Reason
	}
	if failureDecision != nil {
		failed.Retryable = &failureDecision.Retryable
	}
	e.records.Append(workflowruntime.RuntimeRecord{
		Kind:          workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	diagnostic := "Provider execution failed."
	if failureDetail != nil && strings.TrimSpace(failureDetail.Message) != "" {
		diagnostic = failureDetail.Message
	}
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
) workerexecution.ProviderInferenceRequest {
	outputSchema := ""
	if req.OutputSchema != nil {
		if encoded, err := json.Marshal(req.OutputSchema); err == nil {
			outputSchema = string(encoded)
		}
	}
	inferReq := workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: dispatchID,
		},
		UserMessage:   req.Prompt,
		Model:         req.Model,
		ModelProvider: req.ModelProvider,
		OutputSchema:  outputSchema,
		SessionID:     sessionID,
		RunnerID:      strings.TrimSpace(req.Command),
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

func providerSessionFields(session *workerexecution.ProviderSessionMetadata) (providerName, sessionRef string) {
	if session == nil {
		return "", ""
	}
	providerName = workerexecution.CanonicalProviderSessionProvider(session.Provider)
	if providerName == "" {
		providerName = strings.TrimSpace(session.Provider)
	}
	sessionRef = strings.TrimSpace(session.ID)
	return providerName, sessionRef
}
