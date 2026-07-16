package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workersservice "github.com/portpowered/infinite-you/pkg/workers/service"
	"go.uber.org/zap"
)

// FactoryDefinitionService owns current-factory read models for the phase-one
// compatibility facade.
type FactoryDefinitionService interface {
	GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error)
	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
}

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (fs *FactoryService) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.requireDefinitions().GetCurrentNamedFactory(ctx)
}

func (fs *FactoryService) buildSessionEditableFactoryReplacement(
	ctx context.Context,
	sessionRootDir string,
	factoryDir string,
	sessionID string,
	name factoryapi.FactoryName,
) (*factoryRuntimeBundle, error) {
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return replacement, nil
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (fs *FactoryService) GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.requireDefinitions().GetCurrentNamedFactory(ctx)
}

type factoryDefinitionHost struct {
	*FactoryService
}

var _ factorydefinition.Host = factoryDefinitionHost{}

func (h factoryDefinitionHost) PersistRootDir() string {
	if h.FactoryService == nil {
		return ""
	}
	rootDir := h.FactoryService.factoryRootDir
	if rootDir == "" && h.FactoryService.cfg != nil {
		rootDir = h.FactoryService.cfg.Dir
	}
	return rootDir
}

func (h factoryDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	if h.FactoryService == nil || h.FactoryService.cfg == nil {
		return nil
	}
	return h.FactoryService.cfg.WorkstationLoader
}

func (h factoryDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.currentRuntimeConfig()
}

func (h factoryDefinitionHost) WorkflowID() string {
	if h.FactoryService == nil {
		return ""
	}
	return h.FactoryService.workflowID()
}

func (h factoryDefinitionHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.requireSession(sessionID)
}

func (h factoryDefinitionHost) SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.sessionRuntimeConfig(sessionID)
}

func (h factoryDefinitionHost) SessionFactoryPersistRoot(session *factorysessions.LiveSession) string {
	if h.FactoryService == nil {
		return ""
	}
	return sessionFactoryPersistRoot(h.FactoryService.factoryRootDir, session)
}

func (h factoryDefinitionHost) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if h.FactoryService == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.requireDefinitions().GetCurrentFactoryForSession(ctx, sessionID)
}

func (h factoryDefinitionHost) WithActivationLock(fn func() error) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.withActivationLock(fn)
}

func (h factoryDefinitionHost) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.requireIdleRuntimeForSession(ctx, sessionID)
}

func (h factoryDefinitionHost) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.activateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (h factoryDefinitionHost) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	if h.FactoryService == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.FactoryService.replaceFactoryLayoutAtDir(targetDir, prepared)
}

func (h factoryDefinitionHost) SaveNow() time.Time {
	if h.FactoryService == nil || h.FactoryService.clock == nil {
		return time.Now().UTC()
	}
	return h.FactoryService.clock.Now().UTC()
}

func (h factoryDefinitionHost) RunSessionID() string {
	if h.FactoryService == nil {
		return ""
	}
	return h.FactoryService.runSessionID()
}

func (h factoryDefinitionHost) SessionForActivation(sessionID string) *factorysessions.LiveSession {
	if h.FactoryService == nil {
		return nil
	}
	return h.FactoryService.sessionByID(sessionID)
}

func (h factoryDefinitionHost) NamedFactoryActivationPaths(session *factorysessions.LiveSession) (persistRoot, folderPath string) {
	if h.FactoryService == nil {
		return "", ""
	}
	return h.FactoryService.namedFactoryActivationPaths(session)
}

func (h factoryDefinitionHost) RequireIdleBeforeNamedFactoryActivation(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.FactoryService.requireIdleBeforeNamedFactoryActivation(ctx, sessionID, session)
}

func (h factoryDefinitionHost) SwapPersistedNamedFactoryRuntime(
	ctx context.Context,
	sessionID string,
	session *factorysessions.LiveSession,
	persistRoot string,
	folderPath string,
	factoryDir string,
	name string,
) error {
	if h.FactoryService == nil {
		return fmt.Errorf("factory service is required")
	}
	replacement, err := h.FactoryService.buildReplacementFactoryRuntime(ctx, folderPath, factoryDir, sessionID)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return h.FactoryService.applyNamedFactoryReplacement(ctx, sessionID, session, persistRoot, name, replacement)
}

