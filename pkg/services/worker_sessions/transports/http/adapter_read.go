package http

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type workerSessionScope struct {
	requestedID  string
	effectiveID  string
	defaultScope bool
}

// GetWorkerSessionObservation verifies the request identity and returns the
// one authoritative observation associated with it in the opened session.
func (a *Adapter) GetWorkerSessionObservation(
	ctx context.Context,
	sessionID, provider, kind, id string,
) (factoryapi.WorkerSessionObservation, error) {
	normalizedProvider, normalizedKind, normalizedID, err := validateProviderReference(a, sessionID, provider, kind, id)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	provider, kind, id = normalizedProvider, normalizedKind, normalizedID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionObservation{}, errors.New("Worker Sessions service is required")
	}
	observation, err := observations.GetObservation(ctx, workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if observation, err = scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	return WorkerSessionObservationToAPI(observation), nil
}

// GetWorkerSessionObservationByWorkerSessionID returns the canonical
// Worker-ID observation for the explicitly opened Factory Session. Provider
// Session association is enrichment only and is never required at this
// boundary.
func (a *Adapter) GetWorkerSessionObservationByWorkerSessionID(
	ctx context.Context,
	sessionID, workerSessionID string,
) (factoryapi.WorkerSessionObservation, error) {
	normalizedWorkerSessionID, err := validateWorkerSessionReference(a, sessionID, workerSessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	workerSessionID = normalizedWorkerSessionID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionObservation{}, errors.New("Worker Sessions service is required")
	}
	observation, err := observations.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if observation, err = scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	return WorkerSessionObservationToAPI(observation), nil
}

// ReadWorkerSessionTranscript returns the normalized transcript for one
// terminal Worker Session identified by its exact Provider Session reference.
func (a *Adapter) ReadWorkerSessionTranscript(
	ctx context.Context,
	sessionID, provider, kind, id string,
) (factoryapi.WorkerSessionTranscriptResponse, error) {
	normalizedProvider, normalizedKind, normalizedID, err := validateProviderReference(a, sessionID, provider, kind, id)
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	provider, kind, id = normalizedProvider, normalizedKind, normalizedID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("Worker Sessions service is required")
	}
	observation, err := observations.GetObservation(ctx, workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if _, err := scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	result, err := observations.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("read Worker Session transcript: %w", err)
	}
	response := WorkerSessionTranscriptToAPI(result)
	response.FactorySessionId = stringPtr(strings.TrimSpace(sessionID))
	return response, nil
}

// ReadWorkerSessionTranscriptByWorkerSessionID reads the normalized history
// for the canonical Worker Session identity. A missing provider-native
// transcript remains an explicit service outcome; callers can use the Worker
// Session event route for the canonical history in that case.
func (a *Adapter) ReadWorkerSessionTranscriptByWorkerSessionID(
	ctx context.Context,
	sessionID, workerSessionID string,
) (factoryapi.WorkerSessionTranscriptResponse, error) {
	normalizedWorkerSessionID, err := validateWorkerSessionReference(a, sessionID, workerSessionID)
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	workerSessionID = normalizedWorkerSessionID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("Worker Sessions service is required")
	}
	observation, err := observations.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if _, err := scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	result, err := observations.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("read Worker Session transcript: %w", err)
	}
	response := WorkerSessionTranscriptToAPI(result)
	response.FactorySessionId = stringPtr(strings.TrimSpace(sessionID))
	return response, nil
}

// StreamWorkerSessionEvents returns the detached identity envelope together
// with the canonical retained/live subscription for the exact Provider
// Session reference. The caller owns closing the subscription.
func (a *Adapter) StreamWorkerSessionEvents(
	ctx context.Context,
	sessionID, provider, kind, id string,
	replayOnly bool,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	return a.StreamWorkerSessionEventsWithCursor(ctx, sessionID, provider, kind, id, replayOnly, nil)
}

// StreamWorkerSessionEventsWithCursor is the cursor-aware provider-reference
// compatibility adapter. It resolves the exact Provider Session first, then
// delegates the same typed cursor to the Worker Session observation service.
func (a *Adapter) StreamWorkerSessionEventsWithCursor(
	ctx context.Context,
	sessionID, provider, kind, id string,
	replayOnly bool,
	cursor *workersessions.ObservationCursor,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	normalizedProvider, normalizedKind, normalizedID, err := validateProviderReference(a, sessionID, provider, kind, id)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}
	provider, kind, id = normalizedProvider, normalizedKind, normalizedID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("Worker Sessions service is required")
	}

	request := workersessions.GetObservationRequest{
		ProviderSession: providers.SessionRef{Provider: providers.ID(provider), Kind: kind, ID: id},
	}
	observation, err := observations.GetObservation(ctx, request)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if observation, err = scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	subscription, err := observations.StreamObservations(ctx, workersessions.StreamObservationsRequest{
		ProviderSession: request.ProviderSession,
		// Carry the documented default explicitly so the canonical ledger
		// receives the bounded stream policy at the transport boundary.
		Limit:      workersessions.DefaultObservationStreamLimit,
		ReplayOnly: replayOnly,
		Cursor:     cursor,
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
	return a.StreamWorkerSessionEventsByWorkerSessionIDWithCursor(ctx, sessionID, workerSessionID, replayOnly, nil)
}

// StreamWorkerSessionEventsByWorkerSessionIDWithCursor opens the canonical
// Worker-ID stream with an exclusive durable/live reconnect cursor.
func (a *Adapter) StreamWorkerSessionEventsByWorkerSessionIDWithCursor(
	ctx context.Context,
	sessionID, workerSessionID string,
	replayOnly bool,
	cursor *workersessions.ObservationCursor,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	normalizedWorkerSessionID, err := validateWorkerSessionReference(a, sessionID, workerSessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}
	workerSessionID = normalizedWorkerSessionID
	if ctx == nil {
		ctx = context.Background()
	}
	if err = ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, err
	}
	scope, err := a.resolveWorkerSessionScope(ctx, sessionID)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("resolve Factory Session scope: %w", err)
	}
	observations := a.observationsForScope(scope)
	if observations == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("Worker Sessions service is required")
	}
	observation, err := observations.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("get Worker Session observation: %w", err)
	}
	if observation, err = scopeWorkerSessionObservation(observation, scope); err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("scope Worker Session observation: %w", err)
	}
	subscription, err := observations.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
		Limit:           workersessions.DefaultObservationStreamLimit,
		ReplayOnly:      replayOnly,
		Cursor:          cursor,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("stream Worker Session events: %w", err)
	}
	if subscription.NextFunc == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	return WorkerSessionObservationToAPI(observation), subscription, nil
}

