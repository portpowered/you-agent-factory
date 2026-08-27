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
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Adapter maps Worker Sessions observation projections to the generated HTTP
// contract. Work remains the authority for deciding whether the requested Work
// exists; Worker Sessions remains the authority for correlated attempts.
type observationService interface {
	ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error)
	ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error)
	GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error)
	GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error)
	ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error)
	ReadTranscriptByWorkerSessionID(context.Context, workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error)
	StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error)
	StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error)
}

// topLevelObservationService is the narrow capability consumed by the
// process-wide list adapter. A fleet view may combine runtime-bound services
// without becoming the owner of Work-scoped reads or lifecycle operations.
type topLevelObservationService interface {
	ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error)
}

type startService interface {
	Start(context.Context, workersessions.StartRequest) (workersessions.StartResult, error)
}

type continuationService interface {
	Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error)
}

type interruptService interface {
	Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error)
}

type controlService interface {
	Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
	Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error)
}

// SessionScope is the transport's detached view of the identity needed to
// select and validate one Factory Session's Worker Session observations.
// EffectiveID is used for the internal observation lookup; the requested
// selector remains the public response alias.
type SessionScope struct {
	EffectiveID string
	IsDefault   bool
}

// SessionScopeResolver supplies the already-opened session identity to the
// Worker Sessions transport without coupling this package to Factory Sessions.
// The application composition root adapts the owning Factory Sessions service.
type SessionScopeResolver interface {
	ResolveWorkerSessionScope(context.Context, string) (SessionScope, error)
}

type sessionObservationResolver interface {
	WorkerSessionsObservationForSession(string) workersessions.ObservationService
}

type workerSessionScope struct {
	requestedID  string
	effectiveID  string
	defaultScope bool
}

