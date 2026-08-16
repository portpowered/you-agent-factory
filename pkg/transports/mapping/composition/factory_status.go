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
	sessions factoryStatusSessionReader
}

// newFactoryStatusAPI takes only the session reader: Bind rejects a nil
// factorysessions.Service before constructing this API, so every Factory status
// projection -- current Factory included -- resolves through Factory Sessions.
// There is no legacy Factory Runtime observation fallback left to reach.
func newFactoryStatusAPI(sessions factoryStatusSessionReader) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{sessions: sessions}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}

	result, err := api.sessions.ObserveForSession(ctx, sessionID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return factoryruntime.FactoryStatusFromObservation(result.Observation), nil
}
