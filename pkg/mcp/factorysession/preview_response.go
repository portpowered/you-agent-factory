package factorysession

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

// PreviewToolResponse maps one Factory preview outcome into the stable MCP tool
// response envelope shared by validate and start-preview tools.
func PreviewToolResponse(result factoryapi.FactoryPreviewResult, err error) ToolResponse[factoryapi.FactoryPreviewResult] {
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	if !result.Valid {
		envelope := validationErrorEnvelopeFromPreview(result)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &result}
}
