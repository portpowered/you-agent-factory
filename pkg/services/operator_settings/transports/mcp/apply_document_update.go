package operatorsettingsmcp

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// DocumentProviderModelInput is the MCP request shape for provider/model updates.
type DocumentProviderModelInput struct {
	Provider *string `json:"provider,omitempty"`
	Model    *string `json:"model,omitempty"`
}

// ApplyDocumentUpdateInput is the MCP request shape for
// you.operator_settings.apply_document_update.
type ApplyDocumentUpdateInput struct {
	Path                 string                     `json:"path"`
	ExpectedBackendScope string                     `json:"expectedBackendScope"`
	ProviderModel        DocumentProviderModelInput `json:"providerModel"`
}

// ApplyDocumentUpdate applies one semantic operator document update through the
// you.operator_settings.apply_document_update MCP tool.
func ApplyDocumentUpdate(
	ctx context.Context,
	service operatorsettings.Service,
	input ApplyDocumentUpdateInput,
) ToolResponse[operatorsettings.ApplyDocumentUpdateResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[operatorsettings.ApplyDocumentUpdateResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[operatorsettings.ApplyDocumentUpdateResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[operatorsettings.ApplyDocumentUpdateResult]{Error: &envelope}
	}

	result, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:                 input.Path,
		ExpectedBackendScope: input.ExpectedBackendScope,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{
			Provider: input.ProviderModel.Provider,
			Model:    input.ProviderModel.Model,
		},
	})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[operatorsettings.ApplyDocumentUpdateResult]{Error: &envelope}
	}
	return ToolResponse[operatorsettings.ApplyDocumentUpdateResult]{Result: &result}
}
