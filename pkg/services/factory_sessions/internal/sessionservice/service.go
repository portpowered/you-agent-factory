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
	definitionRuntime *SessionRuntime
	liveRuntime       liveruntime.Service
	streams           *stream.Manager
	reconnects        factorysessions.ReconnectCursorValidator
	results           factoryruntime.SessionResultProjectionOperation
	responseEvents    responsestreamservice.Service
	durable           durableexecution.Service
	invoker           interface {
		InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
	}
	activate          func(context.Context, string) error
	activationGateway factorysessions.DefinitionActivationGateway
}

// ForRuntime keeps an already-bound Factory Sessions gateway stable.
func (s *Service) ForRuntime(factorysessions.RuntimeBinding) (factorysessions.Service, error) {
	if s == nil {
		return nil, fmt.Errorf("Factory Sessions gateway is required")
	}
	return s, nil
}

// AttachDefinitionRuntime binds the already-constructed Session runtime to the
// root gateway. The binding is one-way: current-Factory lifecycle policy stays
// in Sessions and the Definitions root is consumed only as a unary capability.
func (s *Service) AttachDefinitionRuntime(runtime *SessionRuntime) *Service {
	if s != nil && runtime != nil {
		s.definitionRuntime = runtime
	}
	return s
}

// ReadCurrentFactoryForSession routes current-Factory reads through the
// Session-owned lifecycle implementation.
func (s *Service) ReadCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
) (factorydefinitions.EditableFactory, error) {
	if s == nil || s.definitionRuntime == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Session definition runtime is required")
	}
	return s.definitionRuntime.ReadCurrentFactoryForSession(ctx, sessionID)
}

// SaveFactoryForSession routes current-Factory persistence through the
// Session-owned lifecycle implementation.
func (s *Service) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factorydefinitions.SaveMode,
	request factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	if s == nil || s.definitionRuntime == nil {
		return factorydefinitions.EditableFactory{}, fmt.Errorf("Factory Session definition runtime is required")
	}
	return s.definitionRuntime.SaveFactoryForSession(ctx, sessionID, mode, request)
}

// ActivateFactory routes named-Factory activation through Sessions-owned
// locking, idle checks, runtime replacement, and rollback.
func (s *Service) ActivateFactory(ctx context.Context, name string) error {
	if s == nil || s.definitionRuntime == nil {
		return fmt.Errorf("Factory Session definition runtime is required")
	}
	return s.definitionRuntime.ActivateFactory(ctx, name)
}

// InvokeFactorySession routes invocation through the root-owned invocation
// capability attached by the runtime assembly. The nil capability case is
// explicit so inert construction never starts a runtime as a side effect.
func (s *Service) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	if s == nil || s.invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	result, err := s.invoker.InvokeFactorySession(ctx, sessionID, request)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return factorysessions.InvocationResult{
		RequestID:     result.RequestID,
		TraceID:       result.TraceID,
		Status:        factorysessions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult,
		ErrorCode:     result.ErrorCode,
		Message:       result.Message,
		SessionID:     result.SessionID,
		WorkID:        result.WorkID,
		WorkName:      result.WorkName,
		WorkState:     result.WorkState,
	}, nil
}

// ActivateNamedFactory routes named-factory activation through the root-owned
// runtime callback. Definition policy remains in Factory Definitions; this
// method only exposes the Sessions-owned serialization boundary.
func (s *Service) ActivateNamedFactory(ctx context.Context, name string) error {
	if s != nil && s.definitionRuntime != nil {
		return s.definitionRuntime.ActivateFactory(ctx, name)
	}
	if s == nil || s.activate == nil {
		return fmt.Errorf("Factory Session activation service is required")
	}
	return s.activate(ctx, name)
}

// DefinitionActivationGateway exposes the already-attached narrow activation
// edge for the Definitions composition boundary. It is implemented by the
// concrete root without adding another capability to the peer-facing Service
// method set.
func (s *Service) DefinitionActivationGateway() factorysessions.DefinitionActivationGateway {
	if s == nil {
		return nil
	}
	return s.activationGateway
}

// bindRootCapabilities attaches runtime-owned capabilities after the live
// session assembly has been created. It is intentionally private to the
// Sessions implementation; peers receive only Service.
func (s *Service) bindRootCapabilities(
	invoker interface {
		InvokeFactorySession(context.Context, string, factorysessions.InvocationRequest) (factorydefinitions.FactoryInvocationResult, error)
	},
	activate func(context.Context, string) error,
	activationGateway factorysessions.DefinitionActivationGateway,
) {
	if s == nil {
		return
	}
	s.invoker = invoker
	s.activate = activate
	s.activationGateway = activationGateway
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
