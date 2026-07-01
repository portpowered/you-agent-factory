package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
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

type runtimeFactoryDefinitionService struct {
	service *FactoryService
}

var _ FactoryDefinitionService = (*runtimeFactoryDefinitionService)(nil)

func newFactoryDefinitionService(fs *FactoryService) FactoryDefinitionService {
	return &runtimeFactoryDefinitionService{service: fs}
}

func (fs *FactoryService) requireDefinitions() FactoryDefinitionService {
	if fs == nil {
		return newFactoryDefinitionService(nil)
	}
	if fs.definitions == nil {
		fs.definitions = newFactoryDefinitionService(fs)
	}
	return fs.definitions
}

func (s *runtimeFactoryDefinitionService) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	fs := s.service
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	name, err := configpersist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := fs.currentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return fs.serializeNamedFactory(apisurface.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return factoryapi.Factory{}, ErrCurrentFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current factory %q: %w", name, err)
	}

	return fs.serializeNamedFactory(factoryapi.FactoryName(name), current, true)
}

func (fs *FactoryService) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return factoryapi.HybridLogicalTimestamp{
			Logical:  apitypes.Int64String(version.Logical),
			Physical: version.Physical.UTC(),
		}, nil
	}

	info, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: modified,
	}, nil
}

func (fs *FactoryService) withCurrentFactoryVersion(
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

func (fs *FactoryService) serializeNamedFactory(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true, false); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize current factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

// serializeNamedFactoryUpsertResponse returns the PUT upsert read model with thin
// portable DOC/SCRIPT bundled files (disk-backed targets without inline content).
func (fs *FactoryService) serializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, false, false); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("merge named factory portable bundled files: %w", err)
		}
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize upsert factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

func sameFactoryDir(left, right string) bool {
	return factorysessions.SameFactoryDir(left, right)
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
	return factorysave.SessionFactoryPersistRoot(serviceRootDir, session)
}

type factorySaveHost struct {
	*FactoryService
}

var _ factorysave.Host = factorySaveHost{}

func (h factorySaveHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return h.FactoryService.requireSession(sessionID)
}

func (h factorySaveHost) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return h.FactoryService.GetCurrentFactoryForSession(ctx, sessionID)
}

func (h factorySaveHost) WithActivationLock(fn func() error) error {
	return h.FactoryService.withActivationLock(fn)
}

func (h factorySaveHost) RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error {
	return h.FactoryService.requireIdleRuntimeForSession(ctx, sessionID)
}

func (h factorySaveHost) ActivateSessionEditableFactory(
	ctx context.Context,
	session *factorysessions.LiveSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	return h.FactoryService.activateSessionEditableFactory(ctx, session, sessionID, sessionRootDir, factoryDir, name, runtimeName)
}

func (h factorySaveHost) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return h.FactoryService.replaceFactoryLayoutAtDir(targetDir, prepared)
}

func (h factorySaveHost) CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	return h.FactoryService.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

func (h factorySaveHost) SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	return h.FactoryService.sessionRuntimeConfig(sessionID)
}

func (h factorySaveHost) SerializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	return h.FactoryService.serializeNamedFactoryUpsertResponse(name, runtimeCfg)
}

func newFactorySaveService(fs *FactoryService) *factorysave.Service {
	return factorysave.New(
		fs.factoryRootDir,
		fs.clock,
		fs.workstationLoaderFromConfig,
		factorySaveHost{fs},
	)
}

func wireFactorySaveCollaborator(fs *FactoryService, cfg *FactoryServiceConfig) factorySaveSaver {
	if cfg != nil && cfg.FactorySave != nil {
		return cfg.FactorySave
	}
	return newFactorySaveService(fs)
}

func (fs *FactoryService) workstationLoaderFromConfig() factoryconfig.WorkstationLoader {
	if fs == nil || fs.cfg == nil {
		return nil
	}
	return fs.cfg.WorkstationLoader
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

// LocalModelDomain wires pkg/localmodels runtime dependencies constructed at
// service build time and copied onto each factoryRuntimeBundle.
type LocalModelDomain = localModelDomain

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *FactoryServiceConfig) LocalModelDomain {
	return newRuntimeLocalModelDependencies(cfg)
}

