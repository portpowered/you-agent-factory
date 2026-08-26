package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
)

// Service is the canonical Factory Session application gateway for open, read, and lifecycle behavior.
type Service struct {
	host              Host
	liveRuntime       liveruntime.Service
	liveChange        factorysessioncontracts.LiveChangeCoordinator
	streams           *stream.Manager
	reconnects        factorysessions.ReconnectCursorValidator
	results           factoryruntime.SessionResultProjectionOperation
	responseEvents    responsestreamservice.Service
	durable           durableexecution.Service
	recordedHistory   func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error)
	invoker           roles.SessionInvoker
	activate          func(context.Context, string) error
	activationGateway factorydefinitions.DefinitionActivationGateway
}

// bindRecordedSessionHistory connects each runtime gateway to the
// process-scoped recorded-history read owned by the Factory Sessions assembly.
// The gateway remains the owner of the opened runtime's durable execution, but
// history must be read from the process-wide artifact inventory so the HTTP
// transport sees the same combined session list as the root service.
func (s *Service) bindRecordedSessionHistory(
	lister func(context.Context, factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error),
) {
	if s != nil {
		s.recordedHistory = lister
	}
}

// New constructs a session gateway with explicit host and dataplane dependencies.
func New(host LegacyHost, responseStreams *responsestream.Registry) *Service {
	return NewWithResponseStreams(host, responseStreams)
}

// NewWithResponseStreams constructs a session gateway around an explicitly
// injected response-stream registry.
func NewWithResponseStreams(host LegacyHost, responseStreams *responsestream.Registry) *Service {
	return NewWithStreamDependencies(host, host, host, responseStreams)
}

// NewWithStreamDependencies separates session control-plane callbacks from
// canonical response-stream lookup and telemetry dependencies.
func NewWithStreamDependencies(host Host, sessions stream.SessionResolver, observer stream.Observer, responseStreams *responsestream.Registry) *Service {
	return NewWithReconnectValidation(host, sessions, observer, responseStreams, nil, nil)
}

// NewWithReconnectValidation injects Recordings-owned reconnect validation
// without exposing its ledger implementation to Factory Sessions.
func NewWithReconnectValidation(
	host Host,
	sessions stream.SessionResolver,
	observer stream.Observer,
	responseStreams *responsestream.Registry,
	reconnects factorysessions.ReconnectCursorValidator,
	results factoryruntime.SessionResultProjectionOperation,
) *Service {
	return NewWithResponseService(host, sessions, observer, responseStreams, reconnects, results, nil)
}

// NewWithResponseService injects the owner-private response-stream policy used
// by the outer Factory Sessions boundary.
func NewWithResponseService(
	host Host,
	sessions stream.SessionResolver,
	observer stream.Observer,
	responseStreams *responsestream.Registry,
	reconnects factorysessions.ReconnectCursorValidator,
	results factoryruntime.SessionResultProjectionOperation,
	responseEvents responsestreamservice.Service,
) *Service {
	return NewWithLiveChangeCoordinator(
		host, sessions, observer, responseStreams, reconnects, results, responseEvents, nil,
	)
}

// NewWithLiveChangeCoordinator constructs the session gateway with the
// process-scoped coordinator supplied by Factory Sessions wire. Runtime state,
// event history, application, clock, and logger remain operation inputs.
func NewWithLiveChangeCoordinator(
	host Host,
	sessions stream.SessionResolver,
	observer stream.Observer,
	responseStreams *responsestream.Registry,
	reconnects factorysessions.ReconnectCursorValidator,
	results factoryruntime.SessionResultProjectionOperation,
	responseEvents responsestreamservice.Service,
	liveChange factorysessioncontracts.LiveChangeCoordinator,
) *Service {
	if host == nil || sessions == nil || observer == nil || responseStreams == nil {
		return nil
	}
	liveRuntime, err := liveruntimewire.NewService(liveRuntimeDependencies(host))
	if err != nil {
		return nil
	}
	var durable durableexecution.Service
	if execution := host.DurableExecution(); execution != nil {
		durable, err = durableexecutionwire.NewService(execution)
		if err != nil {
			return nil
		}
	}
	return &Service{
		host:           host,
		liveRuntime:    liveRuntime,
		liveChange:     liveChange,
		streams:        stream.NewManagerWithResponseService(sessions, observer, responseStreams, responseEvents),
		reconnects:     reconnects,
		results:        results,
		responseEvents: responseEvents,
		durable:        durable,
	}
}

