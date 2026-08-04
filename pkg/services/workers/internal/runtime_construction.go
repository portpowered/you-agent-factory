package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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

// sessionBuildDependencies holds the resolved, final construction-time values
// for one session-build's independently constructed Workers runtime.
// reboundProviders is non-nil only when resolveSessionBuildDependencies
// rebound a fresh Providers instance for this build (as opposed to reusing
// base's own); it is the exact instance a caller must release if the build
// never adopts it into base's shared provider lifecycle sink.
type sessionBuildDependencies struct {
	providerRunner          workers.CommandRunner
	scriptRunner            workers.CommandRunner
	progressPublisher       workers.ProgressPublisher
	providerCommandInjected bool
	scriptCommandInjected   bool
	providerRegistry        workers.ProviderRegistry
	providersService        providers.Service
	reboundProviders        providers.Service
}

// resolveSessionBuildDependencies resolves the final runner/publisher/
// provider values for one session-build from base's own values. Nil
// providerRunner/scriptRunner/progressPublisher arguments preserve base's own
// value. Supplying a non-nil providerRunner rebinds the provider registry
// through base's registered rebinder.
func (s *Service) resolveSessionBuildDependencies(
	providerRunner, scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
) (sessionBuildDependencies, error) {
	deps := sessionBuildDependencies{
		providerRunner:          s.providerCommandRunner,
		scriptRunner:            s.scriptCommandRunner,
		progressPublisher:       s.progressPublisher,
		providerCommandInjected: s.providerCommandInjected,
		scriptCommandInjected:   s.scriptCommandInjected,
		providerRegistry:        s.providerRegistry,
		providersService:        s.providers,
	}
	if scriptRunner != nil {
		deps.scriptRunner = scriptRunner
		deps.scriptCommandInjected = true
	}
	if progressPublisher != nil {
		deps.progressPublisher = progressPublisher
	}
	if providerRunner == nil {
		return deps, nil
	}
	deps.providerRunner = providerRunner
	deps.providerCommandInjected = true
	reboundRegistry, reboundProviders, err := rebindProviderRegistry(s.providerRegistry, providerRunner, s.providerRegistryRebinder)
	if err != nil {
		return sessionBuildDependencies{}, err
	}
	if reboundRegistry != nil {
		deps.providerRegistry = reboundRegistry
	}
	if reboundProviders != nil {
		deps.providersService = reboundProviders
		deps.reboundProviders = reboundProviders
	}
	return deps, nil
}

// releaseUnadoptedProviders closes a freshly rebound Providers instance that
// was never adopted into base's shared provider lifecycle sink -- either
// because construction failed after the rebind, or because the sink had
// already closed by the time adoption was attempted. Without this, that
// instance would never be closed by anything. The returned error is the
// fallback Close's own failure, if any, so a caller can preserve it alongside
// the primary error instead of discarding evidence that the release itself
// did not succeed.
func releaseUnadoptedProviders(deps sessionBuildDependencies) error {
	if deps.reboundProviders == nil {
		return nil
	}
	if lifecycle, ok := deps.reboundProviders.(providers.Lifecycle); ok {
		return lifecycle.Close(context.Background())
	}
	return nil
}

// constructSessionBuildRuntime builds the independent Service instance for
// one session-build from base's own session-scoped construction
// collaborators, varying only the resolved runner/publisher/provider values.
// It does not touch base's own fields and does not register or release
// deps.reboundProviders -- that ownership decision belongs to the caller.
func (s *Service) constructSessionBuildRuntime(deps sessionBuildDependencies) (*Service, error) {
	built, err := NewRuntimeWithSelection(
		s.sessions,
		s.models,
		deps.providersService,
		s.modelsScope,
		deps.providerRunner,
		deps.scriptRunner,
		deps.progressPublisher,
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
		deps.providerCommandInjected,
		deps.scriptCommandInjected,
		false,
		deps.providerRegistry,
		s.providerRegistryRebinder,
	)
	if err != nil {
		return nil, err
	}
	runtime, ok := built.(*Service)
	if !ok || runtime == nil {
		return nil, fmt.Errorf("construct Worker runtime services: unexpected runtime implementation")
	}
	return runtime, nil
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
// removed clone-based seam relied on. If base has already closed by the time
// adoption is attempted, or construction fails after a rebind, the rebound
// Providers instance is released immediately instead of leaking, and no
// runtime is returned.
func (s *Service) newSessionBuildRuntime(
	providerRunner, scriptRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
) (runtime *Service, err error) {
	if s == nil || s.scriptFactory == nil {
		return nil, fmt.Errorf("construct Worker runtime services: base service is required")
	}
	deps, err := s.resolveSessionBuildDependencies(providerRunner, scriptRunner, progressPublisher)
	if err != nil {
		return nil, err
	}
	adopted := false
	defer func() {
		if adopted {
			return
		}
		if releaseErr := releaseUnadoptedProviders(deps); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release unadopted provider lifecycle: %w", releaseErr))
		}
	}()

	built, err := s.constructSessionBuildRuntime(deps)
	if err != nil {
		return nil, err
	}
	if s.providerLifecycles != nil {
		built.providerLifecycles = s.providerLifecycles
	}
	if deps.reboundProviders != nil && !built.providerLifecycles.Add(deps.reboundProviders) {
		return nil, fmt.Errorf("construct Worker runtime services: base runtime is already closed")
	}
	adopted = true
	return built, nil
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
