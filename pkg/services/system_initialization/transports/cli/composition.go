package cli

import (
	"context"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// InitializeSystemOperation is the composition-facing system-initialization role
// that Wire adapts from the Bootstrap root before lifecycle commands run.
type InitializeSystemOperation = initializerapplication.SystemInitializationOperation

// InitializeOperation is the composition-facing initialize role with full CLI
// presentation inputs when composition owns writers or diagnostics.
type InitializeOperation func(InitializeConfig) (systeminitialization.Result, error)

// BindInitializeSystem returns the composition-facing operation closure that
// delegates system initialization to the Bootstrap-owned CLI adapter Service.
// Wire and other composition roots inject the returned function without
// constructing the Service at the composition boundary.
func BindInitializeSystem(root systeminitialization.Service) InitializeSystemOperation {
	if root == nil {
		return nil
	}
	return func(ctx context.Context, homeDir string) error {
		return InitializeSystem(ctx, homeDir, root)
	}
}

// BindInitialize returns the composition-facing operation closure that delegates
// Bootstrap initialize presentation to the owned CLI adapter Service.
func BindInitialize(root systeminitialization.Service) InitializeOperation {
	if root == nil {
		return nil
	}
	return func(cfg InitializeConfig) (systeminitialization.Result, error) {
		return Initialize(cfg, root)
	}
}
