package operatorsettingsmcp

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// LoadDocumentInput is the MCP request shape for you.operator_settings.load_document.
type LoadDocumentInput struct {
	Path            string `json:"path"`
	RequireExisting bool   `json:"requireExisting"`
}

// LoadDocument returns detached operator document facts through the
// you.operator_settings.load_document MCP tool.
func LoadDocument(
	ctx context.Context,
	service operatorsettings.Service,
	input LoadDocumentInput,
) ToolResponse[operatorsettings.LoadDocumentResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[operatorsettings.LoadDocumentResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}

	result, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            input.Path,
		RequireExisting: input.RequireExisting,
	})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}
	return ToolResponse[operatorsettings.LoadDocumentResult]{Result: &result}
}
