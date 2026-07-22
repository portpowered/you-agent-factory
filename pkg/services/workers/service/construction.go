package service

import (
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

func buildExecutionFactories(
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	factoryDocs workers.FactoryDocsLoader,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
) (*workerprovider.Factory, *workerexecutor.ScriptFactory, workers.CommandRunner, workers.CommandRunner, error) {
	if providerRunner == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: provider command runner is required")
	}
	if scriptRunner == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: script command runner is required")
	}
	if commandClock == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: command clock is required")
	}
	if allocator == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: Agy PTY allocator is required")
	}
	if factoryDocs == nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: Factory docs loader is required")
	}
	providerFactory, err := workerprovider.NewFactory(
		providerRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		temporaryFiles,
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: %w", err)
	}
	scriptFactory, err := workerexecutor.NewScriptFactory(scriptRunner, commandClock, factoryDocs)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("construct Worker execution service: %w", err)
	}
	return providerFactory, scriptFactory, providerRunner, scriptRunner, nil
}

// WithCommandRunners returns an inert copy whose executor factories use the
// supplied runtime-specific wrappers. Nil preserves the existing edge.
func (s *Service) WithCommandRunners(providerRunner, scriptRunner workers.CommandRunner) (workers.RuntimeService, error) {
	if s == nil || s.providerFactory == nil || s.scriptFactory == nil {
		return nil, fmt.Errorf("construct Worker runtime services: base service is required")
	}
	clone := *s
	var err error
	if providerRunner != nil {
		clone.providerFactory, err = s.providerFactory.WithCommandRunner(providerRunner)
		if err != nil {
			return nil, err
		}
		clone.providerCommandRunner = providerRunner
		clone.providerCommandInjected = true
	}
	if scriptRunner != nil {
		clone.scriptFactory, err = s.scriptFactory.WithCommandRunner(scriptRunner)
		if err != nil {
			return nil, err
		}
		clone.scriptCommandRunner = scriptRunner
		clone.scriptCommandInjected = true
	}
	clone.executorBuilder = workerconstruction.New(
		clone.providerFactory,
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

// WithProgressPublisher returns a runtime-specific copy that publishes
// provider subprocess progress. An explicitly injected provider runner always
// wins over the generated progress wrapper.
func (s *Service) WithProgressPublisher(
	runner workers.CommandRunner,
	publisher workers.ProgressPublisher,
	publisherSet bool,
	logger logging.Logger,
) (workers.RuntimeService, error) {
	if s == nil {
		return nil, fmt.Errorf("Worker execution service is required")
	}
	if s.ProviderCommandInjected() {
		return s, nil
	}
	if publisherSet && publisher != nil && runner == nil {
		runner = workerprovider.NewInferenceProgressPublishingCommandRunnerWithRunner(
			s.providerCommandRunner,
			publisher,
			logging.EnsureLogger(logger),
		)
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
