package service

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	factorysessions.Service
	registry                     sessionregistry.Service
	state                        *sessionruntime.Service
	streams                      streamManager
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
	detachedMu                   sync.RWMutex
	detachedGateways             map[string]factorysessions.Service
	workAdmissionsMu             sync.Mutex
	workAdmissions               map[string][]*workAdmissionProjection
	// beforeWorkAdmissionProjectionRegistration is only populated by the
	// same-package replacement-window regression. It makes the otherwise
	// scheduler-dependent capture/registration gap deterministic without
	// changing the production dependency graph.
	beforeWorkAdmissionProjectionRegistration func()
	workReadMetricsRecorder                   roles.InvocationMetricsRecorder
	detachedGatewayOrder                      []string
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
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) roles.RuntimeAssembly {
	if clock == nil || eventIDs == nil || sessionIDs == nil || resolveHome == nil || directoryInspection == nil || namedPaths == nil || invocationInputFiles == nil || initialWorkFiles == nil || sessionResultProjection == nil || identityService == nil || responseStreamService == nil || liveChangeCoordinator == nil {
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
		detachedGateways:             make(map[string]factorysessions.Service),
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
		projection := a.workAdmissionProjection(sessionID, runtime, ledger)
		if a.workRuntimeGenerationIsCurrent(sessionID, runtime, ledger) {
			return workRuntimeAdapter{
				sessionID:   sessionID,
				clock:       runtime.Clock,
				runtime:     runtimebinding.ServiceForSession(session),
				ingress:     ingress,
				admissions:  projection,
				ledger:      ledger,
				readMetrics: a.workReadMetricsRecorder,
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
	factorysessions.Service,
	roles.SessionInvoker,
	factorysessions.DefinitionHost,
	factorydefinitions.DefinitionActivationGateway,
	error,
) {
	if a == nil || a.state == nil || a.registry == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("Factory Sessions assembly is required")
	}
	a.workReadMetricsRecorder = invocationMetricsRecorder
	if startupRuntime == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("default Factory Runtime is required")
	}
	identity := selectCompletionSessionIdentity(factorySessionID, startupSpec)
	runtimeConfig, ok := startupRuntime.LoadedRuntimeConfig().(factorydefinitions.LoadedFactorySource)
	if !ok || runtimeConfig == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("constructed runtime config does not expose Factory Definition snapshots")
	}
	// A fresh default runtime now receives its canonical metrics identity before
	// construction. Keep the legacy session-identity edge at its historical
	// completion boundary so cancellation and other caller-visible effects do
	// not move earlier in the opening lifecycle.
	if startupSpec.CanonicalSessionIDGenerated && a.sessionIDs != nil {
		_ = a.sessionIDs()
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
	a.registry.Upsert(session, true)

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
	invoker, err := NewInvocationOwner(runtime, a.interpolation, a.invocationWorkTypes, a.ttsObservability, a.invocationInputFiles)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	gateway.bindRootCapabilities(invoker, runtime.ActivateNamedFactory, runtime.DefinitionActivationGateway())
	a.registerDetachedGateway(identity.id, gateway)
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

// registerDetachedGateway records the runtime gateway that owns one session.
// The process root retains this routing table; it never replaces its service
// slot and never constructs a second runtime-bound service.
func (a *Assembly) registerDetachedGateway(sessionID string, owner factorysessions.Service) {
	if a == nil || owner == nil {
		return
	}
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return
	}
	a.detachedMu.Lock()
	defer a.detachedMu.Unlock()
	if a.detachedGateways == nil {
		a.detachedGateways = make(map[string]factorysessions.Service)
	}
	if _, exists := a.detachedGateways[id]; !exists {
		a.detachedGatewayOrder = append(a.detachedGatewayOrder, id)
	}
	a.detachedGateways[id] = owner
}

func (a *Assembly) detachedOwner(sessionID string) (factorysessions.Service, error) {
	if a == nil {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	id := strings.TrimSpace(sessionID)
	a.detachedMu.RLock()
	owner, ok := a.detachedGateways[id]
	a.detachedMu.RUnlock()
	if !ok || owner == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, id)
	}
	return owner, nil
}

