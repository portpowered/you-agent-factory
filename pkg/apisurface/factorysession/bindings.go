package factorysession

import (
	"context"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// DurableSessionBindings groups the mock-backed durable session transport seams
// constructed from one shared service instance.
type DurableSessionBindings struct {
	Execution apisurface.DurableSessionExecutionAPI
	Listing   apisurface.DurableSessionListingAPI
	Read      DurableSessionReadAPI
}

// DurableSessionReadAPI is the durable session inspection read seam used by
// GET /factory-sessions/{session_id} before lifecycle control routes are wired.
type DurableSessionReadAPI interface {
	GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error)
}

// NewDurableSessionBindings constructs execution, listing, and read seams over one service.
func NewDurableSessionBindings(service factorysessionexecution.Service) DurableSessionBindings {
	lifecycle := NewLifecycleAPI(service)
	return DurableSessionBindings{
		Execution: NewExecutionAPI(service),
		Listing:   NewListingAPI(service),
		Read:      lifecycle,
	}
}
