// Package http owns the Worker Sessions HTTP representation boundary.
package http

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps Worker Sessions observation projections to the generated HTTP
// contract. Work remains the authority for deciding whether the requested Work
// exists; Worker Sessions remains the authority for correlated attempts.
type Adapter struct {
	observations workersessions.ObservationService
	work         work.Service
}

// GetWorkerSessionObservation verifies the request identity and returns the
// one authoritative observation associated with it in the opened session.
func (a *Adapter) GetWorkerSessionObservation(
	ctx context.Context,
	sessionID, provider, kind, id string,
) (factoryapi.WorkerSessionObservation, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionObservation{}, errors.New("session id is required")
	}
	provider = strings.TrimSpace(provider)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if provider == "" || kind == "" || id == "" {
		return factoryapi.WorkerSessionObservation{}, errors.New("provider, kind, and id are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	observation, err := a.observations.GetObservation(ctx, workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	return WorkerSessionObservationToAPI(observation), nil
}

// StreamWorkerSessionEvents returns the detached identity envelope together
// with the canonical retained/live subscription for the exact Provider
// Session reference. The caller owns closing the subscription.
func (a *Adapter) StreamWorkerSessionEvents(
	ctx context.Context,
	sessionID, provider, kind, id string,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, nil, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionObservation{}, nil, errors.New("session id is required")
	}
	provider = strings.TrimSpace(provider)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if provider == "" || kind == "" || id == "" {
		return factoryapi.WorkerSessionObservation{}, nil, errors.New("provider, kind, and id are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, nil, err
	}

	request := workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	}
	observation, err := a.observations.GetObservation(ctx, request)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, nil, fmt.Errorf("get Worker Session observation: %w", err)
	}
	subscription, err := a.observations.StreamObservations(ctx, workersessions.StreamObservationsRequest{
		ProviderSession: request.ProviderSession,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, nil, fmt.Errorf("stream Worker Session events: %w", err)
	}
	if subscription == nil {
		return factoryapi.WorkerSessionObservation{}, nil, workersessions.ErrObservationSourceUnavailable
	}
	return WorkerSessionObservationToAPI(observation), subscription, nil
}

// NewAdapter binds the exact roots required by the Worker Sessions list
// operation.
func NewAdapter(
	observations workersessions.ObservationService,
	workRoot work.Service,
) *Adapter {
	if observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{observations: observations, work: workRoot}
}

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
	if _, err := a.work.GetWork(ctx, sessionID, workID); err != nil {
		return factoryapi.ListWorkerSessionsResponse{}, err
	}

	result, err := a.observations.ListObservations(ctx, workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		if errors.Is(err, workersessions.ErrObservationWorkNotFound) {
			return factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{}}, nil
		}
		return factoryapi.ListWorkerSessionsResponse{}, fmt.Errorf("list Worker Session observations: %w", err)
	}
	sortObservations(result.Observations)
	return ListWorkerSessionsResponseToAPI(result), nil
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
