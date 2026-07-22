package commandregistry

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ModelsHandler is the Models transport-owned Cobra command surface.
type ModelsHandler interface {
	List(*cobra.Command, []string) error
	Inspect(*cobra.Command, []string) error
	Invoke(*cobra.Command, []string) error
	Pull(*cobra.Command, []string) error
}

// NewModelsRegistry attaches the injected Models-owned handler by manifest ID.
func NewModelsRegistry(handler ModelsHandler) (*Registry, error) {
	if handler == nil {
		return nil, fmt.Errorf("build models handler registry: handler is required")
	}
	required := []struct {
		id  string
		run RunE
	}{
		{"you.models.list", handler.List},
		{"you.models.inspect", handler.Inspect},
		{"you.models.invoke", handler.Invoke},
		{"you.models.pull", handler.Pull},
	}
	registry := NewRegistry()
	for _, entry := range required {
		if err := registry.Register(entry.id, entry.run); err != nil {
			return nil, fmt.Errorf("build models handler registry: %w", err)
		}
	}
	return registry, nil
}
