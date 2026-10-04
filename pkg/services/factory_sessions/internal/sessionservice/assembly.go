package service

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Assembly retains Factory Sessions-owned mutable registries while peer
// services are constructed against its root resolver roles.
type Assembly struct {
	roles.SessionGateway
	registry                     sessionregistry.Service
	state                        *sessionruntime.Service
	streams                      StreamManager
	invocationAuthority          InvocationAuthority
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	liveChangeCoordinator        factorysessioncontracts.LiveChangeCoordinator
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	recordedSessionInventory     recordings.RecordedSessionInventory
	directoryInspection          roles.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
	responseStreams              responsestreamservice.Service
	workAdmissionsMu             sync.Mutex
	workAdmissions               map[string][]*workAdmissionProjection
	// beforeWorkAdmissionProjectionRegistration is only populated by the
	// same-package replacement-window regression. It makes the otherwise
	// scheduler-dependent capture/registration gap deterministic without
	// changing the production dependency graph.
	beforeWorkAdmissionProjectionRegistration func()
}

// StreamManager supplies reusable provider progress and completion behavior.
type StreamManager interface {
	InferenceProgressPublisherFactory(*zap.Logger) func(string) factorysessions.ProgressPublisher
	DispatchCompletionObserverFactory() func(string) func(string)
}

// NewAssembly constructs an empty live-session directory.
func NewAssembly(
	registry sessionregistry.Service,
	state *sessionruntime.Service,
	streams StreamManager,
	authority InvocationAuthority,
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
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) roles.RuntimeAssembly {
	return &Assembly{
		SessionGateway:               &Service{},
		registry:                     registry,
		state:                        state,
		streams:                      streams,
		invocationAuthority:          authority,
		newJavaScriptCheckpointStore: newJavaScriptCheckpointStore,
		liveChangeCoordinator:        liveChangeCoordinator,
		sessionResultProjection:      sessionResultProjection,
		interpolation:                interpolation,
		invocationWorkTypes:          invocationWorkTypes,
		ttsObservability:             ttsObservability,
		eventIDs:                     eventIDs,
		sessionIDs:                   sessionIDs,
		resolveHome:                  resolveHome,
		recordedSessionInventory:     recordedSessionInventory,
		directoryInspection:          directoryInspection,
		namedPaths:                   namedPaths,
		invocationInputFiles:         invocationInputFiles,
		initialWorkFiles:             initialWorkFiles,
		identity:                     identityService,
		responseStreams:              responseStreamService,
		workAdmissions:               make(map[string][]*workAdmissionProjection),
	}
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
	// Replacement publishes the new session generation before retiring the old
	// generation's projection. Revalidate after registration so a resolver that
	// captured the old generation in that window releases it and retries rather
	// than recreating stale state after retirement.
	for {
		session := a.Resolve(sessionID)
		if session == nil || runtimebinding.ServiceForSession(session) == nil {
			a.releaseWorkAdmissionProjection(sessionID)
			return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
		}
		runtime := session.Runtime
		ingress, _ := runtimebinding.WorkAndEventIngressForLiveRuntime(runtime)
		var ledger recordings.Ledger
		if bundle := runtimebinding.BundleFromSession(session); bundle != nil {
			ledger = bundle.RecordingLedger()
		}
		if a.beforeWorkAdmissionProjectionRegistration != nil {
			a.beforeWorkAdmissionProjectionRegistration()
		}
		aliases := workSessionAliases(session)
		projection := a.workAdmissionProjection(sessionID, aliases, runtime, ledger)
		if a.workRuntimeGenerationIsCurrent(sessionID, runtime, ledger) {
			return workRuntimeAdapter{
				sessionID:      sessionID,
				sessionAliases: aliases,
				clock:          runtime.Clock,
				runtime:        runtimebinding.ServiceForSession(session),
				ingress:        ingress,
				admissions:     projection,
				ledger:         ledger,
				readMetrics:    session.InvocationMetricsRecorder,
			}, nil
		}
		a.discardWorkAdmissionProjection(sessionID, projection)
	}
}

func (a *Assembly) workRuntimeGenerationIsCurrent(
	sessionID string,
	runtime *factorysessions.LiveRuntime,
	ledger recordings.Ledger,
) bool {
	if a == nil || runtime == nil {
		return false
	}
	session := a.Resolve(sessionID)
	if session == nil || session.Runtime != runtime {
		return false
	}
	var currentLedger recordings.Ledger
	if bundle := runtimebinding.BundleFromSession(session); bundle != nil {
		currentLedger = bundle.RecordingLedger()
	}
	return sameLedger(currentLedger, ledger)
}

