package service

import (
	"context"
	"fmt"
	"path/filepath"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Assembly retains Factory Sessions-owned mutable registries while peer
// services are constructed against its root resolver roles.
type Assembly struct {
	factorysessions.Service
	registry                     sessionregistry.Service
	state                        *sessionruntime.Service
	streams                      streamManager
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	directoryInspection          roles.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
	responseStreams              responsestreamservice.Service
}

type streamManager interface {
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
}

// NewAssembly constructs an empty live-session directory.
func NewAssembly(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	clock factoryruntime.Clock,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreamService responsestreamservice.Service,
) roles.RuntimeAssembly {
	if clock == nil || eventIDs == nil || sessionIDs == nil || resolveHome == nil || directoryInspection == nil || namedPaths == nil || invocationInputFiles == nil || initialWorkFiles == nil || sessionResultProjection == nil || identityService == nil || responseStreamService == nil {
		return nil
	}
	registry := sessionregistry.New()
	responses, err := responseStreamService.NewStreamRegistry(clock)
	if err != nil {
		return nil
	}
	state := sessionruntime.NewWithResponseService(registry, responses, nil, clock, eventIDs, sessionIDs, responseStreamService)
	return &Assembly{
		Service:                      &Service{},
		registry:                     registry,
		state:                        state,
		streams:                      runtimebinding.NewStreamManager(state),
		newJavaScriptCheckpointStore: newJavaScriptCheckpointStore,
		sessionResultProjection:      sessionResultProjection,
		interpolation:                interpolation,
		invocationWorkTypes:          invocationWorkTypes,
		ttsObservability:             ttsObservability,
		eventIDs:                     eventIDs,
		sessionIDs:                   sessionIDs,
		resolveHome:                  resolveHome,
		directoryInspection:          directoryInspection,
		namedPaths:                   namedPaths,
		invocationInputFiles:         invocationInputFiles,
		initialWorkFiles:             initialWorkFiles,
		identity:                     identityService,
		responseStreams:              responseStreamService,
	}
}

// ForRuntime keeps an already-bound runtime view stable when it is passed
// through code that only knows the public Factory Sessions contract.
func (a *Assembly) ForRuntime(factorysessions.RuntimeBinding) (factorysessions.Service, error) {
	if a == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime: service is required")
	}
	return a, nil
}

func (a *Assembly) CurrentRuntime() *factorysessions.LiveRuntime {
	if a == nil || a.state == nil {
		return nil
	}
	return a.state.CurrentRuntime()
}

func (a *Assembly) Resolve(sessionID string) *livesession.LiveSession {
	if a == nil || a.state == nil {
		return nil
	}
	return a.state.Resolve(sessionID)
}

// ResolveWorkRuntime adapts the Factory Sessions registry to Work's
// consumer-owned runtime port.
func (a *Assembly) ResolveWorkRuntime(sessionID string) (work.Runtime, error) {
	session := a.Resolve(sessionID)
	if session == nil || session.Runtime == nil || session.Runtime.Factory == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	return workRuntimeAdapter{sessionID: sessionID, runtime: session.Runtime.Factory}, nil
}

func (a *Assembly) WithRuntimeRead(read func(*factorysessions.LiveRuntime) error) error {
	if a == nil || a.state == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return a.state.WithRuntimeRead(read)
}

func (a *Assembly) InferenceProgressPublisherFactory(logger *zap.Logger) func(string) factorysessions.ProgressPublisher {
	if a == nil || a.streams == nil {
		return nil
	}
	return a.streams.InferenceProgressPublisherFactory(logger)
}

