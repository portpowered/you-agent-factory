package livechild

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
)

// ProviderChildExecutor routes one child agent.run through a real provider inference call.
type ProviderChildExecutor struct {
	sessionID string
	executor  providerexecution.Executor
	records   workflowruntime.ChildRecordSink
}

// NewProviderChildExecutor constructs one provider-backed child executor.
func NewProviderChildExecutor(
	sessionID string,
	executor providerexecution.Executor,
	records workflowruntime.ChildRecordSink,
) *ProviderChildExecutor {
	return &ProviderChildExecutor{
		sessionID: sessionID,
		executor:  executor,
		records:   records,
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
	e.records.AppendChildDispatch(base, workflowruntime.ChildDispatchStatusRunning)

	execution, err := e.executor.Execute(ctx, providerexecution.ExecutionInput{
		Request: providerInferenceRequestFromChild(e.sessionID, dispatchID, req),
		Attempt: 1,
	})
	if err != nil {
		providerName, providerSessionRef = providerSessionFields(execution.ProviderSession)
		return e.failedChildResult(base, req, dispatchID, childIndex, providerName, providerSessionRef, execution.FailureDetail)
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

func (e *ProviderChildExecutor) failedChildResult(
	base workflowruntime.ChildDispatchRecord,
	req workflowruntime.ChildExecutionRequest,
	dispatchID string,
	childIndex int,
	providerName string,
	providerSessionRef string,
	failureDetail *interfaces.FailureDetail,
) (workflowruntime.ChildExecutionResult, error) {
	failed := base
	failed.Status = workflowruntime.ChildDispatchStatusFailed
	failed.Provider = providerName
	failed.ProviderSessionRef = providerSessionRef
	failed.FailureDetail = failureDetail
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