func (a *Assembly) discardWorkAdmissionProjection(
	sessionID string,
	target *workAdmissionProjection,
) {
	if a == nil || target == nil {
		return
	}
	a.workAdmissionsMu.Lock()
	projections := a.workAdmissions[sessionID]
	remaining := make([]*workAdmissionProjection, 0, len(projections))
	removed := false
	for _, projection := range projections {
		if projection == target {
			removed = true
			continue
		}
		remaining = append(remaining, projection)
	}
	if len(remaining) == 0 {
		delete(a.workAdmissions, sessionID)
	} else {
		a.workAdmissions[sessionID] = remaining
	}
	a.workAdmissionsMu.Unlock()
	if removed {
		target.Release()
	}
}

func (a *Assembly) workAdmissionProjection(
	sessionID string,
	aliases []string,
	runtime *factorysessions.LiveRuntime,
	ledger recordings.Ledger,
) *workAdmissionProjection {
	if a == nil {
		return nil
	}
	a.workAdmissionsMu.Lock()
	if a.workAdmissions == nil {
		a.workAdmissions = make(map[string][]*workAdmissionProjection)
	}
	var projection *workAdmissionProjection
	for _, candidate := range a.workAdmissions[sessionID] {
		if candidate.matchesGeneration(runtime, ledger) {
			projection = candidate
			break
		}
	}
	if projection == nil {
		projection = newWorkAdmissionProjectionForGeneration(sessionID, runtime, ledger, runtime.Clock)
		projection.sessionAliases = aliases
		a.workAdmissions[sessionID] = append(a.workAdmissions[sessionID], projection)
	}
	a.workAdmissionsMu.Unlock()
	projection.Bind(ledger)
	return projection
}

func (a *Assembly) releaseWorkAdmissionProjection(sessionID string) {
	if a == nil {
		return
	}
	a.workAdmissionsMu.Lock()
	projections := a.workAdmissions[sessionID]
	delete(a.workAdmissions, sessionID)
	a.workAdmissionsMu.Unlock()
	for _, projection := range projections {
		projection.Release()
	}
}

