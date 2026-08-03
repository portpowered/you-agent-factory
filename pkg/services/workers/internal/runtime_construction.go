package internal

import (
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
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

// newSessionBuildRuntime constructs an independent Workers runtime for one
// Factory Session build from base's own session-scoped construction
// collaborators, with its own final provider/script command runners and
// progress publisher supplied directly at construction. Nil arguments
// preserve base's own value. Unlike the removed rebuiltForBuild, this calls
// the real constructor with final dependencies instead of cloning and
// mutating an existing instance: base's own fields are never touched, the
// returned Service is independently immutable for its own lifetime, and it
// naturally receives its own independent workstation pool the same way any
// freshly constructed runtime does. Freshly rebound Providers services are
// added to base's own shared lifecycle sink so every session-build
// activation over one Factory Session still closes together, exactly once,
// at session close -- the same accumulate-and-close-once semantics the
// removed clone-based seam relied on.
func (s *Service) newSessionBuildRuntime(
	providerRunner, scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
) (*Service, error) {
	if s == nil || s.scriptFactory == nil {
		return nil, fmt.Errorf("construct Worker runtime services: base service is required")
	}
	resolvedProviderRunner := s.providerCommandRunner
	providerCommandInjected := s.providerCommandInjected
	providersService := s.providers
	providerRegistry := s.providerRegistry
	if providerRunner != nil {
		resolvedProviderRunner = providerRunner
		providerCommandInjected = true
		reboundRegistry, reboundProviders, err := rebindProviderRegistry(s.providerRegistry, providerRunner, s.providerRegistryRebinder)
		if err != nil {
			return nil, err
		}
		if reboundRegistry != nil {
			providerRegistry = reboundRegistry
		}
		if reboundProviders != nil {
			providersService = reboundProviders
		}
	}
	resolvedScriptRunner := s.scriptCommandRunner
	scriptCommandInjected := s.scriptCommandInjected
	if scriptRunner != nil {
		resolvedScriptRunner = scriptRunner
		scriptCommandInjected = true
	}
	resolvedProgressPublisher := s.progressPublisher
	if progressPublisher != nil {
		resolvedProgressPublisher = progressPublisher
	}
	built, err := NewRuntimeWithSelection(
		s.sessions,
		s.models,
		providersService,
		s.modelsScope,
		resolvedProviderRunner,
		resolvedScriptRunner,
		resolvedProgressPublisher,
		s.allocator,
		s.logger,
		s.verbose,
		s.factoryRunnerID,
		s.runWorktree,
		s.invocationSkipPermissionsOverride,
		s.providerOverride,
		s.clock,
		s.processEnvironment,
		s.currentWorkingDirectory,
		nil,
		s.interpolation,
		s.executionPolicy,
		s.factoryDocs,
		s.resolveSymlinks,
		s.executableLocator,
		s.executableInspector,
		s.executableFiles,
		s.operatingSystem,
		s.worktreePreparer,
		s.agentRunHarness,
		s.retryRandom,
		s.workstationFiles,
		s.temporaryFiles,
		s.decisionEnvelopes,
		providerCommandInjected,
		scriptCommandInjected,
		false,
		providerRegistry,
		s.providerRegistryRebinder,
	)
	if err != nil {
		return nil, err
	}
	runtime, ok := built.(*Service)
	if !ok || runtime == nil {
		return nil, fmt.Errorf("construct Worker runtime services: unexpected runtime implementation")
	}
	if s.providerLifecycles != nil {
		runtime.providerLifecycles = s.providerLifecycles
	}
	if providerRunner != nil && providersService != s.providers {
		runtime.providerLifecycles.Add(providersService)
	}
	return runtime, nil
}

// NewSessionBuildRuntime returns an independently constructed Workers runtime
// for one Factory Session build, with its final provider/script command
// runners and progress publisher supplied at construction rather than
// replaced on an existing instance. It is the sole surviving construction
// path for per-session-build runner/publisher selection, kept off the public
// RuntimeService contract and reachable only through the Workers wire
// boundary.
func NewSessionBuildRuntime(
	base workers.RuntimeService,
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
) (workers.RuntimeService, error) {
	service, ok := base.(*Service)
	if !ok || service == nil {
		return nil, fmt.Errorf("Workers runtime service has an unsupported implementation")
	}
	return service.newSessionBuildRuntime(providerRunner, scriptRunner, progressPublisher)
}

func serviceCommandClock(s *Service) workers.Clock {
	return workers.ClockFunc(func() time.Time {
		if s != nil && s.clock != nil {
			return s.clock()
		}
		return time.Now()
	})
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
