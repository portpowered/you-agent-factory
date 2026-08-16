package composition

import (
	"context"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type factoryStatusSessionReader interface {
	ObserveForSession(
		context.Context,
		string,
		factoryruntime.ObserveRequest,
	) (factoryruntime.ObserveResult, error)
}

type factoryStatusAPI struct {
	sessions  factoryStatusSessionReader
	projector factoryruntime.FactoryStatusProjector
}

// newFactoryStatusAPI binds status projection to the Factory Sessions session
// router. Factory Sessions owns session identity; the selected session gateway
// owns the live observation.
func newFactoryStatusAPI(
	sessions factoryStatusSessionReader,
	projector factoryruntime.FactoryStatusProjector,
) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{sessions: sessions, projector: projector}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if api == nil || api.sessions == nil || api.projector == nil {
		return factoryruntime.FactoryStatus{}, factoryruntime.ErrNotRunning
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	result, err := api.sessions.ObserveForSession(ctx, sessionID, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return api.projector.ProjectFactoryStatusFromObservation(result.Observation), nil
}