// retireWorkAdmissionProjection releases only the projection owned by a
// runtime generation that has completed replacement. An in-flight adapter may
// still hold the old projection, so Release preserves its detached admissions
// while retiring the ledger callback and generation identity.
func (a *Assembly) retireWorkAdmissionProjection(
	sessionID string,
	runtime *factorysessions.LiveRuntime,
	record factoryruntime.RuntimeRecord,
) {
	if a == nil || runtime == nil || record == nil {
		return
	}
	ledger := record.RecordingLedger()
	a.workAdmissionsMu.Lock()
	projections := a.workAdmissions[sessionID]
	if len(projections) == 0 {
		a.workAdmissionsMu.Unlock()
		return
	}
	remaining := make([]*workAdmissionProjection, 0, len(projections))
	retired := make([]*workAdmissionProjection, 0, 1)
	for _, projection := range projections {
		if projection.matchesGeneration(runtime, ledger) {
			retired = append(retired, projection)
			continue
		}
		remaining = append(remaining, projection)
	}
	if len(remaining) == 0 {
		delete(a.workAdmissions, sessionID)
	} else {
		a.workAdmissions[sessionID] = remaining
	}
	a.workAdmissionsMu.Unlock()
	for _, projection := range retired {
		projection.Release()
	}
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
	runtimeBuild runtimeports.RuntimeReplacementBuilder,
	startupRuntime runtimeports.RuntimeInstance,
	modelsScope models.RuntimeScopeRef,
	startupSpec factoryruntime.SessionBuildSpec,
	runtimeLifecycle runtimeports.RuntimeLifecycle,
	runtimeSidecars factorysessions.RuntimeSidecars,
	durableExecution durableexecution.Service,
	factoryDefinitions factorydefinitions.Service,
	factorySessionID string,
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
	roles.SessionGateway,
	roles.SessionInvoker,
	factorysessions.DefinitionHost,
	factorydefinitions.DefinitionActivationGateway,
	error,
) {
	if a == nil || a.state == nil || a.registry == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("Factory Sessions assembly is required")
	}
	if startupRuntime == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("default Factory Runtime is required")
	}
	identity := selectCompletionSessionIdentity(factorySessionID, startupSpec)
	runtimeConfig, ok := startupRuntime.LoadedRuntimeConfig().(factorydefinitions.LoadedFactorySource)
	if !ok || runtimeConfig == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("constructed runtime config does not expose Factory Definition snapshots")
	}
	session := livesession.NewWithRuntimeID(
		identity.id,
		startupRuntime.Directory(),
		startupRuntime.FolderDirectory(),
		startupRuntime.LoadedRuntimeConfig().RuntimeBaseDir(),
		identity.target,
		&runtimebinding.SessionState{Instance: startupRuntime, Spec: &startupSpec},
		identity.isDefault,
		filepath.Base(startupRuntime.FolderDirectory()),
		clock,
		a.sessionIDs,
		a.eventIDs,
		identity.runtimeID,
	)
	if session == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("construct live Factory Session: clock and response-event identity generator are required")
	}
	session.RuntimeEventSessionID = completionEventScopeID(identity.id, startupSpec)
	session.RetainedRuntimeMetricsSessionIDs = retainedRuntimeMetricsSessionIDs(
		livesession.CanonicalID(session),
		startupSpec.ResumeSourceCanonicalSessionID,
	)
	session.InvocationMetricsRecorder = invocationMetricsRecorder
	responseEvents, err := a.responseStreams.NewEventStore(livesession.CanonicalID(session), clock)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("construct live Factory Session response events: %w", err)
	}
	session.ResponseEvents = responseEvents
	session.Runtime = &factorysessions.LiveRuntime{
		Factory:               startupRuntime.RuntimeService(),
		WorkAndEventIngress:   runtimebinding.DeclaredWorkAndEventIngress(startupRuntime.RuntimeService()),
		Clock:                 clock,
		BackendScopeID:        startupRuntime.BackendScope(),
		RuntimeConfig:         runtimeConfig,
		LiveChangeEvents:      runtimebinding.NewLiveChangeEventLog(startupRuntime.RecordingLedger()),
		LiveChangeApplication: runtimebinding.NewLiveChangeApplication(startupRuntime.RuntimeService()),
		LiveChangeAdmission:   runtimebinding.NewLiveChangeAdmission(startupRuntime.RuntimeService()),
		LiveChangeLogger:      startupRuntime.RuntimeLogger(),
	}
	startupRuntime.AddEventTypeRecorder(func(eventType factorydefinitions.FactoryEventType) {
		if eventType == factorydefinitions.FactoryEventTypeSessionCompleted {
			a.responseStreams.Complete(session.ResponseEvents)
		}
	})
	runtime := NewSessionRuntime(
		factoryRootDir,
		clock,
		baseLogger,
		logger,
		runtimeBuild,
		startupRuntime,
		modelsScope,
		runtimeLifecycle,
		runtimeSidecars,
		durableExecution,
		factoryDefinitions,
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
		return nil, nil, nil, nil, nil, fmt.Errorf("Factory Sessions runtime is required")
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("Factory Session runtime state is required")
	}
	bound.Owner = runtime
	runtime.startupSessionID = identity.id
	runtime.bindRuntimeReadMetrics(startupRuntime)
	runtime.releaseWorkAdmissionProjection = a.releaseWorkAdmissionProjection
	runtime.retireWorkAdmissionProjection = a.retireWorkAdmissionProjection
	gateway := NewWithLiveChangeCoordinator(
		SessionServiceHost(runtime),
		a.state,
		sessionruntime.NewResponseStreamObserver(runtimebinding.ResponseStreamRuntimeFromSessionHandle),
		a.state.ResponseStreams(),
		runtime.ReconnectCursorValidator(),
		a.sessionResultProjection,
		a.responseStreams,
		a.liveChangeCoordinator,
	)
	gateway = runtime.AttachSessionGateway(gateway)
	gateway.bindRecordedSessionHistory(a.ListSessions)
	invoker, err := NewInvocationOwner(runtime, a.invocationAuthority, a.interpolation, a.invocationWorkTypes, a.ttsObservability, a.invocationInputFiles)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	bound.Invoker = invoker
	a.registry.Upsert(session, true)
	gateway.bindRootCapabilities(invoker, runtime.ActivateNamedFactory, runtime.DefinitionActivationGateway())
	// The per-runtime gateway is returned to the operation caller. The
	// process-scoped assembly keeps its original stable service slot so
	// concurrent session completions cannot replace or race the shared root.
	return runtime, gateway, invoker, definitionHost{runtime: runtime}, runtime.DefinitionActivationGateway(), nil
}

type completionSessionIdentity struct {
	id        string
	isDefault bool
	target    factorysessions.TargetRef
	runtimeID string
}

func selectCompletionSessionIdentity(factorySessionID string, startupSpec factoryruntime.SessionBuildSpec) completionSessionIdentity {
	sessionID := strings.TrimSpace(factorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	isDefault := sessionID == factorysessions.DefaultSessionID
	target := factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: sessionID}
	if isDefault {
		target = factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault}
	}
	runtimeID := ""
	if isDefault {
		metricsSessionID := strings.TrimSpace(startupSpec.MetricsSessionID)
		if metricsSessionID != "" && metricsSessionID != factorysessions.DefaultSessionID {
			runtimeID = metricsSessionID
		} else if livesession.IsUUIDID(startupSpec.SessionID) {
			runtimeID = strings.TrimSpace(startupSpec.SessionID)
		}
	}
	return completionSessionIdentity{id: sessionID, isDefault: isDefault, target: target, runtimeID: runtimeID}
}

