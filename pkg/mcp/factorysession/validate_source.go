package factorysession

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

// ValidateSource runs the canonical Factory preview contract for the
// you.factory_session.validate_source MCP tool without provider execution.
func ValidateSource(input factoryapi.FactoryPreviewRequest) ToolResponse[factoryapi.FactoryPreviewResult] {
	previewInput, err := apisurface.FactoryPreviewRequestFromAPI(input)
	if err != nil {
		envelope := requestValidationErrorEnvelope(err)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}

	preview := apisurface.FactoryPreviewResultFromPreview(apisurface.BuildFactoryPreview(previewInput))
	if !preview.Valid {
		envelope := validationErrorEnvelopeFromPreview(preview)
		return ToolResponse[factoryapi.FactoryPreviewResult]{Error: &envelope}
	}
	return ToolResponse[factoryapi.FactoryPreviewResult]{Result: &preview}
}
