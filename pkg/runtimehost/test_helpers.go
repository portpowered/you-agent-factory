package runtimehost

import (
	"context"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// TestHostOptions configures a host constructed for cross-package unit tests.
type TestHostOptions struct {
	Logger *zap.Logger
	Policy CoordinatorPolicy
	Config *Config
	Clock  factory.Clock
}

// NewTestHost constructs a host for unit tests in other packages.
func NewTestHost(opts TestHostOptions) *Host {
	host := &Host{
		logger: opts.Logger,
		policy: opts.Policy,
		cfg:    opts.Config,
		clock:  opts.Clock,
	}
	if host.logger == nil {
		host.logger = zap.NewNop()
	}
	return host
}

// BindTestStartupRuntime attaches a startup bundle and default session for unit tests.
func BindTestStartupRuntime(h *Host, bundle *factoryservice.Bundle) {
	if h == nil {
		return
	}
	if h.sessions == nil {
		h.sessions = factorysessions.NewRegistry()
	}
	handle := &liveRuntimeHandle{Bundle: bundle, RunDone: make(chan struct{})}
	h.registerLiveSession(DefaultFactorySessionID, handle, FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
	}, true)
	h.setRunState(context.Background(), DefaultFactorySessionID, handle)
}

// ServiceConfig returns the host startup config when present.
func (h *Host) ServiceConfig() *Config {
	if h == nil {
		return nil
	}
	return h.cfg
}

// CoordinatorPolicy exposes the normalized coordinator policy for tests.
func (h *Host) CoordinatorPolicy() CoordinatorPolicy {
	if h == nil {
		return CoordinatorPolicy{}
	}
	return h.coordinatorPolicy()
}

// SessionByID exposes session lookup for cross-package tests.
func (h *Host) SessionByID(sessionID string) *factorysessions.LiveSession {
	return h.sessionByID(sessionID)
}

// LiveSessionHandle exposes the live runtime handle for a session in tests.
func LiveSessionHandle(session *factorysessions.LiveSession) *factoryservice.Handle {
	return liveSessionHandle(session)
}

// LiveSessionBuildSpec exposes session build spec lookup for tests.
func LiveSessionBuildSpec(session *factorysessions.LiveSession) *runtimebuild.SessionBuildSpec {
	return liveSessionBuildSpec(session)
}

// SetFactorySaveForTest injects the factory-save collaborator in unit tests.
func (h *Host) SetFactorySaveForTest(collaborator FactorySaveSaver) {
	if h != nil {
		h.factorySave = collaborator
	}
}

// SetDefinitionsForTest injects the factory-definition collaborator in unit tests.
func (h *Host) SetDefinitionsForTest(collaborator FactoryDefinitionService) {
	if h != nil {
		h.definitions = collaborator
	}
}

// SetSessionGatewayForTest injects the session gateway collaborator in unit tests.
func (h *Host) SetSessionGatewayForTest(gateway SessionGateway) {
	if h != nil {
		h.sessionGateway = gateway
	}
}

// SetModelServiceForTest injects the model service collaborator in unit tests.
func (h *Host) SetModelServiceForTest(modelAPI apisurface.ModelAPI) {
	if h != nil {
		h.modelService = modelAPI
	}
}

// SetPolicyForTest injects explicit coordinator policy in unit tests.
func (h *Host) SetPolicyForTest(policy CoordinatorPolicy) {
	if h != nil {
		h.policy = policy
	}
}

// SetSessionsForTest injects the session registry in unit tests.
func (h *Host) SetSessionsForTest(sessions *factorysessions.Registry) {
	if h != nil {
		h.sessions = sessions
	}
}

// SetModelAssetsForTest injects the model asset puller in unit tests.
func (h *Host) SetModelAssetsForTest(puller modelAssetPuller) {
	if h != nil {
		h.modelAssets = puller
	}
}

// SetCoordinatorForTest injects the factory coordinator in unit tests.
func (h *Host) SetCoordinatorForTest(coordinator FactoryCoordinator) {
	if h != nil {
		h.coordinator = coordinator
	}
}

// SetConfigForTest injects startup config in unit tests.
func (h *Host) SetConfigForTest(cfg *Config) {
	if h != nil {
		h.cfg = cfg
	}
}

// SessionGatewayCollaborator exposes the session gateway for tests.
func (h *Host) SessionGatewayCollaborator() SessionGateway {
	if h == nil {
		return nil
	}
	return h.sessionGateway
}

