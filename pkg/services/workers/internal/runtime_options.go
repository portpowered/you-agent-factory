package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution/recording"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/skippermissions"
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
	providerOverride providers.Service,
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
	// A session-scoped command runner rebuilds the Providers root together with
	// its catalog projection. Keep runtime worker construction on that rebound
	// root when no explicit request-scoped override was supplied; an explicit
	// override remains authoritative for mock/replay and functional callers.
	if providerOverride == nil {
		providerOverride = s.providerOverride
	}
	if providerOverride == nil && s.providerCommandInjected && s.providers != nil {
		providerOverride = s.providers
	}
	now := clock
	if now == nil {
		now = s.clock
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
		s.providers,
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
	if err := s.buildConfiguredRuntimeWorkers(
		executors, runtimeConfig, factoryConfig, factoryRunnerID, workflowContext, logger,
		invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher,
		scriptRecorder, inferenceRecorder, agentRunRecorder, now, decorators,
	); err != nil {
		return nil, err
	}
	s.buildConfiguredLogicalWorkstations(executors, runtimeConfig, factoryConfig, factoryRunnerID, workflowContext, logger, now)
	return executors, nil
}

func (s *Service) buildConfiguredRuntimeWorkers(
	executors map[string]workers.WorkerExecutor,
	runtimeConfig interfaces.RuntimeConfigLookup,
	factoryConfig *interfaces.FactoryConfig,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride providers.Service,
	inferenceProgressPublisher workers.ProgressPublisher,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	agentRunRecorder workers.AgentRunEventRecorder,
	now func() time.Time,
	decorators []workerconstruction.RunnerDecorator,
) error {
	for _, configured := range factoryConfig.Workers {
		definition, ok := runtimeConfig.Worker(configured.Name)
		if !ok || definition == nil || definition.Type == "" {
			executors[configured.Name] = &workerexecutor.NoopExecutor{}
			continue
		}
		// Poller/hosted Worker shapes are Automations-owned ingress sources.
		// They submit Work Requests and never enter Workers executor construction.
		if interfaces.IsPollerWorkerType(definition.Type) {
			continue
		}
		result, err := s.executorBuilder.Build(
			runtimeConfig, configured.Name, factoryRunnerID, workflowContext, logger,
			invocationSkipPermissionsOverride, providerOverride, inferenceProgressPublisher,
			scriptRecorder, inferenceRecorder, agentRunRecorder, now, s.processEnvironment, s.currentWorkingDirectory, decorators,
		)
		if err != nil {
			return fmt.Errorf("construct worker %q: %w", configured.Name, err)
		}
		if result.Dispatch == nil {
			return fmt.Errorf("unsupported worker type for worker %q: %s", configured.Name, definition.Type)
		}
		executors[configured.Name] = result.Dispatch
	}
	return nil
}

func (s *Service) buildConfiguredLogicalWorkstations(
	executors map[string]workers.WorkerExecutor,
	runtimeConfig interfaces.RuntimeConfigLookup,
	factoryConfig *interfaces.FactoryConfig,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	now func() time.Time,
) {
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
}

func (s *Service) rebindProvidersCommandRunner(logger logging.Logger) error {
	if s == nil || !s.providerCommandInjected || s.providerCommandRunner == nil || s.providersRebinder == nil {
		return nil
	}
	providersRunner := workers.CommandRunnerWithLogging(
		s.providerCommandRunner,
		logging.EnsureLogger(logger),
		serviceCommandClock(s),
	)
	reboundProviders, err := rebindProvidersService(s.providers, providersRunner, s.providersRebinder)
	if err != nil {
		return fmt.Errorf("rebind Providers service for session command logging: %w", err)
	}
	return applyReboundProvidersService(s, reboundProviders)
}

func (s *Service) runtimeRunnerDecorators(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	recorder workers.ModelEventRecorder,
	now func() time.Time,
	useRegistryCapabilities bool,
	_ workers.ProgressPublisher,
) []workerconstruction.RunnerDecorator {
	decorators := make([]workerconstruction.RunnerDecorator, 0, 4)
	if useRegistryCapabilities && s.providers != nil {
		decorators = append(decorators, func(inner workers.Runner, _ *interfaces.FactoryWorkerConfig) workers.Runner {
			return registryCapabilityRunner{next: inner, providers: s.providers}
		})
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
	providerServices ...providers.Service,
) error {
	if cfg == nil {
		return fmt.Errorf("factory config is required")
	}
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	var providerService providers.Service
	if len(providerServices) > 0 {
		providerService = providerServices[0]
	}
	return validateRuntimeSelectionsWithProviders(
		cfg,
		factoryRunnerID,
		runtimeCfg,
		executableLocator,
		skipCommandAvailability,
		invocationSkipPermissionsOverride,
		providerService,
	)
}

type runnerSelectionPreflight struct{ skipCommandAvailability bool }

func validateRuntimeSelectionsWithProviders(
	cfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	executableLocator platformprocess.ExecutableLocator,
	skipCommandAvailability bool,
	invocationSkipPermissionsOverride *bool,
	providerService providers.Service,
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
		providerService,
	)
}

func validateWorkerLoadPreflight(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight, invocationSkipPermissionsOverride *bool, providerService providers.Service) error {
	if err := validateConfiguredWorkstationRunners(cfg, factoryRunnerID, runtimeCfg, executableLocator, preflight, providerService); err != nil {
		return err
	}
	return validateInvocationSkipPermissionsWorkers(cfg, runtimeCfg, invocationSkipPermissionsOverride, providerService)
}

