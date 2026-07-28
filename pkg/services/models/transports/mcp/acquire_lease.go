package modelmcp

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

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