// NewLiveSessionStateForTest constructs session handle state for unit tests.
func NewLiveSessionStateForTest(spec *runtimebuild.SessionBuildSpec) *LiveSessionState {
	return &liveSessionState{spec: spec}
}

// CurrentRuntimeConfig exposes the active runtime config for tests.
func (h *Host) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return h.currentRuntimeConfig()
}

// CurrentRunState exposes active run state for tests.
func (h *Host) CurrentRunState() *hostRunState {
	return h.currentRunState()
}

// CurrentSession exposes the active session for tests.
func (h *Host) CurrentSession() *factorysessions.LiveSession {
	return h.currentSession()
}

// Core exposes the composed runtime graph for tests.
func (h *Host) Core() *Core {
	if h == nil {
		return nil
	}
	return h.core
}

// SessionInvocationWaitInput carries invocation wait inputs for tests.
type SessionInvocationWaitInput = sessionInvocationWaitInput

// ResolveInvocationWaitTerminal exposes invocation wait classification for tests.
func (h *Host) ResolveInvocationWaitTerminal(
	sessionID string,
	input SessionInvocationWaitInput,
	worldState interfaces.FactoryWorldState,
	packagedTTSInvocation bool,
	primaryErr *invocations.PrimaryResultError,
) apisurface.FactoryInvocationResult {
	return h.resolveInvocationWaitTerminal(sessionID, input, worldState, packagedTTSInvocation, primaryErr)
}

// ResponseStreamsForTest exposes the session response stream set for tests.
func (s *LiveSessionState) ResponseStreamsForTest() *factorysessions.SessionResponseStreamSet {
	if s == nil {
		return nil
	}
	return s.responseStreams
}

// NewLiveSessionStateWithHandleForTest constructs session state bound to a handle for tests.
func NewLiveSessionStateWithHandleForTest(handle *factoryservice.Handle) *LiveSessionState {
	return &liveSessionState{handle: handle}
}

// LiveSessionRuntimeStateForTest exposes typed live session state for tests.
func LiveSessionRuntimeStateForTest(session *factorysessions.LiveSession) *LiveSessionState {
	return liveSessionRuntimeState(session)
}

// NewLiveSessionStateWithHandleAndStreamsForTest constructs session state with streams for tests.
func NewLiveSessionStateWithHandleAndStreamsForTest(
	handle *factoryservice.Handle,
	streams *factorysessions.SessionResponseStreamSet,
) *LiveSessionState {
	return &liveSessionState{handle: handle, responseStreams: streams}
}

// SetNewSessionResponseStreamForTest overrides session response stream construction in tests.
func (h *Host) SetNewSessionResponseStreamForTest(ctor func() *factorysessions.SessionResponseStream) {
	if h != nil {
		h.newSessionResponseStream = ctor
	}
}

// HostedWorkersForTest exposes hosted worker config for tests.
func (h *Host) HostedWorkersForTest() hostedworkers.Config {
	if h == nil {
		return hostedworkers.Config{}
	}
	return h.hostedWorkers
}

// ModelAssetsInitializedForTest reports whether model assets collaborator is wired.
func (h *Host) ModelAssetsInitializedForTest() bool {
	return h != nil && h.modelAssets != nil
}

// SetLoggerForTest injects the host logger in unit tests.
func (h *Host) SetLoggerForTest(logger *zap.Logger) {
	if h != nil {
		h.logger = logger
	}
}

// Logger exposes the host logger for tests.
func (h *Host) Logger() *zap.Logger {
	if h == nil {
		return nil
	}
	return h.logger
}

// SetDurableExecutionForTest injects durable execution service in unit tests.
func (h *Host) SetDurableExecutionForTest(service factorysessionexecution.Service) {
	if h != nil {
		h.durableExecution = service
	}
}

// DurableExecution exposes durable execution service for tests.
func (h *Host) DurableExecution() factorysessionexecution.Service {
	if h == nil {
		return nil
	}
	return h.durableExecution
}

// SetHostedWorkersForTest injects hosted-worker config in unit tests.
func (h *Host) SetHostedWorkersForTest(cfg hostedworkers.Config) {
	if h != nil {
		h.hostedWorkers = cfg
	}
}

// SetStartupBundleForTest injects the pre-run startup bundle in unit tests.
func (h *Host) SetStartupBundleForTest(bundle *factoryRuntimeBundle) {
	if h != nil {
		h.setStartupBundle(bundle)
	}
}