// OpenFactorySession runs an owner-defined open request through control-plane
// policy and live dataplane startup.
func (s *Service) OpenFactorySession(
	ctx context.Context,
	request factorysessions.OpenRequest,
) (*factorysessions.OpenResult, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	if request.ValidateOnly && request.InitNewFactory {
		return nil, sessionvalidation.New(
			factorysessions.ValidationReasonRequired,
			"initNewFactory",
			fmt.Errorf("initNewFactory cannot be combined with validateOnly"),
		)
	}
	return s.OpenFactorySessionFromFolder(
		ctx,
		request.FolderPath,
		request.Target,
		request.ValidateOnly,
		request.InitNewFactory,
	)
}

// OpenFactorySessionFromFolder runs folder-scoped open policy without transport mapping.
func (s *Service) OpenFactorySessionFromFolder(
	ctx context.Context,
	folderPath string,
	target *factorysessions.TargetRef,
	validateOnly bool,
	initNewFactory bool,
) (*factorysessions.OpenResult, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory session gateway is required")
	}
	result, err := controlplane.OpenFromFolder(
		ctx,
		s.host,
		s.liveRuntime,
		folderPath,
		target,
		validateOnly,
		initNewFactory,
	)
	if err != nil || result == nil || result.SessionID == "" {
		return result, err
	}
	session := s.liveRuntime.Resolve(result.SessionID)
	if session == nil {
		return result, nil
	}
	result.Session = &factorysessions.ScopedLiveSessionSummary{
		ID: livesession.CanonicalID(session), FactoryDir: session.FactoryDir,
		FolderPath: session.FolderPath, Project: session.Project,
		IsDefault: session.IsDefault, Target: session.Target,
	}
	return result, nil
}

func liveRuntimeDependencies(host Host) liveruntime.Dependencies {
	return liveruntime.Dependencies{
		OpenForTarget:          host.OpenLiveSessionForTarget,
		ListSessionIDs:         host.ListLiveSessionIDs,
		GetSession:             host.GetLiveSession,
		RequireSession:         host.RequireSession,
		BuildProjectionContext: host.BuildSessionProjectionContext,
		SessionFactory:         host.SessionFactory,
		StopSession:            host.StopLiveSession,
		ObserveControl:         host.ObserveLiveLifecycleControl,
	}
}

// ApplyLiveChange owns the service-root live-change boundary. The concrete
// application and dispatch/resource coordination are explicit fields on the
// selected LiveRuntime, while this gateway owns session identity and locking.
func (s *Service) ApplyLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
) (factorysessions.LiveChangeResult, error) {
	return s.runLiveChange(ctx, sessionID, request, "")
}

// RecoverLiveChange closes the one pending request event for requestID after a
// crash or process restart without requiring the caller to resubmit its body.
func (s *Service) RecoverLiveChange(
	ctx context.Context,
	sessionID string,
	requestID string,
) (factorysessions.LiveChangeResult, error) {
	return s.runLiveChange(ctx, sessionID, factorysessions.LiveChangeRequest{}, requestID)
}

func (s *Service) runLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	recoverRequestID string,
) (factorysessions.LiveChangeResult, error) {
	if s == nil || s.host == nil {
		return factorysessions.LiveChangeResult{}, fmt.Errorf("Factory Sessions gateway is required")
	}
	session, durableResult, durable, err := s.resolveLiveChangeSession(ctx, sessionID, request, recoverRequestID)
	if durable || err != nil {
		return durableResult, err
	}
	if session.Runtime == nil {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	session.LiveChangeMu.Lock()
	defer session.LiveChangeMu.Unlock()

	release, acquireErr := acquireLiveChangeAdmission(ctx, session)
	if acquireErr != nil {
		return factorysessions.LiveChangeResult{}, acquireErr
	}
	if release != nil {
		defer release()
	}

	runtime := session.Runtime
	if runtime.Clock == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "live change clock is unavailable",
		}
	}
	stateProvider := liveChangeStateProvider(runtime)
	if s.liveChange == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "live change coordinator is unavailable",
		}
	}
	operation := factorysessions.LiveChangeOperation{
		StateProvider: stateProvider,
		Events:        runtime.LiveChangeEvents,
		Application:   runtime.LiveChangeApplication,
		Now:           runtime.Clock.Now,
		Logger:        runtime.LiveChangeLogger,
	}
	canonicalID := livesession.CanonicalID(session)
	var result factorysessions.LiveChangeResult
	var applyErr error
	if recoverRequestID != "" {
		result, applyErr = s.liveChange.RecoverLiveChange(ctx, canonicalID, recoverRequestID, operation)
	} else {
		result, applyErr = s.liveChange.ApplyLiveChange(ctx, canonicalID, request, operation)
	}
	if applyErr == nil && (result.Outcome == factorysessions.LiveChangeOutcomeApplied || result.Outcome == factorysessions.LiveChangeOutcomeReplayed) {
		if revision, ok := runtimebinding.ServiceForLiveRuntime(runtime).(factoryruntime.ResourceCapacityRevisionService); ok {
			revision.SetFactoryRevision(result.NewRevision)
		}
	}
	return result, applyErr
}

