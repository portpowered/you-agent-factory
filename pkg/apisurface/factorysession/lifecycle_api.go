package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// LifecycleAPI adapts factorysessionexecution.Service durable session reads to the
// public durable read model. Lifecycle control methods are wired in later stories.
type LifecycleAPI struct {
	Service factorysessionexecution.Service
}

// NewLifecycleAPI constructs one durable session read transport seam over service.
func NewLifecycleAPI(service factorysessionexecution.Service) *LifecycleAPI {
	return &LifecycleAPI{Service: service}
}

// GetDurableFactorySession returns one durable session inspection read model.
func (a *LifecycleAPI) GetDurableFactorySession(
	ctx context.Context,
	sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	result, err := a.Service.GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return SessionReadResponseToAPI(result), nil
}
