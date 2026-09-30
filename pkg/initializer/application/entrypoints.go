package application

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
)

// ProcessContext owns process-signal subscription and cancellation lifecycle
// for one reusable Process invocation.
func (i *Initializer) ProcessContext(ctx context.Context) (context.Context, func()) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

// Initializer owns process lifecycle after CLI parsing and forwards run intent
// to the injected command execution operation.
type Initializer struct {
	stdio                processcontract.StdioHandler
	systemInitialization SystemInitializationOperation
}

// SystemInitializationOperation is the exact pre-lifecycle setup role owned by
// the Initializer. Wire adapts the system-initialization service to this role.
type SystemInitializationOperation func(context.Context, string) error

func NewInitializer(
	stdio processcontract.StdioHandler,
	systemInitialization SystemInitializationOperation,
) (*Initializer, error) {
	if stdio == nil {
		return nil, fmt.Errorf("stdio handler is required")
	}
	if systemInitialization == nil {
		return nil, fmt.Errorf("system initialization service is required")
	}
	return &Initializer{
		stdio:                stdio,
		systemInitialization: systemInitialization,
	}, nil
}

func (i *Initializer) Run(
	ctx context.Context,
	intent processcontract.RunIntent,
	selection processcontract.RunSelection,
) error {
	if selection == nil {
		return fmt.Errorf("initialize run service: run selection is required")
	}
	return selection.Run(ctx, intent)
}

func (i *Initializer) Stdio(ctx context.Context, intent processcontract.MCPIntent) error {
	if i == nil || i.stdio == nil {
		return fmt.Errorf("initialize stdio service: stdio handler is required")
	}
	return i.stdio(ctx, intent)
}

func (i *Initializer) InitializeSystem(ctx context.Context, homeDir string) error {
	if i == nil || i.systemInitialization == nil {
		return fmt.Errorf("system initialization service is required")
	}
	return i.systemInitialization(ctx, homeDir)
}
