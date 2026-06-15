package service

import (
	"context"
	"errors"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var _ apisurface.DurableSessionListingAPI = (*FactoryService)(nil)

func (fs *FactoryService) ListDurableFactorySessions(
	ctx context.Context,
	params factoryapi.ListFactorySessionsParams,
) (factoryapi.ListFactorySessionsResponse, error) {
	req, err := factorysession.ListSessionsRequestFromAPI(params)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	result, err := fs.ListDurableExecutionSessions(ctx, req)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return factorysession.ListSessionsResponseToAPI(result), nil
}

// ListDurableExecutionSessions returns the shared durable session listing projection
// used by API merge logic before workspace rows are combined.
func (fs *FactoryService) ListDurableExecutionSessions(
	ctx context.Context,
	req factorysessionexecution.ListSessionsRequest,
) (factorysessionexecution.ListSessionsResult, error) {
	return fs.durableExecutionService().ListSessions(ctx, req)
}

func (fs *FactoryService) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	result, err := fs.durableExecutionService().GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return factorysession.SessionReadResponseToAPI(result), nil
}

func (fs *FactoryService) GetDurableFactorySessionResult(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetFactorySessionResultsParams,
) (factoryapi.FactorySessionResult, error) {
	req, err := factorysession.ResultRequestFromAPI(params)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	result, err := fs.durableExecutionService().GetResult(ctx, sessionID, req)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	return factorysession.ResultResponseToAPI(result), nil
}

func (fs *FactoryService) ReadDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	params factoryapi.GetEventsBySessionIdParams,
) (*interfaces.FactoryEventStream, error) {
	reconnect, err := factorysession.EventReconnectRequestFromAPI(params)
	if err != nil {
		return nil, err
	}
	result, err := fs.durableExecutionService().ReadEvents(ctx, sessionID, reconnect)
	if err != nil {
		if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
			return nil, fmt.Errorf("%w: %v", apisurface.ErrInvalidEventReconnectCursor, err)
		}
		return nil, err
	}
	return factorysession.FactoryEventStreamFromReadResult(result), nil
}