type sessionStatusObserver interface {
	ObserveForSession(
		context.Context,
		string,
		factoryruntime.ObserveRequest,
	) (factoryruntime.ObserveResult, error)
}

// ObserveForSession preserves the session identity while routing observation
// to the runtime gateway that owns that session.
func (a *Assembly) ObserveForSession(
	ctx context.Context,
	sessionID string,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	observer, ok := owner.(sessionStatusObserver)
	if !ok {
		return factoryruntime.ObserveResult{}, fmt.Errorf(
			"%w: session observation capability unavailable",
			factorysessions.ErrDetachedServiceUnavailable,
		)
	}
	return observer.ObserveForSession(ctx, sessionID, request)
}

func (a *Assembly) detachedLiveControlOwner(sessionID string) (factorysessions.LiveControlService, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return nil, err
	}
	control, ok := owner.(factorysessions.LiveControlService)
	if !ok {
		return nil, fmt.Errorf("%w: live control capability unavailable", factorysessions.ErrDetachedServiceUnavailable)
	}
	return control, nil
}

func (a *Assembly) detachedLiveResultOwner(sessionID string) (factorysessions.LiveResultService, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return nil, err
	}
	results, ok := owner.(factorysessions.LiveResultService)
	if !ok {
		return nil, fmt.Errorf("%w: live result capability unavailable", factorysessions.ErrDetachedServiceUnavailable)
	}
	return results, nil
}

func (a *Assembly) activeDetachedOwner() (factorysessions.Service, error) {
	if a == nil {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	a.detachedMu.RLock()
	defer a.detachedMu.RUnlock()
	for index := len(a.detachedGatewayOrder) - 1; index >= 0; index-- {
		owner := a.detachedGateways[a.detachedGatewayOrder[index]]
		if owner != nil {
			return owner, nil
		}
	}
	return nil, factorysessions.ErrDetachedServiceUnavailable
}

func (a *Assembly) detachedOwners() []factorysessions.Service {
	if a == nil {
		return nil
	}
	a.detachedMu.RLock()
	defer a.detachedMu.RUnlock()
	owners := make([]factorysessions.Service, 0, len(a.detachedGatewayOrder))
	for _, id := range a.detachedGatewayOrder {
		owner := a.detachedGateways[id]
		if owner == nil || containsDetachedOwner(owners, owner) {
			continue
		}
		owners = append(owners, owner)
	}
	return owners
}

func containsDetachedOwner(owners []factorysessions.Service, candidate factorysessions.Service) bool {
	candidateValue := reflect.ValueOf(candidate)
	for _, owner := range owners {
		ownerValue := reflect.ValueOf(owner)
		if !candidateValue.IsValid() || !ownerValue.IsValid() || candidateValue.Type() != ownerValue.Type() {
			continue
		}
		if candidateValue.Type().Comparable() {
			if candidateValue.Interface() == ownerValue.Interface() {
				return true
			}
		}
	}
	return false
}

func (a *Assembly) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	result, err := owner.StartAsync(ctx, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return factorysessions.SyncStartResult{}, err
	}
	result, err := owner.StartSync(ctx, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) ResumeInterruptedSession(ctx context.Context, sessionID string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	result, err := owner.ResumeInterruptedSession(ctx, sessionID, request)
	if err == nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) OpenFactorySession(ctx context.Context, request factorysessions.OpenRequest) (*factorysessions.OpenResult, error) {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return nil, err
	}
	result, err := owner.OpenFactorySession(ctx, request)
	if err == nil && result != nil {
		a.registerDetachedGateway(result.SessionID, owner)
	}
	return result, err
}

func (a *Assembly) InvokeFactorySession(ctx context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return owner.InvokeFactorySession(ctx, sessionID, request)
}

func (a *Assembly) ActivateNamedFactory(ctx context.Context, name string) error {
	owner, err := a.activeDetachedOwner()
	if err != nil {
		return err
	}
	return owner.ActivateNamedFactory(ctx, name)
}

