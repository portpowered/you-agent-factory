package internal

import (
	"fmt"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
)

func buildExecutionFactories(
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	factoryDocs workers.FactoryDocsLoader,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
) (*workerexecutor.ScriptFactory, workers.CommandRunner, workers.CommandRunner, error) {
	if providerRunner == nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: provider command runner is required")
	}
	if scriptRunner == nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: script command runner is required")
	}
	if commandClock == nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: command clock is required")
	}
	if allocator == nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: Agy PTY allocator is required")
	}
	if factoryDocs == nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: Factory docs loader is required")
	}
	scriptFactory, err := workerexecutor.NewScriptFactory(scriptRunner, commandClock, factoryDocs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("construct Worker execution service: %w", err)
	}
	return scriptFactory, providerRunner, scriptRunner, nil
}

// WithCommandRunners returns an inert copy whose executor factories use the
// supplied runtime-specific wrappers. Nil preserves the existing edge.
func (s *Service) WithCommandRunners(providerRunner, scriptRunner workers.CommandRunner) (workers.RuntimeService, error) {
	if s == nil || s.scriptFactory == nil {
		return nil, fmt.Errorf("construct Worker runtime services: base service is required")
	}
	clone := *s
	var err error
	if providerRunner != nil {
		clone.providerCommandRunner = providerRunner
		clone.providerCommandInjected = true
		reboundRegistry, reboundProviders, err := rebindProviderRegistry(s.providerRegistry, providerRunner, s.providerRegistryRebinder)
		if err != nil {
			return nil, err
		}
		if err := applyReboundProviderRegistry(&clone, reboundRegistry, reboundProviders); err != nil {
			return nil, err
		}
		clone.providerLifecycles.Add(reboundProviders)
	}
	if scriptRunner != nil {
		clone.scriptFactory, err = s.scriptFactory.WithCommandRunner(scriptRunner)
		if err != nil {
			return nil, err
		}
		clone.scriptCommandRunner = scriptRunner
		clone.scriptCommandInjected = true
	}
	// Each opened Factory Runtime owns an independent workstation lifecycle.
	// Sharing the process-level pool would couple route admission, cancellation,
	// and terminal stop state across otherwise separate Factory Sessions.
	clone.Root = clone.Root.ReplaceWorkstations(workstationswire.NewService())
	clone.executorBuilder = rebuildExecutorBuilder(
		s.executorBuilder,
		clone.providers,
		clone.scriptFactory,
		clone.interpolation,
		clone.executionPolicy,
		clone.factoryDocs,
		clone.worktreePreparer,
		clone.agentRunHarness,
		clone.retryRandom,
		clone.workstationFiles,
		clone.decisionEnvelopes,
	)
	return &clone, nil
}

func serviceCommandClock(s *Service) workers.Clock {
	return workers.ClockFunc(func() time.Time {
		if s != nil && s.clock != nil {
			return s.clock()
		}
		return time.Now()
	})
}

func rebuildExecutorBuilder(
	current workerconstruction.Builder,
	providersService providers.Service,
	scriptFactory *workerexecutor.ScriptFactory,
	interpolation factorydefinitionswire.InvocationInterpolationService,
	executionPolicy factorydefinitionswire.WorkstationExecutionPolicyService,
	factoryDocs workers.FactoryDocsLoader,
	worktreePreparer workers.FactoryWorktreePreparer,
	agentRunHarness workeragentrun.HarnessAdapter,
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	decisionEnvelopes factorydefinitionswire.DecisionEnvelopeService,
) workerconstruction.Builder {
	if existing, ok := current.(*workerconstruction.Service); ok {
		return existing.WithExecutionFactories(providersService, scriptFactory)
	}
	return workerconstruction.New(
		providersService,
		scriptFactory,
		interpolation,
		executionPolicy,
		factoryDocs,
		worktreePreparer,
		agentRunHarness,
		retryRandom,
		workstationFiles,
		decisionEnvelopes,
	)
}

// WithProgressPublisher returns a runtime-specific copy that publishes
// provider subprocess progress. An explicitly injected provider runner always
// wins over the generated progress wrapper.
func (s *Service) WithProgressPublisher(
	runner workers.CommandRunner,
	_ workers.ProgressPublisher,
	_ bool,
	_ logging.Logger,
) (workers.RuntimeService, error) {
	if s == nil {
		return nil, fmt.Errorf("Worker execution service is required")
	}
	if s.ProviderCommandInjected() {
		return s, nil
	}
	if runner == nil {
		return s, nil
	}
	return s.WithCommandRunners(runner, nil)
}

func (s *Service) ProviderCommandInjected() bool {
	return s != nil && s.providerCommandInjected
}

func (s *Service) ProviderCommandRunner() workers.CommandRunner {
	if s == nil {
		return nil
	}
	return s.providerCommandRunner
}

func (s *Service) ScriptCommandRunner() workers.CommandRunner {
	if s == nil {
		return nil
	}
	return s.scriptCommandRunner
}