var _ FactoryDefinitionService = (*factorydefinition.Service)(nil)

func newFactoryDefinitionService(fs *FactoryService) FactoryDefinitionService {
	return factorydefinition.New(factoryDefinitionHost{FactoryService: fs})
}

func (fs *FactoryService) requireDefinitions() FactoryDefinitionService {
	if fs == nil {
		return factorydefinition.New(factoryDefinitionHost{})
	}
	if fs.definitions == nil {
		fs.definitions = newFactoryDefinitionService(fs)
	}
	return fs.definitions
}

func (fs *FactoryService) definitionService() *factorydefinition.Service {
	if svc, ok := fs.requireDefinitions().(*factorydefinition.Service); ok {
		return svc
	}
	return nil
}

func (fs *FactoryService) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if svc := fs.definitionService(); svc != nil {
		return svc.CurrentFactoryDefinitionVersionAtRoot(rootDir, name)
	}
	return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("factory definition service is required")
}

// factorySaveSaver is the injectable factory-save collaborator seam.
type factorySaveSaver interface {
	Save(
		ctx context.Context,
		sessionID string,
		mode factoryapi.FactorySaveMode,
		request factoryapi.Factory,
	) (factoryapi.Factory, error)
}

var _ factorySaveSaver = (*factorysave.Service)(nil)

// SaveFactoryForSession is the single orchestrated pipeline for session-scoped
// factory submission. It delegates to the factorysave collaborator.
func (fs *FactoryService) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if fs == nil || fs.factorySave == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	return fs.factorySave.Save(ctx, sessionID, mode, request)
}

// SaveCurrentFactoryForSession replaces the current factory definition for one
// live session using REPLACE_CURRENT semantics.
func (fs *FactoryService) SaveCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return fs.SaveFactoryForSession(ctx, sessionID, factoryapi.FactorySaveModeReplaceCurrent, request)
}

func sessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	return factorydefinition.SessionFactoryPersistRoot(serviceRootDir, session)
}

type factorySaveHost struct {
	*FactoryService
}

var _ factorysave.Host = factorySaveHost{}

func (h factorySaveHost) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return svc.Save(ctx, sessionID, mode, request)
}

func newFactorySaveService(fs *FactoryService) *factorysave.Service {
	return factorysave.New(factorySaveHost{fs})
}

func wireFactorySaveCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) factorySaveSaver {
	if cfg != nil && cfg.FactorySave != nil {
		return cfg.FactorySave
	}
	return newFactorySaveService(fs)
}

func (fs *FactoryService) withActivationLock(fn func() error) error {
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()
	return fn()
}

func (fs *FactoryService) activateSessionEditableFactory(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, factoryDir, sessionID, name)
	if err != nil {
		return err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return err
	}
	return fs.replaceSessionRuntime(ctx, session, runtimeName, replacement)
}

func (fs *FactoryService) replaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return configpersist.ReplaceFactoryLayoutAtDirWithPreparedWithResult(
		targetDir,
		prepared,
		configpersist.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
}

// Factory service composition seams (wire / BuildFactoryService). Co-located here
// to keep the root pkg/service package within the pkg-file-count cap.

// FactoryServiceRoot holds the absolutized factory directory and base logger
// after FactoryServiceConfig normalization during service construction.
type FactoryServiceRoot struct {
	FactoryRootDir string
	BaseLogger     *zap.Logger
}

// ResolveFactoryServiceRoot absolutizes cfg.Dir, assigns cfg.Logger, and mints
// cfg.RuntimeInstanceID when empty.
func ResolveFactoryServiceRoot(cfg *FactoryServiceConfig) (FactoryServiceRoot, error) {
	factoryRootDir, baseLogger, err := resolveFactoryServiceRoot(cfg)
	if err != nil {
		return FactoryServiceRoot{}, err
	}
	return FactoryServiceRoot{
		FactoryRootDir: factoryRootDir,
		BaseLogger:     baseLogger,
	}, nil
}

// NewFactorySessionsRegistry constructs the live session registry collaborator.
func NewFactorySessionsRegistry() *factorysessions.Registry {
	return factorysessions.NewRegistry()
}

