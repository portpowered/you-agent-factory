package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// NewModelServiceFromCore constructs a ModelService from a composed FactoryCore
// without building the root FactoryService compatibility facade.
func NewModelServiceFromCore(core *FactoryCore) ModelService {
	if core == nil {
		return newModelService(modelServiceDependencies{})
	}
	cfg := core.ServiceConfig()
	policy := serviceCoordinatorPolicyFromConfig(cfg)
	return newModelService(modelServiceDependencies{
		runtimeConfig: func() *factoryconfig.LoadedFactoryConfig {
			return coreStartupRuntimeConfig(core)
		},
		modelAssetPuller: func() modelAssetPuller {
			return core.ModelAssetPuller()
		},
		modelHost: func() modelhost.Host {
			return core.ModelHost()
		},
		modelInvocationExecutor: func(runtimeCfg *factoryconfig.LoadedFactoryConfig, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
			return modelInvocationExecutorFromCore(core, policy, runtimeCfg, factoryCfg, workerName)
		},
		factoryRunnerID: func() string {
			runtimeCfg := coreStartupRuntimeConfig(core)
			factoryCfg := (*interfaces.FactoryConfig)(nil)
			if runtimeCfg != nil {
				factoryCfg = runtimeCfg.FactoryConfig()
			}
			return effectiveFactoryRunnerID(policy.runnerID, factoryCfg)
		},
		logger: core.Logger(),
		modelPullMetrics: func() ModelPullMetricsRecorder {
			if cfg == nil {
				return nil
			}
			return cfg.ModelPullMetricsRecorder
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
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
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
		if sameFactoryDir(persistRoot, rootDir) {
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
		return bundle.runtimeCfg
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
	if bundle == nil || bundle.runtimeCfg == nil {
		return nil, fmt.Errorf("factory session runtime is not available")
	}
	return bundle.runtimeCfg, nil
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
		modelDomain = localModelDomain{
			resources:      bundle.modelResources,
			assets:         bundle.modelAssets,
			runtime:        bundle.localModelRuntime,
			host:           bundle.modelHost,
			manager:        bundle.localModels,
			leaseExecution: bundle.leaseExecution,
		}
		if bundle.factory != nil {
			workflowContext = runtime.WorkflowContext(bundle.factory)
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
		time.Now,
		modelDomain,
	)
	workstationExecutor, ok := executor.(*workerexecutor.WorkstationExecutor)
	if !ok || workstationExecutor.Executor == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return workstationExecutor.Executor, nil
}
