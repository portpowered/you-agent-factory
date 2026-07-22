package service

import (
	"fmt"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/execution/recording"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/runner"
	"github.com/portpowered/infinite-you/pkg/services/workers/skippermissions"
)

// BuildRuntimeExecutors constructs every configured runtime worker through the
// same canonical executor builder used for direct model invocation.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (s *Service) BuildRuntimeExecutors(
	runtimeConfig interfaces.RuntimeConfigLookup,
	factoryConfig *interfaces.FactoryConfig,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workers.ProgressPublisher,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	modelRecorder workers.ModelEventRecorder,
	agentRunRecorder workers.AgentRunEventRecorder,
	clock func() time.Time,
) (map[string]workers.WorkerExecutor, error) {
	if s == nil {
		return nil, fmt.Errorf("Worker execution service is required")
	}
	if factoryConfig == nil {
		return nil, fmt.Errorf("factory config is required")
	}
	if runtimeConfig == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	if s.executorBuilder == nil {
		return nil, fmt.Errorf("Worker application components are required")
	}
	if logger == nil {
		logger = logging.NoopLogger{}
	}
	now := clock
	if now == nil {
		now = s.clock
	}
	if providerOverride == nil {
		providerOverride = s.providerOverride
	}
	factoryRunnerID = EffectiveFactoryRunnerID(factoryRunnerID, factoryConfig)
	preflight := runnerSelectionPreflight{skipCommandAvailability: providerOverride != nil || s.providerCommandInjected || skipBuiltInRunnerPrerequisiteValidation}
	if err := ValidateRuntimeSelections(factoryConfig, factoryRunnerID, runtimeConfig, s.executableLocator, preflight.skipCommandAvailability, invocationSkipPermissionsOverride); err != nil {
		return nil, err
	}

	decorators := s.runtimeRunnerDecorators(runtimeConfig, factoryConfig, modelRecorder, now)
	executors := make(map[string]workers.WorkerExecutor, len(factoryConfig.Workers)+len(factoryConfig.Workstations))
	for _, configured := range factoryConfig.Workers {
		definition, ok := runtimeConfig.Worker(configured.Name)
		if !ok || definition == nil || definition.Type == "" {
			executors[configured.Name] = &workerexecutor.NoopExecutor{}
			continue
		}
		result, err := s.executorBuilder.Build(
			runtimeConfig, configured.Name, factoryRunnerID, workflowContext, logger,
			invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher,
			scriptRecorder, inferenceRecorder, agentRunRecorder, now, s.processEnvironment, s.currentWorkingDirectory, decorators,
		)
		if err != nil {
			return nil, fmt.Errorf("construct worker %q: %w", configured.Name, err)
		}
		if result.Dispatch == nil {
			return nil, fmt.Errorf("unsupported worker type for worker %q: %s", configured.Name, definition.Type)
		}
		executors[configured.Name] = result.Dispatch
	}
	for _, workstation := range factoryConfig.Workstations {
		definition, ok := runtimeConfig.Workstation(workstation.Name)
		if !ok || definition == nil || definition.Type != interfaces.WorkstationTypeLogical || definition.WorkerTypeName != "" {
			continue
		}
		result := s.executorBuilder.BuildLogical(
			runtimeConfig, workstation.Name, factoryRunnerID, workflowContext, logger, now, s.processEnvironment, s.currentWorkingDirectory,
		)
		executors[workstation.Name] = result.Dispatch
	}
	return executors, nil
}

func (s *Service) runtimeRunnerDecorators(runtimeCfg interfaces.RuntimeConfigLookup, factoryCfg *interfaces.FactoryConfig, recorder workers.ModelEventRecorder, now func() time.Time) []workerconstruction.RunnerDecorator {
	return []workerconstruction.RunnerDecorator{
		func(inner workers.Runner, definition *interfaces.FactoryWorkerConfig) workers.Runner {
			return wrapLocalModelRunner(
				inner, s.models, factoryCfg, definition,
			)
		},
		func(inner workers.Runner, definition *interfaces.FactoryWorkerConfig) workers.Runner {
			return modelrecording.NewRunner(inner, factoryCfg, definition, recorder, now)
		},
	}
}

// EffectiveFactoryRunnerID resolves an explicit runtime runner before the
// Factory definition default.
func EffectiveFactoryRunnerID(override string, cfg *interfaces.FactoryConfig) string {
	if runner := workerrunner.NormalizeRunnerID(override); runner != "" {
		return runner
	}
	if cfg == nil {
		return ""
	}
	return workerrunner.NormalizeRunnerID(cfg.Runner)
}

// ValidateRuntimeSelections checks runner and permissions policy before any
// executor is constructed.
func ValidateRuntimeSelections(
	cfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	executableLocator platformprocess.ExecutableLocator,
	skipCommandAvailability bool,
	invocationSkipPermissionsOverride *bool,
) error {
	if cfg == nil {
		return fmt.Errorf("factory config is required")
	}
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	return validateWorkerLoadPreflight(cfg, factoryRunnerID, runtimeCfg, executableLocator, runnerSelectionPreflight{skipCommandAvailability: skipCommandAvailability}, invocationSkipPermissionsOverride)
}

type runnerSelectionPreflight struct{ skipCommandAvailability bool }

func validateWorkerLoadPreflight(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight, invocationSkipPermissionsOverride *bool) error {
	if err := validateConfiguredWorkstationRunners(cfg, factoryRunnerID, runtimeCfg, executableLocator, preflight); err != nil {
		return err
	}
	return skippermissions.ValidateInvocationSkipPermissionsWorkers(cfg, runtimeCfg, invocationSkipPermissionsOverride)
}

func validateConfiguredWorkstationRunners(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight) error {
	for index, workstation := range cfg.Workstations {
		if configured, ok := runtimeCfg.Workstation(workstation.Name); ok && configured != nil {
			workstation = *configured
		}
		worker, _ := runtimeCfg.Worker(workstation.WorkerTypeName)
		modelProvider, openCodeAgent := "", ""
		if worker != nil {
			modelProvider, openCodeAgent = worker.ModelProvider, worker.OpenCodeAgent
		}
		selection := workerrunner.ResolveRunnerSelection(workstation.Runner, factoryRunnerID, modelProvider)
		if err := workerrunner.ValidateOpenCodeAgentForRunnerSelection(workstation.OpenCodeAgent, openCodeAgent, selection); err != nil {
			return fmt.Errorf("workstations[%d](%s).openCodeAgent: %w", index, workstation.Name, err)
		}
		if selection.Source == workerexecution.RunnerSelectionSourceDefault {
			continue
		}
		if _, ok := workerrunner.BuiltInRunnerMetadata(selection.RunnerID); !ok {
			return fmt.Errorf("workstations[%d](%s).runner: unknown runner %q", index, workstation.Name, selection.RunnerID)
		}
		if status, ok := workers.BuiltInRunnerStatus(selection.RunnerID); ok && !status.Available {
			return fmt.Errorf("workstations[%d](%s).runner: %s", index, workstation.Name, status.UnavailableReason)
		}
		if !preflight.skipCommandAvailability {
			if err := workers.ValidateBuiltInRunnerPrerequisites(executableLocator, selection.RunnerID); err != nil {
				return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, err)
			}
		}
	}
	return nil
}
