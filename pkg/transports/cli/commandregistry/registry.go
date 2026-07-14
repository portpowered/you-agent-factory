package commandregistry

import (
	"fmt"

	"github.com/spf13/cobra"
)

// RunE is a handwritten Cobra handler bound to one stable command ID.
type RunE func(cmd *cobra.Command, args []string) error

// Registry maps stable command IDs to handwritten RunE handlers.
type Registry struct {
	handlers map[string]RunE
}

// NewRegistry constructs an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]RunE)}
}

// Register binds one stable command ID to a handwritten handler.
// Duplicate registration fails observably.
func (r *Registry) Register(commandID string, handler RunE) error {
	if r == nil {
		return fmt.Errorf("register %q: registry is nil", commandID)
	}
	if commandID == "" {
		return fmt.Errorf("register handler: command ID is required")
	}
	if handler == nil {
		return fmt.Errorf("register %q: handler is required", commandID)
	}
	if _, exists := r.handlers[commandID]; exists {
		return fmt.Errorf("register %q: duplicate handler registration", commandID)
	}
	r.handlers[commandID] = handler
	return nil
}

// Lookup returns the handwritten handler for one stable command ID.
func (r *Registry) Lookup(commandID string) (RunE, error) {
	if r == nil {
		return nil, fmt.Errorf("lookup %q: registry is nil", commandID)
	}
	handler, ok := r.handlers[commandID]
	if !ok {
		return nil, fmt.Errorf("lookup %q: handler not registered", commandID)
	}
	return handler, nil
}

// AttachRunE sets cmd.RunE from the registry entry for commandID.
func (r *Registry) AttachRunE(cmd *cobra.Command, commandID string) error {
	if cmd == nil {
		return fmt.Errorf("attach %q: command is nil", commandID)
	}
	handler, err := r.Lookup(commandID)
	if err != nil {
		return err
	}
	cmd.RunE = handler
	return nil
}
