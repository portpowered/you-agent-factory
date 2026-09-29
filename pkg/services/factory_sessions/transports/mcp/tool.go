// Package factorysession exposes MCP tool discovery for durable Factory Session
// operations backed by the shared factorysessionexecution service contract.
package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Tool names use Factory Session vocabulary and align with durable REST routes.
const (
	ToolListSessions   = "you.factory_session.list"
	ToolValidateSource = "you.factory_session.validate_source"
	ToolStartSync      = "you.factory_session.start_sync"
	ToolStartAsync     = "you.factory_session.start_async"
	ToolGetSession     = "you.factory_session.get"
	ToolGetResult      = "you.factory_session.get_result"
	ToolListDispatches = "you.factory_session.list_dispatches"
	ToolListArtifacts  = "you.factory_session.list_artifacts"
	ToolControl        = "you.factory_session.control"
	ToolReadEvents     = "you.factory_session.read_events"
	ToolSubagent       = "you.subagent"
)

// Stable error envelope fields shared by every dynamic workflow MCP tool.
var sharedErrorStableFields = []string{
	"error.code",
	"error.message",
	"error.retryable",
	"error.sessionId",
	"error.details",
}

// ToolDefinition is one discoverable MCP tool with typed schemas and documented
// stable response fields for success and error envelopes.
type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	InputSchema         map[string]any `json:"inputSchema"`
	OutputSchema        map[string]any `json:"outputSchema"`
	SuccessStableFields []string       `json:"successStableFields"`
	ErrorStableFields   []string       `json:"errorStableFields"`
}

// ToolErrorEnvelope is the stable MCP failure shape for Factory Session tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	SessionID string         `json:"sessionId,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// MarshalJSON encodes one tool definition for MCP hosts and mock clients.
func (t ToolDefinition) MarshalJSON() ([]byte, error) {
	type alias ToolDefinition
	return json.Marshal(alias(t))
}

// ValidateSource runs the canonical Factory preview contract for the
// you.factory_session.validate_source MCP tool without provider execution.
func ValidateSource(
	ctx context.Context,
	workflows factoryruntime.WorkflowPreviewOperation,
	input factoryapi.FactoryPreviewRequest,
) ToolResponse[factoryapi.FactoryPreviewResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryapi.FactoryPreviewResult](ctx); done {
		return response
	}
	previewInput, err := apisurface.FactoryPreviewInputFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}

	if workflows == nil {
		envelope := requestValidationErrorEnvelope(fmt.Errorf("workflow preview is unavailable"))
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	workflowPreview, err := workflows.PreviewWorkflow(ctx, previewInput)
	if err != nil {
		if envelope, ok := contextRequestErrorEnvelope(err); ok {
			return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
		}
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	preview := apisurface.FactoryPreviewResultFromPreview(workflowPreview)
	if !preview.Valid {
		envelope := validationErrorEnvelopeFromPreview(preview)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &preview}
}

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
func Subagent(ctx context.Context, target factorysessionexecution.TargetExecutionService, workingRoot string, generateID factorysessionexecution.SessionIDGenerator, input SubagentInput) ToolResponse[SubagentResult] {
	if err := validateSubagentRequest(ctx, target, generateID, input); err != nil {
		var envelope ToolErrorEnvelope
		if errors.Is(err, errMissingRequestContext) || ctx == nil || ctx.Err() != nil || target == nil || generateID == nil {
			envelope = executionErrorEnvelope(err)
		} else {
			envelope = ToolErrorEnvelope{Code: errorCodeBadRequest, Message: err.Error(), Retryable: false}
		}
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	requestID := generateID()
	startArgs := map[string]any{"workingRoot": workingRoot}
	started, err := target.StartAsync(ctx, factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source: factorysessionexecution.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Args: startArgs,
	})
	if err != nil {
		return subagentExecutionFailure(err)
	}
	if started.SessionID == "" {
		return subagentExecutionFailure(fmt.Errorf("subagent Factory Session identity is missing"))
	}
	args := subagentInvocationArgs(input)
	invocationRequest := factorysessionexecution.InvocationRequest{
		Args: &args, RequestID: &requestID,
		TimeoutMillis: input.TimeoutMillis,
	}
	result, invokeErr := target.InvokeFactorySession(ctx, started.SessionID, invocationRequest)
	closeErr := target.CloseFactorySession(context.WithoutCancel(ctx), started.SessionID)
	if result.Status != factorysessionexecution.InvocationTerminalStatusCompleted {
		return subagentTerminalFailure(started.SessionID, result, input.TimeoutMillis)
	}
	if err := errors.Join(invokeErr, closeErr); err != nil {
		return subagentExecutionFailure(err)
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

func subagentInvocationArgs(input SubagentInput) map[string]any {
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
	return args
}

func validateSubagentRequest(ctx context.Context, target factorysessionexecution.TargetExecutionService, generateID factorysessionexecution.SessionIDGenerator, input SubagentInput) error {
	switch {
	case ctx == nil:
		return errMissingRequestContext
	case ctx.Err() != nil:
		return ctx.Err()
	case target == nil:
		return errors.New("Factory Session target execution service is unavailable")
	case generateID == nil:
		return errors.New("subagent request ID generator is unavailable")
	case strings.TrimSpace(input.Prompt) == "":
		return fmt.Errorf("prompt is required")
	case input.TimeoutMillis != nil && *input.TimeoutMillis <= 0:
		return fmt.Errorf("timeoutMillis must be greater than zero")
	default:
		return nil
	}
}

func subagentExecutionFailure(err error) ToolResponse[SubagentResult] {
	envelope := executionErrorEnvelope(err)
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentTerminalFailure(sessionID string, result factorysessionexecution.InvocationResult, timeoutMillis *int64) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.execution_failed",
		Message:   "subagent execution failed before producing a result",
		Retryable: false,
		SessionID: sessionID,
		Details:   map[string]any{"status": result.Status},
	}
	if result.ErrorCode != "" {
		envelope.Details["invocationCode"] = result.ErrorCode
	}
	if result.Status == factorysessionexecution.InvocationTerminalStatusTimedOut {
		envelope.Code = "factory_session.subagent.timed_out"
		envelope.Message = "subagent timed out before producing a result; workspace edits may have occurred"
		envelope.Details["partialEffectsPossible"] = true
		if timeoutMillis != nil {
			envelope.Details["timeoutMillis"] = *timeoutMillis
		}
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}
