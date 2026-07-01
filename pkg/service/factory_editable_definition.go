// backendsizecheck:ignore-file compose helpers remain with service until dedicated compose package splits.
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
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	"go.uber.org/zap"
)

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
func ResolveFactoryServiceRoot(cfg *runtimehost.Config) (FactoryServiceRoot, error) {
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

// NewLocalModelDomain constructs the local-model collaborator group for a build.
func NewLocalModelDomain(cfg *runtimehost.Config) LocalModelDomain {
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
	cfg *runtimehost.Config,
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
			runtimehost.NewInferenceProgressPublisherFactory(sessions, baseLogger),
			runtimehost.NewSessionDispatchCompletionObserverFactory(sessions),
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
	cfg *runtimehost.Config,
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
	cfg *runtimehost.Config,
	root FactoryServiceRoot,
) (FactoryConfigLoadResult, error) {
	logger := runtimebuild.NewSessionLogger(
		root.BaseLogger,
		runtimehost.DefaultFactorySessionID,
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
func ServiceClockForCompose(cfg *runtimehost.Config, load FactoryConfigLoadResult) factory.Clock {
	return serviceClockForMode(cfg.Clock, load.ReplayArtifact)
}

// NewHostedWorkersConfig builds the hosted-workers collaborator from service config.
func NewHostedWorkersConfig(
	cfg *runtimehost.Config,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	return buildHostedWorkersConfig(cfg, logger, clock)
}

// BuildFactoryCore constructs the normalized runtime graph without attaching a transport host.
func BuildFactoryCore(ctx context.Context, cfg *runtimehost.Config) (*runtimehost.Core, error) {
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
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

// ComposeFactoryService constructs a compatibility host using explicit collaborators.
func ComposeFactoryService(
	ctx context.Context,
	cfg *runtimehost.Config,
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
	return FactoryServiceShell{Host: NewFactoryServiceFromCore(core)}, nil
}

// ComposeFactoryCore constructs a runtimehost.Core using explicit composition collaborators.
func ComposeFactoryCore(
	ctx context.Context,
	cfg *runtimehost.Config,
	root FactoryServiceRoot,
	collaborators FactoryServiceCollaborators,
	load FactoryConfigLoadResult,
	clock factory.Clock,
	hostedWorkers hostedworkers.Config,
) (*runtimehost.Core, error) {
	if err := runtimehost.ValidateReplayModeConfig(cfg); err != nil {
		return nil, err
	}
	coreBuilt := false
	var runtimeBundle *factoryservice.Bundle
	defer func() {
		if !coreBuilt && runtimeBundle != nil {
			_ = CloseRuntimeBundleSinksForCompose(runtimeBundle.LogSink, runtimeBundle.MetricsSink)
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

	replaySideEffects, replayFactoryOpts, err := ReplayFactoryModeOptionsForCompose(load.ReplayArtifact)
	if err != nil {
		return nil, err
	}
	defaultSessionSpec, err := collaborators.RuntimeBuild.BuildSpec(ctx, runtimebuild.SessionSpecInput{
		Dir:                   cfg.Dir,
		FolderPath:            root.FactoryRootDir,
		SessionID:             runtimehost.DefaultFactorySessionID,
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
	runtimeBundle = AsRuntimeBundleForCompose(runtimeBundleAny)
	if runtimeBundle == nil {
		return nil, fmt.Errorf("default runtime bundle is required")
	}
	collaborators.Sessions.Upsert(factorysessions.NewLiveSession(
		runtimehost.DefaultFactorySessionID,
		runtimeBundle.Dir,
		runtimeBundle.FolderPath,
		runtimeBundle.RuntimeCfg.RuntimeBaseDir(),
		runtimehost.FactorySessionTargetRef{Kind: runtimehost.FactorySessionTargetKindDefault},
		runtimehost.NewStartupLiveSessionHandle(runtimeBundle, &defaultSessionSpec),
		true,
		filepath.Base(runtimeBundle.FolderPath),
	), true)

	coreBuilt = true
	return runtimehost.NewCore(
		cfg,
		root.FactoryRootDir,
		root.BaseLogger,
		collaborators.Sessions,
		collaborators.RuntimeBuild,
		collaborators.LocalModels,
		hostedWorkers,
		clock,
		runtimeBundle,
		runtimeBundle.Logger,
		WireModelAssetPullerForCompose(cfg, collaborators.LocalModels.Assets),
	), nil
}

// ModelService is the transport-facing model catalog seam after pkg/models/service extraction.
type ModelService = apisurface.ModelAPI

// NewModelServiceFromCore constructs a ModelService from a composed FactoryCore
// without building the root FactoryService compatibility facade.
func NewModelServiceFromCore(core *runtimehost.Core) ModelService {
	if core == nil {
		return modelsservice.New(modelsservice.Dependencies{})
	}
	cfg := core.ServiceConfig()
	policy := runtimehost.CoordinatorPolicyFromConfig(cfg)
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
			return runtimehost.NewModelPullMetricsHostAdapter(cfg.ModelPullMetricsRecorder)
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
			return effectiveFactoryRunnerID(runtimehost.CoordinatorPolicyRunnerID(policy), factoryCfg)
		},
	})
}

// NewFactoryDefinitionServiceFromCore constructs a FactoryDefinitionService from
// a composed FactoryCore without building the root FactoryService facade.
func NewFactoryDefinitionServiceFromCore(core *runtimehost.Core) FactoryDefinitionService {
	return &coreFactoryDefinitionService{core: core}
}

type coreFactoryDefinitionService struct {
	core *runtimehost.Core
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
func StartupWorkerConfigFromCore(core *runtimehost.Core, name string) (*interfaces.WorkerConfig, bool) {
	runtimeCfg := coreStartupRuntimeConfig(core)
	if runtimeCfg == nil {
		return nil, false
	}
	return runtimeCfg.Worker(name)
}

func coreStartupRuntimeConfig(core *runtimehost.Core) *factoryconfig.LoadedFactoryConfig {
	if core == nil {
		return nil
	}
	if bundle := core.StartupBundle(); bundle != nil {
		return bundle.RuntimeCfg
	}
	return nil
}

func sessionRuntimeConfigFromCore(core *runtimehost.Core, sessionID string) (*factoryconfig.LoadedFactoryConfig, error) {
	if core == nil {
		return nil, fmt.Errorf("factory core is required")
	}
	session := core.Sessions().Get(sessionID)
	if session == nil {
		return nil, fmt.Errorf("factory session %q not found", sessionID)
	}
	bundle := runtimehost.LiveSessionBundle(session)
	if bundle == nil || bundle.RuntimeCfg == nil {
		return nil, fmt.Errorf("factory session runtime is not available")
	}
	return bundle.RuntimeCfg, nil
}

func factorysaveSessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	return factorysave.SessionFactoryPersistRoot(serviceRootDir, session)
}

func serializeNamedFactoryFromCore(
	core *runtimehost.Core,
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
	core *runtimehost.Core,
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
	core *runtimehost.Core,
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
	core *runtimehost.Core,
	policy runtimehost.CoordinatorPolicy,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	if runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	logger := logging.NewZapLogger(core.Logger(), runtimehost.CoordinatorPolicyVerbose(policy))
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
		effectiveFactoryRunnerID(runtimehost.CoordinatorPolicyRunnerID(policy), factoryCfg),
		workflowContext,
		logger,
		runtimehost.CoordinatorPolicyProviderOverride(policy),
		nil,
		runtimehost.CoordinatorPolicyProviderCommandRunnerOverride(policy),
		runtimehost.CoordinatorPolicyCommandRunnerOverride(policy),
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
