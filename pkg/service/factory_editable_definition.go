// backendsizecheck:ignore-file core-backed definition and model composition helpers remain with editable definition until dedicated core-from seams split.
// pkgmaintcheck:ignore-file-lines core-backed definition and model composition helpers remain with editable definition until dedicated core-from seams split.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
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

func (fs *FactoryService) serializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	svc := fs.definitionService()
	if svc == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return svc.SerializeNamedFactoryUpsertResponse(name, current)
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

func (h factorySaveHost) RequireFreshEditableFactoryVersionAtRoot(
	rootDir string,
	name factoryapi.FactoryName,
	baseVersion *factoryapi.HybridLogicalTimestamp,
) error {
	currentVersion, err := h.CurrentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return fmt.Errorf("factory definition service is required")
	}
	return svc.RequireFreshEditableFactoryVersion(baseVersion, currentVersion)
}

func (h factorySaveHost) NextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return factoryapi.HybridLogicalTimestamp{}
	}
	return svc.NextEditableFactoryVersion(current, now)
}

func (h factorySaveHost) PreparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return svc.PreparePersistedFactoryPayload(segment, factory, version)
}

func (h factorySaveHost) SaveReplaceCurrentForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return svc.SaveReplaceCurrentForSession(ctx, sessionID, request)
}

func (h factorySaveHost) SaveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	svc := h.FactoryService.definitionService()
	if svc == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return svc.SaveUpsertNamedAndActivateForSession(ctx, sessionID, request)
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
type LocalModelDomain = factoryservice.LocalModelDomain

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *FactoryServiceConfig) LocalModelDomain {
	return factoryservice.NewLocalModelDomain(hostConfigFromService(cfg))
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
	startupLocalModels := NewLocalModelDomain(cfg)
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

// ServiceConfig returns the normalized service config used to compose the core.
func (core *FactoryCore) ServiceConfig() *FactoryServiceConfig {
	if core == nil {
		return nil
	}
	return core.cfg
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
		LocalModelsInitialized:   core.LocalModels().Manager != nil,
		ModelAssetsInitialized:   core.ModelAssetPuller() != nil,
		DefinitionsInitialized:   true,
		HostedWorkersLoggerReady: core.HostedWorkers().Logger != nil,
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
		runtimeBundle.Dir,
		runtimeBundle.FolderPath,
		runtimeBundle.RuntimeCfg.RuntimeBaseDir(),
		FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
		&liveSessionState{bundle: runtimeBundle, spec: &defaultSessionSpec},
		true,
		filepath.Base(runtimeBundle.FolderPath),
	), true)

	coreBuilt = true
	return &FactoryCore{
		cfg:           cfg,
		root:          root,
		collaborators: collaborators,
		hostedWorkers: hostedWorkers,
		clock:         clock,
		startupBundle: runtimeBundle,
		logger:        runtimeBundle.Logger,
		modelAssets:   wireModelAssetPuller(cfg, collaborators.LocalModels.Assets),
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

// ModelService is the transport-facing model catalog seam after pkg/models/service extraction.
type ModelService = apisurface.ModelAPI

// NewModelServiceFromCore constructs a ModelService from a composed FactoryCore
// without building the root FactoryService compatibility facade.
func NewModelServiceFromCore(core *FactoryCore) ModelService {
	if core == nil {
		return modelsservice.New(modelsservice.Dependencies{})
	}
	cfg := core.ServiceConfig()
	policy := serviceCoordinatorPolicyFromConfig(cfg)
	return modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig {
			return coreStartupRuntimeConfig(core)
		},
		ModelHost: func() modelhost.Host {
			return core.ModelHost()
		},
		ModelAssetPuller: func() localmodels.AssetPuller {
			return core.ModelAssetPuller()
		},
		Logger: func() *zap.Logger {
			return core.Logger()
		},
		ModelPullMetrics: func() modelsservice.PullMetricsRecorder {
			if cfg == nil || cfg.ModelPullMetricsRecorder == nil {
				return nil
			}
			return modelPullMetricsHostAdapter{inner: cfg.ModelPullMetricsRecorder}
		},
		ModelInvocationExecutor: func(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return modelInvocationExecutorFromCore(core, policy, runtimeCfg, factoryCfg, workerName)
		},
		FactoryRunnerID: func() string {
			runtimeCfg := coreStartupRuntimeConfig(core)
			factoryCfg := (*interfaces.FactoryConfig)(nil)
			if runtimeCfg != nil {
				factoryCfg = runtimeCfg.FactoryConfig()
			}
			return effectiveFactoryRunnerID(policy.runnerID, factoryCfg)
		},
	})
}

// NewFactoryDefinitionServiceFromCore constructs a FactoryDefinitionService from
// a composed FactoryCore without building the root FactoryService facade.
func NewFactoryDefinitionServiceFromCore(core *FactoryCore) FactoryDefinitionService {
	return &coreFactoryDefinitionService{core: core}
}

type coreFactoryDefinitionService struct {
	core *FactoryCore
}

