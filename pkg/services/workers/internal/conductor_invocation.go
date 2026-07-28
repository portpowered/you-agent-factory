package internal

import (
	"context"
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/invocation"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

// NewConductorInvocationWithProgress constructs a direct-invocation executor that
// routes Codex and Claude through the provider-neutral conductor while retaining
// the legacy subprocess path for other providers.
func NewConductorInvocationWithProgress(
	registry *providerregistry.Registry,
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	if registry == nil {
		return NewInvocationWithProgress(
			commandRunner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem,
			progressPublisher, temporaryFileSystems...,
		)
	}
	legacyProvider, err := NewProviderFromCommandRunner(
		commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFileSystems...,
	)
	if err != nil {
		return nil, fmt.Errorf("construct conductor invocation legacy provider: %w", err)
	}
	inner := workerexecutor.RunnerFromProvider(legacyProvider)
	runner := conductorInvocationRunner{
		next:      inner,
		conductor: providerconductor.New(registry),
		providers: registry,
		publish:   progressPublisher,
	}
	return workerinvocation.NewExecutor(runnerInferProvider{runner: runner}), nil
}

type runnerInferProvider struct {
	runner workers.Runner
}

func (p runnerInferProvider) Infer(
	ctx context.Context,
	req workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	result, err := p.runner.Execute(ctx, req)
	if err != nil {
		return workers.InferenceResponse{}, err
	}
	return workers.InferenceResponse{
		Content:         result.Content,
		ProviderSession: workers.CloneProviderSessionMetadata(result.ProviderSession),
		Diagnostics:     result.Diagnostics,
	}, nil
}

func (p runnerInferProvider) Execute(
	ctx context.Context,
	req workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return p.runner.Execute(ctx, req)
}
