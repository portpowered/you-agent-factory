package livechild

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ProviderChildExecutor routes one child agent.run through a real provider inference call.
type ProviderChildExecutor struct {
	sessionID   string
	executor    workers.InvocationExecutor
	records     workflowresult.JavaScriptChildRecordSink
	childValues workflowresult.JavaScriptChildValues
	workingDir  string
	maxRetries  int
	sleep       func(context.Context, time.Duration) error
}

const liveChildInitialRetryBackoff = 100 * time.Millisecond

// NewProviderChildExecutor constructs one provider-backed child executor.
func NewProviderChildExecutor(
	sessionID string,
	executor workers.InvocationExecutor,
	records workflowresult.JavaScriptChildRecordSink,
	childValues workflowresult.JavaScriptChildValues,
) *ProviderChildExecutor {
	return NewRetryingProviderChildExecutor(sessionID, executor, records, 0, childValues)
}

// NewRetryingProviderChildExecutor constructs a provider-backed child executor
// with the effective workflow policy's bounded retry limit.
func NewRetryingProviderChildExecutor(
	sessionID string,
	executor workers.InvocationExecutor,
	records workflowresult.JavaScriptChildRecordSink,
	maxRetries int,
	childValues workflowresult.JavaScriptChildValues,
	workingDirectories ...string,
) *ProviderChildExecutor {
	if maxRetries < 0 {
		maxRetries = 0
	}
	workingDir := ""
	if len(workingDirectories) > 0 {
		workingDir = strings.TrimSpace(workingDirectories[0])
	}
	return &ProviderChildExecutor{
		sessionID:   sessionID,
		executor:    executor,
		records:     records,
		childValues: childValues,
		workingDir:  workingDir,
		maxRetries:  maxRetries,
		sleep:       sleepWithContext,
	}
}

