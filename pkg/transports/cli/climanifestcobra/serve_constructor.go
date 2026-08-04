package climanifestcobra

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// NewServeCommand builds the independently injected `you serve` family
// through the accepted generic manifest constructor, mirroring NewMCPCommand.
func NewServeCommand(handler ResolvedCobraHandler) (*cobra.Command, error) {
	manifest, err := generated.ServeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewServeCommandFromManifest(manifest, handler)
}

// NewServeCommandFromManifest projects and detaches the complete serve family.
func NewServeCommandFromManifest(
	manifest climanifest.Manifest,
	handler ResolvedCobraHandler,
) (*cobra.Command, error) {
	if handler == nil {
		return nil, fmt.Errorf("build serve command: handler is required")
	}
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	parentRecord, err := manifest.CommandByID("you.serve")
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	if parentRecord.Runnable {
		return nil, fmt.Errorf("build serve command: %q must remain non-runnable", parentRecord.ID)
	}
	acpRecord, err := manifest.CommandByID("you.serve.acp")
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	if err := climanifestgen.AssertServeFamilyCommandID(parentRecord.ID); err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	if err := climanifestgen.AssertServeFamilyCommandID(acpRecord.ID); err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	manifest.Commands = map[string]climanifest.Command{
		rootRecord.ID:   rootRecord,
		parentRecord.ID: parentRecord,
		acpRecord.ID:    acpRecord,
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		ResolvedCobraHandlers: ResolvedCobraHandlerRegistry{
			acpRecord.Handler.ID: handler,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build serve command: %w", err)
	}
	parent, _, err := root.Find([]string{parentRecord.Name})
	if err != nil {
		return nil, fmt.Errorf("build serve command: find projected command: %w", err)
	}
	root.RemoveCommand(parent)
	return parent, nil
}
