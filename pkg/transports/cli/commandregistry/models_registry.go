package commandregistry

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// ModelsHandler is the Models transport-owned Cobra command surface.
type ModelsHandler interface {
	List(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Inspect(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
	Invoke(*cobra.Command, []string) error
	Pull(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error
}

// NewModelsRegistry retains raw Cobra dispatch only for Models commands that
// have not yet migrated to typed resolved inputs.
func NewModelsRegistry(handler ModelsHandler) (*Registry, error) {
	if handler == nil {
		return nil, fmt.Errorf("build models handler registry: handler is required")
	}
	registry := NewRegistry()
	if err := registry.Register("you.models.invoke", handler.Invoke); err != nil {
		return nil, fmt.Errorf("build models handler registry: %w", err)
	}
	return registry, nil
}