// LocalModelDomain wires pkg/models/local runtime dependencies constructed at
// service build time and copied onto each factoryRuntimeBundle.
type LocalModelDomain = factoryservice.LocalModelDomain

// LocalModelDomainDependencies adapts compatibility configuration to the
// canonical model-package construction contract.
func LocalModelDomainDependencies(cfg *FactoryServiceConfig) modelhost.LocalDomainDependencies {
	return factoryservice.LocalModelDomainDependencies(hostConfigFromService(cfg))
}

// FactoryServiceCollaborators groups explicit S6 composition collaborators.
type FactoryServiceCollaborators struct {
	Sessions         *factorysessions.Registry
	LocalModels      LocalModelDomain
	RuntimeBuild     *runtimebuild.Service
	WorkersScheduler *workersservice.Service
}

// NewFactoryServiceCollaborators builds S6 collaborators using the provided
// session registry and freshly constructed local-model dependencies.
func NewFactoryServiceCollaborators(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
) (FactoryServiceCollaborators, error) {
	if sessions == nil {
		return FactoryServiceCollaborators{}, fmt.Errorf("construct Factory Service collaborators: Factory Session registry is required")
	}
	startupLocalModels, err := modelhost.NewLocalDomain(LocalModelDomainDependencies(cfg))
	if err != nil {
		return FactoryServiceCollaborators{}, err
	}
	hostedWorkers := NewHostedWorkersConfig(cfg, baseLogger, clock)
	runtimeBuild, err := newRuntimeBuildService(
		cfg,
		clock,
		baseLogger,
		&startupLocalModels,
		newInferenceProgressPublisherFactory(sessions, baseLogger),
		newSessionDispatchCompletionObserverFactory(sessions),
	)
	if err != nil {
		return FactoryServiceCollaborators{}, err
	}
	return FactoryServiceCollaborators{
		Sessions:         sessions,
		LocalModels:      startupLocalModels,
		RuntimeBuild:     runtimeBuild,
		WorkersScheduler: workersservice.NewWorkersSchedulerService(workersSchedulerServiceConfig(cfg, clock, baseLogger, hostedWorkers)),
	}, nil
}

// ServiceClockForCompose selects the factory clock for the loaded replay artifact.
func ServiceClockForCompose(cfg *FactoryServiceConfig, load FactoryConfigLoadResult) factory.Clock {
	return serviceClockForMode(cfg.Clock, load.ReplayArtifact)
}

// NewHostedWorkersConfig builds the hosted-workers collaborator from service config.
func NewHostedWorkersConfig(
	cfg *FactoryServiceConfig,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return buildHostedWorkersConfig(cfg, logger, clock)
}

// ComposeCollaboratorSnapshot records whether S6 collaborators were initialized
// on a built FactoryService. Tests compare snapshots across wire and direct build paths.
type ComposeCollaboratorSnapshot struct {
	SessionsInitialized         bool
	RuntimeBuildInitialized     bool
	WorkersSchedulerInitialized bool
	LocalModelsInitialized      bool
	ModelAssetsInitialized      bool
	ModelServiceInitialized     bool
	FactorySaveInitialized      bool
	DefinitionsInitialized      bool
	HostedWorkersLoggerReady    bool
	BundleModelResources        bool
	BundleLocalModels           bool
}

// FactoryCore owns the normalized runtime graph assembled before transport
// facades or runtime loops begin.
type FactoryCore struct {
	cfg              *FactoryServiceConfig
	root             FactoryServiceRoot
	collaborators    FactoryServiceCollaborators
	hostedWorkers    hostedworkers.Config
	clock            factory.Clock
	startupBundle    *factoryRuntimeBundle
	logger           *zap.Logger
	modelAssets      modelAssetPuller
	durableExecution factorysessionexecution.Service
}

// FactoryRootDir returns the canonical factory root selected at build time.
func (core *FactoryCore) FactoryRootDir() string {
	if core == nil {
		return ""
	}
	return core.root.FactoryRootDir
}

// BaseLogger returns the base logger used for runtime graph assembly.
func (core *FactoryCore) BaseLogger() *zap.Logger {
	if core == nil {
		return nil
	}
	return core.root.BaseLogger
}

