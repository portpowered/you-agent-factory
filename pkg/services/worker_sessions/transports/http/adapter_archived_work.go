package http

import (
	"context"
	"errors"
	"slices"
	"sort"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Durable membership is read-only authority. A default alias cannot identify an
// original capture owner, and a missing witness preserves the Work read error.
func (a *Adapter) listArchivedWorkerSessionsForWork(ctx context.Context, sessionID, workID string, missing error) (factoryapi.ListWorkerSessionsResponse, error) {
	result, err := a.mergeArchivedWorkerSessionsForWork(ctx, sessionID, workID, factoryapi.ListWorkerSessionsResponse{})
	if err != nil {
		return result, err
	}
	if len(result.Sessions) == 0 {
		return factoryapi.ListWorkerSessionsResponse{}, missing
	}
	return result, nil
}

func (a *Adapter) mergeArchivedWorkerSessionsForWork(ctx context.Context, sessionID, workID string, live factoryapi.ListWorkerSessionsResponse) (factoryapi.ListWorkerSessionsResponse, error) {
	if a.logs == nil || sessionID == defaultFactorySessionAlias {
		return live, nil
	}
	page, err := a.logs.ListObservations(ctx, workersessions.ListObservationsRequest{FactorySessionID: sessionID, WorkID: workID})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return factoryapi.ListWorkerSessionsResponse{}, ctxErr
	}
	if errors.Is(err, workersessions.ErrObservationWorkNotFound) {
		return live, nil
	}
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	retained, selected := archivedWorkObservations(page.Observations, sessionID, workID)
	attribution, err := a.resolveWorkAttribution(ctx, selected)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	// Captured physical facts replace a matching ended runtime row. Attempts
	// with a different identity remain distinct, including repeated Work runs.
	for _, row := range retained {
		mapped := workerSessionObservationWithWorkToAPI(row, attribution[row.WorkerSessionID])
		live.Sessions = replaceArchivedWorkRow(live.Sessions, mapped, sessionID)
	}
	sortArchivedWorkRows(live.Sessions)
	return live, ctx.Err()
}

func archivedWorkObservations(rows []workersessions.Observation, sessionID, workID string) ([]workersessions.Observation, []workersessions.Observation) {
	retained := make([]workersessions.Observation, 0, len(rows))
	for _, row := range rows {
		if row.FactorySessionID == sessionID && slices.Contains(row.WorkIDs, workID) {
			retained = append(retained, row)
		}
	}
	selected := make([]workersessions.Observation, len(retained))
	for index, row := range retained {
		selected[index] = row
		selected[index].WorkIDs = []string{workID}
	}
	return retained, selected
}

func sortArchivedWorkRows(rows []factoryapi.WorkerSessionObservation) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		if left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt) {
			return left.StartedAt.Before(*right.StartedAt)
		}
		if (left.StartedAt == nil) != (right.StartedAt == nil) {
			return left.StartedAt != nil
		}
		if left.AttemptId != right.AttemptId {
			return left.AttemptId < right.AttemptId
		}
		return left.WorkerSessionId < right.WorkerSessionId
	})
}

func replaceArchivedWorkRow(rows []factoryapi.WorkerSessionObservation, mapped factoryapi.WorkerSessionObservation, sessionID string) []factoryapi.WorkerSessionObservation {
	for index, existing := range rows {
		if existing.WorkerSessionId == mapped.WorkerSessionId && existing.AttemptId == mapped.AttemptId && existing.FactorySessionId != nil && *existing.FactorySessionId == sessionID {
			rows[index] = mapped
			return rows
		}
	}
	return append(rows, mapped)
}
