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
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"

	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// Assembly retains Factory Sessions-owned mutable registries while peer
// services are constructed against its root resolver roles.
type Assembly struct {
	roles.SessionGateway
	factoryDefinitions           factorydefinitions.Service
	reconnectCursorValidator     factorysessions.ReconnectCursorValidator
	worldStateProjector          factoryruntime.WorldStateProjector
	registry                     sessionregistry.Service
	state                        *sessionruntime.Service
	streams                      StreamManager
	projectionReader             runtimebinding.SessionProjectionOwner
	factoryScaffoldInitializer   factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator     factorysessions.EditableFactoryValidator
	invocationMetricsRecorder    roles.InvocationMetricsRecorder
	namedFactoryActivator        func(context.Context, string) error
	definitionActivationGateway  factorydefinitions.DefinitionActivationGateway
	invoker                      roles.InvocationService
	scopeControl                 SessionScopeControl
	scopeActivation              SessionScopeActivation
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	recordedHistory              RecordedHistory
	directoryInspection          roles.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
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
	gateway roles.SessionGateway,
	registry sessionregistry.Service,
	state *sessionruntime.Service,
	streams StreamManager,
	invoker roles.InvocationService,
	control SessionScopeControl,
	activation SessionScopeActivation,
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreamService responsestreamservice.Service,
	recordedHistory RecordedHistory,
	projectionReader runtimebinding.SessionProjectionOwner,
	namedFactoryActivator func(context.Context, string) error,
	definitionActivationGateway factorydefinitions.DefinitionActivationGateway,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	invocationMetricsRecorder roles.InvocationMetricsRecorder,
	factoryDefinitions factorydefinitions.Service,
	reconnectCursorValidator factorysessions.ReconnectCursorValidator,
	worldStateProjector factoryruntime.WorldStateProjector,
) roles.RuntimeAssembly {
	return &Assembly{
		factoryDefinitions:           factoryDefinitions,
		reconnectCursorValidator:     reconnectCursorValidator,
		worldStateProjector:          worldStateProjector,
		SessionGateway:               gateway,
		factoryScaffoldInitializer:   factoryScaffoldInitializer,
		editableFactoryValidator:     editableFactoryValidator,
		invocationMetricsRecorder:    invocationMetricsRecorder,
		registry:                     registry,
		state:                        state,
		streams:                      streams,
		projectionReader:             projectionReader,
		namedFactoryActivator:        namedFactoryActivator,
		definitionActivationGateway:  definitionActivationGateway,
		invoker:                      invoker,
		scopeControl:                 control,
		scopeActivation:              activation,
		newJavaScriptCheckpointStore: newJavaScriptCheckpointStore,
		sessionResultProjection:      sessionResultProjection,
		eventIDs:                     eventIDs,
		sessionIDs:                   sessionIDs,
		resolveHome:                  resolveHome,
		recordedHistory:              recordedHistory,
		directoryInspection:          directoryInspection,
		namedPaths:                   namedPaths,
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

// ResolveWorkerSessionWorkRuntime binds the selected read without registering
// or catching up the unrelated Work admission projection on its first request.
func (a *Assembly) ResolveWorkerSessionWorkRuntime(sessionID string) (work.WorkerSessionWorkRuntimeReader, error) {
	for {
		session := a.Resolve(sessionID)
		if session == nil || runtimebinding.ServiceForSession(session) == nil {
			return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, sessionID)
		}
		runtime := session.Runtime
		var ledger recordings.Ledger
		if bundle := runtimebinding.BundleFromSession(session); bundle != nil {
			ledger = bundle.RecordingLedger()
		}
		if a.workRuntimeGenerationIsCurrent(sessionID, runtime, ledger) {
			return workRuntimeAdapter{runtime: runtimebinding.ServiceForSession(session)}, nil
		}
	}
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

func (a *Assembly) WithRuntimeReadForSession(sessionID string, read func(*factorysessions.LiveRuntime) error) error {
	if a == nil || a.state == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return a.state.WithRuntimeReadForSession(sessionID, read)
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

// RegisterOpening publishes scoped state through the fixed owner. Acquisition
// remains owned by the caller; release retires only this registration. The
// returned canonical record remains selected even if the registry key changes.
func (a *Assembly) RegisterOpening(ctx context.Context, facts roles.SessionOpeningFacts,
	record factoryruntime.RuntimeRecord, completion factoryruntime.RuntimeInitialCompletion, replacement factoryruntime.RuntimeReplacementBuilder, lifecycle factoryruntime.RuntimeLifecycle, sidecars factoryruntime.RuntimeSidecars, clock factoryruntime.Clock, logger *zap.Logger,
) (roles.ApplicationRuntime, *livesession.LiveSession, factorysessions.DefinitionHost, factorydefinitions.DefinitionActivationGateway, func(context.Context) error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if record == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("default Factory Runtime is required")
	}
	session, err := a.prepareOpeningSession(facts, record, completion, clock)
	startupRuntime := record
	if session == nil {
		return nil, nil, nil, nil, nil, err
	}
	runtime := &SessionRuntime{
		owner: a, openingSession: session,
		factoryRootDir: facts.FactoryRootDir, dir: facts.Directory, executionBaseDir: facts.ExecutionBaseDir,
		runtimeMode: facts.RuntimeMode, backendScopeID: facts.BackendScopeID,
		workFile: facts.WorkFile, workflowID: facts.WorkflowID, modelsScope: facts.ModelsScope,
		clock: clock, logger: logger,
		runtimeBuild: replacement, runtimeLifecycle: lifecycle, runtimeSidecars: sidecars,
		startupSessionID: session.ID, runtimeID: facts.RuntimeID, generationID: facts.GenerationID,
	}
	release := func(releaseCtx context.Context) error { return a.releaseOpening(releaseCtx, runtime, session) }
	if err != nil {
		if session.ResponseEvents == nil {
			release = nil
		}
		return nil, session, nil, nil, release, err
	}
	runtime.runtimeState.SetStartup(startupRuntime)
	bound := runtimebinding.SessionStateFrom(session)
	bound.Owner = runtime
	bound.Clock = clock
	bound.ProjectionBackendScope = facts.BackendScopeID
	bound.Logger = logger
	runtime.bindRuntimeReadMetrics(startupRuntime)
	if err := ctx.Err(); err != nil {
		return nil, session, nil, nil, release, err
	}
	a.registry.Upsert(session, true)
	logger.Debug("registered Factory Session opening", zap.String("session_id", facts.FactorySessionID),
		zap.String("runtime_id", facts.RuntimeID), zap.String("generation_id", facts.GenerationID))
	return runtime, session, definitionHost{runtime: runtime}, a.definitionActivationGateway, release, nil
}

func (a *Assembly) prepareOpeningSession(facts roles.SessionOpeningFacts, record factoryruntime.RuntimeRecord, completion factoryruntime.RuntimeInitialCompletion,
	clock factoryruntime.Clock,
) (*livesession.LiveSession, error) {
	startupRuntime := record
	identity := selectCompletionSessionIdentity(facts.FactorySessionID, completion)
	runtimeConfig, ok := startupRuntime.LoadedRuntimeConfig().(factorydefinitions.LoadedFactorySource)
	if !ok || runtimeConfig == nil {
		return nil, fmt.Errorf("constructed runtime config does not expose Factory Definition snapshots")
	}
	session := livesession.NewWithRuntimeID(
		identity.id,
		startupRuntime.Directory(),
		startupRuntime.FolderDirectory(),
		startupRuntime.LoadedRuntimeConfig().RuntimeBaseDir(),
		identity.target,
		&runtimebinding.SessionState{Instance: startupRuntime, Spec: &completion},
		identity.isDefault,
		filepath.Base(startupRuntime.FolderDirectory()),
		clock,
		a.sessionIDs,
		a.eventIDs,
		identity.runtimeID,
	)
	if session == nil {
		return nil, fmt.Errorf("construct live Factory Session: clock and response-event identity generator are required")
	}
	session.RuntimeEventSessionID = completionEventScopeID(identity.id, completion)
	session.RetainedRuntimeMetricsSessionIDs = retainedRuntimeMetricsSessionIDs(
		livesession.CanonicalID(session),
		completion.ResumeSourceCanonicalSessionID,
	)
	session.InvocationMetricsRecorder = a.invocationMetricsRecorder
	responseEvents, err := a.responseStreams.NewEventStore(livesession.CanonicalID(session), clock)
	session.ResponseEvents = responseEvents
	if err != nil {
		return session, fmt.Errorf("construct live Factory Session response events: %w", err)
	}
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
	return session, nil
}

// releaseOpening detaches a captured registration without closing Runtime,
// Models, or durable resources owned by the outer acquisition cleanup.
func (a *Assembly) releaseOpening(ctx context.Context, scope *SessionRuntime, session *livesession.LiveSession) error {
	scope.releaseMu.Lock()
	defer scope.releaseMu.Unlock()
	if scope.releaseDone {
		return nil
	}
	if err := a.scopeActivation.Retire(ctx, SessionScope{Session: session}); err != nil {
		scope.logger.Warn("release Factory Session registration failed", zap.Error(err),
			zap.String("session_id", session.ID), zap.String("runtime_id", scope.runtimeID), zap.String("generation_id", scope.generationID))
		return err
	}
	// A runtime replacement retains this response history. Its current
	// registration, rather than the retired generation, owns completion.
	current := a.state.Resolve(session.ID)
	if current == nil || current.ResponseEvents != session.ResponseEvents {
		a.responseStreams.Complete(session.ResponseEvents)
	}
	a.retireWorkAdmissionProjection(session.ID, session.Runtime, runtimebinding.BundleFromSession(session))
	scope.runtimeState.ClearStartup()
	scope.releaseDone = true
	scope.logger.Debug("released Factory Session registration", zap.String("session_id", session.ID),
		zap.String("runtime_id", scope.runtimeID), zap.String("generation_id", scope.generationID))
	return nil
}

type completionSessionIdentity struct {
	id        string
	isDefault bool
	target    factorysessions.TargetRef
	runtimeID string
}

func selectCompletionSessionIdentity(factorySessionID string, completion factoryruntime.RuntimeInitialCompletion) completionSessionIdentity {
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
		metricsSessionID := strings.TrimSpace(completion.MetricsSessionID)
		if metricsSessionID != "" && metricsSessionID != factorysessions.DefaultSessionID {
			runtimeID = metricsSessionID
		} else if livesession.IsUUIDID(completion.SessionID) {
			runtimeID = strings.TrimSpace(completion.SessionID)
		}
	}
	return completionSessionIdentity{id: sessionID, isDefault: isDefault, target: target, runtimeID: runtimeID}
}

func completionEventScopeID(factorySessionID string, completion factoryruntime.RuntimeInitialCompletion) string {
	if sourceID := strings.TrimSpace(completion.ResumeSourceCanonicalSessionID); sourceID != "" {
		return sourceID
	}
	return strings.TrimSpace(factorySessionID)
}

type definitionHost struct {
	runtime *SessionRuntime
}

func (h definitionHost) PersistRootDir() string {
	if h.runtime.factoryRootDir != "" {
		return h.runtime.factoryRootDir
	}
	return h.runtime.dir
}
func (h definitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader { return nil }
func (h definitionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return h.runtime.currentRuntimeConfig()
}
func (h definitionHost) WorkflowID() string { return h.runtime.workflowID }
func (h definitionHost) RequireSession(id string) (*factorydefinitions.DefinitionSession, error) {
	session, err := runtimebinding.RequireLiveSession(h.runtime.owner.state, id)
	return projectDefinitionSession(session), err
}
func (h definitionHost) SessionRuntimeConfig(id string) (factorydefinitions.LoadedFactorySource, error) {
	return runtimebinding.RuntimeConfigForSession(h.runtime.owner.state, id)
}
func (h definitionHost) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	return logicaltarget.SessionFactoryPersistRoot(h.runtime.factoryRootDir, h.liveSession(session))
}
func (h definitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return h.runtime.owner.editableFactoryValidator(ctx, snapshot, nil)
}
func (h definitionHost) GetCurrentFactorySnapshotForSession(ctx context.Context, id string) (*factorydefinitions.FactorySnapshot, error) {
	current, err := h.runtime.owner.factoryDefinitions.GetCurrentFactoryForSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if current.Snapshot == nil {
		return nil, fmt.Errorf("current factory snapshot is unavailable")
	}
	return current.Snapshot, nil
}
func (h definitionHost) ReplaceFactoryLayoutAtDir(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, fmt.Errorf("factory layout replacement is owned by Factory Definitions")
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
	if live, err := runtimebinding.RequireLiveSession(h.runtime.owner.state, session.ID); err == nil && live != nil {
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
	if bound == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("%w: session invocation owner is unavailable", factorysessions.ErrRuntimeNotAvailable)
	}
	result, err := a.invoker.Invoke(ctx, sessionID, request)
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
	if a == nil || a.namedFactoryActivator == nil {
		return factorysessions.ErrRuntimeNotAvailable
	}
	return a.namedFactoryActivator(ctx, name)
}

func (a *Assembly) GetFactorySession(ctx context.Context, sessionID string) (factorysessions.SessionProjection, error) {
	return controlplane.GetLiveFactorySession(ctx, a, sessionID)
}

func (a *Assembly) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	return controlplane.ListLiveFactorySessions(ctx, a)
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

// FactoryConfigForSession narrows the existing selected projection for model invocation.
func (a *Assembly) FactoryConfigForSession(ctx context.Context, sessionID string) (*factorydefinitions.FactoryConfig, error) {
	projection, err := a.GetFactorySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return projection.Context.FactoryCfg, nil
}

// BindHistoricalOpening keeps optional historical routing at its existing owner.
func (a *Assembly) BindHistoricalOpening(sessionID string, owner durableexecution.Service) (func(), error) {
	binder, ok := a.SessionGateway.(interface {
		BindHistoricalExecution(string, durableexecution.Service) func()
	})
	if !ok {
		return nil, fmt.Errorf("historical replay Sessions routing is unavailable")
	}
	return binder.BindHistoricalExecution(sessionID, owner), nil
}