// Logger returns the startup runtime logger.
func (core *FactoryCore) Logger() *zap.Logger {
	if core == nil {
		return nil
	}
	return core.logger
}

// Clock returns the normalized service clock.
func (core *FactoryCore) Clock() factory.Clock {
	if core == nil {
		return nil
	}
	return core.clock
}

// Sessions returns the live session registry collaborator owned by the core.
func (core *FactoryCore) Sessions() *factorysessions.Registry {
	if core == nil {
		return nil
	}
	return core.collaborators.Sessions
}

// WorkersScheduler returns the workers scheduling collaborator owned by the core.
func (core *FactoryCore) WorkersScheduler() *workersservice.Service {
	if core == nil {
		return nil
	}
	return core.collaborators.WorkersScheduler
}

// RuntimeBuild returns the runtime-build collaborator owned by the core.
func (core *FactoryCore) RuntimeBuild() *runtimebuild.Service {
	if core == nil {
		return nil
	}
	return core.collaborators.RuntimeBuild
}

// ModelHost returns the process-wide model host collaborator.
func (core *FactoryCore) ModelHost() modelhost.Host {
	if core == nil {
		return nil
	}
	return core.collaborators.LocalModels.Host
}

// LocalModels returns the startup local-model collaborator group.
func (core *FactoryCore) LocalModels() LocalModelDomain {
	if core == nil {
		return LocalModelDomain{}
	}
	return core.collaborators.LocalModels
}

// HostedWorkers returns the hosted-worker config built for this runtime graph.
func (core *FactoryCore) HostedWorkers() hostedworkers.Config {
	if core == nil {
		return hostedworkers.Config{}
	}
	return core.hostedWorkers
}

// StartupBundle returns the pre-run runtime bundle built during composition.
func (core *FactoryCore) StartupBundle() *factoryRuntimeBundle {
	if core == nil {
		return nil
	}
	return core.startupBundle
}

// ModelAssetPuller returns the explicit model asset collaborator used by
// direct model operations.
func (core *FactoryCore) ModelAssetPuller() modelAssetPuller {
	if core == nil {
		return nil
	}
	return core.modelAssets
}

// DurableExecution returns the durable execution collaborator owned by this graph.
func (core *FactoryCore) DurableExecution() factorysessionexecution.Service {
	if core == nil {
		return nil
	}
	return core.durableExecution
}

// ComposeCollaboratorSnapshot reports initialized core collaborators for
// equivalence tests.
func (core *FactoryCore) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if core == nil {
		return ComposeCollaboratorSnapshot{}
	}
	bundle := core.StartupBundle()
	snapshot := ComposeCollaboratorSnapshot{
		SessionsInitialized:         core.Sessions() != nil,
		RuntimeBuildInitialized:     core.RuntimeBuild() != nil,
		WorkersSchedulerInitialized: core.WorkersScheduler() != nil,
		LocalModelsInitialized:      core.LocalModels().Manager != nil,
		ModelAssetsInitialized:      core.ModelAssetPuller() != nil,
		DefinitionsInitialized:      true,
		HostedWorkersLoggerReady:    core.HostedWorkers().Logger != nil,
	}
	if bundle != nil {
		snapshot.BundleModelResources = bundle.ModelResources != nil
		snapshot.BundleLocalModels = bundle.LocalModels != nil
	}
	return snapshot
}

