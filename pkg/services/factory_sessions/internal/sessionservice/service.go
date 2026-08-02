package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
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
