package application

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
)

type StdioRunnerBuilder func(
	context.Context,
	initializer.StdioSessionOpener,
	io.Reader,
	io.Writer,
) (initializer.LocalRuntimeRunner, error)

type OpenedStdioRunnerBuilder func(
	context.Context,
	initializer.OpenedStdioApplication,
	io.Reader,
	io.Writer,
) (initializer.LocalRuntimeRunner, error)

// NewRuntimeRunnerBuilder consumes a lifecycle plan supplied by the already
// injected application opening edge. It contains no product component
// selection or ordering policy.
func NewRuntimeRunnerBuilder(
	managedRunners runtimeapplication.ManagedRunnerFactory,
) (initializer.RuntimeRunnerBuilder, error) {
	if managedRunners == nil {
		return nil, errors.New("application lifecycle operation is required")
	}
	return func(
		ctx context.Context,
		openApplication initializer.ApplicationOpeningOperation,
	) (initializer.LocalRuntimeRunner, error) {
		if ctx == nil {
			return nil, errors.New("build application: context is required")
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build application: %w", err)
		}
		if openApplication == nil {
			return nil, errors.New("build application: opening operation is required")
		}
		opened, err := openApplication(ctx)
		if err != nil {
			return nil, err
		}
		return buildOpenedRunner(opened, managedRunners)
	}, nil
}

func NewStdioRunnerBuilder(
	managedRunners runtimeapplication.ManagedRunnerFactory,
) (StdioRunnerBuilder, error) {
	if managedRunners == nil {
		return nil, errors.New("stdio lifecycle operation is required")
	}
	return func(
		ctx context.Context,
		openSession initializer.StdioSessionOpener,
		input io.Reader,
		output io.Writer,
	) (initializer.LocalRuntimeRunner, error) {
		if err := validateStdioInputs(ctx, openSession, input, output); err != nil {
			return nil, fmt.Errorf("build stdio application: %w", err)
		}
		opened, err := openSession(ctx, input, output)
		if err != nil {
			return nil, fmt.Errorf("open stdio application: %w", err)
		}
		return buildOpenedRunner(opened, managedRunners)
	}, nil
}

func NewOpenedStdioRunnerBuilder(
	managedRunners runtimeapplication.ManagedRunnerFactory,
) (OpenedStdioRunnerBuilder, error) {
	if managedRunners == nil {
		return nil, errors.New("opened stdio lifecycle operation is required")
	}
	return func(
		ctx context.Context,
		opened initializer.OpenedStdioApplication,
		input io.Reader,
		output io.Writer,
	) (initializer.LocalRuntimeRunner, error) {
		if err := validateStdioInputs(ctx, opened.OpenSession, input, output); err != nil {
			return nil, fmt.Errorf("construct runtime-backed stdio: %w", err)
		}
		application, err := opened.OpenSession(ctx, input, output)
		if err != nil {
			return nil, fmt.Errorf("open runtime-backed stdio: %w", err)
		}
		return buildOpenedRunner(application, managedRunners)
	}, nil
}

func buildOpenedRunner(
	opened initializer.OpenedApplication,
	managedRunners runtimeapplication.ManagedRunnerFactory,
) (initializer.LocalRuntimeRunner, error) {
	runner, err := managedRunners(opened.Plan, opened.Diagnostics)
	if err != nil {
		return nil, lifecycle.CloseResources(
			opened.Plan.Resources,
			fmt.Errorf("build application: %w", err),
		)
	}
	if runner == nil {
		return nil, lifecycle.CloseResources(
			opened.Plan.Resources,
			errors.New("build application: managed application factory returned nil"),
		)
	}
	return runner, nil
}

func validateStdioInputs(
	ctx context.Context,
	openSession initializer.StdioSessionOpener,
	input io.Reader,
	output io.Writer,
) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case openSession == nil:
		return errors.New("session opening operation is required")
	case input == nil:
		return errors.New("input is required")
	case output == nil:
		return errors.New("output is required")
	default:
		return nil
	}
}
