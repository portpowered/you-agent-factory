package application

import (
	"context"
	"fmt"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// ProviderRegistry is the narrow immutable provider identity projection
// retained by the application process. The provider domain implements it
// without reversing the initializer dependency direction.
type ProviderRegistry interface {
	CanonicalIdentity(string) (string, error)
}

// ProcessLifecycle is the exact application-owned cleanup role for inert
// services that retain resources lazily while commands execute.
type ProcessLifecycle interface {
	Close(context.Context) error
}

// Process is the inert, behavior-bearing process entrypoint assembled by
// Wire. It retains only its command/lifecycle roles and immutable provider
// authority, not runtime configuration, service graphs, or edge bundles; the
// lazy Initializer owns runtime construction after CLI parsing.
type Process struct {
	commandFactory processcontract.CommandFactory
	initializer    processcontract.Initializer
	providers      ProviderRegistry
	lifecycle      ProcessLifecycle
}

func NewProcess(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
	lifecycle ProcessLifecycle,
) (*Process, error) {
	if providers == nil {
		return nil, fmt.Errorf("construct application process: provider registry is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("construct application process: lifecycle is required")
	}
	return &Process{
		commandFactory: commandFactory,
		initializer:    initializer,
		providers:      providers,
		lifecycle:      lifecycle,
	}, nil
}

// Close releases resources retained by services during prior Execute calls.
// It is safe to call more than once after active command invocations stop.
func (p *Process) Close(ctx context.Context) error {
	if p == nil || p.lifecycle == nil {
		return nil
	}
	return p.lifecycle.Close(ctx)
}

// ProviderRegistry returns the immutable provider authority composed for this
// process. It is exposed for embedding and customer-scale verification.
func (p *Process) ProviderRegistry() ProviderRegistry {
	if p == nil {
		return nil
	}
	return p.providers
}

// Execute constructs and runs one fresh command tree using invocation-local
// process boundaries. The Process remains reusable for later invocations.
func (p *Process) Execute(input Input) error {
	normalized, err := normalize(input)
	if err != nil {
		return err
	}
	if p == nil || p.initializer == nil {
		return fmt.Errorf("execute application process: initializer is required")
	}
	ctx, stop := p.initializer.ProcessContext(normalized.context)
	if ctx == nil || stop == nil {
		return fmt.Errorf("execute application process: initializer returned an invalid process context")
	}
	defer stop()
	if p.commandFactory == nil {
		return fmt.Errorf("execute application process: command factory is required")
	}
	ctx = processcontract.WithWorkingDirectory(ctx, normalized.workingDir)
	ctx = processcontract.WithStdinTTY(ctx, normalized.stdinIsTTY)
	ctx = processcontract.WithStdoutTTY(ctx, normalized.stdoutIsTTY)
	ctx = processcontract.WithStderrTTY(ctx, normalized.stderrIsTTY)
	return p.commandFactory.ExecuteCommand(processcontract.CommandInvocation{
		Arguments: normalized.argumentsCopy(), Stdin: normalized.stdin,
		Stdout: normalized.stdout, Stderr: normalized.stderr, Context: ctx,
		HomeDir:   func() (string, error) { return homeDir(normalized) },
		LookupEnv: normalized.lookupEnv, Initializer: p.initializer,
	})
}
