package modelmcp

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

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