func validateInvocationSkipPermissionsWorkers(
	cfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	invocationOverride *bool,
	providerService providers.Service,
) error {
	if invocationOverride == nil || !*invocationOverride || cfg == nil || runtimeCfg == nil {
		return nil
	}
	for _, workerCfg := range cfg.Workers {
		worker, ok := runtimeCfg.Worker(workerCfg.Name)
		if !ok || worker == nil || worker.Type == "" {
			continue
		}
		if err := skippermissions.ValidateInvocationSkipPermissionsForWorker(worker, invocationOverride); err != nil {
			// A configured Providers-catalog integration (currently ACP) applies
			// this policy at its protocol boundary, not through a known CLI flag.
			if providerService != nil && strings.TrimSpace(worker.ModelProvider) != "" {
				if _, selectionErr := providerService.ResolveSelection(
					context.Background(),
					providers.ResolveSelectionRequest{ModelProvider: worker.ModelProvider},
				); selectionErr == nil {
					continue
				}
			}
			return fmt.Errorf("worker %q: %w", workerCfg.Name, err)
		}
	}
	return nil
}

func validateConfiguredWorkstationRunners(cfg *interfaces.FactoryConfig, factoryRunnerID string, runtimeCfg interfaces.RuntimeConfigLookup, executableLocator platformprocess.ExecutableLocator, preflight runnerSelectionPreflight, providerService providers.Service) error {
	for index, workstation := range cfg.Workstations {
		if configured, ok := runtimeCfg.Workstation(workstation.Name); ok && configured != nil {
			workstation = *configured
		}
		worker, _ := runtimeCfg.Worker(workstation.WorkerTypeName)
		modelProvider := ""
		if worker != nil {
			modelProvider = worker.ModelProvider
		}
		// Invocation placeholders are resolved for each dispatch. They are not
		// concrete provider identities during the invocation-neutral runtime
		// preflight, so defer provider validation until interpolation.
		if strings.Contains(modelProvider, "${") {
			modelProvider = ""
		}
		workstationRunner := workstation.Runner
		if strings.Contains(workstationRunner, "${") {
			workstationRunner = ""
		}
		selection, selectionErr := resolveRuntimeRunnerSelection(
			context.Background(),
			providerService,
			workstationRunner,
			factoryRunnerID,
			modelProvider,
		)
		if selectionErr != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, selectionErr)
		}
		if selection.Source == workerexecution.RunnerSelectionSourceDefault {
			continue
		}
		if err := validateRuntimeRunnerIdentity(context.Background(), providerService, selection.RunnerID); err != nil {
			return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, err)
		}
		if !preflight.skipCommandAvailability {
			if err := validateRuntimeRunnerPrerequisites(context.Background(), providerService, executableLocator, selection.RunnerID); err != nil {
				return fmt.Errorf("workstations[%d](%s).runner: %w", index, workstation.Name, err)
			}
		}
	}
	return nil
}

func resolveRuntimeRunnerSelection(
	ctx context.Context,
	providerService providers.Service,
	workstationRunner string,
	factoryRunnerID string,
	modelProvider string,

) (workers.ResolvedRunnerSelection, error) {
	if providerService != nil {
		resolved, err := providerService.ResolveSelection(ctx, providers.ResolveSelectionRequest{
			Workstation:   workstationRunner,
			Factory:       factoryRunnerID,
			ModelProvider: modelProvider,
		})
		if err != nil {
			return workers.ResolvedRunnerSelection{}, err
		}
		return workers.ResolvedRunnerSelection{
			RunnerID: providerRunnerID(resolved.Provider),
			Source:   workerSelectionSource(resolved.Source),
		}, nil
	}
	return workerrunner.ResolveRunnerSelection(workstationRunner, factoryRunnerID, modelProvider), nil
}

func validateRuntimeRunnerIdentity(ctx context.Context, providerService providers.Service, runnerID string) error {
	if providerService != nil {
		resolved, err := providerService.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: runnerID})
		if err != nil {
			return err
		}
		_, err = providerService.GetProvider(ctx, providers.GetProviderRequest{ID: resolved.ID})
		return err
	}
	if _, ok := workerrunner.BuiltInRunnerMetadata(runnerID); !ok {
		return fmt.Errorf("unknown runner %q", runnerID)
	}
	if status, ok := workerrunner.BuiltInRunnerStatus(runnerID); ok && !status.Available {
		return fmt.Errorf("%s", status.UnavailableReason)
	}
	return nil
}

func validateRuntimeRunnerPrerequisites(
	ctx context.Context,
	providerService providers.Service,
	executableLocator platformprocess.ExecutableLocator,
	runnerID string,
) error {
	if providerService != nil {
		resolved, err := providerService.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: runnerID})
		if err != nil {
			return err
		}
		return providerService.ValidatePrerequisites(ctx, providers.ValidatePrerequisitesRequest{ID: resolved.ID})
	}
	return workerrunner.ValidateBuiltInRunnerPrerequisites(executableLocator, runnerID)
}

func providerRunnerID(id providers.ID) string {
	return strings.ToLower(strings.TrimSpace(id.String()))
}

func workerSelectionSource(source providers.SelectionSource) workers.RunnerSelectionSource {
	switch source {
	case providers.SelectionSourceWorkstation:
		return workers.RunnerSelectionSourceWorkstation
	case providers.SelectionSourceFactory:
		return workers.RunnerSelectionSourceFactory
	case providers.SelectionSourceLegacyProvider:
		return workers.RunnerSelectionSourceLegacyProvider
	default:
		return workers.RunnerSelectionSourceDefault
	}
}
