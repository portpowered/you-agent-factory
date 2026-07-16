package runtimehost

import (
	"time"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/workers/executor/agentrun"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	providerstructured "github.com/portpowered/infinite-you/pkg/workers/provider/structured"
	"github.com/portpowered/infinite-you/pkg/workers/skippermissions"
)

func wrapLocalModelRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *workerconfig.Config,
	modelDomain LocalModelDomain,
) workers.Runner {
	return factoryservice.WrapLocalModelRunner(inner, runtimeCfg, factoryCfg, workerDef, modelDomain)
}

func configuredWorkstationExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	inner workers.WorkstationRequestExecutor,
	logger logging.Logger,
) *workerexecutor.WorkstationExecutor {
	return &workerexecutor.WorkstationExecutor{
		RuntimeConfig:   runtimeCfg,
		DefaultRunnerID: factoryRunnerID,
		WorkflowContext: workflowContext,
		Executor:        inner,
		Renderer:        &workerprompting.DefaultPromptRenderer{},
		Logger:          logger,
	}
}

// buildWorkerExecutor creates a WorkstationExecutor wrapping the appropriate
// inner executor for the configured worker type. Returns nil for unsupported types.
func buildWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) workers.WorkerExecutor {
	def, ok := runtimeCfg.Worker(workerName)
	if !ok {
		return nil
	}

	switch def.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		return buildProviderBackedWorkerExecutor(
			runtimeCfg,
			factoryCfg,
			def,
			factoryRunnerID,
			workflowContext,
			logger,
			invocationSkipPermissionsOverride,
			providerOverride,
			inferenceProgressPublisher,
			providerCommandRunner,
			inferenceRecorder,
			modelRecorder,
			agentRunRecorder,
			now,
			modelDomain,
		)
	case interfaces.WorkstationTypeLogical:
		return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, nil, logger)
	case interfaces.WorkerTypeScript:
		return buildScriptWorkerExecutor(
			runtimeCfg,
			def,
			factoryRunnerID,
			workflowContext,
			logger,
			cmdRunner,
			scriptRecorder,
		)
	default:
		return nil
	}
}

func buildProviderBackedWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	def *workerconfig.Config,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
) workers.WorkerExecutor {
	runner := providerBackedRunner(
		def,
		logger,
		invocationSkipPermissionsOverride,
		providerOverride,
		inferenceProgressPublisher,
		providerCommandRunner,
		inferenceRecorder,
		now,
	)
	runner = wrapLocalModelRunner(runner, runtimeCfg, factoryCfg, def, modelDomain)
	runner = modelDomain.Resources.WrapRunner(runner, factoryCfg, def)
	runner = newRecordingModelRunner(runner, factoryCfg, def, modelRecorder, now)
	inferenceExecutor := workerexecutor.NewAgentExecutorWithRunner(
		runtimeCfg,
		runner,
		workerexecutor.WithLogger(logger),
	)
	agentRunExecutor := workeragentrun.NewAgentRunExecutor(
		runtimeCfg,
		runner,
		workeragentrun.WithAgentRunLogger(logger),
		workeragentrun.WithAgentRunEventRecorder(agentRunRecorder),
		workeragentrun.WithAgentRunClock(now),
	)
	inner := &workerexecutor.WorkstationBehaviorRouter{
		RuntimeConfig:     runtimeCfg,
		InferenceExecutor: inferenceExecutor,
		AgentRunExecutor:  agentRunExecutor,
	}
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, inner, logger)
}

func providerBackedRunner(
	def *workerconfig.Config,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	now func() time.Time,
) workers.Runner {
	runner := newProviderRunner(def, logger, invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher, providerCommandRunner)
	if inferenceRecorder == nil {
		return runner
	}
	return wrapRecordingProviderRunner(runner, providerOverride, inferenceRecorder, now)
}

func newProviderRunner(
	def *workerconfig.Config,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
) workers.Runner {
	if providerOverride != nil {
		return workers.RunnerFromProvider(providerOverride)
	}
	return workerprovider.NewScriptWrapProvider(providerRunnerOptions(
		def,
		logger,
		invocationSkipPermissionsOverride,
		inferenceProgressPublisher,
		providerCommandRunner,
	)...)
}

func providerRunnerOptions(
	def *workerconfig.Config,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
) []workerprovider.ScriptWrapProviderOption {
	opts := []workerprovider.ScriptWrapProviderOption{
		workerprovider.WithSkipPermissions(skippermissions.EffectiveSkipPermissions(
			def.SkipPermissions,
			def.Type,
			invocationSkipPermissionsOverride,
		)),
		workerprovider.WithProviderLogger(logger),
	}
	if inferenceProgressPublisher != nil {
		opts = append(opts,
			workerprovider.WithInferenceProgressPublisher(inferenceProgressPublisher),
			workerprovider.WithResponseStreamExecutor(providerstructured.NewExecutor()),
		)
	}
	if providerCommandRunner != nil {
		opts = append(opts, workerprovider.WithProviderCommandRunner(providerCommandRunner))
	}
	return opts
}

func wrapRecordingProviderRunner(
	runner workers.Runner,
	providerOverride workerprovider.Provider,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	now func() time.Time,
) workers.Runner {
	recordingClock := workerprovider.WithRecordingProviderClock(now)
	if providerOverride != nil {
		return workers.RunnerFromProvider(
			workerprovider.NewRecordingProvider(providerOverride, inferenceRecorder, recordingClock),
		)
	}
	providerRunner, ok := runner.(*workerprovider.ScriptWrapProvider)
	if !ok {
		return runner
	}
	return workers.RunnerFromProvider(
		workerprovider.NewRecordingProvider(providerRunner, inferenceRecorder, recordingClock),
	)
}

func buildScriptWorkerExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	def *workerconfig.Config,
	factoryRunnerID string,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	cmdRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
) workers.WorkerExecutor {
	scriptOpts := scriptExecutorOptions(runtimeCfg, scriptRecorder)
	var scriptExec workers.WorkstationRequestExecutor
	if cmdRunner != nil {
		scriptExec = workerexecutor.NewScriptExecutorWithRunner(def, cmdRunner, logger, scriptOpts...)
	} else {
		scriptExec = workerexecutor.NewScriptExecutor(def, logger, scriptOpts...)
	}
	return configuredWorkstationExecutor(runtimeCfg, factoryRunnerID, workflowContext, scriptExec, logger)
}

func scriptExecutorOptions(
	runtimeCfg interfaces.RuntimeConfigLookup,
	scriptRecorder workers.ScriptEventRecorder,
) []workerexecutor.ScriptExecutorOption {
	var scriptOpts []workerexecutor.ScriptExecutorOption
	if runtimeCfg != nil && runtimeCfg.FactoryDir() != "" {
		scriptOpts = append(scriptOpts, workerexecutor.WithScriptFactoryDir(runtimeCfg.FactoryDir()))
	}
	if scriptRecorder != nil {
		scriptOpts = append(scriptOpts, workerexecutor.WithScriptEventRecorder(scriptRecorder))
	}
	return scriptOpts
}