func (s *Service) resolveLiveChangeSession(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	recoverRequestID string,
) (*livesession.LiveSession, factorysessions.LiveChangeResult, bool, error) {
	session, err := s.host.RequireSession(sessionID)
	if err != nil && !isMissingLiveSession(err) {
		return nil, factorysessions.LiveChangeResult{}, false, liveChangeSessionError(err)
	}
	if err != nil || session == nil {
		if result, handled := s.runDurableLiveChange(ctx, sessionID, request, recoverRequestID); handled {
			return nil, result, true, nil
		}
		if err != nil {
			return nil, factorysessions.LiveChangeResult{}, false, liveChangeSessionError(err)
		}
		return nil, factorysessions.LiveChangeResult{}, false, liveChangeSessionError(factorysessions.ErrSessionNotFound)
	}
	return session, factorysessions.LiveChangeResult{}, false, nil
}

type durableLiveChangeCapability interface {
	ApplyLiveChange(context.Context, string, factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error)
	RecoverLiveChange(context.Context, string, string) (factorysessions.LiveChangeResult, error)
}

func (s *Service) runDurableLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	recoverRequestID string,
) (factorysessions.LiveChangeResult, bool) {
	if s == nil || s.durable == nil {
		return factorysessions.LiveChangeResult{}, false
	}
	capability, ok := s.durable.(durableLiveChangeCapability)
	if !ok {
		return factorysessions.LiveChangeResult{}, false
	}
	var (
		result factorysessions.LiveChangeResult
		err    error
	)
	if recoverRequestID != "" {
		result, err = capability.RecoverLiveChange(ctx, sessionID, recoverRequestID)
	} else {
		result, err = capability.ApplyLiveChange(ctx, sessionID, request)
	}
	if errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		return factorysessions.LiveChangeResult{}, false
	}
	return result, true
}

func isMissingLiveSession(err error) bool {
	return errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound)
}

func acquireLiveChangeAdmission(
	ctx context.Context,
	session *livesession.LiveSession,
) (func(), error) {
	if session == nil || session.Runtime == nil || session.Runtime.LiveChangeAdmission == nil {
		return nil, nil
	}
	release, err := session.Runtime.LiveChangeAdmission.AcquireLiveChange(ctx, livesession.CanonicalID(session))
	if err != nil {
		return nil, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "live change coordination is unavailable",
			Cause:   err,
		}
	}
	return release, nil
}

func liveChangeStateProvider(runtime *factorysessions.LiveRuntime) livechange.StateProvider {
	return func(ctx context.Context, id string) (factorysessions.LiveChangeSessionState, error) {
		service := runtimebinding.ServiceForLiveRuntime(runtime)
		if service == nil || runtime.LiveChangeEvents == nil {
			return factorysessions.LiveChangeSessionState{}, factorysessions.ErrRuntimeNotAvailable
		}
		observed, err := service.Observe(ctx, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeFull})
		if err != nil {
			return factorysessions.LiveChangeSessionState{}, err
		}
		state := livechange.ProjectState(id, runtime.LiveChangeEvents.LiveChangeEvents())
		state.Lifecycle = liveChangeLifecycleFromObservation(observed.Observation)
		return state, nil
	}
}

func liveChangeLifecycleFromObservation(observation factoryruntime.Observation) factorysessions.LiveChangeLifecycle {
	switch observation.Status {
	case factoryruntime.ObservationStatusFinished:
		return factorysessions.LiveChangeLifecycleCompleted
	case factoryruntime.ObservationStatusIdle:
		return factorysessions.LiveChangeLifecycleIdle
	default:
		return liveChangeLifecycleFromFactoryState(observation.Health.FactoryState)
	}
}

var _ factorysessions.LiveChangeService = (*Service)(nil)

func liveChangeSessionError(err error) error {
	if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrNotFound) {
		return &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorSessionNotFound, Message: "Factory Session was not found", Cause: err,
		}
	}
	return err
}

func liveChangeLifecycleFromFactoryState(factoryState string) factorysessions.LiveChangeLifecycle {
	switch strings.ToUpper(strings.TrimSpace(factoryState)) {
	case "PAUSED":
		return factorysessions.LiveChangeLifecyclePaused
	case "FAILED":
		return factorysessions.LiveChangeLifecycleFailed
	case "COMPLETED":
		return factorysessions.LiveChangeLifecycleCompleted
	default:
		return factorysessions.LiveChangeLifecycleRunning
	}
}

