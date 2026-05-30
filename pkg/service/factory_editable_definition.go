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
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"go.uber.org/zap"
)

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (fs *FactoryService) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.GetCurrentNamedFactory(ctx)
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
func (fs *FactoryService) GetCurrentNamedFactory(_ context.Context) (factoryapi.Factory, error) {
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
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true); err != nil {
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
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, false); err != nil {
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

func (h factorySaveHost) ReplaceDefaultFactoryDefinition(sessionRootDir string, payload []byte) (func(), error) {
	return h.FactoryService.replaceDefaultFactoryDefinition(sessionRootDir, payload)
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

func (fs *FactoryService) replaceDefaultFactoryDefinition(sessionRootDir string, payload []byte) (func(), error) {
	return configpersist.ReplaceDefaultFactoryDefinition(sessionRootDir, payload)
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
		Sessions:     sessions,
		LocalModels:  startupLocalModels,
		RuntimeBuild: newRuntimeBuildService(cfg, clock, baseLogger, &startupLocalModels),
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
	return newRuntimeBuildService(cfg, clock, baseLogger, localModels)
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
	ModelAssetsInitialized   bool
	FactorySaveInitialized   bool
	HostedWorkersLoggerReady bool
	BundleModelResources     bool
	BundleLocalModels        bool
}

// ComposeCollaboratorSnapshot reports initialized S6 collaborators for equivalence tests.
func (fs *FactoryService) ComposeCollaboratorSnapshot() ComposeCollaboratorSnapshot {
	if fs == nil {
		return ComposeCollaboratorSnapshot{}
	}
	bundle := fs.currentRuntimeBundle()
	snapshot := ComposeCollaboratorSnapshot{
		SessionsInitialized:      fs.sessions != nil,
		RuntimeBuildInitialized:  fs.runtimeBuild != nil,
		ModelAssetsInitialized:   fs.modelAssets != nil,
		FactorySaveInitialized:   fs.factorySave != nil,
		HostedWorkersLoggerReady: fs.hostedWorkers.Logger != nil,
	}
	if bundle != nil {
		snapshot.BundleModelResources = bundle.modelResources != nil
		snapshot.BundleLocalModels = bundle.localModels != nil
	}
	return snapshot
}

// ComposeFactoryService constructs *FactoryService using explicit S6 collaborators.
func ComposeFactoryService(
	ctx context.Context,
	cfg *FactoryServiceConfig,
	root FactoryServiceRoot,
	collaborators FactoryServiceCollaborators,
	load FactoryConfigLoadResult,
	clock factory.Clock,
) (*FactoryService, error) {
	if err := validateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	serviceBuilt := false
	var runtimeBundle *factoryRuntimeBundle
	defer func() {
		if !serviceBuilt && runtimeBundle != nil && runtimeBundle.logSink != nil {
			_ = runtimeBundle.logSink.Close()
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
	runtimeBundleAny, err := collaborators.RuntimeBuild.BuildFromLoadedConfig(ctx, runtimebuild.BuildInput{
		Dir:                   cfg.Dir,
		FolderPath:            root.FactoryRootDir,
		SessionID:             defaultFactorySessionID,
		LoadedFactoryCfg:      load.LoadedFactoryCfg,
		BaseLogger:            root.BaseLogger,
		RuntimeInstanceID:     cfg.RuntimeInstanceID,
		Clock:                 clock,
		RecordPath:            runtimebuild.SessionScopedRecordPath(cfg.RecordPath, defaultFactorySessionID),
		WorkflowID:            cfg.WorkflowID,
		ProviderOverride:      providerOverrideForMode(cfg, replaySideEffects),
		ProviderCommandRunner: providerCommandRunnerForMode(cfg, load.LoadedFactoryCfg),
		CommandRunnerOverride: commandRunnerOverrideForMode(cfg, load.LoadedFactoryCfg, replaySideEffects),
		AdditionalFactoryOpts: replayFactoryOpts,
	})
	if err != nil {
		return nil, err
	}
	runtimeBundle = asRuntimeBundle(runtimeBundleAny)

	serviceBuilt = true
	fs := &FactoryService{
		factoryRootDir: root.FactoryRootDir,
		sessions:       collaborators.Sessions,
		hostedWorkers:  buildHostedWorkersConfig(cfg, runtimeBundle.logger, clock),
		startupBundle:  runtimeBundle,
		cfg:            cfg,
		modelAssets:    wireModelAssetPuller(cfg, collaborators.LocalModels.assets),
		baseLogger:     root.BaseLogger,
		logger:         runtimeBundle.logger,
		clock:          clock,
		runtimeBuild:   collaborators.RuntimeBuild,
	}
	fs.factorySave = wireFactorySaveCollaborator(fs, cfg)
	return fs, nil
}
