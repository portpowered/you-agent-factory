package climanifestcobra

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const workerSessionsListHandlerID = "you.worker-sessions.list.handler"

// NewWorkerSessionsFamilyCommand builds the detached `you worker-sessions`
// observation family from generated metadata and a stable handler registry.
func NewWorkerSessionsFamilyCommand(registry *commandregistry.Registry) (*cobra.Command, error) {
	manifest, err := generated.WorkerSessionsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	rootRecord, err := rootManifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	manifest.Commands[rootRecord.ID] = rootRecord
	return NewWorkerSessionsFamilyCommandFromManifest(manifest, registry)
}

// NewWorkerSessionsFamilyCommandFromManifest projects one worker-session
// family snapshot. The supplied manifest is validated against the stable
// family IDs before any Cobra command is returned.
func NewWorkerSessionsFamilyCommandFromManifest(
	manifest climanifest.Manifest,
	registry *commandregistry.Registry,
) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build worker sessions family command: registry is required")
	}
	if err := validateWorkerSessionsManifest(manifest); err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	registered, err := registry.LookupHandlers(workerSessionsListHandlerID)
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	if registered.RunE == nil || registered.ResolvedRunE != nil {
		return nil, fmt.Errorf("build worker sessions family command: handler %q must provide RunE", workerSessionsListHandlerID)
	}
	workerSessionsHandler := registered
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	root, err := NewCommandTree(manifest, GenericBindings{
		Handlers: HandlerRegistry{
			rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil },
		},
		CobraHandlers: CobraHandlerRegistry{
			workerSessionsListHandlerID: func(cmd *cobra.Command, args []string, _ map[string]any, _ resolvedinput.Inputs) error {
				if workerSessionsHandler.PreRunE != nil {
					if err := workerSessionsHandler.PreRunE(cmd, args); err != nil {
						return err
					}
				}
				return workerSessionsHandler.RunE(cmd, args)
			},
		},
		GuardUnknownSubcommands: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: %w", err)
	}
	workerSessions, _, err := root.Find([]string{"worker-sessions"})
	if err != nil {
		return nil, fmt.Errorf("build worker sessions family command: find worker-sessions: %w", err)
	}
	if workerSessions == nil {
		return nil, fmt.Errorf("build worker sessions family command: worker-sessions command is unavailable")
	}
	root.RemoveCommand(workerSessions)
	workerSessions.SilenceUsage = true
	return workerSessions, nil
}

func validateWorkerSessionsManifest(manifest climanifest.Manifest) error {
	if manifest.RootPath != "you" {
		return fmt.Errorf("manifest root path = %q, want %q", manifest.RootPath, "you")
	}
	if len(manifest.Commands) != len(climanifestgen.WorkerSessionsFamilyCommandIDs)+1 {
		return fmt.Errorf("manifest command count = %d, want %d", len(manifest.Commands), len(climanifestgen.WorkerSessionsFamilyCommandIDs)+1)
	}
	for commandID, record := range manifest.Commands {
		if commandID != "you" {
			if err := climanifestgen.AssertWorkerSessionsFamilyCommandID(commandID); err != nil {
				return err
			}
		}
		if record.ID != commandID {
			return fmt.Errorf("manifest command key %q has record ID %q", commandID, record.ID)
		}
	}
	parent, err := manifest.CommandByID("you.worker-sessions")
	if err != nil {
		return err
	}
	if parent.Runnable {
		return fmt.Errorf("command %q must remain non-runnable", parent.ID)
	}
	list, err := manifest.CommandByID("you.worker-sessions.list")
	if err != nil {
		return err
	}
	if !list.Runnable || list.Handler == nil || list.Handler.ID != workerSessionsListHandlerID {
		return fmt.Errorf("command %q must declare runnable handler %q", list.ID, workerSessionsListHandlerID)
	}
	if root, err := manifest.CommandByID("you"); err != nil {
		return err
	} else if root.Handler == nil || root.Handler.ID == "" {
		return fmt.Errorf("root command %q must declare a handler", root.ID)
	}
	return nil
}
