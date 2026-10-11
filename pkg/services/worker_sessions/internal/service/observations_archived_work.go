package service

import (
	"context"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Exact durable membership extends Work discovery without creating ownership.
// Select the original Factory and opening Work before projecting terminal facts.
func (r *registry) withArchivedWorkObservations(ctx context.Context, req workersessions.ListObservationsRequest, live []workersessions.Observation) (workersessions.ListObservationsResult, error) {
	if r.logs != nil && req.FactorySessionID != "" && req.FactorySessionID != "~default" {
		owners := make(map[historyIdentity]struct{})
		indices := make(map[historyIdentity]int, len(live))
		for index, row := range live {
			identity := identityOfHistory(row)
			indices[identity] = index
			if !row.State.Terminal() {
				owners[identity] = struct{}{}
			}
		}
		retained, err := r.logs.readArchivedHistory(ctx, workersessions.ListWorkerSessionObservationsRequest{
			FactorySessionID: req.FactorySessionID,
		}, owners, req.WorkID)
		if err != nil {
			return workersessions.ListObservationsResult{}, err
		}
		for _, row := range retained {
			row = r.withContinuationCapability(ctx, row)
			if index, exists := indices[identityOfHistory(row)]; exists {
				live[index] = row
			} else {
				live = append(live, row)
			}
		}
	}
	if err := observationContextError(ctx); err != nil {
		return workersessions.ListObservationsResult{}, err
	}
	if len(live) == 0 {
		return workersessions.ListObservationsResult{}, workersessions.ErrObservationWorkNotFound
	}
	sortObservationAttempts(live)
	r.logger.Info("worker session archived Work observation list", "factorySessionID", req.FactorySessionID, "workID", req.WorkID, "result_count", len(live))
	return workersessions.ListObservationsResult{Observations: live}, nil
}
