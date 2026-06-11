package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// ListingAPI adapts factorysessionexecution.Service to apisurface.DurableSessionListingAPI.
type ListingAPI struct {
	Service factorysessionexecution.Service
}

var _ apisurface.DurableSessionListingAPI = (*ListingAPI)(nil)

// NewListingAPI constructs one durable session listing transport seam over service.
func NewListingAPI(service factorysessionexecution.Service) *ListingAPI {
	return &ListingAPI{Service: service}
}

// ListDurableFactorySessions lists durable and in-process execution sessions for one scope.
func (a *ListingAPI) ListDurableFactorySessions(
	ctx context.Context,
	params factoryapi.ListFactorySessionsParams,
) (factoryapi.ListFactorySessionsResponse, error) {
	request, err := ListSessionsRequestFromAPI(params)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	result, err := a.Service.ListSessions(ctx, request)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return ListSessionsResponseToAPI(result), nil
}
