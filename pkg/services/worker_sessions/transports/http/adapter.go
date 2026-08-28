// Package http owns the Worker Sessions HTTP representation boundary.
package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
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

func firstSessionScopeResolver(resolvers []SessionScopeResolver) SessionScopeResolver {
	for _, resolver := range resolvers {
		if resolver != nil {
			return resolver
		}
	}
	return nil
}