func (a *Assembly) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.SessionProjection{}, err
	}
	return owner.GetFactorySession(ctx, sessionID)
}

func (a *Assembly) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.SessionReadResult{}, err
	}
	return owner.GetSession(ctx, sessionID)
}

func (a *Assembly) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	owners := a.detachedOwners()
	if len(owners) == 0 {
		return nil, factorysessions.ErrDetachedServiceUnavailable
	}
	result := make([]factorysessions.ReadProjection, 0)
	seen := make(map[string]struct{})
	for _, owner := range owners {
		projections, err := owner.ListFactorySessions(ctx)
		if err != nil {
			return nil, err
		}
		for _, projection := range projections {
			id := projection.Context.FactorySessionID
			if id != "" {
				if _, exists := seen[id]; exists {
					continue
				}
				seen[id] = struct{}{}
			}
			result = append(result, projection)
		}
	}
	return result, nil
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
	owner, err := a.detachedLiveControlOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.PauseLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) ResumeLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedLiveControlOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.ResumeLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) CancelLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedLiveLifecycleControlOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.CancelLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) TerminateLiveFactorySession(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedLiveLifecycleControlOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return owner.TerminateLiveFactorySession(ctx, sessionID, request)
}

func (a *Assembly) CloseFactorySession(ctx context.Context, sessionID string) error {
	owner, err := a.detachedLiveControlOwner(sessionID)
	if err != nil {
		return err
	}
	return owner.CloseFactorySession(ctx, sessionID)
}

func (a *Assembly) detachedLiveLifecycleControlOwner(sessionID string) (factorysessions.LiveLifecycleControlService, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return nil, err
	}
	control, ok := owner.(factorysessions.LiveLifecycleControlService)
	if !ok {
		return nil, fmt.Errorf("%w: live lifecycle control capability unavailable", factorysessions.ErrDetachedServiceUnavailable)
	}
	return control, nil
}

func (a *Assembly) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.Pause(ctx, sessionID, request)
	})
}

func (a *Assembly) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.Resume(ctx, sessionID, request)
	})
}

func (a *Assembly) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.Cancel(ctx, sessionID, request)
	})
}

func (a *Assembly) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.Terminate(ctx, sessionID, request)
	})
}

func (a *Assembly) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.Approve(ctx, sessionID, request)
	})
}

func (a *Assembly) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.RetryDispatch(ctx, sessionID, request)
	})
}

func (a *Assembly) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	return a.forwardDurableControl(sessionID, func(owner factorysessions.Service) (factorysessions.LifecycleControlResult, error) {
		return owner.InterruptDispatch(ctx, sessionID, request)
	})
}

func (a *Assembly) forwardDurableControl(
	sessionID string,
	operation func(factorysessions.Service) (factorysessions.LifecycleControlResult, error),
) (factorysessions.LifecycleControlResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return operation(owner)
}

func (a *Assembly) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	owner, err := a.detachedOwner(sessionID)
	if err != nil {
		return factorysessions.ResultReadResult{}, err
	}
	return owner.GetResult(ctx, sessionID, request)
}

func (a *Assembly) GetFactorySessionResult(ctx context.Context, sessionID string) (factoryruntime.LiveSessionResult, error) {
	owner, err := a.detachedLiveResultOwner(sessionID)
	if err != nil {
		return factoryruntime.LiveSessionResult{}, err
	}
	return owner.GetFactorySessionResult(ctx, sessionID)
}

func (a *Assembly) GetFactorySessionPartialResult(ctx context.Context, sessionID string) (factoryruntime.PartialSessionResult, error) {
	owner, err := a.detachedLiveResultOwner(sessionID)
	if err != nil {
		return factoryruntime.PartialSessionResult{}, err
	}
	return owner.GetFactorySessionPartialResult(ctx, sessionID)
}

func (a *Assembly) SubscribeFactoryResponseEvents(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	owner, err := a.detachedOwner(request.SessionID)
	if err != nil {
		return nil, err
	}
	return owner.SubscribeFactoryResponseEvents(ctx, request)
}
