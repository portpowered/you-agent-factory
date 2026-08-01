package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// FamilyHandler is the raw CLI forwarding contract. The top-level CLI keeps
// the original Cobra command and argument slice intact; an owner adapter owns
// decoding, validation, presentation, and root invocation.
type FamilyHandler func(*cobra.Command, []string) error

// CommandFamily is one owner-published command family in the process-scoped
// CLI registry.
type CommandFamily struct {
	Name    string
	Handler FamilyHandler
}

// FamilyRegistry is an immutable command-family registry composed once by
// Wire. It has no product roots or input-resolution policy.
type FamilyRegistry struct {
	families []CommandFamily
	byName   map[string]FamilyHandler
}

// NewFamilyRegistry validates and snapshots the owner-published families.
func NewFamilyRegistry(families []CommandFamily) (*FamilyRegistry, error) {
	if len(families) == 0 {
		return nil, fmt.Errorf("CLI family registry requires at least one family")
	}
	registry := &FamilyRegistry{
		families: make([]CommandFamily, 0, len(families)),
		byName:   make(map[string]FamilyHandler, len(families)),
	}
	for index, family := range families {
		name := strings.TrimSpace(family.Name)
		if name == "" {
			return nil, fmt.Errorf("CLI family registry family %d has no name", index)
		}
		if family.Handler == nil {
			return nil, fmt.Errorf("CLI family registry family %q has no handler", name)
		}
		if _, exists := registry.byName[name]; exists {
			return nil, fmt.Errorf("CLI family registry contains duplicate family %q", name)
		}
		registry.families = append(registry.families, CommandFamily{Name: name, Handler: family.Handler})
		registry.byName[name] = family.Handler
	}
	return registry, nil
}

// Families returns a detached catalog in composition order.
func (registry *FamilyRegistry) Families() []CommandFamily {
	if registry == nil {
		return nil
	}
	return append([]CommandFamily(nil), registry.families...)
}

// Lookup returns the owner handler registered for name.
func (registry *FamilyRegistry) Lookup(name string) (FamilyHandler, bool) {
	if registry == nil {
		return nil, false
	}
	handler, ok := registry.byName[strings.TrimSpace(name)]
	return handler, ok
}

// Dispatch forwards the original command and arguments to one registered
// owner handler. No flag parsing or argument copying occurs here.
func (registry *FamilyRegistry) Dispatch(name string, command *cobra.Command, arguments []string) error {
	handler, ok := registry.Lookup(name)
	if !ok {
		return fmt.Errorf("CLI family %q is not registered", strings.TrimSpace(name))
	}
	if command == nil {
		return fmt.Errorf("CLI family %q dispatch requires a command", strings.TrimSpace(name))
	}
	return handler(command, arguments)
}