// ComposeCollaboratorSnapshot reports initialized S6 collaborators for equivalence tests.
func (fs *FactoryService) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if fs == nil {
		return ComposeCollaboratorSnapshot{}
	}
	snapshot := ComposeCollaboratorSnapshot{
		ModelServiceInitialized: fs.modelService != nil,
		FactorySaveInitialized:  fs.factorySave != nil,
		DefinitionsInitialized:  fs.definitions != nil,
	}
	if fs.core != nil {
		coreSnapshot := fs.core.ComposeCollaboratorSnapshot()
		coreSnapshot.ModelServiceInitialized = snapshot.ModelServiceInitialized
		coreSnapshot.FactorySaveInitialized = snapshot.FactorySaveInitialized
		coreSnapshot.DefinitionsInitialized = snapshot.DefinitionsInitialized
		return coreSnapshot
	}
	bundle := fs.currentRuntimeBundle()
	snapshot.SessionsInitialized = fs.sessions != nil
	snapshot.RuntimeBuildInitialized = fs.runtimeBuild != nil
	snapshot.WorkersSchedulerInitialized = fs.workersScheduler != nil
	snapshot.ModelAssetsInitialized = fs.modelAssets != nil
	snapshot.HostedWorkersLoggerReady = fs.hostedWorkers.Logger != nil
	if bundle != nil {
		snapshot.BundleModelResources = bundle.ModelResources != nil
		snapshot.BundleLocalModels = bundle.LocalModels != nil
	}
	return snapshot
}

// FactoryServiceShell is the pre-factorysave FactoryService assembly product for
// Wire composition.
type FactoryServiceShell struct {
	Service *FactoryService
}

// BuildFactoryCore constructs the normalized runtime graph without attaching a
// transport-facing compatibility facade.
func BuildFactoryCore(ctx context.Context, cfg *FactoryServiceConfig) (*FactoryCore, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	root, err := ResolveFactoryServiceRoot(cfg)
	if err != nil {
		return nil, err
	}
	if err := ensureServiceBackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	load, err := LoadFactoryConfigForCompose(cfg, root)
	if err != nil {
		return nil, err
	}
	clock := ServiceClockForCompose(cfg, load)
	collaborators, err := NewFactoryServiceCollaborators(cfg, clock, root.BaseLogger, NewFactorySessionsRegistry())
	if err != nil {
		return nil, err
	}
	return ComposeFactoryCore(
		ctx,
		cfg,
		root,
		collaborators,
		load,
		clock,
		NewHostedWorkersConfig(cfg, root.BaseLogger, clock),
	)
}

// ProvideFactorySaveCollaborator constructs the factorysave collaborator for a
// built FactoryService shell.
func ProvideFactorySaveCollaborator(
	shell FactoryServiceShell,
	cfg *FactoryServiceConfig,
) factorySaveSaver {
	return wireFactorySaveCollaborator(shell.Service, cfg)
}

// AttachFactorySaveCollaborator assigns the factorysave collaborator on the
// service shell and returns the assembled FactoryService.
func AttachFactorySaveCollaborator(
	shell FactoryServiceShell,
	factorySave factorySaveSaver,
) *FactoryService {
	if shell.Service != nil {
		shell.Service.factorySave = factorySave
	}
	return shell.Service
}

