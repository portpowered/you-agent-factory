package commandregistry

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// RunE is a handwritten Cobra handler bound to one stable command ID.
type RunE func(cmd *cobra.Command, args []string) error

// ResolvedRunE is a handwritten Cobra handler that consumes invocation-local
// inputs addressed only by stable manifest input IDs.
type ResolvedRunE func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error

// CommandHandlers is the handwritten lifecycle attached to one runnable Cobra command.
type CommandHandlers struct {
	PreRunE      RunE
	RunE         RunE
	ResolvedRunE ResolvedRunE
}

// Registry maps stable command IDs to handwritten command handlers.
type Registry struct {
	handlers map[string]CommandHandlers
}

// NewRegistry constructs an empty handler registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]CommandHandlers)}
}

// Register binds one stable command ID to a handwritten handler.
// Duplicate registration fails observably.
func (r *Registry) Register(commandID string, handler RunE) error {
	return r.RegisterHandlers(commandID, CommandHandlers{RunE: handler})
}

// RegisterResolved binds one stable handler ID to a resolved-input handler.
func (r *Registry) RegisterResolved(handlerID string, handler ResolvedRunE) error {
	return r.RegisterHandlers(handlerID, CommandHandlers{ResolvedRunE: handler})
}

// RegisterHandlers binds one stable command ID to its handwritten lifecycle.
// Duplicate registration fails observably.
func (r *Registry) RegisterHandlers(commandID string, handlers CommandHandlers) error {
	if r == nil {
		return fmt.Errorf("register %q: registry is nil", commandID)
	}
	if commandID == "" {
		return fmt.Errorf("register handler: command ID is required")
	}
	if (handlers.RunE == nil) == (handlers.ResolvedRunE == nil) {
		return fmt.Errorf("register %q: handler is required", commandID)
	}
	if _, exists := r.handlers[commandID]; exists {
		return fmt.Errorf("register %q: duplicate handler registration", commandID)
	}
	r.handlers[commandID] = handlers
	return nil
}

// Lookup returns the handwritten handler for one stable command ID.
func (r *Registry) Lookup(commandID string) (RunE, error) {
	if r == nil {
		return nil, fmt.Errorf("lookup %q: registry is nil", commandID)
	}
	handlers, ok := r.handlers[commandID]
	if !ok {
		return nil, fmt.Errorf("lookup %q: handler not registered", commandID)
	}
	return handlers.RunE, nil
}

// LookupHandlers returns the handwritten lifecycle for one stable command ID.
func (r *Registry) LookupHandlers(commandID string) (CommandHandlers, error) {
	if r == nil {
		return CommandHandlers{}, fmt.Errorf("lookup %q: registry is nil", commandID)
	}
	handlers, ok := r.handlers[commandID]
	if !ok {
		return CommandHandlers{}, fmt.Errorf("lookup %q: handler not registered", commandID)
	}
	return handlers, nil
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

// AttachHandlers sets cmd.PreRunE and cmd.RunE from the registry entry.
func (r *Registry) AttachHandlers(cmd *cobra.Command, commandID string) error {
	if cmd == nil {
		return fmt.Errorf("attach %q: command is nil", commandID)
	}
	handlers, err := r.LookupHandlers(commandID)
	if err != nil {
		return err
	}
	cmd.PreRunE = handlers.PreRunE
	cmd.RunE = handlers.RunE
	return nil
}
