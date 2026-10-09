package http

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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
	workModel, err := a.work.ResolveWorkerSessionWork(ctx, sessionID, workID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return factoryapi.ListWorkerSessionsResponse{}, ctxErr
		}
		if !errors.Is(err, work.ErrWorkNotFound) {
			if _, scopeErr := a.resolveWorkerSessionScope(ctx, sessionID); scopeErr != nil {
				return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("resolve Factory Session scope: %w", scopeErr)
			}
		}
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}

	return a.listWorkerSessionsForWork(ctx, scope, workID, workModel.Name)
}

func (a *Adapter) listWorkerSessionsForWork(
	ctx context.Context,
	scope workerSessionScope,
	workID string,
	workName string,
) (factoryapi.ListWorkerSessionsResponse, error) {
	if err := ctx.Err(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.ListWorkerSessionsResponse{}, errors.New("Worker Sessions service is required")
	}
	result, err := observations.ListObservations(ctx, workersessions.ListObservationsRequest{
		FactorySessionID: scope.effectiveID,
		WorkID:           workID,
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return factoryapi.ListWorkerSessionsResponse{}, ctxErr
	}
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
	history string,
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
		Scope:   workersessions.ObservationScope(strings.TrimSpace(scope)),
		History: workersessions.ObservationHistory(history),
		States:  make([]workersessions.State, 0, len(states)),
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
	if err := request.Validate(); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
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
	// Each session read clones its runtime snapshot. Share that read only within
	// this request, including failed reads, and keep session identities separate.
	namesBySession := make(map[string]map[string]string)
	available := make(map[string]bool)
	requests := make([]recordings.WorkerWorkAttributionRequest, 0, len(observations))
	for _, observation := range observations {
		if len(observation.WorkIDs) == 0 || strings.TrimSpace(observation.WorkIDs[0]) == "" {
			continue
		}
		workID := strings.TrimSpace(observation.WorkIDs[0])
		sessionID := strings.TrimSpace(observation.FactorySessionID)
		attribution[observation.WorkerSessionID] = workerSessionWorkAttribution{WorkID: workID}
		if sessionID == "" {
			continue // An absent association cannot authorize default-scope enrichment.
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		names, loaded := namesBySession[sessionID]
		if !loaded {
			names = make(map[string]string)
			if a.work != nil {
				// No filters or pagination: include superseded Work just as GetWork
				// does, so optional names do not depend on the current Work state.
				result, err := a.work.ListWork(ctx, sessionID, work.ListOptions{
					IncludeSuperseded: true, MaxResults: math.MaxInt,
				})
				if err == nil {
					available[sessionID] = true
					for _, model := range result.Results {
						if _, exists := names[model.WorkID]; !exists {
							names[model.WorkID] = model.Name
						}
					}
					// Exact cursor identity takes precedence over Work identity,
					// matching the Work service's selected-read lookup.
					for _, model := range result.Results {
						if model.CursorID != "" {
							names[model.CursorID] = model.Name
						}
					}
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			namesBySession[sessionID] = names
		}
		attribution[observation.WorkerSessionID] = workerSessionWorkAttribution{WorkID: workID, WorkName: names[workID]}
		if !available[sessionID] {
			requests = append(requests, recordings.WorkerWorkAttributionRequest{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: sessionID, WorkID: workID})
		}
	}
	if len(requests) > 0 && a.attribution != nil {
		results, err := a.attribution.ResolveWorkerWorkAttribution(ctx, requests)
		if err != nil {
			return nil, err
		}
		for _, result := range results {
			attribution[result.WorkerSessionID] = workerSessionWorkAttribution{WorkID: result.WorkID, WorkName: result.WorkName, HistoryUnavailable: result.HistoryUnavailable}
		}
	}
	return attribution, nil
}

func (a *Adapter) presentWorkerSessionObservation(ctx context.Context, observation workersessions.Observation) (factoryapi.WorkerSessionObservation, error) {
	attribution, err := a.resolveWorkAttribution(ctx, []workersessions.Observation{observation})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	return workerSessionObservationWithWorkToAPI(observation, attribution[observation.WorkerSessionID]), nil
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
