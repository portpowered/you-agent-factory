package composition

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type factoryStatusAPI struct {
	runtime   factoryruntime.Service
	projector factoryruntime.FactoryStatusProjector
}

// newFactoryStatusAPI binds status projection to the already-opened Runtime
// capability. Factory Sessions owns session identity and controls; Runtime
// owns the live observation itself.
func newFactoryStatusAPI(runtime factoryruntime.Service, projector factoryruntime.FactoryStatusProjector) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{runtime: runtime, projector: projector}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	_ = sessionID
	if api == nil || api.runtime == nil || api.projector == nil {
		return factoryruntime.FactoryStatus{}, factoryruntime.ErrNotRunning
	}
	result, err := api.runtime.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return api.projector.ProjectFactoryStatusFromObservation(result.Observation), nil
}
