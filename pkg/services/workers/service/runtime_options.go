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
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
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
	if err := s.rebindProvidersCommandRunner(logger); err != nil {
		return nil, err
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
	if err := ValidateRuntimeSelections(
		factoryConfig,
		factoryRunnerID,
		runtimeConfig,
		s.executableLocator,
		preflight.skipCommandAvailability,
		invocationSkipPermissionsOverride,
		s.providerRegistry,
	); err != nil {
		return nil, err
	}

	decorators := s.runtimeRunnerDecorators(
		runtimeConfig,
		factoryConfig,
		modelRecorder,
		now,
		providerOverride == nil,
		inferenceProgressPublisher,
	)
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

func (s *Service) rebindProvidersCommandRunner(logger logging.Logger) error {
	if s == nil || !s.providerCommandInjected || s.providerCommandRunner == nil || s.providerRegistryRebinder == nil {
		return nil
	}
	providersRunner := workerprocess.CommandRunnerWithLogging(
		s.providerCommandRunner,
		logging.EnsureLogger(logger),
		serviceCommandClock(s),
	)
	reboundRegistry, err := rebindProviderRegistry(s.providerRegistry, providersRunner, s.providerRegistryRebinder)
	if err != nil {
		return fmt.Errorf("rebind provider registry for session command logging: %w", err)
	}
	return applyReboundProviderRegistry(s, reboundRegistry)
}

func (s *Service) runtimeRunnerDecorators(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	recorder workers.ModelEventRecorder,
	now func() time.Time,
	useRegistryCapabilities bool,
	progressPublisher workers.ProgressPublisher,
) []workerconstruction.RunnerDecorator {
	decorators := make([]workerconstruction.RunnerDecorator, 0, 4)
	if useRegistryCapabilities && s.providerRegistry != nil {
		decorators = append(decorators, func(inner workers.Runner, _ *interfaces.FactoryWorkerConfig) workers.Runner {
			return registryCapabilityRunner{next: inner, providers: s.providerRegistry}
		})
		if s.invocationConductor != nil {
			decorators = append(decorators, func(inner workers.Runner, _ *interfaces.FactoryWorkerConfig) workers.Runner {
				return conductorInvocationRunner{
					next:      inner,
					conductor: s.invocationConductor,
					providers: s.providerRegistry,
					publish:   progressPublisher,
				}
			})
		}
	}
	return append(decorators,
		func(inner workers.Runner, definition *interfaces.FactoryWorkerConfig) workers.Runner {
			return resolveInferenceRunner(
				inner, s.models, s.modelsScope, factoryCfg, definition,
			)
		},
		func(inner workers.Runner, definition *interfaces.FactoryWorkerConfig) workers.Runner {
			return modelrecording.NewRunner(inner, factoryCfg, definition, recorder, now)
		},
	)
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
	providerRegistries ...*providerregistry.Registry,
) error {
	if cfg == nil {
		return fmt.Errorf("factory config is required")
	}
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	var providers *providerregistry.Registry
	if len(providerRegistries) > 0 {
		providers = providerRegistries[0]
	}
	return validateRuntimeSelectionsWithRegistry(
		cfg,
		factoryRunnerID,
		runtimeCfg,
		executableLocator,
		skipCommandAvailability,
		invocationSkipPermissionsOverride,
		providers,
	)
}

type runnerSelectionPreflight struct{ skipCommandAvailability bool }

func validateRuntimeSelectionsWithRegistry(
	cfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	executableLocator platformprocess.ExecutableLocator,
	skipCommandAvailability bool,
	invocationSkipPermissionsOverride *bool,
	providers *providerregistry.Registry,
) error {
	if cfg == nil {
		return fmt.Errorf("factory config is required")
	}
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	return validateWorkerLoadPreflight(
		cfg,
		factoryRunnerID,
		runtimeCfg,
		executableLocator,
		runnerSelectionPreflight{skipCommandAvailability: skipCommandAvailability},
		invocationSkipPermissionsOverride,
		providers,
	)
}

func validateWorkerLoadPreflight(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight, invocationSkipPermissionsOverride *bool, providers *providerregistry.Registry) error {
	if err := validateConfiguredWorkstationRunners(cfg, factoryRunnerID, runtimeCfg, executableLocator, preflight, providers); err != nil {
		return err
	}
	return skippermissions.ValidateInvocationSkipPermissionsWorkers(cfg, runtimeCfg, invocationSkipPermissionsOverride)
}

func validateConfiguredWorkstationRunners(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight, providers *providerregistry.Registry) error {
	for index, workstation := range cfg.Workstations {
		if configured, ok := runtimeCfg.Workstation(workstation.Name); ok && configured != nil {
			workstation = *configured
		}
		worker, _ := runtimeCfg.Worker(workstation.WorkerTypeName)
		modelProvider, openCodeAgent := "", ""
		if worker != nil {
			modelProvider, openCodeAgent = worker.ModelProvider, worker.OpenCodeAgent
		}
		selection, selectionErr := resolveRuntimeRunnerSelection(
			providers,
			workstation.Runner,
			factoryRunnerID,
			modelProvider,
		)
		if selectionErr != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, selectionErr)
		}
		if err := workerrunner.ValidateOpenCodeAgentForRunnerSelection(workstation.OpenCodeAgent, openCodeAgent, selection); err != nil {
			return fmt.Errorf("workstations[%d](%s).openCodeAgent: %w", index, workstation.Name, err)
		}
		if selection.Source == workerexecution.RunnerSelectionSourceDefault {
			continue
		}
		if err := validateRuntimeRunnerIdentity(providers, selection.RunnerID); err != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, err)
		}
		if !preflight.skipCommandAvailability {
			if err := validateRuntimeRunnerPrerequisites(providers, executableLocator, selection.RunnerID); err != nil {
				return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, err)
			}
		}
	}
	return nil
}

func resolveRuntimeRunnerSelection(
	providers *providerregistry.Registry,
	workstationRunner string,
	factoryRunnerID string,
	modelProvider string,
) (workers.ResolvedRunnerSelection, error) {
	if providers != nil {
		return providers.ResolveRunnerSelection(workstationRunner, factoryRunnerID, modelProvider)
	}
	return workerrunner.ResolveRunnerSelection(workstationRunner, factoryRunnerID, modelProvider), nil
}

func validateRuntimeRunnerIdentity(providers *providerregistry.Registry, runnerID string) error {
	if providers != nil {
		_, err := providers.RunnerMetadata(runnerID)
		return err
	}
	if _, ok := workerrunner.BuiltInRunnerMetadata(runnerID); !ok {
		return fmt.Errorf("unknown runner %q", runnerID)
	}
	if status, ok := workers.BuiltInRunnerStatus(runnerID); ok && !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}
	return nil
}

func validateRuntimeRunnerPrerequisites(
	providers *providerregistry.Registry,
	executableLocator platformprocess.ExecutableLocator,
	runnerID string,
) error {
	if providers != nil {
		return providers.ValidateRunnerPrerequisites(executableLocator, runnerID)
	}
	return workers.ValidateBuiltInRunnerPrerequisites(executableLocator, runnerID)
}