// FactoryServiceCollaborators groups explicit S6 composition collaborators.
type FactoryServiceCollaborators struct {
	Sessions     *factorysessions.Registry
	LocalModels  LocalModelDomain
	RuntimeBuild *runtimebuild.Service
}

// NewFactoryServiceCollaborators builds S6 collaborators using the provided
// session registry and freshly constructed local-model dependencies.
func NewFactoryServiceCollaborators(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	sessions *factorysessions.Registry,
) FactoryServiceCollaborators {
	startupLocalModels := newRuntimeLocalModelDependencies(cfg)
	return FactoryServiceCollaborators{
		Sessions:    sessions,
		LocalModels: startupLocalModels,
		RuntimeBuild: newRuntimeBuildService(
			cfg,
			clock,
			baseLogger,
			&startupLocalModels,
			newInferenceProgressPublisherFactory(sessions, baseLogger),
			newSessionDispatchCompletionObserverFactory(sessions),
		),
	}
}

// NewFactoryServiceCollaboratorsFromParts assembles collaborators from explicit
// wire-provided parts.
func NewFactoryServiceCollaboratorsFromParts(
	sessions *factorysessions.Registry,
	localModels LocalModelDomain,
	runtimeBuild *runtimebuild.Service,
) FactoryServiceCollaborators {
	return FactoryServiceCollaborators{
		Sessions:     sessions,
		LocalModels:  localModels,
		RuntimeBuild: runtimeBuild,
	}
}

// NewRuntimeBuildService constructs the runtimebuild collaborator for wire.
func NewRuntimeBuildService(
	cfg *FactoryServiceConfig,
	clock factory.Clock,
	baseLogger *zap.Logger,
	localModels *LocalModelDomain,
) *runtimebuild.Service {
	return newRuntimeBuildService(cfg, clock, baseLogger, localModels, nil, nil)
}

// FactoryConfigLoadResult carries factory config load outputs needed before
// runtime bundle construction.
type FactoryConfigLoadResult struct {
	LoadedFactoryCfg *factoryconfig.LoadedFactoryConfig
	ReplayArtifact   *interfaces.ReplayArtifact
	SessionLogger    *zap.Logger
}