// ComposeFactoryService constructs *FactoryService using explicit S6 collaborators.
func ComposeFactoryService(
	ctx context.Context,
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
	collaborators FactoryServiceCollaborators,
	load FactoryConfigLoadResult,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (FactoryServiceShell, error) {
	core, err := ComposeFactoryCore(ctx, cfg, root, collaborators, load, clock, hostedWorkers)
	if err != nil {
		return FactoryServiceShell{}, err
	}
	return FactoryServiceShell{Service: NewFactoryServiceFromCore(core)}, nil
}

// ComposeFactoryCore constructs a FactoryCore using explicit composition
// collaborators without attaching runtime loops or transport facades.
func ComposeFactoryCore(
	ctx context.Context,
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
	collaborators FactoryServiceCollaborators,
	load FactoryConfigLoadResult,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*FactoryCore, error) {
	if err := validateFactoryCoreComposition(cfg, collaborators); err != nil {
		return nil, err
	}
	if core, portable := composePortableReplayCore(cfg, root, collaborators, load, clock, hostedWorkers); portable {
		return core, nil
	}
	if err := ensureServiceBackendScope(cfg, root.BaseLogger); err != nil {
		return nil, err
	}
	coreBuilt := false
	var runtimeBundle *factoryRuntimeBundle
	defer func() {
		if !coreBuilt && runtimeBundle != nil {
			_ = closeRuntimeBundleSinks(runtimeBundle.LogSink, runtimeBundle.MetricsSink)
		}
	}()
	if cfg.ReplayPath == "" {
		resolvedDir, err := factoryconfig.ResolveCurrentFactoryDir(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		resolvedDir, err = factorysessions.AbsolutizeFactoryDirectory(resolvedDir)
		if err != nil {
			return nil, fmt.Errorf("resolve factory dir: %w", err)
		}
		cfg.Dir = resolvedDir
	}
	durableExecution, err := composeDurableExecution(cfg, root, clock)
	if err != nil {
		return nil, err
	}
	collaborators.RuntimeBuild, err = composePetriRecordingRuntimeBuild(collaborators.RuntimeBuild, durableExecution)
	if err != nil {
		return nil, err
	}
	replaySideEffects, replayFactoryOpts, err := replayFactoryModeOptions(load.ReplayArtifact)
	if err != nil {
		return nil, err
	}
	defaultSessionSpec, err := collaborators.RuntimeBuild.BuildSpec(ctx, runtimebuild.SessionSpecInput{
		Dir:                                    cfg.Dir,
		FolderPath:                             root.FactoryRootDir,
		SessionID:                              defaultFactorySessionID,
		ExecutionBaseDir:                       cfg.ExecutionBaseDir,
		LoadedFactoryCfg:                       load.LoadedFactoryCfg,
		RuntimeInstanceID:                      cfg.RuntimeInstanceID,
		SideEffects:                            replaySideEffects,
		AdditionalFactoryOpts:                  replayFactoryOpts,
		PreserveCompatibilityDefaultRecordPath: true,
	})
	if err != nil {
		return nil, err
	}
	runtimeBundleAny, err := collaborators.RuntimeBuild.Build(ctx, defaultSessionSpec)
	if err != nil {
		return nil, err
	}
	runtimeBundle = asRuntimeBundle(runtimeBundleAny)
	if runtimeBundle == nil {
		return nil, fmt.Errorf("default runtime bundle is required")
	}
	defaultSession := factorysessions.NewLiveSession(
		defaultFactorySessionID,
		runtimeBundle.Dir,
		runtimeBundle.FolderPath,
		runtimeBundle.RuntimeCfg.RuntimeBaseDir(),
		FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
		&liveSessionState{bundle: runtimeBundle, spec: &defaultSessionSpec},
		true,
		filepath.Base(runtimeBundle.FolderPath),
	)
	factorysessions.BindResponseEventCompletion(defaultSession, runtimeBundle.EventHistory.AddGeneratedRecorder)
	collaborators.Sessions.Upsert(defaultSession, true)

	coreBuilt = true
	return &FactoryCore{
		cfg:              cfg,
		root:             root,
		collaborators:    collaborators,
		hostedWorkers:    hostedWorkers,
		clock:            clock,
		startupBundle:    runtimeBundle,
		logger:           runtimeBundle.Logger,
		modelAssets:      wireModelAssetPuller(cfg, collaborators.LocalModels.Assets),
		durableExecution: durableExecution,
	}, nil
}

func validateFactoryCoreComposition(cfg *FactoryServiceConfig, collaborators FactoryServiceCollaborators) error {
	if collaborators.WorkersScheduler == nil {
		return fmt.Errorf("compose factory core: worker sidecar owner is required")
	}
	return validateReplayModeConfig(cfg)
}

// NewFactoryServiceFromCore wraps a built core in the phase-one compatibility
// facade returned to CLI and HTTP entrypoints.
func NewFactoryServiceFromCore(core *FactoryCore) *FactoryService {
	if core == nil {
		return nil
	}
	svc := &FactoryService{
		core:             core,
		factoryRootDir:   core.FactoryRootDir(),
		sessions:         core.Sessions(),
		hostedWorkers:    core.HostedWorkers(),
		policy:           serviceCoordinatorPolicyFromConfig(core.cfg),
		startupBundle:    core.StartupBundle(),
		cfg:              core.cfg,
		modelAssets:      core.ModelAssetPuller(),
		baseLogger:       core.BaseLogger(),
		logger:           core.Logger(),
		clock:            core.Clock(),
		runtimeBuild:     core.RuntimeBuild(),
		workersScheduler: core.WorkersScheduler(),
		durableExecution: core.DurableExecution(),
	}
	svc.coordinator = newFactoryCoordinator(svc)
	svc.definitions = newFactoryDefinitionService(svc)
	return svc
}
