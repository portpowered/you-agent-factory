package modelmcp

import (
	"context"

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
		envelope := executionErrorEnvelope(err)
		return ToolResponse[models.ListModelsResult]{Error: &envelope}
	}
	return ToolResponse[models.ListModelsResult]{Result: &result}
}
