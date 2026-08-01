package modelmcp

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// ListCatalogInput is the MCP request shape for you.model.list_catalog.
type ListCatalogInput struct {
	RuntimeScopeRef string `json:"runtimeScopeRef"`
}

// ListCatalog returns detached catalog summaries through the you.model.list_catalog
// MCP tool.
func ListCatalog(
	ctx context.Context,
	service models.Service,
	input ListCatalogInput,
) ToolResponse[models.ListModelsResult] {
	if response, done := requestContextErrorResponse[models.ListModelsResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[models.ListModelsResult]{Error: &envelope}
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(input.RuntimeScopeRef)
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode list catalog input", err)
		return ToolResponse[models.ListModelsResult]{Error: &envelope}
	}
	result, err := service.ListCatalog(ctx, models.ListModelsRequest{Scope: scope})
	if err != nil {
		envelope := listCatalogErrorEnvelope(err)
		return ToolResponse[models.ListModelsResult]{Error: &envelope}
	}
	return ToolResponse[models.ListModelsResult]{Result: &result}
}

// PrepareAssetsInput is the MCP request shape for you.model.prepare_assets.
type PrepareAssetsInput struct {
	RuntimeScopeRef string `json:"runtimeScopeRef"`
	Name            string `json:"name"`
}

// PrepareAssets makes scoped model assets available through the
// you.model.prepare_assets MCP tool.
func PrepareAssets(
	ctx context.Context,
	service models.Service,
	input PrepareAssetsInput,
) ToolResponse[models.PrepareModelAssetsResult] {
	if response, done := requestContextErrorResponse[models.PrepareModelAssetsResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[models.PrepareModelAssetsResult]{Error: &envelope}
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(input.RuntimeScopeRef)
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode prepare assets input", err)
		return ToolResponse[models.PrepareModelAssetsResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Name) == "" {
		envelope := decodeInputErrorEnvelope("decode prepare assets input", errEmptyModelName)
		return ToolResponse[models.PrepareModelAssetsResult]{Error: &envelope}
	}
	result, err := service.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope: scope,
		Name:  input.Name,
	})
	if err != nil {
		envelope := prepareAssetsErrorEnvelope(err)
		return ToolResponse[models.PrepareModelAssetsResult]{Error: &envelope}
	}
	return ToolResponse[models.PrepareModelAssetsResult]{Result: &result}
}

// AcquireLeaseInput is the MCP request shape for you.model.acquire_lease.
type AcquireLeaseInput struct {
	RuntimeScopeRef string `json:"runtimeScopeRef"`
	Name            string `json:"name"`
	Holder          string `json:"holder"`
}

// AcquireLease reserves scoped model capacity through the
// you.model.acquire_lease MCP tool.
func AcquireLease(
	ctx context.Context,
	service models.Service,
	input AcquireLeaseInput,
) ToolResponse[AcquireLeaseResult] {
	if response, done := requestContextErrorResponse[AcquireLeaseResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[AcquireLeaseResult]{Error: &envelope}
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(input.RuntimeScopeRef)
	if err != nil {
		envelope := decodeInputErrorEnvelope("decode acquire lease input", err)
		return ToolResponse[AcquireLeaseResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Name) == "" {
		envelope := decodeInputErrorEnvelope("decode acquire lease input", errEmptyModelName)
		return ToolResponse[AcquireLeaseResult]{Error: &envelope}
	}
	if strings.TrimSpace(input.Holder) == "" {
		envelope := decodeInputErrorEnvelope("decode acquire lease input", errEmptyLeaseHolder)
		return ToolResponse[AcquireLeaseResult]{Error: &envelope}
	}
	result, err := service.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   input.Name,
		Holder: input.Holder,
	})
	if err != nil {
		envelope := acquireLeaseErrorEnvelope(err)
		return ToolResponse[AcquireLeaseResult]{Error: &envelope}
	}
	projected := projectAcquireLeaseResult(result)
	return ToolResponse[AcquireLeaseResult]{Result: &projected}
}

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
