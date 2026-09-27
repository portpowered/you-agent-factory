package factorysession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// SubagentInput is the simplified request accepted by you.subagent.
type SubagentInput struct {
	Prompt          string `json:"prompt"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	TimeoutMillis   *int64 `json:"timeoutMillis,omitempty"`
}

// SubagentResult returns the child response as readable text with the identity
// of the Factory Session used for the invocation.
type SubagentResult struct {
	SessionID string `json:"sessionId"`
	Status    string `json:"status"`
	Text      string `json:"text,omitempty"`
}

// Subagent invokes the packaged @you/subagent Factory through the canonical
// on-demand target execution service.
func Subagent(ctx context.Context, target factorysessions.TargetExecutionService, workingRoot string, input SubagentInput) ToolResponse[SubagentResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if err := ctx.Err(); err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if target == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Prompt) == "" {
		envelope := requestValidationErrorEnvelope(fmt.Errorf("prompt is required"))
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if input.TimeoutMillis != nil && *input.TimeoutMillis <= 0 {
		envelope := requestValidationErrorEnvelope(fmt.Errorf("timeoutMillis must be greater than zero"))
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	requestID, err := newSubagentRequestID()
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	startArgs := map[string]any{"workingRoot": workingRoot}
	started, err := target.StartAsync(ctx, factorysessions.StartRequest{
		RequestID: requestID,
		Source: factorysessions.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Args: startArgs,
	})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if started.SessionID == "" {
		envelope := executionErrorEnvelope(fmt.Errorf("subagent Factory Session identity is missing"))
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	args := map[string]any{"input": input.Prompt}
	if input.Provider != "" {
		args["workerProvider"] = input.Provider
	}
	if input.Model != "" {
		args["workerModel"] = input.Model
	}
	if input.ReasoningEffort != "" {
		args["workerReasoningEffort"] = input.ReasoningEffort
	}
	invocationRequest := factorysessions.InvocationRequest{
		Args: &args, RequestID: &requestID,
		TimeoutMillis: input.TimeoutMillis,
	}
	result, invokeErr := target.InvokeFactorySession(ctx, started.SessionID, invocationRequest)
	closeErr := target.CloseFactorySession(context.WithoutCancel(ctx), started.SessionID)
	if err := errors.Join(invokeErr, closeErr); err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	if result.Status != factorysessions.InvocationTerminalStatusCompleted {
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("subagent finished with status %s", result.Status)
		}
		envelope := executionErrorEnvelope(errors.New(message))
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	response := SubagentResult{SessionID: started.SessionID, Status: string(result.Status)}
	for _, part := range result.PrimaryResult {
		if part.Type == work.WorkContentPartTypeText {
			if response.Text != "" {
				response.Text += "\n"
			}
			response.Text += part.Text
		}
	}
	return ToolResponse[SubagentResult]{Result: &response}
}

func newSubagentRequestID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "mcp-subagent-" + hex.EncodeToString(random[:]), nil
}
