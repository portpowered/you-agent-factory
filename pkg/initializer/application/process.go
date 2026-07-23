package application

import (
	"fmt"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// ProviderRegistry is the narrow immutable provider identity projection
// retained by the application process. The provider domain implements it
// without reversing the initializer dependency direction.
type ProviderRegistry interface {
	CanonicalIdentity(string) (string, error)
}

// Process is the inert, behavior-bearing process entrypoint assembled by
// Wire. It retains only its command/lifecycle roles and immutable provider
// authority, not runtime configuration, service graphs, or edge bundles; the
// lazy Initializer owns runtime construction after CLI parsing.
type Process struct {
	commandFactory processcontract.CommandFactory
	initializer    processcontract.Initializer
	providers      ProviderRegistry
}

func NewProcess(
	commandFactory processcontract.CommandFactory,
	initializer processcontract.Initializer,
	providers ProviderRegistry,
) (*Process, error) {
	if providers == nil {
		return nil, fmt.Errorf("construct application process: provider registry is required")
	}
	return &Process{
		commandFactory: commandFactory,
		initializer:    initializer,
		providers:      providers,
	}, nil
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
	return p.commandFactory.ExecuteCommand(processcontract.CommandInvocation{
		Arguments: normalized.argumentsCopy(), Stdin: normalized.stdin,
		Stdout: normalized.stdout, Stderr: normalized.stderr, Context: ctx,
		HomeDir:   func() (string, error) { return homeDir(normalized) },
		LookupEnv: normalized.lookupEnv, Initializer: p.initializer,
	})
}