// GetTopLevelWorkerSessionObservation resolves one Worker Session using only
// its stable identity. Provider association facts remain service-owned.
func (a *Adapter) GetTopLevelWorkerSessionObservation(
	ctx context.Context,
	workerSessionID string,
) (factoryapi.WorkerSessionObservation, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, errors.New("Worker Sessions service is required")
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return factoryapi.WorkerSessionObservation{}, errors.New("worker session id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionObservation{}, err
	}
	observation, err := a.observations.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, fmt.Errorf("get top-level Worker Session observation: %w", err)
	}
	return WorkerSessionObservationToAPI(observation), nil
}

// ReadTopLevelWorkerSessionTranscript resolves the exact Provider Session
// association recorded for one Worker Session before projecting its transcript.
func (a *Adapter) ReadTopLevelWorkerSessionTranscript(
	ctx context.Context,
	workerSessionID string,
) (factoryapi.WorkerSessionTranscriptResponse, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("Worker Sessions service is required")
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("worker session id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, err
	}
	result, err := a.observations.ReadTranscriptByWorkerSessionID(ctx, workersessions.ReadTranscriptByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, fmt.Errorf("read top-level Worker Session transcript: %w", err)
	}
	return WorkerSessionTranscriptToAPI(result), nil
}

// StreamTopLevelWorkerSessionEvents resolves the canonical event topic by
// Worker Session identity and returns the detached observation envelope with
// its retained-then-live subscription.
func (a *Adapter) StreamTopLevelWorkerSessionEvents(
	ctx context.Context,
	workerSessionID string,
	replayOnly bool,
) (factoryapi.WorkerSessionObservation, workersessions.ObservationSubscription, error) {
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, errors.New("Worker Sessions service is required")
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
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("get top-level Worker Session observation: %w", err)
	}
	subscription, err := a.observations.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: workerSessionID,
		Limit:           workersessions.DefaultObservationStreamLimit,
		ReplayOnly:      replayOnly,
	})
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, fmt.Errorf("stream top-level Worker Session events: %w", err)
	}
	if subscription.NextFunc == nil {
		return factoryapi.WorkerSessionObservation{}, workersessions.ObservationSubscription{}, workersessions.ErrObservationSourceUnavailable
	}
	return WorkerSessionObservationToAPI(observation), subscription, nil
}

func (a *Adapter) resolveWorkerSessionScope(ctx context.Context, sessionID string) (workerSessionScope, error) {
	requestedID := strings.TrimSpace(sessionID)
	scope := workerSessionScope{requestedID: requestedID, effectiveID: requestedID}
	if a == nil || a.resolver == nil {
		return scope, nil
	}
	resolved, err := a.resolver.ResolveWorkerSessionScope(ctx, requestedID)
	if err != nil {
		return workerSessionScope{}, err
	}
	if effectiveID := strings.TrimSpace(resolved.EffectiveID); effectiveID != "" {
		scope.effectiveID = effectiveID
	}
	scope.defaultScope = resolved.IsDefault
	return scope, nil
}

func (a *Adapter) observationsForScope(scope workerSessionScope) observationService {
	if a == nil {
		return nil
	}
	if resolver, ok := a.resolver.(sessionObservationResolver); ok {
		if observations := resolver.WorkerSessionsObservationForSession(scope.effectiveID); observations != nil {
			return observations
		}
	}
	return a.observations
}

func scopeWorkerSessionObservation(
	observation workersessions.Observation,
	scope workerSessionScope,
) (workersessions.Observation, error) {
	expectedSessionID := strings.TrimSpace(scope.effectiveID)
	actualSessionID := strings.TrimSpace(observation.FactorySessionID)
	if actualSessionID != "" && actualSessionID != expectedSessionID &&
		!(scope.defaultScope && actualSessionID == defaultFactorySessionAlias) {
		return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
	}
	observation.FactorySessionID = scope.requestedID
	if observation.FactorySessionID == "" {
		observation.FactorySessionID = expectedSessionID
	}
	return observation, nil
}

const defaultFactorySessionAlias = "~default"

func validateProviderReference(
	a *Adapter,
	sessionID, provider, kind, id string,
) (string, string, string, error) {
	if a == nil || a.observations == nil {
		return "", "", "", errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", "", "", errors.New("session id is required")
	}
	provider = strings.TrimSpace(provider)
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if provider == "" || kind == "" || id == "" {
		return "", "", "", errors.New("provider, kind, and id are required")
	}
	return provider, kind, id, nil
}

func validateWorkerSessionReference(a *Adapter, sessionID, workerSessionID string) (string, error) {
	if a == nil || a.observations == nil {
		return "", errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", errors.New("session id is required")
	}
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return "", errors.New("worker session id is required")
	}
	return workerSessionID, nil
}
