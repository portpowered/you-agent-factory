package service

import (
	"fmt"
	"os"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

// dirExists returns true if the path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func effectiveFactoryRunnerID(override string, factoryCfg *interfaces.FactoryConfig) string {
	if runner := interfaces.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if factoryCfg == nil {
		return ""
	}
	return interfaces.NormalizeRunnerID(factoryCfg.Runner)
}

// loadWorkersFromConfig instantiates worker executors from the loaded runtime config.
// Workers missing AGENTS.md keep the existing noop behavior so topology-only tests continue to work.
func loadWorkersFromConfig(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	providerOverride workerprovider.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelResources *localModelResourceLimiter,
	localModels *managedLocalModelManager,
) ([]factory.FactoryOption, error) {
	var opts []factory.FactoryOption
	logger.Info("loading workers from runtime config", "working-directory", factoryDir)
	if factoryCfg == nil {
		return nil, fmt.Errorf("factory config is required")
	}
	preflight := runnerSelectionPreflight{
		skipCommandAvailability: providerOverride != nil || providerCommandRunner != nil || skipBuiltInRunnerPrerequisiteValidation,
	}
	if err := validateConfiguredWorkstationRunners(factoryCfg, factoryRunnerID, runtimeCfg, preflight); err != nil {
		return nil, err
	}
	for _, workerCfg := range factoryCfg.Workers {
		logger.Debug("loading worker", "worker", workerCfg.Name)
		def, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || def == nil || def.Type == "" {
			logger.Debug("no AGENTS.md for worker; using noop executor", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, &workers.NoopExecutor{}))
			continue
		}
		executor := buildWorkerExecutor(runtimeCfg, factoryCfg, workerCfg.Name, factoryRunnerID, logger, providerOverride, providerCommandRunner, cmdRunner, scriptRecorder, inferenceRecorder, modelRecorder, now, modelResources, localModels)
		if executor != nil {
			logger.Info("loaded worker", "worker", workerCfg.Name)
			opts = append(opts, factory.WithWorkerExecutor(workerCfg.Name, executor))
		} else {
			logger.Error("failed to load worker", "worker", workerCfg.Name)
			return nil, fmt.Errorf("unsupported worker type for worker %q: %s", workerCfg.Name, def.Type)
		}
	}
	for _, workstationCfg := range factoryCfg.Workstations {
		def, ok := runtimeCfg.Workstation(workstationCfg.Name)
		if !ok || def == nil {
			continue
		}
		if def.Type != interfaces.WorkstationTypeLogical || def.WorkerTypeName != "" {
			continue
		}
		logger.Info("loading workerless logical workstation", "workstation", workstationCfg.Name)
		opts = append(opts, factory.WithWorkerExecutor(workstationCfg.Name, &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}))
	}
	return opts, nil
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
	factoryRunnerID string,
	logger logging.Logger,
	providerOverride workerprovider.Provider,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	now func() time.Time,
	modelResources *localModelResourceLimiter,
	localModels *managedLocalModelManager,
) workers.WorkerExecutor {
	def, ok := runtimeCfg.Worker(workerName)
	if !ok {
		return nil
	}

	switch def.Type {
	case interfaces.WorkerTypeModel:
		var runner workers.Runner
		if providerOverride != nil {
			runner = workers.RunnerFromProvider(providerOverride)
		} else {
			var providerOpts []workerprovider.ScriptWrapProviderOption
			providerOpts = append(providerOpts, workerprovider.WithSkipPermissions(def.SkipPermissions))
			providerOpts = append(providerOpts, workerprovider.WithProviderLogger(logger))
			if providerCommandRunner != nil {
				providerOpts = append(providerOpts, workerprovider.WithProviderCommandRunner(providerCommandRunner))
			}
			runner = workerprovider.NewScriptWrapProvider(providerOpts...)
		}
		if inferenceRecorder != nil {
			if providerOverride != nil {
				provider := workerprovider.NewRecordingProvider(
					providerOverride,
					inferenceRecorder,
					workerprovider.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			} else if providerRunner, ok := runner.(*workerprovider.ScriptWrapProvider); ok {
				provider := workerprovider.NewRecordingProvider(
					providerRunner,
					inferenceRecorder,
					workerprovider.WithRecordingProviderClock(now),
				)
				runner = workers.RunnerFromProvider(provider)
			}
		}

		agentOpts := []workers.AgentExecutorOption{
			workers.WithLogger(logger),
		}
		runner = localModels.wrapRunner(runner, runtimeCfg, factoryCfg, def)
		runner = modelResources.wrapRunner(runner, factoryCfg, def)
		runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
		agentExec := workers.NewAgentExecutorWithRunner(runtimeCfg, runner, agentOpts...)
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        agentExec,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkstationTypeLogical:
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	case interfaces.WorkerTypeScript:
		var scriptOpts []workers.ScriptExecutorOption
		if runtimeCfg != nil && runtimeCfg.FactoryDir() != "" {
			scriptOpts = append(scriptOpts, workers.WithScriptFactoryDir(runtimeCfg.FactoryDir()))
		}
		if scriptRecorder != nil {
			scriptOpts = append(scriptOpts, workers.WithScriptEventRecorder(scriptRecorder))
		}
		var scriptExec workers.WorkstationRequestExecutor
		if cmdRunner != nil {
			scriptExec = workers.NewScriptExecutorWithRunner(def, cmdRunner, logger, scriptOpts...)
		} else {
			scriptExec = workers.NewScriptExecutor(def, logger, scriptOpts...)
		}
		return &workers.WorkstationExecutor{
			RuntimeConfig:   runtimeCfg,
			DefaultRunnerID: factoryRunnerID,
			Executor:        scriptExec,
			Renderer:        &workerprompting.DefaultPromptRenderer{},
			Logger:          logger,
		}
	default:
		return nil
	}
}
