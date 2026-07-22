package runtimeapplication

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// ManagedRunner executes one caller-supplied lifecycle plan through the
// singular manager.
type ManagedRunner struct {
	manager     *lifecycle.Manager
	plan        lifecycle.Plan
	diagnostics runtimeartifact.Diagnostics
}

// ManagedRunnerFactory is the injected lifecycle activation constructor used
// by application operations after a session-owned runtime has been opened.
type ManagedRunnerFactory func(lifecycle.Plan, runtimeartifact.Diagnostics) (*ManagedRunner, error)

func NewManagedRunner(
	plan lifecycle.Plan,
	diagnostics runtimeartifact.Diagnostics,
) (*ManagedRunner, error) {
	if err := lifecycle.Validate(plan); err != nil {
		return nil, fmt.Errorf("initialize application: %w", err)
	}
	return &ManagedRunner{
		manager:     lifecycle.NewManager(),
		plan:        plan,
		diagnostics: diagnostics,
	}, nil
}

func (r *ManagedRunner) Run(ctx context.Context) error {
	if r == nil || r.manager == nil {
		return fmt.Errorf("managed application is required")
	}
	return r.manager.Run(ctx, r.plan)
}

func (r *ManagedRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	if r == nil {
		return runtimeartifact.Diagnostics{}
	}
	return r.diagnostics
}
