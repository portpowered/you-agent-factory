package contracts

import "context"

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
