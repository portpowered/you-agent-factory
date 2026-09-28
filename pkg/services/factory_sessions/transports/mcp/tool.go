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
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	WorkingRoot     string `json:"workingRoot,omitempty"`
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
// Factory Sessions Service.
func Subagent(ctx context.Context, target factorysessionexecution.Service, workingRoot string, generateID factorysessionexecution.SessionIDGenerator, input SubagentInput) ToolResponse[SubagentResult] {
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
	if input.WorkingRoot != "" {
		workingRoot = input.WorkingRoot
	}
	started, err := target.Start(ctx, factorysessionexecution.SessionStartRequest{
		Mode:           factorysessionexecution.SessionOperationModeLive,
		ActivationOnly: true,
		Correlation:    factorysessionexecution.SessionOperationCorrelation{RequestID: requestID},
		Definition: factorysessionexecution.SessionDefinitionSelection{
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Source: factorysessionexecution.Source{
			Kind:      factoryruntime.WorkflowSourceKindFactoryID,
			FactoryID: factorydefinitions.PackagedSubagentFactoryName,
		},
		Args:       map[string]any{"workingRoot": workingRoot},
		FolderPath: workingRoot,
		RuntimeSelection: &factorysessionexecution.SessionRuntimeSelection{
			ExecutionBaseDir: workingRoot,
			Mode:             factorysessionexecution.SessionRuntimeModeService,
		},
	})
	if err != nil {
		return subagentExecutionFailure(err)
	}
	if started.SessionID == "" {
		return subagentExecutionFailure(fmt.Errorf("subagent Factory Session identity is missing"))
	}
	args := subagentInvocationArgs(input)
	args["workingRoot"] = workingRoot
	var timeoutMillis int64
	if input.TimeoutMillis != nil {
		timeoutMillis = *input.TimeoutMillis
	}
	result, invokeErr := target.Invoke(ctx, factorysessionexecution.SessionInvokeRequest{
		SessionID:   started.SessionID,
		Correlation: factorysessionexecution.SessionOperationCorrelation{RequestID: requestID},
		Args:        args,
		Wait:        factorysessionexecution.SessionOperationWait{TimeoutMillis: timeoutMillis, CancelOnTimeout: true},
	})
	_, closeErr := target.Control(context.WithoutCancel(ctx), factorysessionexecution.SessionControlRequest{
		SessionID: started.SessionID,
		Mode:      factorysessionexecution.SessionOperationModeLive,
		Operation: factorysessionexecution.SessionControlClose,
	})
	if closeErr != nil {
		return subagentCleanupFailure(started.SessionID, requestID)
	}
	if invokeErr != nil {
		return subagentInvokeError(invokeErr, started.SessionID, requestID, timeoutMillis, input)
	}
	if result.Status != factorysessionexecution.InvocationTerminalStatusCompleted {
		return subagentTerminalFailure(started.SessionID, result, timeoutMillis, input)
	}
	response := SubagentResult{
		SessionID: started.SessionID,
		Status:    string(result.Status),
		Text:      subagentPrimaryText(result.PrimaryResult),
	}
	if strings.TrimSpace(response.Text) == "" {
		envelope := ToolErrorEnvelope{
			Code:      "factory_session.subagent.empty_result",
			Message:   "subagent completed without returning any text result; the ACP peer may have closed without producing output",
			SessionID: started.SessionID,
			Details:   map[string]any{"status": string(result.Status)},
		}
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	return ToolResponse[SubagentResult]{Result: &response}
}

// subagentPrimaryText joins the text parts of a completed invocation with
// newlines, ignoring non-text content parts.
func subagentPrimaryText(parts []work.WorkContentPart) string {
	text := ""
	for _, part := range parts {
		if part.Type == work.WorkContentPartTypeText {
			if text != "" {
				text += "\n"
			}
			text += part.Text
		}
	}
	return text
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

func validateSubagentRequest(ctx context.Context, target factorysessionexecution.Service, generateID factorysessionexecution.SessionIDGenerator, input SubagentInput) error {
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

func subagentCleanupFailure(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.cleanup_failed",
		Message:   "subagent session cleanup failed after execution; workspace edits may have occurred",
		SessionID: sessionID,
		Details:   map[string]any{"partialEffectsPossible": true, "requestId": requestID},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentInvocationFailure(sessionID, requestID string) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.invocation_failed",
		Message:   "subagent invocation failed before producing a result",
		SessionID: sessionID,
		Details:   map[string]any{"partialEffectsPossible": true, "requestId": requestID},
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentInvokeError(err error, sessionID, requestID string, timeoutMillis int64, input SubagentInput) ToolResponse[SubagentResult] {
	if errors.Is(err, context.DeadlineExceeded) {
		return subagentInvocationTimeout(sessionID, requestID, timeoutMillis, input)
	}
	return subagentInvocationFailure(sessionID, requestID)
}

func subagentInvocationTimeout(sessionID, requestID string, timeoutMillis int64, input SubagentInput) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.timed_out",
		Message:   "subagent timed out before producing a result; workspace edits may have occurred",
		Retryable: false,
		SessionID: sessionID,
		Details: map[string]any{
			"partialEffectsPossible": true,
			"requestId":              requestID,
			"suggestedAction":        "Inspect the workspace for partial edits before retrying with another configured model or a longer timeout.",
		},
	}
	if input.Provider != "" {
		envelope.Details["provider"] = input.Provider
	}
	if input.Model != "" {
		envelope.Details["model"] = input.Model
	}
	if timeoutMillis > 0 {
		envelope.Details["timeoutMillis"] = timeoutMillis
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}

func subagentTerminalFailure(sessionID string, result factorysessionexecution.InvocationResult, timeoutMillis int64, input SubagentInput) ToolResponse[SubagentResult] {
	envelope := ToolErrorEnvelope{
		Code:      "factory_session.subagent.execution_failed",
		Message:   "subagent execution failed before producing a result",
		SessionID: sessionID,
		Details:   map[string]any{"status": string(result.Status)},
	}
	if result.ErrorCode != "" {
		envelope.Details["invocationCode"] = result.ErrorCode
	}
	if result.Status == factorysessionexecution.InvocationTerminalStatusTimedOut {
		envelope.Code = "factory_session.subagent.timed_out"
		envelope.Message = "subagent timed out before producing a result; workspace edits may have occurred"
		envelope.Details["partialEffectsPossible"] = true
		envelope.Details["suggestedAction"] = "Inspect the workspace for partial edits before retrying with another configured model or a longer timeout."
		if input.Provider != "" {
			envelope.Details["provider"] = input.Provider
		}
		if input.Model != "" {
			envelope.Details["model"] = input.Model
		}
		if timeoutMillis > 0 {
			envelope.Details["timeoutMillis"] = timeoutMillis
		}
		if result.RequestID != "" {
			envelope.Details["requestId"] = result.RequestID
		}
		if result.TraceID != "" {
			envelope.Details["traceId"] = result.TraceID
		}
		if result.WorkID != "" {
			envelope.Details["workId"] = result.WorkID
		}
		return ToolResponse[SubagentResult]{Error: &envelope}
	}
	switch workers.WorkFailureType(result.FailureReason) {
	case workers.WorkFailureTypeThrottled:
		envelope.Code = "factory_session.subagent.provider_throttled"
		envelope.Message = "provider is temporarily unavailable due to usage or capacity limits"
		envelope.Retryable = true
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeThrottled)
	case workers.WorkFailureTypeAuthFailure:
		envelope.Code = "factory_session.subagent.provider_auth_failure"
		envelope.Message = "provider authentication failed"
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeAuthFailure)
	case workers.WorkFailureTypeTimeout:
		envelope.Code = "factory_session.subagent.provider_timeout"
		envelope.Message = "provider request timed out"
		envelope.Retryable = true
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeTimeout)
	case workers.WorkFailureTypeMisconfigured:
		envelope.Code = "factory_session.subagent.provider_misconfigured"
		envelope.Message = "subagent provider is misconfigured; check provider setup and capabilities"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeMisconfigured)
		if input.Provider == "pi" {
			envelope.Details["suggestedAction"] = "Run `pi --version` to verify Pi version; pi-acp requires Pi 0.81.0+. Upgrade via `npm install -g @earendil-works/pi-coding-agent@latest`. Check Pi's selected model endpoint if the version is current."
		} else {
			envelope.Details["suggestedAction"] = "Verify the provider configuration and ensure the required executable or model is available"
		}
	case workers.WorkFailureTypeMissingExecutable:
		envelope.Code = "factory_session.subagent.provider_executable_missing"
		envelope.Message = "required provider executable is unavailable"
		envelope.Retryable = false
		envelope.Details["failureReason"] = string(workers.WorkFailureTypeMissingExecutable)
		envelope.Details["suggestedAction"] = "Install the provider command on PATH or update its configured executable path, then retry."
	}
	return ToolResponse[SubagentResult]{Error: &envelope}
}