// SessionRuntime forwards the optional resource-capacity ports to the active
// runtime. Durable JavaScript children invoke this Factory Sessions façade, so
// forwarding the ports keeps their resource leases on the same admission gate
// used by live capacity changes and ordinary Factory dispatches.
func (fs *SessionRuntime) activeResourceCapacityService() factoryruntime.ResourceCapacityService {
	if fs == nil {
		return nil
	}
	service, _ := fs.currentRuntimeService().(factoryruntime.ResourceCapacityService)
	return service
}

func (fs *SessionRuntime) activeAdmittedResourceCapacityService() factoryruntime.AdmittedResourceCapacityService {
	if fs == nil {
		return nil
	}
	service, _ := fs.currentRuntimeService().(factoryruntime.AdmittedResourceCapacityService)
	return service
}

func (fs *SessionRuntime) activeResourceCapacityAdmission() factoryruntime.ResourceCapacityAdmission {
	if fs == nil {
		return nil
	}
	service, _ := fs.currentRuntimeService().(factoryruntime.ResourceCapacityAdmission)
	return service
}

func (fs *SessionRuntime) activeResourceCapacityLeaseAdmission() factoryruntime.ResourceCapacityLeaseAdmission {
	if fs == nil {
		return nil
	}
	service, _ := fs.currentRuntimeService().(factoryruntime.ResourceCapacityLeaseAdmission)
	return service
}

func (fs *SessionRuntime) activeResourceCapacityRevisionService() factoryruntime.ResourceCapacityRevisionService {
	if fs == nil {
		return nil
	}
	service, _ := fs.currentRuntimeService().(factoryruntime.ResourceCapacityRevisionService)
	return service
}

func (fs *SessionRuntime) PreviewResourceCapacity(
	ctx context.Context,
	request factoryruntime.ResourceCapacityRequest,
) (factoryruntime.ResourceCapacityResult, error) {
	service := fs.activeResourceCapacityService()
	if service == nil {
		return factoryruntime.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	return service.PreviewResourceCapacity(ctx, request)
}

func (fs *SessionRuntime) SetResourceCapacity(
	ctx context.Context,
	request factoryruntime.ResourceCapacityRequest,
) (factoryruntime.ResourceCapacityResult, error) {
	service := fs.activeResourceCapacityService()
	if service == nil {
		return factoryruntime.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	return service.SetResourceCapacity(ctx, request)
}

func (fs *SessionRuntime) PreviewResourceCapacityAdmitted(
	ctx context.Context,
	request factoryruntime.ResourceCapacityRequest,
) (factoryruntime.ResourceCapacityResult, error) {
	service := fs.activeAdmittedResourceCapacityService()
	if service == nil {
		return factoryruntime.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime admitted resource capacity is unavailable")
	}
	return service.PreviewResourceCapacityAdmitted(ctx, request)
}

func (fs *SessionRuntime) SetResourceCapacityAdmitted(
	ctx context.Context,
	request factoryruntime.ResourceCapacityRequest,
) (factoryruntime.ResourceCapacityResult, error) {
	service := fs.activeAdmittedResourceCapacityService()
	if service == nil {
		return factoryruntime.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime admitted resource capacity is unavailable")
	}
	return service.SetResourceCapacityAdmitted(ctx, request)
}

func (fs *SessionRuntime) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	service := fs.activeResourceCapacityAdmission()
	if service == nil {
		return nil, fmt.Errorf("Factory Runtime resource admission is unavailable")
	}
	return service.AcquireResourceCapacityAdmission(ctx)
}

func (fs *SessionRuntime) AcquireResourceCapacityLease(
	ctx context.Context,
	request factoryruntime.ResourceCapacityLeaseRequest,
) (*factoryruntime.ResourceCapacityLease, error) {
	service := fs.activeResourceCapacityLeaseAdmission()
	if service == nil {
		return nil, fmt.Errorf("Factory Runtime resource lease admission is unavailable")
	}
	return service.AcquireResourceCapacityLease(ctx, request)
}

func (fs *SessionRuntime) CurrentFactoryRevision() int {
	service := fs.activeResourceCapacityRevisionService()
	if service == nil {
		return 0
	}
	return service.CurrentFactoryRevision()
}

func (fs *SessionRuntime) SetFactoryRevision(revision int) {
	service := fs.activeResourceCapacityRevisionService()
	if service != nil {
		service.SetFactoryRevision(revision)
	}
}