type Adapter struct {
	observations observationService
	topLevel     topLevelObservationService
	starter      startService
	continuer    continuationService
	interrupter  interruptService
	controller   controlService
	work         work.Service
	resolver     SessionScopeResolver
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
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionObservation{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionObservation{}, errors.New("session id is required")
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
	if a == nil || a.observations == nil {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("Worker Sessions service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return factoryapi.WorkerSessionTranscriptResponse{}, errors.New("session id is required")
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

// NewAdapter binds the exact roots required by the Worker Sessions list
// operation.
func NewAdapter(
	observations observationService,
	workRoot work.Service,
	resolvers ...SessionScopeResolver,
) *Adapter {
	if observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{observations: observations, topLevel: observations, work: workRoot, resolver: firstSessionScopeResolver(resolvers)}
}

// NewAdapterWithStart binds the Worker Sessions start capability in addition
// to the observation and Work read capabilities retained by NewAdapter.
func NewAdapterWithStart(
	starter startService,
	observations observationService,
	workRoot work.Service,
	resolvers ...SessionScopeResolver,
) *Adapter {
	if starter == nil || observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{starter: starter, observations: observations, topLevel: observations, work: workRoot, resolver: firstSessionScopeResolver(resolvers)}
}

// NewAdapterWithStartAndContinue binds both direct admission operations while
// retaining the observation and Work read capabilities of the base adapter.
func NewAdapterWithStartAndContinue(
	starter startService,
	continuer continuationService,
	observations observationService,
	workRoot work.Service,
	resolvers ...SessionScopeResolver,
) *Adapter {
	if starter == nil || continuer == nil || observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{starter: starter, continuer: continuer, observations: observations, topLevel: observations, work: workRoot, resolver: firstSessionScopeResolver(resolvers)}
}

// NewAdapterWithStartAndContinueAndInterrupt binds the direct admission and
// replacement operations while retaining the observation and Work read
// capabilities of the base adapter.
func NewAdapterWithStartAndContinueAndInterrupt(
	starter startService,
	continuer continuationService,
	interrupter interruptService,
	observations observationService,
	workRoot work.Service,
	resolvers ...SessionScopeResolver,
) *Adapter {
	if starter == nil || continuer == nil || interrupter == nil || observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{
		starter: starter, continuer: continuer, interrupter: interrupter,
		observations: observations, topLevel: observations, work: workRoot, resolver: firstSessionScopeResolver(resolvers),
	}
}

// NewAdapterWithStartAndContinueAndInterruptAndControl binds the complete
// direct Worker Session lifecycle surface while retaining the observation and
// Work read capabilities of the base adapter.
func NewAdapterWithStartAndContinueAndInterruptAndControl(
	starter startService,
	continuer continuationService,
	interrupter interruptService,
	controller controlService,
	observations observationService,
	workRoot work.Service,
	resolvers ...SessionScopeResolver,
) *Adapter {
	if starter == nil || continuer == nil || interrupter == nil || controller == nil || observations == nil || workRoot == nil {
		return nil
	}
	return &Adapter{
		starter: starter, continuer: continuer, interrupter: interrupter,
		controller: controller, observations: observations, topLevel: observations, work: workRoot, resolver: firstSessionScopeResolver(resolvers),
	}
}

// WithTopLevelObservationService returns a copy whose fleet-wide list reads
// use the supplied process-level view. Work-scoped reads and lifecycle
// controls continue to use the runtime-bound service already held by the
// adapter.
func (a *Adapter) WithTopLevelObservationService(service topLevelObservationService) *Adapter {
	if a == nil || service == nil {
		return a
	}
	bound := *a
	bound.topLevel = service
	return &bound
}

// StartWorkerSession maps one typed HTTP request to the Worker Sessions-owned
// asynchronous start operation. The service returns only at its admission
// barrier; terminal execution is deliberately not awaited here.
func (a *Adapter) StartWorkerSession(
	ctx context.Context,
	request factoryapi.WorkerSessionStartRequest,
) (factoryapi.WorkerSessionStartResponse, error) {
	if a == nil || a.starter == nil {
		return factoryapi.WorkerSessionStartResponse{}, errors.New("Worker Sessions start service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start, err := WorkerSessionStartRequestFromAPI(request)
	if err != nil {
		return factoryapi.WorkerSessionStartResponse{}, err
	}
	result, err := a.starter.Start(ctx, start)
	if err != nil {
		return factoryapi.WorkerSessionStartResponse{}, err
	}
	return WorkerSessionStartResponseToAPI(start.RequestID, result), nil
}

// ContinueWorkerSession maps one source-addressed HTTP request to the
// Worker Sessions continuation barrier. A successful response is emitted
// only after the successor is reserved, observable, and admitted.
func (a *Adapter) ContinueWorkerSession(
	ctx context.Context,
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionContinueRequest,
) (factoryapi.WorkerSessionContinueResponse, error) {
	if a == nil || a.continuer == nil {
		return factoryapi.WorkerSessionContinueResponse{}, errors.New("Worker Sessions continuation service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	continuation, err := WorkerSessionContinueRequestFromAPI(sourceWorkerSessionID, request)
	if err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, err
	}
	result, err := a.continuer.Continue(ctx, continuation)
	if err != nil {
		return factoryapi.WorkerSessionContinueResponse{}, err
	}
	return WorkerSessionContinueResponseToAPI(result), nil
}

// InterruptWorkerSession maps one source-addressed HTTP request to the
// Worker Sessions-owned interrupt barrier. A successful response is emitted
// only after source cancellation and successor admission are authoritative.
func (a *Adapter) InterruptWorkerSession(
	ctx context.Context,
	sourceWorkerSessionID string,
	request factoryapi.WorkerSessionInterruptRequest,
) (factoryapi.WorkerSessionInterruptResponse, error) {
	if a == nil || a.interrupter == nil {
		return factoryapi.WorkerSessionInterruptResponse{}, errors.New("Worker Sessions interrupt service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	interrupt, err := WorkerSessionInterruptRequestFromAPI(sourceWorkerSessionID, request)
	if err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, err
	}
	result, err := a.interrupter.Interrupt(ctx, interrupt)
	if err != nil {
		return factoryapi.WorkerSessionInterruptResponse{}, err
	}
	return WorkerSessionInterruptResponseToAPI(result), nil
}

// ControlWorkerSession maps one exact top-level Worker Session control to the
// corresponding Worker Sessions root operation. The action is selected by
// the route, so callers cannot submit an arbitrary action payload or identity.
func (a *Adapter) ControlWorkerSession(
	ctx context.Context,
	workerSessionID string,
	action workersessions.ControlAction,
) (factoryapi.WorkerSessionControlResponse, error) {
	if a == nil || a.controller == nil {
		return factoryapi.WorkerSessionControlResponse{}, errors.New("Worker Sessions control service is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request := workersessions.ControlRequest{ID: strings.TrimSpace(workerSessionID)}
	if err := request.Validate(); err != nil {
		return factoryapi.WorkerSessionControlResponse{}, err
	}
	var (
		result workersessions.ControlResult
		err    error
	)
	switch action {
	case workersessions.ControlActionPause:
		result, err = a.controller.Pause(ctx, request)
	case workersessions.ControlActionResume:
		result, err = a.controller.Resume(ctx, request)
	case workersessions.ControlActionCancel:
		result, err = a.controller.Cancel(ctx, request)
	case workersessions.ControlActionTerminate:
		result, err = a.controller.Terminate(ctx, request)
	default:
		return factoryapi.WorkerSessionControlResponse{}, fmt.Errorf("unsupported Worker Session control action %q", action)
	}
	if err != nil {
		return factoryapi.WorkerSessionControlResponse{}, err
	}
	return WorkerSessionControlResponseToAPI(result), nil
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

func firstSessionScopeResolver(resolvers []SessionScopeResolver) SessionScopeResolver {
	for _, resolver := range resolvers {
		if resolver != nil {
			return resolver
		}
	}
	return nil
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
