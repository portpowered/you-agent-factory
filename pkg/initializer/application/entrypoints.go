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

// Initializer owns lifecycle selection and activation after CLI parsing.
type Initializer struct {
	stdio processcontract.StdioApplicationOpener
}

func NewInitializer(stdio processcontract.StdioApplicationOpener) (*Initializer, error) {
	if stdio == nil {
		return nil, fmt.Errorf("stdio application opener is required")
	}
	return &Initializer{stdio: stdio}, nil
}

func (i *Initializer) Run(
	ctx context.Context,
	intent processcontract.RunIntent,
	selection processcontract.RunSelection,
) error {
	if selection == nil {
		return fmt.Errorf("initialize run service: run selection is required")
	}
	operation, err := selection.Open(ctx, intent)
	if err != nil {
		return fmt.Errorf("initialize run service: %w", err)
	}
	return operation.Run(ctx)
}

func (i *Initializer) Stdio(ctx context.Context, intent processcontract.MCPIntent) error {
	if i == nil || i.stdio == nil {
		return fmt.Errorf("initialize stdio service: stdio application opener is required")
	}
	application, err := i.stdio.OpenStdio(ctx, intent)
	if err != nil {
		return fmt.Errorf("initialize stdio service: %w", err)
	}
	if application == nil {
		return fmt.Errorf("initialize stdio service: stdio application is required")
	}
	return application.Run(ctx)
}
