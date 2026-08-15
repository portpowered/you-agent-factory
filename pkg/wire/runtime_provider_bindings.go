package wire

import (
	"context"
	"errors"
	"fmt"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

// newConfiguredProvidersService always installs the shell-free Antigravity
// print-mode command effect. An injected serviceedges.Edges.AgyPTYHost exists
// only to satisfy the legacy PTY allocator construction port and must never
// suppress the canonical command adapter; the command effect unconditionally
// takes priority over the legacy PTY effect in executionwire's built-in
// dependency selection.
func newConfiguredProvidersService(
	options []providerswire.Option,
	agyRunner workers.CommandRunner,
) (providers.Service, error) {
	options = append(options, providerswire.WithAgyCommandRunner(providerCommandEffect(agyRunner)))
	return providerswire.NewService(options...)
}

// provideRuntimeProviderBindings builds graph-worker provider bindings over the
// Factory Runtime's effective command runner, including mock and replay wrappers.
func provideRuntimeProviderBindings(
	edges serviceedges.Edges,
	integrations []operatorsettings.ACPIntegration,
	runner workers.CommandRunner,
) (workers.ProviderRegistry, providers.Service, workerswire.ProviderRegistryRebinder, error) {
	runtimeProviders, err := provideConfiguredProvidersService(edges, integrations, runner)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("construct runtime Providers service: %w", err)
	}
	runtimeRegistry, err := workerswire.NewProviderRegistry(context.Background(), runtimeProviders)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("construct runtime provider registry: %w", err)
	}
	rebinder := workerswire.ProviderRegistryRebinder(func(reboundRunner workers.CommandRunner) (workers.ProviderRegistry, providers.Service, error) {
		reboundProviders, rebuildErr := provideConfiguredProvidersService(edges, integrations, reboundRunner)
		if rebuildErr != nil {
			return nil, nil, rebuildErr
		}
		reboundRegistry, registryErr := workerswire.NewProviderRegistry(context.Background(), reboundProviders)
		return reboundRegistry, reboundProviders, registryErr
	})
	return runtimeRegistry, runtimeProviders, rebinder, nil
}

// providerCommandEffect projects the request-scoped Workers command edge into
// the Providers-owned process effect. The projection lives in the application
// composition root because Workers still owns mock-worker and command-log
// policy while Providers owns the adapter-facing effect vocabulary.
func providerCommandEffect(runner workers.CommandRunner) providerswire.CommandRunner {
	if runner == nil {
		return nil
	}
	return providerCommandRunner{runner: runner}
}

type providerCommandRunner struct {
	runner workers.CommandRunner
}

func (runner providerCommandRunner) Run(
	ctx context.Context,
	request providerswire.CommandRequest,
) (providerswire.CommandResult, error) {
	result, err := runner.runner.Run(ctx, workers.CommandRequest{
		Command:         request.Command,
		Args:            append([]string(nil), request.Args...),
		Stdin:           append([]byte(nil), request.Stdin...),
		Env:             append([]string(nil), request.Env...),
		WorkDir:         request.WorkDir,
		DispatchID:      request.AttemptID,
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationName,
		Execution:       request.Execution,
		ExecutionLogger: request.ExecutionLogger,
	})
	return providerswire.CommandResult{
		Stdout:   append([]byte(nil), result.Stdout...),
		Stderr:   append([]byte(nil), result.Stderr...),
		ExitCode: result.ExitCode,
	}, err
}

func (runner providerCommandRunner) RunStreaming(
	ctx context.Context,
	request providerswire.CommandRequest,
	observer providerswire.OutputChunkObserver,
) (providerswire.CommandResult, error) {
	workerRequest := workers.CommandRequest{
		Command:         request.Command,
		Args:            append([]string(nil), request.Args...),
		Stdin:           append([]byte(nil), request.Stdin...),
		Env:             append([]string(nil), request.Env...),
		WorkDir:         request.WorkDir,
		DispatchID:      request.AttemptID,
		WorkerType:      request.WorkerType,
		WorkstationName: request.WorkstationName,
		Execution:       request.Execution,
		ExecutionLogger: request.ExecutionLogger,
	}
	streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	})
	if !ok {
		result, err := runner.runner.Run(ctx, workerRequest)
		if observerErr := publishProviderCommandOutput(observer, result.Stdout, result.Stderr); err == nil {
			err = observerErr
		}
		return providerCommandResult(result), err
	}
	var observerErr error
	result, err := streaming.RunStreaming(ctx, workerRequest, func(stream string, chunk []byte) {
		if observerErr != nil || observer == nil {
			return
		}
		observerErr = observer(stream, append([]byte(nil), chunk...))
	})
	if err == nil {
		err = observerErr
	}
	return providerCommandResult(result), err
}

func providerCommandResult(result workers.CommandResult) providerswire.CommandResult {
	return providerswire.CommandResult{
		Stdout:   append([]byte(nil), result.Stdout...),
		Stderr:   append([]byte(nil), result.Stderr...),
		ExitCode: result.ExitCode,
	}
}

func publishProviderCommandOutput(
	observer providerswire.OutputChunkObserver,
	stdout []byte,
	stderr []byte,
) error {
	if observer == nil {
		return nil
	}
	if len(stdout) > 0 {
		if err := observer(providerswire.OutputStreamStdout, append([]byte(nil), stdout...)); err != nil {
			return err
		}
	}
	if len(stderr) > 0 {
		return observer(providerswire.OutputStreamStderr, append([]byte(nil), stderr...))
	}
	return nil
}

// providerPTYAllocator projects the Providers-owned PTY effect into the
// legacy Workers consumer boundary. It is a composition-only bridge; neither
// provider adapters nor the Providers root depend on Workers PTY contracts.
func providerPTYAllocator(allocator providerswire.PTYAllocator) workers.PTYAllocator {
	if allocator == nil {
		return nil
	}
	return providerPTYAllocatorAdapter{allocator: allocator}
}

type providerPTYAllocatorAdapter struct {
	allocator providerswire.PTYAllocator
}

func (adapter providerPTYAllocatorAdapter) Allocate(
	ctx context.Context,
	launch workers.PTYProcessLaunch,
	config workers.PTYSessionConfig,
) (workers.PTYSession, error) {
	session, err := adapter.allocator.Allocate(ctx, providerswire.PTYProcessLaunch{
		Executable: launch.Executable,
		Argv:       append([]string(nil), launch.Argv...),
		WorkDir:    launch.WorkDir,
		Env:        append([]string(nil), launch.Env...),
	}, providerswire.PTYSessionConfig{
		MaxCaptureBytes: config.MaxCaptureBytes,
		IdleTimeout:     config.IdleTimeout,
		HardTimeout:     config.HardTimeout,
	})
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, errors.New("provider PTY allocator returned a nil session")
	}
	return providerPTYSessionAdapter{session: session}, nil
}

type providerPTYSessionAdapter struct {
	session providerswire.PTYSession
}

func (adapter providerPTYSessionAdapter) Run(ctx context.Context) (workers.PTYSessionResult, error) {
	result, err := adapter.session.Run(ctx)
	return workers.PTYSessionResult{
		ExitCode:    result.ExitCode,
		RawBytes:    append([]byte(nil), result.RawBytes...),
		CleanedText: result.CleanedText,
		TimedOut:    result.TimedOut,
		CapacityHit: result.CapacityHit,
	}, err
}

func (adapter providerPTYSessionAdapter) Close() error {
	return adapter.session.Close()
}
