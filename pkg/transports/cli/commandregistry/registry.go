package commandregistry

import (
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// RunE is a handwritten Cobra handler bound to one stable command ID.
type RunE func(cmd *cobra.Command, args []string) error

// CommandHandlers is the handwritten lifecycle attached to one runnable Cobra command.
type CommandHandlers struct {
	PreRunE RunE
	RunE    RunE
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

// RegisterHandlers binds one stable command ID to its handwritten lifecycle.
// Duplicate registration fails observably.
func (r *Registry) RegisterHandlers(commandID string, handlers CommandHandlers) error {
	if r == nil {
		return fmt.Errorf("register %q: registry is nil", commandID)
	}
	if commandID == "" {
		return fmt.Errorf("register handler: command ID is required")
	}
	if handlers.RunE == nil {
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

const (
	SubmitHandlerID      = "you.submit.handler"
	SubmitBatchHandlerID = "you.submit.batch.handler"
)

// SubmitHandler consumes one invocation's local and inherited resolved inputs.
type SubmitHandler func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error

// SubmitHandlers supplies the two executable submit-family bindings.
type SubmitHandlers struct {
	Submit      SubmitHandler
	SubmitBatch SubmitHandler
}

// SubmitRegistry contains exactly the stable handlers accepted by the submit family.
type SubmitRegistry struct {
	handlers map[string]SubmitHandler
}

// NewSubmitRegistry constructs a complete submit-only handler registry.
func NewSubmitRegistry(handlers SubmitHandlers) (*SubmitRegistry, error) {
	if handlers.Submit == nil {
		return nil, fmt.Errorf("build submit handler registry: handler %q is required", SubmitHandlerID)
	}
	if handlers.SubmitBatch == nil {
		return nil, fmt.Errorf("build submit handler registry: handler %q is required", SubmitBatchHandlerID)
	}
	return &SubmitRegistry{handlers: map[string]SubmitHandler{
		SubmitHandlerID:      handlers.Submit,
		SubmitBatchHandlerID: handlers.SubmitBatch,
	}}, nil
}

// Lookup returns the executable binding for one stable submit handler ID.
func (r *SubmitRegistry) Lookup(handlerID string) (SubmitHandler, error) {
	if r == nil {
		return nil, fmt.Errorf("lookup submit handler %q: registry is required", handlerID)
	}
	handler, ok := r.handlers[handlerID]
	if !ok {
		return nil, fmt.Errorf("lookup submit handler %q: handler is not registered", handlerID)
	}
	return handler, nil
}

// Verify rejects missing, extra, duplicate, or unknown submit handler metadata.
func (r *SubmitRegistry) Verify(manifest climanifest.Manifest) error {
	if r == nil {
		return fmt.Errorf("verify submit handler coverage: registry is required")
	}
	expected := map[string]bool{
		SubmitHandlerID:      true,
		SubmitBatchHandlerID: true,
	}
	seen := make(map[string]bool, len(expected))
	var unknown []string
	for _, record := range manifest.Commands {
		if !record.Runnable || record.Handler == nil || record.Handler.ID == "" {
			return fmt.Errorf("verify submit handler coverage: command %q must declare a runnable handler", record.ID)
		}
		handlerID := record.Handler.ID
		if !expected[handlerID] {
			unknown = append(unknown, handlerID)
			continue
		}
		if seen[handlerID] {
			return fmt.Errorf("verify submit handler coverage: handler %q is declared more than once", handlerID)
		}
		if _, err := r.Lookup(handlerID); err != nil {
			return fmt.Errorf("verify submit handler coverage: %w", err)
		}
		seen[handlerID] = true
	}
	var missing []string
	for handlerID := range expected {
		if !seen[handlerID] {
			missing = append(missing, handlerID)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	if len(missing) > 0 || len(unknown) > 0 {
		return fmt.Errorf(
			"verify submit handler coverage: missing=%v unknown=%v",
			missing,
			unknown,
		)
	}
	return nil
}
