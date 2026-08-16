package composition

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type factoryStatusSessionReader interface {
	ObserveForSession(context.Context, string, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error)
}

type factoryStatusAPI struct {
	runtime  factoryruntime.Service
	sessions factoryStatusSessionReader
}

func newFactoryStatusAPI(
	runtime factoryruntime.Service,
	sessions factoryStatusSessionReader,
) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{runtime: runtime, sessions: sessions}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if sessionID == "" {
		if api.sessions != nil {
			result, err := api.sessions.ObserveForSession(ctx, factorysessions.DefaultSessionID, factoryruntime.ObserveRequest{
				Scope: factoryruntime.ObservationScopeFull,
			})
			if err != nil {
				return factoryruntime.FactoryStatus{}, err
			}
			return factoryruntime.FactoryStatusFromObservation(result.Observation), nil
		}
		result, err := api.runtime.Observe(ctx, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScopeFull,
		})
		if err != nil {
			return factoryruntime.FactoryStatus{}, err
		}
		return factoryruntime.FactoryStatusFromObservation(result.Observation), nil
	}

	result, err := api.sessions.ObserveForSession(ctx, sessionID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return factoryruntime.FactoryStatusFromObservation(result.Observation), nil
}
