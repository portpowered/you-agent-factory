package composition

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type factoryStatusSessionReader interface {
	GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.StateSnapshot, error)
}

type factoryStatusAPI struct {
	runtime   factoryruntime.APIFactory
	sessions  factoryStatusSessionReader
	projector factoryruntime.FactoryStatusProjector
}

func newFactoryStatusAPI(
	runtime factoryruntime.APIFactory,
	sessions factoryStatusSessionReader,
	projector factoryruntime.FactoryStatusProjector,
) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{runtime: runtime, sessions: sessions, projector: projector}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	var (
		snapshot *factoryruntime.StateSnapshot
		err      error
	)
	if sessionID == "" {
		snapshot, err = api.runtime.GetEngineStateSnapshot(ctx)
	} else {
		snapshot, err = api.sessions.GetEngineStateSnapshotForSession(ctx, sessionID)
	}
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return api.projector.ProjectFactoryStatus(snapshot), nil
}