var _ FactoryDefinitionService = (*coreFactoryDefinitionService)(nil)

func (s *coreFactoryDefinitionService) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	core := s.core
	if core == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory core is required")
	}

	rootDir := core.FactoryRootDir()
	cfg := core.ServiceConfig()
	name, err := configpersist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := coreStartupRuntimeConfig(core)
			if currentRuntime != nil && factorysessions.SameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return serializeNamedFactoryFromCore(core, apisurface.DefaultCurrentFactoryName, currentRuntime, true)
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
	if cfg != nil {
		workstationLoader = cfg.WorkstationLoader
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current factory %q: %w", name, err)
	}

	return serializeNamedFactoryFromCore(core, factoryapi.FactoryName(name), current, true)
}

func (s *coreFactoryDefinitionService) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	core := s.core
	if core == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory core is required")
	}
	session := core.Sessions().Get(sessionID)
	if session == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory session %q not found", sessionID)
	}
	runtimeCfg, err := sessionRuntimeConfigFromCore(core, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	rootDir := factorysessions.SessionFactoryRootDir(core.FactoryRootDir(), session)
	factoryName := factorysessions.FactoryName(rootDir, runtimeCfg)
	versionRootDir := rootDir
	if persistRoot := factorysaveSessionFactoryPersistRoot(core.FactoryRootDir(), session); persistRoot != "" {
		if pointerName, err := configpersist.ReadCurrentFactoryPointer(persistRoot); err == nil {
			pointerFactoryName := factoryapi.FactoryName(pointerName)
			if session.IsDefault || pointerFactoryName == factoryName {
				factoryName = pointerFactoryName
			}
		}
		if factorysessions.SameFactoryDir(persistRoot, rootDir) {
			versionRootDir = persistRoot
		}
	}
	serialized, err := serializeNamedFactoryFromCore(core, factoryName, runtimeCfg, true)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return withCurrentFactoryVersionFromCore(core, versionRootDir, serialized.Name, serialized)
}

// StartupWorkerConfigFromCore returns the named worker from the composed startup runtime.
func StartupWorkerConfigFromCore(core *FactoryCore, name string) (*interfaces.WorkerConfig, bool) {
	runtimeCfg := coreStartupRuntimeConfig(core)
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

func coreStartupRuntimeConfig(core *FactoryCore) *factoryconfig.LoadedFactoryConfig {
	if core == nil {
		return nil
	}
	if bundle := core.StartupBundle(); bundle != nil {
		return bundle.RuntimeCfg
	}
	return nil
}

func sessionRuntimeConfigFromCore(core *FactoryCore, sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	if core == nil {
		return nil, fmt.Errorf("factory core is required")
	}
	session := core.Sessions().Get(sessionID)
	if session == nil {
		return nil, fmt.Errorf("factory session %q not found", sessionID)
	}
	bundle := liveSessionBundle(session)
	if bundle == nil || bundle.RuntimeCfg == nil {
		return nil, fmt.Errorf("factory session runtime is not available")
	}
	return bundle.RuntimeCfg, nil
}

func factorysaveSessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	return factorysave.SessionFactoryPersistRoot(serviceRootDir, session)
}

func serializeNamedFactoryFromCore(
	core *FactoryCore,
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
	workflowID := ""
	if core != nil && core.ServiceConfig() != nil {
		workflowID = strings.TrimSpace(core.ServiceConfig().WorkflowID)
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(workflowID),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize current factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

func withCurrentFactoryVersionFromCore(
	core *FactoryCore,
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := currentFactoryDefinitionVersionAtRootFromCore(core, rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
}

func currentFactoryDefinitionVersionAtRootFromCore(
	core *FactoryCore,
	rootDir string,
	name factoryapi.FactoryName,
) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if core != nil && core.ServiceConfig() != nil {
		workstationLoader = core.ServiceConfig().WorkstationLoader
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

func modelInvocationExecutorFromCore(
	core *FactoryCore,
	policy serviceCoordinatorPolicy,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(core.Logger(), policy.verbose)
	bundle := core.StartupBundle()
	var modelDomain localModelDomain
	var workflowContext *factory_context.FactoryContext
	if bundle != nil {
		modelDomain = LocalModelDomain{
			Resources:      bundle.ModelResources,
			Assets:         bundle.ModelAssets,
			Runtime:        bundle.LocalModelRuntime,
			Manager:        bundle.LocalModels,
			Host:           bundle.ModelHost,
			LeaseExecution: bundle.LeaseExecution,
		}
		if bundle.Factory != nil {
			workflowContext = runtime.WorkflowContext(bundle.Factory)
		}
	}
	executor := buildWorkerExecutor(
		runtimeCfg,
		factoryCfg,
		workerName,
		effectiveFactoryRunnerID(policy.runnerID, factoryCfg),
		workflowContext,
		logger,
		policy.providerOverride,
		nil,
		policy.providerCommandRunnerOverride,
		policy.commandRunnerOverride,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		modelDomain,
	)
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}
