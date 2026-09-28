package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	runtimeapplication "github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// NewLifecycleRunnerBuilder activates a plan that its product owner has
// already composed. It closes owned resources if runner construction fails.
func NewLifecycleRunnerBuilder(
	managedRunners runtimeapplication.ManagedRunnerFactory,
) (initializer.LifecycleRunnerBuilder, error) {
	if managedRunners == nil {
		return nil, errors.New("application lifecycle operation is required")
	}
	return func(
		ctx context.Context,
		plan lifecycle.Plan,
		diagnostics runtimeartifact.Diagnostics,
		ready <-chan initializer.RuntimeHostBinding,
	) (initializer.LocalRuntimeRunner, error) {
		if ctx == nil {
			return nil, lifecycle.CloseResources(plan.Resources, errors.New("build application: context is required"))
		}
		if err := ctx.Err(); err != nil {
			return nil, lifecycle.CloseResources(plan.Resources, fmt.Errorf("build application: %w", err))
		}
		runner, err := managedRunners(plan, diagnostics)
		if err != nil {
			return nil, lifecycle.CloseResources(plan.Resources, fmt.Errorf("build application: %w", err))
		}
		if runner == nil {
			return nil, lifecycle.CloseResources(plan.Resources, errors.New("build application: managed application factory returned nil"))
		}
		runner.SetRuntimeHostReady(ready)
		return runner, nil
	}, nil
}
