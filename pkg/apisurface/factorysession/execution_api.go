package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ExecutionAPI adapts factorysessionexecution.Service to apisurface.DurableSessionExecutionAPI.
type ExecutionAPI struct {
	Service factorysessionexecution.Service
}

var _ apisurface.DurableSessionExecutionAPI = (*ExecutionAPI)(nil)

// NewExecutionAPI constructs one durable session execution transport seam over service.
func NewExecutionAPI(service factorysessionexecution.Service) *ExecutionAPI {
	return &ExecutionAPI{Service: service}
}

// StartDurableFactorySessionAsync starts one durable session without waiting for completion.
func (a *ExecutionAPI) StartDurableFactorySessionAsync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionExecutionResponse, error) {
	startReq, err := StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	result, err := a.Service.StartAsync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return AsyncStartResponseToAPI(result), nil
}

// StartDurableFactorySessionSync starts one durable session and waits for sync outcome.
func (a *ExecutionAPI) StartDurableFactorySessionSync(
	ctx context.Context,
	request factoryapi.FactorySessionExecutionRequest,
) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	startReq, err := StartRequestFromAPI(request)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	result, err := a.Service.StartSync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	return SyncStartResponseToAPI(result), nil
}
