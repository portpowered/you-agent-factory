package modelmcp

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// InferenceInputPayload is the nested MCP inference input for invoke_with_lease.
type InferenceInputPayload struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// InvokeWithLeaseInput is the MCP request shape for you.model.invoke_with_lease.
type InvokeWithLeaseInput struct {
	RuntimeScopeRef string                `json:"runtimeScopeRef"`
	LeaseRef        string                `json:"leaseRef"`
	Holder          string                `json:"holder"`
	ModelName       string                `json:"modelName"`
	Operation       string                `json:"operation"`
	ResponseMode    string                `json:"responseMode,omitempty"`
	Input           InferenceInputPayload `json:"input"`
}

// InvokeWithLease runs one scoped model operation under an issued lease through
// the you.model.invoke_with_lease MCP tool.
func InvokeWithLease(
	ctx context.Context,
	service models.Service,
	input InvokeWithLeaseInput,
) ToolResponse[InvokeWithLeaseResult] {
	if response, done := requestContextErrorResponse[InvokeWithLeaseResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(input.RuntimeScopeRef)
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode invoke with lease input", err)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	lease, err := (models.ModelLeaseRef{}).Parse(input.LeaseRef)
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode invoke with lease input", err)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Holder) == "" {
		envelope := decodeInputErrorEnvelope("decode invoke with lease input", errEmptyLeaseHolder)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.ModelName) == "" {
		envelope := decodeInputErrorEnvelope("decode invoke with lease input", errEmptyModelName)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Operation) == "" {
		envelope := decodeInputErrorEnvelope("decode invoke with lease input", errEmptyModelOperation)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	request := models.InvokeModelRequest{
		Scope:     scope,
		Lease:     lease,
		Holder:    input.Holder,
		ModelName: input.ModelName,
		Operation: input.Operation,
		Input: models.InferenceInput{
			ContentType: input.Input.ContentType,
			Content:     input.Input.Content,
		},
	}
	if strings.TrimSpace(input.ResponseMode) != "" {
		request.ResponseMode = models.ResponseMode(input.ResponseMode)
	}
	result, err := service.InvokeModelWithLease(ctx, request)
	if err != nil {
		envelope := invokeWithLeaseErrorEnvelope(err)
		return ToolResponse[InvokeWithLeaseResult]{Error: &envelope}
	}
	projected := projectInvokeWithLeaseResult(result)
	return ToolResponse[InvokeWithLeaseResult]{Result: &projected}
}
