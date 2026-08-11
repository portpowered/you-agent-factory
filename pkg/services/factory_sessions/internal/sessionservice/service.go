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
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
	liveruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime"
	liveruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/live_runtime/wire"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/stream"
)

// Service is the canonical Factory Session application gateway for open, read, and lifecycle behavior.
type Service struct {
	host              Host
	liveRuntime       liveruntime.Service
	streams           *stream.Manager
	reconnects        factorysessions.ReconnectCursorValidator
	results           factoryruntime.SessionResultProjectionOperation
	responseEvents    responsestreamservice.Service
	durable           durableexecution.Service
	invoker           roles.SessionInvoker
	activate          func(context.Context, string) error
	activationGateway factorydefinitions.DefinitionActivationGateway
}

// ForRuntime keeps an already-bound Factory Sessions gateway stable.
func (s *Service) ForRuntime(factorysessions.RuntimeBinding) (factorysessions.Service, error) {
	if s == nil {
		return nil, fmt.Errorf("Factory Sessions gateway is required")
	}
	return s, nil
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
	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, liveChangeSessionError(err)
	}
	if session == nil {
		return factorysessions.LiveChangeResult{}, liveChangeSessionError(factorysessions.ErrSessionNotFound)
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
	stateProvider := liveChangeStateProvider(runtime)
	coordinator := livechange.New(nil, runtime.LiveChangeLogger)
	canonicalID := livesession.CanonicalID(session)
	if recoverRequestID != "" {
		return coordinator.Recover(ctx, canonicalID, recoverRequestID, stateProvider, runtime.LiveChangeEvents, runtime.LiveChangeApplication)
	}
	return coordinator.Apply(ctx, canonicalID, request, stateProvider, runtime.LiveChangeEvents, runtime.LiveChangeApplication)
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
		if runtime == nil || runtime.Factory == nil || runtime.LiveChangeEvents == nil {
			return factorysessions.LiveChangeSessionState{}, factorysessions.ErrRuntimeNotAvailable
		}
		observed, err := runtime.Factory.Observe(ctx, factoryruntime.ObserveRequest{Scope: factoryruntime.ObservationScopeFull})
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