func (a *Assembly) DispatchCompletionObserverFactory() func(string) func(string) {
	if a == nil || a.streams == nil {
		return nil
	}
	return a.streams.DispatchCompletionObserverFactory()
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func (a *Assembly) Complete(
	factoryRootDir string,
	clock factoryruntime.Clock,
	baseLogger *zap.Logger,
	logger *zap.Logger,
	runtimeBuild factoryruntime.ReplacementBuilder,
	startupRuntime factoryruntime.HostedInstance,
	startupSpec factoryruntime.SessionBuildSpec,
	runtimeLifecycle factoryruntime.Lifecycle,
	runtimeSidecars factorysessions.RuntimeSidecars,
	durableExecution factorysessions.ExecutionService,
	dir string,
	executionBaseDir string,
	runtimeMode factorydefinitions.RuntimeMode,
	backendScopeID string,
	workFile string,
	workflowID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	reconnectCursorValidator factorysessions.ReconnectCursorValidator,
	worldStateProjector factoryruntime.WorldStateProjector,
	invocationMetricsRecorder roles.InvocationMetricsRecorder,
) (
	roles.ApplicationRuntime,
	factorysessions.Service,
	roles.SessionInvoker,
	factorysessions.DefinitionHost,
	error,
) {
	if a == nil || a.state == nil || a.registry == nil {
		return nil, nil, nil, nil, fmt.Errorf("Factory Sessions assembly is required")
	}
	if startupRuntime == nil {
		return nil, nil, nil, nil, fmt.Errorf("default Factory Runtime is required")
	}
	runtimeConfig, ok := startupRuntime.LoadedRuntimeConfig().(factorydefinitions.LoadedFactorySource)
	if !ok || runtimeConfig == nil {
		return nil, nil, nil, nil, fmt.Errorf("constructed runtime config does not expose Factory Definition snapshots")
	}
	session := livesession.New(
		factorysessions.DefaultSessionID,
		startupRuntime.Directory(),
		startupRuntime.FolderDirectory(),
		startupRuntime.LoadedRuntimeConfig().RuntimeBaseDir(),
		factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault},
		&runtimebinding.SessionState{Instance: startupRuntime, Spec: &startupSpec},
		true,
		filepath.Base(startupRuntime.FolderDirectory()),
		clock,
		a.sessionIDs,
		a.eventIDs,
	)
	if session == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct live Factory Session: clock and response-event identity generator are required")
	}
	responseEvents, err := a.responseStreams.NewEventStore(livesession.CanonicalID(session), clock)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct live Factory Session response events: %w", err)
	}
	session.ResponseEvents = responseEvents
	session.Runtime = &factorysessions.LiveRuntime{
		Factory:        startupRuntime.RuntimeService(),
		BackendScopeID: startupRuntime.BackendScope(),
		RuntimeConfig:  runtimeConfig,
	}
	startupRuntime.AddEventTypeRecorder(func(eventType factorydefinitions.FactoryEventType) {
		if eventType == factorydefinitions.FactoryEventTypeSessionCompleted {
			a.responseStreams.Complete(session.ResponseEvents)
		}
	})
	a.registry.Upsert(session, true)

	runtime := NewSessionRuntime(
		factoryRootDir,
		clock,
		baseLogger,
		logger,
		runtimeBuild,
		startupRuntime,
		runtimeLifecycle,
		runtimeSidecars,
		durableExecution,
		dir,
		executionBaseDir,
		runtimeMode,
		backendScopeID,
		workFile,
		workflowID,
		workstationLoader,
		loadFactory,
		factoryScaffoldInitializer,
		editableFactoryValidator,
		reconnectCursorValidator,
		worldStateProjector,
		invocationMetricsRecorder,
		a.newJavaScriptCheckpointStore,
		a.sessionResultProjection,
		a.state,
		a.sessionIDs,
		a.resolveHome,
		a.directoryInspection,
		a.namedPaths,
		a.initialWorkFiles,
		a.identity,
	)
	if runtime == nil {
		return nil, nil, nil, nil, fmt.Errorf("Factory Sessions runtime is required")
	}
	gateway := NewWithResponseService(
		SessionServiceHost(runtime),
		a.state,
		sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle),
		a.state.ResponseStreams(),
		runtime.ReconnectCursorValidator(),
		a.sessionResultProjection,
		a.responseStreams,
	)
	gateway = runtime.AttachSessionGateway(gateway)
	a.Service = gateway
	invoker, err := NewInvocationOwner(runtime, a.interpolation, a.invocationWorkTypes, a.ttsObservability, a.invocationInputFiles)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return runtime, gateway, invoker, definitionHost{runtime: runtime}, nil
}

type definitionHost struct {
	runtime *SessionRuntime
}

func (h definitionHost) callbacks() DefinitionHostCallbacks {
	return DefinitionCallbacks(h.runtime)
}

func (h definitionHost) PersistRootDir() string { return h.callbacks().PersistRootDir() }
func (h definitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return h.callbacks().WorkstationLoader()
}
func (h definitionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return h.callbacks().CurrentRuntimeConfig()
}
func (h definitionHost) WorkflowID() string { return h.callbacks().WorkflowID() }
func (h definitionHost) RequireSession(id string) (*factorydefinitions.DefinitionSession, error) {
	session, err := h.callbacks().RequireSession(id)
	return projectDefinitionSession(session), err
}
func (h definitionHost) SessionRuntimeConfig(id string) (factorydefinitions.LoadedFactorySource, error) {
	return h.callbacks().SessionRuntimeConfig(id)
}
func (h definitionHost) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	return h.callbacks().SessionFactoryPersistRoot(h.liveSession(session))
}
func (h definitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return h.callbacks().ValidateEditableFactorySnapshot(ctx, snapshot)
}
func (h definitionHost) GetCurrentFactorySnapshotForSession(ctx context.Context, id string) (*factorydefinitions.FactorySnapshot, error) {
	return h.callbacks().GetCurrentFactorySnapshotForSession(ctx, id)
}
func (h definitionHost) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return h.callbacks().ReplaceFactoryLayoutAtDir(targetDir, prepared)
}
func (h definitionHost) AttachFactoryDefinitions(definitions factorydefinitions.Service) factorydefinitions.Service {
	return h.runtime.AttachFactoryDefinitionService(definitions)
}

var _ factorysessions.DefinitionActivationGatewayProvider = definitionHost{}

func (h definitionHost) DefinitionActivationGateway() factorysessions.DefinitionActivationGateway {
	return h.runtime.DefinitionActivationGateway()
}

func projectDefinitionSession(
	session *livesession.LiveSession,
) *factorydefinitions.DefinitionSession {
	if session == nil {
		return nil
	}
	return &factorydefinitions.DefinitionSession{
		ID:         session.ID,
		IsDefault:  session.IsDefault,
		FolderPath: session.FolderPath,
		FactoryDir: session.FactoryDir,
	}
}

func (h definitionHost) liveSession(
	session *factorydefinitions.DefinitionSession,
) *livesession.LiveSession {
	if session == nil {
		return nil
	}
	if live, err := h.callbacks().RequireSession(session.ID); err == nil && live != nil {
		return live
	}
	return &livesession.LiveSession{
		ID: session.ID,
		SessionState: livesession.SessionState{
			FolderPath: session.FolderPath,
			FactoryDir: session.FactoryDir,
		},
		IsDefault: session.IsDefault,
	}
}
