package contracts

import (
	"context"
	sessionprojectionfacts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/sessionprojectionfacts"
)

// WorkerWorkAttributionRequest selects an existing captured association.
type WorkerWorkAttributionRequest struct {
	WorkerSessionID  string
	FactorySessionID string
	WorkID           string
}

// WorkerWorkAttribution retains only an explicit recorded name.
type WorkerWorkAttribution struct {
	WorkerSessionID    string
	FactorySessionID   string
	WorkID             string
	WorkName           string
	HistoryUnavailable bool
}

// WorkerWorkAttributionReader is the read-only, query-local batch capability.
type WorkerWorkAttributionReader interface {
	ResolveWorkerWorkAttribution(context.Context, []WorkerWorkAttributionRequest) ([]WorkerWorkAttribution, error)
}

// WorkOriginProjectionReader selects retained Work ancestry and named, typed Work
// identities within one Factory Session without replaying its recording.
type WorkOriginProjectionReader interface {
	CurrentWorkOriginFacts(context.Context, string, string, string) (sessionprojectionfacts.WorkOriginFacts, error)
}
