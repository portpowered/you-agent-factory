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
type observationService interface {
	ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error)
	GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error)
	GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error)
	ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error)
	StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error)
	StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error)
}

type Adapter struct {
	observations observationService
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

// ReadWorkerSessionTranscript returns the normalized transcript for one
// terminal Worker Session identified by its exact Provider Session reference.
func (a *Adapter) ReadWorkerSessionTranscript(
	ctx context.Context,
	sessionID, provider, kind, id string,
) (factoryapi.WorkerSessionTranscriptResponse, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("session id is required")
	}
	provider = strings.TrimSpace(provider)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if provider == "" || kind == "" || id == "" {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("provider, kind, and id are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	result, err := a.observations.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("read Worker Session transcript: %w", err)
	}
	return WorkerSessionTranscriptToAPI(result), nil
}

// StreamWorkerSessionEvents returns the detached identity envelope together
// with the canonical retained/live subscription for the exact Provider
// Session reference. The caller owns closing the subscription.
func (a *Adapter) StreamWorkerSessionEvents(
	ctx context.Context,
	sessionID, provider, kind, id string,
	replayOnly bool,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("session id is required")
	}
	provider = strings.TrimSpace(provider)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if provider == "" || kind == "" || id == "" {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("provider, kind, and id are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}

	request := workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	}
	observation, err := a.observations.GetObservation(ctx, request)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	subscription, err := a.observations.StreamObservations(ctx, workersessions.StreamObservationsRequest{
		ProviderSession: request.ProviderSession,
		// Carry the documented default explicitly so the canonical ledger
		// receives the bounded stream policy at the transport boundary.
		Limit:      workersessions.DefaultObservationStreamLimit,
		ReplayOnly: replayOnly,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("stream Worker Session events: %w", err)
	}
	if subscription.NextFunc == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	return WorkerSessionObservationToAPI(observation), subscription, nil
}

// StreamWorkerSessionEventsByWorkerSessionID returns the detached identity
// envelope together with the canonical retained/live subscription for a
// Worker Session that may not have a Provider Session reference.
func (a *Adapter) StreamWorkerSessionEventsByWorkerSessionID(
	ctx context.Context,
	sessionID, workerSessionID string,
	replayOnly bool,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("session id is required")
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("worker session id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}
	observation, err := a.observations.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	subscription, err := a.observations.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
		Limit:           workersessions.DefaultObservationStreamLimit,
		ReplayOnly:      replayOnly,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("stream Worker Session events: %w", err)
	}
	if subscription.NextFunc == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	return WorkerSessionObservationToAPI(observation), subscription, nil
}

// NewAdapter binds the exact roots required by the Worker Sessions list
// operation.
func NewAdapter(
	observations observationService,
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