// LoadFactoryConfigForCompose loads factory.json and replay metadata for wire
// composition after FactoryServiceRoot resolution.
func LoadFactoryConfigForCompose(
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
) (FactoryConfigLoadResult, error) {
	logger := runtimebuild.NewSessionLogger(
		root.BaseLogger,
		defaultFactorySessionID,
		root.FactoryRootDir,
		cfg.Dir,
	)
	loadedFactoryCfg, replayArtifact, err := loadFactoryConfigForService(cfg, logger)
	if err != nil {
		return FactoryConfigLoadResult{}, err
	}
	return FactoryConfigLoadResult{
		LoadedFactoryCfg: loadedFactoryCfg,
		ReplayArtifact:   replayArtifact,
		SessionLogger:    logger,
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
	SessionsInitialized      bool
	RuntimeBuildInitialized  bool
	LocalModelsInitialized   bool
	ModelAssetsInitialized   bool
	ModelServiceInitialized  bool
	FactorySaveInitialized   bool
	DefinitionsInitialized   bool
	HostedWorkersLoggerReady bool
	BundleModelResources     bool
	BundleLocalModels        bool
}

// FactoryCore owns the normalized runtime graph assembled before transport
// facades or runtime loops begin.
type FactoryCore struct {
	cfg           *FactoryServiceConfig
	root          FactoryServiceRoot
	collaborators FactoryServiceCollaborators
	hostedWorkers hostedworkers.Config
	clock         factory.Clock
	startupBundle *factoryRuntimeBundle
	logger        *zap.Logger
	modelAssets   modelAssetPuller
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
	return core.collaborators.LocalModels.host
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

// ComposeCollaboratorSnapshot reports initialized core collaborators for
// equivalence tests.
func (core *FactoryCore) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if core == nil {
		return ComposeCollaboratorSnapshot{}
	}
	bundle := core.StartupBundle()
	snapshot := ComposeCollaboratorSnapshot{
		SessionsInitialized:      core.Sessions() != nil,
		RuntimeBuildInitialized:  core.RuntimeBuild() != nil,
		LocalModelsInitialized:   core.LocalModels().manager != nil,
		ModelAssetsInitialized:   core.ModelAssetPuller() != nil,
		DefinitionsInitialized:   true,
		HostedWorkersLoggerReady: core.HostedWorkers().Logger != nil,
	}
	if bundle != nil {
		snapshot.BundleModelResources = bundle.modelResources != nil
		snapshot.BundleLocalModels = bundle.localModels != nil
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
	snapshot.ModelAssetsInitialized = fs.modelAssets != nil
	snapshot.HostedWorkersLoggerReady = fs.hostedWorkers.Logger != nil
	if bundle != nil {
		snapshot.BundleModelResources = bundle.modelResources != nil
		snapshot.BundleLocalModels = bundle.localModels != nil
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
	collaborators := NewFactoryServiceCollaborators(cfg, clock, root.BaseLogger, NewFactorySessionsRegistry())
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
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	coreBuilt := false
	var runtimeBundle *factoryRuntimeBundle
	defer func() {
		if !coreBuilt && runtimeBundle != nil {
			_ = closeRuntimeBundleSinks(runtimeBundle.logSink, runtimeBundle.metricsSink)
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

	replaySideEffects, replayFactoryOpts, err := replayFactoryModeOptions(load.ReplayArtifact)
	if err != nil {
		return nil, err
	}
	defaultSessionSpec, err := collaborators.RuntimeBuild.BuildSpec(ctx, runtimebuild.SessionSpecInput{
		Dir:                   cfg.Dir,
		FolderPath:            root.FactoryRootDir,
		SessionID:             defaultFactorySessionID,
		ExecutionBaseDir:      cfg.ExecutionBaseDir,
		LoadedFactoryCfg:      load.LoadedFactoryCfg,
		RuntimeInstanceID:     cfg.RuntimeInstanceID,
		SideEffects:           replaySideEffects,
		AdditionalFactoryOpts: replayFactoryOpts,
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
	collaborators.Sessions.Upsert(factorysessions.NewLiveSession(
		defaultFactorySessionID,
		runtimeBundle.dir,
		runtimeBundle.folderPath,
		runtimeBundle.runtimeCfg.RuntimeBaseDir(),
		FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
		&liveSessionState{bundle: runtimeBundle, spec: &defaultSessionSpec},
		true,
		filepath.Base(runtimeBundle.folderPath),
	), true)

	coreBuilt = true
	return &FactoryCore{
		cfg:           cfg,
		root:          root,
		collaborators: collaborators,
		hostedWorkers: hostedWorkers,
		clock:         clock,
		startupBundle: runtimeBundle,
		logger:        runtimeBundle.logger,
		modelAssets:   wireModelAssetPuller(cfg, collaborators.LocalModels.assets),
	}, nil
}

// NewFactoryServiceFromCore wraps a built core in the phase-one compatibility
// facade returned to CLI and HTTP entrypoints.
func NewFactoryServiceFromCore(core *FactoryCore) *FactoryService {
	if core == nil {
		return nil
	}
	svc := &FactoryService{
		core:           core,
		factoryRootDir: core.FactoryRootDir(),
		sessions:       core.Sessions(),
		hostedWorkers:  core.HostedWorkers(),
		policy:         serviceCoordinatorPolicyFromConfig(core.cfg),
		startupBundle:  core.StartupBundle(),
		cfg:            core.cfg,
		modelAssets:    core.ModelAssetPuller(),
		baseLogger:     core.BaseLogger(),
		logger:         core.Logger(),
		clock:          core.Clock(),
		runtimeBuild:   core.RuntimeBuild(),
	}
	svc.coordinator = newFactoryCoordinator(svc)
	svc.definitions = newFactoryDefinitionService(svc)
	return svc
}