func completionEventScopeID(factorySessionID string, startupSpec factoryruntime.SessionBuildSpec) string {
	if sourceID := strings.TrimSpace(startupSpec.ResumeSourceCanonicalSessionID); sourceID != "" {
		return sourceID
	}
	return strings.TrimSpace(factorySessionID)
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
		FolderPath: session.Placement().FolderPath,
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

// ObserveForSession derives observation from the canonical state registry
// without routing through detached gateways.
func (a *Assembly) ObserveForSession(
	ctx context.Context,
	sessionID string,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	session := a.Resolve(strings.TrimSpace(sessionID))
	if session == nil {
		return factoryruntime.ObserveResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	runtime := runtimebinding.ServiceForSession(session)
	if runtime == nil {
		return factoryruntime.ObserveResult{}, fmt.Errorf("%w: %s", factorysessions.ErrRuntimeNotAvailable, strings.TrimSpace(sessionID))
	}
	return runtime.Observe(ctx, request)
}

func (a *Assembly) InvokeFactorySession(ctx context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	session := a.Resolve(sessionID)
	if session == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil || bound.Invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: session invocation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	result, err := bound.Invoker.Invoke(ctx, sessionID, request)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return factorysessions.InvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status:        factorysessions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, FailureReason: result.FailureReason, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}

func (a *Assembly) ActivateNamedFactory(ctx context.Context, name string) error {
	if a == nil || a.state == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	session := a.state.Current()
	if session == nil {
		return factorysessions.ErrSessionNotFound
	}
	bound := runtimebinding.SessionStateFrom(session)
	if bound == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	owner, ok := bound.Owner.(interface {
		ActivateNamedFactory(context.Context, string) error
	})
	if !ok || owner == nil {
		return fmt.Errorf("%w: session activation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	return owner.ActivateNamedFactory(ctx, name)
}

func (a *Assembly) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	return controlplane.GetLiveFactorySession(ctx, a, sessionID)
}

func (a *Assembly) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	return controlplane.ListLiveFactorySessions(ctx, a)
}

func (a *Assembly) recordingRoot() (string, error) {
	if a == nil || a.resolveHome == nil {
		return "", fmt.Errorf("recorded session home directory resolver is required")
	}
	home, err := a.resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve recorded session home directory: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("resolve recorded session home directory: empty path")
	}
	return filepath.Join(home, ".you-agent-factory", "recordings"), nil
}

func (a *Assembly) PauseLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.applyLiveControlLegacy(ctx, sessionID, factorysessions.SessionControlPause, request)
}

func (a *Assembly) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.applyLiveControlLegacy(ctx, sessionID, factorysessions.SessionControlResume, request)
}

func (a *Assembly) CancelLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.applyLiveControlLegacy(ctx, sessionID, factorysessions.SessionControlCancel, request)
}

func (a *Assembly) TerminateLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.applyLiveControlLegacy(ctx, sessionID, factorysessions.SessionControlTerminate, request)
}

func (a *Assembly) CloseFactorySession(ctx context.Context, sessionID string) error {
	return a.CloseSession(ctx, sessionID)
}

func (a *Assembly) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryruntime.LiveSessionResult, error) {
	return controlplane.GetLiveFactorySessionResult(ctx, a, a.sessionResultProjection, sessionID)
}

func (a *Assembly) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryruntime.PartialSessionResult, error) {
	return controlplane.GetLiveFactorySessionPartialResult(ctx, a, sessionID)
}

func (a *Assembly) SubscribeFactoryResponseEvents(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	session := a.Resolve(request.SessionID)
	if session == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, request.SessionID)
	}
	return subscribeLiveResponses(ctx, a.responseStreams, session, request)
}

// workSessionAliases lists every identity a live session's recorded events may
// carry: the registry key, the canonical runtime ID, and the event-scope ID.
// A Work read by exact session ID must match events stamped with the ~default
// alias and vice versa, so both selector forms see the same admissions.
func workSessionAliases(session *livesession.LiveSession) []string {
	if session == nil {
		return nil
	}
	var aliases []string
	for _, candidate := range []string{
		session.ID,
		livesession.CanonicalID(session),
		livesession.EventScopeID(session),
		session.RuntimeFactorySessionID,
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !slices.Contains(aliases, candidate) {
			aliases = append(aliases, candidate)
		}
	}
	return aliases
}