// Execute records queued and running dispatch transitions, calls the provider, and
// appends terminal child dispatch records for shared session inspection.
func (e *ProviderChildExecutor) Execute(ctx context.Context, req workflowresult.JavaScriptChildExecutionRequest) (workflowresult.JavaScriptChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workflowresult.JavaScriptChildExecutionResult{}, err
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	providerName, providerSessionRef := "", ""
	runnerID, selectionErr := workerexecution.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider)
	if selectionErr != nil {
		return workflowresult.JavaScriptChildExecutionResult{}, selectionErr
	}
	if runnerID == "" {
		runnerID = firstLiveChildValue(req.Command, req.ModelProvider)
	}
	artifactID := e.records.NextChildArtifactID()
	artifactRef := workflowresult.FormatArtifactURI(e.sessionID, artifactID)

	base := workflowresult.JavaScriptChildDispatchRecord{
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Attempt:         1,
		Label:           req.Label,
		PromptDigest:    e.childValues.TextDigest(req.Prompt),
		Preset:          req.Preset,
		ModelProvider:   req.ModelProvider,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		SkipPermissions: req.SkipPermissions,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    e.childValues.SchemaDigest(req.OutputSchema),
		RunnerID:        runnerID,
		ExecutionMode:   workflowresult.JavaScriptChildExecutionModeLive,
		ArtifactRef:     artifactRef,
	}

	e.records.AppendChildDispatch(base, workflowresult.JavaScriptChildDispatchStatusQueued)

	inferReq := providerInferenceRequestFromChild(dispatchID, req)
	inferReq.WorkingDirectory = e.workingDir
	execution, err := e.executeWithRetry(ctx, inferReq, base)
	base.Attempt = execution.Attempt
	if err != nil {
		providerName, providerSessionRef = providerSessionFields(execution.ProviderSession)
		return e.failedChildResult(base, req, dispatchID, childIndex, providerName, providerSessionRef, execution.FailureDetail, execution.FailureDecision)
	}

	providerName, providerSessionRef = providerSessionFields(execution.ProviderSession)
	output := providerChildOutput(req, execution.Response.Content)

	completed := base
	completed.Status = workflowresult.JavaScriptChildDispatchStatusCompleted
	completed.Provider = providerName
	completed.ProviderSessionRef = providerSessionRef
	completed.ArtifactRef = artifactRef
	completed.Output = e.childValues.CloneOutputMap(output)
	e.records.Append(workflowresult.JavaScriptRuntimeRecord{
		Kind:          workflowresult.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &completed,
	})

	return workflowresult.JavaScriptChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowresult.JavaScriptChildDispatchStatusCompleted,
		ExecutionMode:      workflowresult.JavaScriptChildExecutionModeLive,
		Output:             output,
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *ProviderChildExecutor) executeWithRetry(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	base workflowresult.JavaScriptChildDispatchRecord,
) (workers.InvocationResult, error) {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return workers.InvocationResult{Attempt: attempt}, err
		}
		running := base
		running.Attempt = attempt
		e.records.AppendChildDispatch(running, workflowresult.JavaScriptChildDispatchStatusRunning)

		result, err := e.executor.Execute(ctx, workers.InvocationInput{Request: req, Attempt: attempt})
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
			return e.executor.Execute(ctx, workers.InvocationInput{Request: req, Attempt: attempt + 1})
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
	base workflowresult.JavaScriptChildDispatchRecord,
	req workflowresult.JavaScriptChildExecutionRequest,
	dispatchID string,
	childIndex int,
	providerName string,
	providerSessionRef string,
	failureDetail *workerexecution.FailureDetail,
	failureDecision *workerexecution.WorkFailureDecision,
) (workflowresult.JavaScriptChildExecutionResult, error) {
	failed := base
	failed.Status = workflowresult.JavaScriptChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	failed.FailureDetail = failureDetail
	if failureDetail != nil {
		failed.FailureClassification = failureDetail.Reason
	}
	if failureDecision != nil {
		failed.Retryable = &failureDecision.Retryable
	}
	e.records.Append(workflowresult.JavaScriptRuntimeRecord{
		Kind:          workflowresult.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	diagnostic := "Provider execution failed."
	if failureDetail != nil && strings.TrimSpace(failureDetail.Message) != "" {
		diagnostic = failureDetail.Message
	}
	return workflowresult.JavaScriptChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             workflowresult.JavaScriptChildDispatchStatusFailed,
		ExecutionMode:      workflowresult.JavaScriptChildExecutionModeLive,
		Diagnostic:         diagnostic,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, fmt.Errorf("%s", diagnostic)
}

func (e *ProviderChildExecutor) childDispatchIdentity(req workflowresult.JavaScriptChildExecutionRequest) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}

func providerInferenceRequestFromChild(
	dispatchID string,
	req workflowresult.JavaScriptChildExecutionRequest,
) workerexecution.ProviderInferenceRequest {
	outputSchema := ""
	if req.OutputSchema != nil {
		if encoded, err := json.Marshal(req.OutputSchema); err == nil {
			outputSchema = string(encoded)
		}
	}
	preset := strings.TrimSpace(req.Preset)
	runnerID, _ := workerexecution.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider)
	if runnerID == "" {
		runnerID = firstLiveChildValue(req.Command, req.ModelProvider)
	}
	inferReq := workerexecution.ProviderInferenceRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: dispatchID,
			WorkerType: preset,
		},
		UserMessage:      req.Prompt,
		Model:            req.Model,
		ModelProvider:    req.ModelProvider,
		OutputSchema:     outputSchema,
		RunnerID:         runnerID,
		ExecutorProvider: strings.TrimSpace(req.ExecutorProvider),
		WorkerType:       preset,
		SkipPermissions:  req.SkipPermissions,
	}
	return inferReq
}

func firstLiveChildValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func providerChildOutput(req workflowresult.JavaScriptChildExecutionRequest, content string) map[string]any {
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
