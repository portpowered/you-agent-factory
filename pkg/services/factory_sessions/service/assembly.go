package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimebinding"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Assembly retains Factory Sessions-owned mutable registries while peer
// services are constructed against its root resolver roles.
type Assembly struct {
	registry                     factorysessions.Registry
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
	directoryInspection          factorysessions.DirectoryInspection
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
	directoryInspection factorysessions.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreamService responsestreamservice.Service,
) factorysessions.RuntimeAssembly {
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

func (a *Assembly) CurrentRuntime() *factorysessions.LiveRuntime {
	if a == nil || a.state == nil {
		return nil
	}
	return a.state.CurrentRuntime()
}

func (a *Assembly) Resolve(sessionID string) *factorysessions.LiveSession {
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
	invocationMetricsRecorder factorysessions.InvocationMetricsRecorder,
) (
	factorysessions.ApplicationRuntime,
	factorysessions.Service,
	factorysessions.SessionInvoker,
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
	session := factorysessions.NewLiveSession(
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
	responseEvents, err := a.responseStreams.NewEventStore(factorysessions.CanonicalFactorySessionID(session), clock)
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
func (h definitionHost) WithActivationLock(run func() error) error {
	return h.callbacks().WithActivationLock(run)
}
func (h definitionHost) RequireIdleRuntimeForSession(ctx context.Context, id string) error {
	return h.callbacks().RequireIdleRuntimeForSession(ctx, id)
}
func (h definitionHost) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) error {
	return h.callbacks().ActivateSessionEditableFactory(
		ctx, h.liveSession(session), sessionID, sessionRootDir, factoryDir, name, runtimeName,
	)
}
func (h definitionHost) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return h.callbacks().ReplaceFactoryLayoutAtDir(targetDir, prepared)
}
func (h definitionHost) SaveNow() time.Time   { return h.callbacks().SaveNow() }
func (h definitionHost) RunSessionID() string { return h.callbacks().RunSessionID() }
func (h definitionHost) SessionForActivation(id string) *factorydefinitions.DefinitionSession {
	return projectDefinitionSession(h.callbacks().SessionForActivation(id))
}
func (h definitionHost) NamedFactoryActivationPaths(session *factorydefinitions.DefinitionSession) (string, string) {
	return h.callbacks().NamedFactoryActivationPaths(h.liveSession(session))
}
func (h definitionHost) RequireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	id string,
	session *factorydefinitions.DefinitionSession,
) error {
	return h.callbacks().RequireIdleBeforeNamedFactoryActivation(ctx, id, h.liveSession(session))
}
func (h definitionHost) SwapPersistedNamedFactoryRuntime(
	ctx context.Context,
	id string,
	session *factorydefinitions.DefinitionSession,
	persistRoot string,
	folderPath string,
	factoryDir string,
	name string,
) error {
	return h.callbacks().SwapPersistedNamedFactoryRuntime(
		ctx, id, h.liveSession(session), persistRoot, folderPath, factoryDir, name,
	)
}
func (h definitionHost) AttachFactoryDefinitions(definitions factorydefinitions.Service) factorydefinitions.Service {
	return h.runtime.AttachFactoryDefinitionService(definitions)
}

func projectDefinitionSession(
	session *factorysessions.LiveSession,
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
) *factorysessions.LiveSession {
	if session == nil {
		return nil
	}
	if live, err := h.callbacks().RequireSession(session.ID); err == nil && live != nil {
		return live
	}
	return &factorysessions.LiveSession{
		ID: session.ID,
		SessionState: factorysessions.SessionState{
			FolderPath: session.FolderPath,
			FactoryDir: session.FactoryDir,
		},
		IsDefault: session.IsDefault,
	}
}
