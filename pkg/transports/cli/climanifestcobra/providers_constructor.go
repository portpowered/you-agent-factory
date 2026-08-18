package climanifestcobra

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// NewProvidersCommand builds and detaches the public `you providers` family
// from the generated CLI manifest.
func NewProvidersCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.ProvidersFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewProvidersCommandFromManifest(manifest, handler)
}

// NewProvidersCommandFromManifest projects exactly the root, providers group,
// and list leaf from one detached manifest snapshot.
func NewProvidersCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build providers command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.providers")
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build providers command: %q must remain non-runnable", parentRecord.ID)
	}
	listRecord, err := manifest.CommandByID("you.providers.list")
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		parentRecord.ID: parentRecord,
		listRecord.ID:   listRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			listRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build providers command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build providers command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	return parent, nil
}
