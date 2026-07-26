package composition

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

type factoryStatusSessionReader interface {
	GetEngineStateSnapshotForSession(context.Context, string) (*factoryruntime.LegacyEngineObservation, error)
}

type factoryStatusAPI struct {
	runtime   factoryruntime.Service
	sessions  factoryStatusSessionReader
	projector factoryruntime.FactoryStatusProjector
}

func newFactoryStatusAPI(
	runtime factoryruntime.Service,
	sessions factoryStatusSessionReader,
	projector factoryruntime.FactoryStatusProjector,
) apisurface.FactoryStatusAPI {
	return &factoryStatusAPI{runtime: runtime, sessions: sessions, projector: projector}
}

func (api *factoryStatusAPI) ProjectFactoryStatus(ctx context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
	if sessionID == "" {
		result, err := api.runtime.Observe(ctx, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScopeFull,
		})
		if err != nil {
			return factoryruntime.FactoryStatus{}, err
		}
		return factoryStatusFromObservation(result.Observation), nil
	}

	snapshot, err := api.sessions.GetEngineStateSnapshotForSession(ctx, sessionID)
	if err != nil {
		return factoryruntime.FactoryStatus{}, err
	}
	return api.projector.ProjectFactoryStatus(snapshot), nil
}

func factoryStatusFromObservation(observation factoryruntime.Observation) factoryruntime.FactoryStatus {
	resources := make([]factoryruntime.FactoryResourceUsage, 0, len(observation.Resources))
	for _, resource := range observation.Resources {
		resources = append(resources, factoryruntime.FactoryResourceUsage{
			Available: resource.AvailableCount,
			Name:      resource.ResourceID,
			Total:     resource.AvailableCount + resource.InUseCount,
		})
	}
	return factoryruntime.FactoryStatus{
		Categories: factoryruntime.FactoryStatusCategories{
			Failed:     observation.Progress.WorkCategories.Failed,
			Initial:    observation.Progress.WorkCategories.Initial,
			Processing: observation.Progress.WorkCategories.Processing,
			Terminal:   observation.Progress.WorkCategories.Terminal,
		},
		FactoryState:           observation.Health.FactoryState,
		LifecycleControlStatus: observation.Health.LifecycleControlStatus,
		Resources:              resources,
		RuntimeStatus:          string(observation.Status),
		TotalTokens:            observation.Progress.TotalWorkCount,
	}
}