// StartupBundle exposes the pre-run startup bundle for tests.
func (h *Host) StartupBundle() *factoryservice.Bundle {
	if h == nil {
		return nil
	}
	return h.startupBundle
}

// RunStateSessionID exposes the active run session ID for tests.
func (rs *hostRunState) SessionID() string {
	if rs == nil {
		return ""
	}
	return rs.sessionID
}

// WireModelServiceCollaborator exposes model collaborator wiring for tests.
func WireModelServiceCollaborator(h *Host, cfg *Config) apisurface.ModelAPI {
	return wireModelServiceCollaborator(h, cfg)
}

// BuildSimpleDashboardRenderInput exposes dashboard render input assembly for tests.
func (h *Host) BuildSimpleDashboardRenderInput(ctx context.Context, now time.Time) (SimpleDashboardRenderInput, error) {
	return h.buildSimpleDashboardRenderInput(ctx, now)
}

// RenderDashboard exposes dashboard rendering for tests.
func (h *Host) RenderDashboard(ctx context.Context) {
	h.renderDashboard(ctx)
}

// SubmitWorkFile exposes startup work-file submission for tests.
func (h *Host) SubmitWorkFile(ctx context.Context) error {
	return h.submitWorkFile(ctx)
}

// BuildReplacementFactoryRuntime exposes named-factory runtime build for tests.
func (h *Host) BuildReplacementFactoryRuntime(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (*factoryRuntimeBundle, error) {
	return h.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
}

// SessionsRegistry exposes the live session registry for tests.
func (h *Host) SessionsRegistry() *factorysessions.Registry {
	if h == nil {
		return nil
	}
	return h.sessions
}

// RuntimeBuildService exposes the runtime-build collaborator for tests.
func (h *Host) RuntimeBuildService() *runtimebuild.Service {
	if h == nil {
		return nil
	}
	return h.runtimeBuild
}

// DefaultSession exposes the selected default live session for tests.
func (h *Host) DefaultSession() *factorysessions.LiveSession {
	if h == nil {
		return nil
	}
	return h.defaultSession()
}

// CurrentRuntimeBundle exposes the active runtime bundle for tests.
func (h *Host) CurrentRuntimeBundle() *factoryRuntimeBundle {
	return h.currentRuntimeBundle()
}

// InvocationInputSourceStructuredArgs is the structured-args input source label.
const InvocationInputSourceStructuredArgs = invocationInputSourceStructuredArgs

// ResolvedSessionInvocationInput carries normalized invocation input for tests.
type ResolvedSessionInvocationInput = resolvedSessionInvocationInput

// ResolveSessionInvocationInput exposes invocation input normalization for tests.
func ResolveSessionInvocationInput(
	cfg *interfaces.FactoryConfig,
	request factoryapi.InvocationRequest,
) (ResolvedSessionInvocationInput, error) {
	return resolveSessionInvocationInput(cfg, request)
}

// RegisterLiveSessionForTest registers a live session handle for unit tests.
func (h *Host) RegisterLiveSessionForTest(
	sessionID string,
	handle *factoryservice.Handle,
	target FactorySessionTarget,
	selectSession bool,
) {
	h.registerLiveSession(sessionID, handle, target, selectSession)
}

// SetRunStateForTest sets the active run state for unit tests.
func (h *Host) SetRunStateForTest(ctx context.Context, sessionID string, handle *factoryservice.Handle) {
	h.setRunState(ctx, sessionID, handle)
}

// RequireIdleRuntimeForTest exposes idle-runtime validation for unit tests.
func (h *Host) RequireIdleRuntimeForTest(ctx context.Context) error {
	return h.requireIdleRuntime(ctx)
}

// CurrentFactoryDefinitionVersionAtRootForTest exposes definition version lookup for tests.
func (h *Host) CurrentFactoryDefinitionVersionAtRootForTest(
	rootDir string,
	name factoryapi.FactoryName,
) (factoryapi.HybridLogicalTimestamp, error) {
	return h.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

// InferenceProgressPublisherForTest exposes inference progress publisher wiring for tests.
func (h *Host) InferenceProgressPublisherForTest(
	sessionID string,
	logger *zap.Logger,
) workerprovider.InferenceProgressPublisher {
	return h.inferenceProgressPublisher(sessionID, logger)
}

// SessionResponseStreamForTest exposes session response stream lookup for tests.
func (h *Host) SessionResponseStreamForTest(
	session *factorysessions.LiveSession,
	dispatchID string,
) *factorysessions.SessionResponseStream {
	return h.sessionResponseStream(session, dispatchID)
}

// SessionResponseStreamsForTest exposes session response stream set lookup for tests.
func (h *Host) SessionResponseStreamsForTest(session *factorysessions.LiveSession) *factorysessions.SessionResponseStreamSet {
	return h.sessionResponseStreams(session)
}

// FactoryRootDir exposes the normalized factory root directory for tests.
func (h *Host) FactoryRootDir() string {
	if h == nil {
		return ""
	}
	return h.factoryRootDir
}

// SetFactoryRootDirForTest sets the factory root directory in unit tests.
func (h *Host) SetFactoryRootDirForTest(dir string) {
	if h != nil {
		h.factoryRootDir = dir
	}
}

// ProbeFactorySessionTargetForTest exposes session target probing for tests.
func (h *Host) ProbeFactorySessionTargetForTest(
	folderPath string,
	factoryDir string,
	ref factorysessions.TargetRef,
) (factorysessions.Target, bool, *factorysessions.DiscoveryFailure) {
	return h.probeFactorySessionTarget(folderPath, factoryDir, ref)
}

// SubmitCronTickForTest exposes cron tick submission for unit tests.
func (h *Host) SubmitCronTickForTest(
	ctx context.Context,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	return h.submitCronTick(ctx, ws, firedAt)
}

// IsCanceledServiceStartupForTest reports canceled startup errors for unit tests.
func IsCanceledServiceStartupForTest(ctx context.Context, err error) bool {
	return isCanceledServiceStartup(ctx, err)
}

// OpenFactorySessionForTest opens a factory session from a directory in unit tests.
func (h *Host) OpenFactorySessionForTest(ctx context.Context, factoryDir string) (string, error) {
	return h.openFactorySession(ctx, factoryDir)
}

// CurrentLiveRuntimeForTest exposes the active live runtime handle for tests.
func (h *Host) CurrentLiveRuntimeForTest() *factoryservice.Handle {
	return h.currentLiveRuntime()
}

// WaitForLiveRuntimeStartForTest waits for a live runtime start in unit tests.
func (h *Host) WaitForLiveRuntimeStartForTest(ctx context.Context, handle *factoryservice.Handle) error {
	return h.waitForLiveRuntimeStart(ctx, handle)
}

// StopFactorySessionForTest stops a factory session in unit tests.
func (h *Host) StopFactorySessionForTest(sessionID string) error {
	return h.stopFactorySession(sessionID)
}

// CurrentRunStateForTest exposes active run state for tests.
func (h *Host) CurrentRunStateForTest() *hostRunState {
	return h.currentRunState()
}

// CurrentFactoryForTest exposes the active factory runtime for tests.
func (h *Host) CurrentFactoryForTest() factory.Factory {
	return h.currentFactory()
}

// SessionScopedRecordPathForTest builds a session-scoped record path for tests.
func SessionScopedRecordPathForTest(basePath string, sessionID string) string {
	return sessionScopedRecordPath(basePath, sessionID)
}

// RuntimeWorkflowContextForTest builds workflow context for tests.
func RuntimeWorkflowContextForTest(cfg *interfaces.FactoryConfig, sessionID string) *factory_context.FactoryContext {
	return runtimeWorkflowContext(cfg, sessionID)
}

// FactorySessionBackendScopeIDForTest resolves backend scope ID for tests.
func (h *Host) FactorySessionBackendScopeIDForTest(session *factorysessions.LiveSession) string {
	return factorySessionBackendScopeID(h, session)
}

// FactorySessionLogicalSessionKeyIDForTest exposes logical session key resolution for tests.
func FactorySessionLogicalSessionKeyIDForTest(session *factorysessions.LiveSession) string {
	return factorySessionLogicalSessionKeyID(session)
}

// RequireSessionForTest resolves a live session for service-package tests.
func RequireSessionForTest(h *Host, sessionID string) (*factorysessions.LiveSession, error) {
	return h.requireSession(sessionID)
}

// SessionFactoryPersistRootForTest resolves the durable persist root for a session in tests.
func SessionFactoryPersistRootForTest(serviceRootDir string, session *factorysessions.LiveSession) string {
	return sessionFactoryPersistRoot(serviceRootDir, session)
}
