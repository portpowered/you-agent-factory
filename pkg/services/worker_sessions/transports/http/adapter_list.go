package http

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ListWorkerSessions verifies Work existence and returns every authoritative
// Worker Session attempt correlated with it. A known Work with no observation
// projection is represented as an empty array rather than a not-found error.
func (a *Adapter) ListWorkerSessions(
	ctx context.Context,
	sessionID string,
	workID string,
) (factoryapi.ListWorkerSessionsResponse, error) {
	if a == nil || a.observations == nil || a.work == nil {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("Worker Sessions and Work services are required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("session id is required")
	}
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("work id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	workModel, err := a.work.GetWork(ctx, sessionID, workID)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}

	return a.listWorkerSessionsForWork(ctx, scope, workID, workModel.Name)
}

func (a *Adapter) listWorkerSessionsForWork(
	ctx context.Context,
	scope workerSessionScope,
	workID string,
	workName string,
) (factoryapi.ListWorkerSessionsResponse, error) {
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("Worker Sessions service is required")
	}
	result, err := observations.ListObservations(ctx, workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		if errors.Is(err, workersessions.ErrObservationWorkNotFound) {
			return factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{}}, nil
		}
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("list Worker Session observations: %w", err)
	}
	sortObservations(result.Observations)
	for index := range result.Observations {
		if result.Observations[index], err = scopeWorkerSessionObservation(result.Observations[index], scope); err != nil {
			return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("scope Worker Session observation: %w", err)
		}
	}
	attribution := make(map[string]workerSessionWorkAttribution, len(result.Observations))
	for _, observation := range result.Observations {
		attribution[observation.WorkerSessionID] = workerSessionWorkAttribution{
			WorkID:   workID,
			WorkName: workName,
		}
	}
	return listWorkerSessionObservationsResponseToAPI(
		workersessions.ListWorkerSessionObservationsResult{Observations: result.Observations},
		attribution,
	), nil
}

// ListTopLevelWorkerSessions returns bounded observations through the stable
// Worker Session identity surface. The Worker Sessions service owns the
// fleet-wide default scope, lifecycle validation, ordering, and cursor semantics.
func (a *Adapter) ListTopLevelWorkerSessions(
	ctx context.Context,
	scope string,
	states []string,
	maxResults *int,
	nextToken *string,
) (factoryapi.ListWorkerSessionsResponse, error) {
	if a == nil || (a.observations == nil && a.topLevel == nil) {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("Worker Sessions service is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	request := workersessions.ListWorkerSessionObservationsRequest{
		Scope:  workersessions.ObservationScope(strings.TrimSpace(scope)),
		States: make([]workersessions.State, 0, len(states)),
	}
	for _, state := range states {
		request.States = append(request.States, workersessions.State(strings.TrimSpace(state)))
	}
	if maxResults != nil {
		request.MaxResults = *maxResults
	}
	if nextToken != nil {
		request.NextToken = strings.TrimSpace(*nextToken)
	}
	topLevel := a.topLevel
	if topLevel == nil {
		topLevel = a.observations
	}
	result, err := topLevel.ListWorkerSessionObservations(ctx, request)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("list top-level Worker Session observations: %w", err)
	}
	attribution, err := a.resolveWorkAttribution(ctx, result.Observations)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	return listWorkerSessionObservationsResponseToAPI(result, attribution), nil
}

// resolveWorkAttribution enriches a Worker Session list without making Work
// state part of the Worker Sessions service contract. A missing Work read is
// an unavailable optional fact: the stable Work ID remains visible and the
// list continues. Context cancellation remains authoritative.
func (a *Adapter) resolveWorkAttribution(
	ctx context.Context,
	observations []workersessions.Observation,
) (map[string]workerSessionWorkAttribution, error) {
	attribution := make(map[string]workerSessionWorkAttribution, len(observations))
	for _, observation := range observations {
		if len(observation.WorkIDs) == 0 || strings.TrimSpace(observation.WorkIDs[0]) == "" {
			continue
		}
		workID := strings.TrimSpace(observation.WorkIDs[0])
		sessionID := strings.TrimSpace(observation.FactorySessionID)
		if sessionID == "" {
			sessionID = workers.DefaultSessionID
		}
		workModel, err := a.work.GetWork(ctx, sessionID, workID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			attribution[observation.WorkerSessionID] = workerSessionWorkAttribution{WorkID: workID}
			continue
		}
		attribution[observation.WorkerSessionID] = workerSessionWorkAttribution{
			WorkID:   workID,
			WorkName: workModel.Name,
		}
	}
	return attribution, nil
}

// sortObservations gives the public list a chronological attempt order while
// retaining stable identity tie-breakers for projections without timestamps.
func sortObservations(observations []workersessions.Observation) {
	sort.SliceStable(observations, func(i, j int) bool {
		left, right := observations[i], observations[j]
		switch {
		case left.StartedAt != nil && right.StartedAt != nil && !left.StartedAt.Equal(*right.StartedAt):
			return left.StartedAt.Before(*right.StartedAt)
		case left.StartedAt != nil && right.StartedAt == nil:
			return true
		case left.StartedAt == nil && right.StartedAt != nil:
			return false
		case left.AttemptID != right.AttemptID:
			return left.AttemptID < right.AttemptID
		default:
			return left.WorkerSessionID < right.WorkerSessionID
		}
	})
}
