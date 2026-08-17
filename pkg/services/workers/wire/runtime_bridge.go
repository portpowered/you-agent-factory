package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/execution/recording"
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

// NewMockCommandRunner decorates a command edge with configured mock behavior.
func NewMockCommandRunner(
	config *workers.MockWorkersConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	next workers.CommandRunner,
) workers.CommandRunner {
	return &runnermockworker.MockWorkerCommandRunner{
		Config:        config,
		RuntimeConfig: runtimeConfig,
		Next:          next,
	}
}

// LocalRuntimeHooks returns Workers-owned recording hooks for the Models runtime.
func LocalRuntimeHooks() workers.LocalRuntimeHooks {
	return modelrecording.Hooks()
}

// NewConductorInvocationWithProgress routes external integrations through the
// conductor. It is the only direct-invocation constructor with a remaining
// production caller (pkg/wire/session_runtime_providers.go composes it for
// standalone Factory Session execution); its zero-caller siblings
// NewInvocation and NewInvocationWithProgress were retired in P6-C.
//
// TODO(P6-C): retire this bridge once that caller passes detached values to
// workers.Service.Execute, which is the named successor boundary.
func NewConductorInvocationWithProgress(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewConductorInvocationWithProgress(
		providersService,
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		progressPublisher,
		temporaryFileSystems...,
	)
}

// NewProviderFromCommandRunner constructs one provider-backed worker from a command runner.
func NewProviderFromCommandRunner(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (providers.Service, error) {
	return workersinternal.NewProviderFromService(providersService)
}

// ResolveTemplateFields exposes the Workers-owned template resolver for composition.
func ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	return workersinternal.ResolveTemplateFields(
		workingDirectory,
		environment,
		tokens,
		workflowContext,
		worktree,
	)
}
