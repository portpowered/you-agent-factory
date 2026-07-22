package climanifestcobra

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// NewDocsCommand builds the independently injected `you docs` command.
func NewDocsCommand(registry *commandregistry.Registry) (*cobra.Command, error) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	return NewDocsCommandFromManifest(manifest, registry)
}

// NewDocsCommandFromManifest builds `you docs` from authored manifest data.
func NewDocsCommandFromManifest(manifest climanifest.Manifest, registry *commandregistry.Registry) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build docs command: registry is required")
	}
	record, err := manifest.CommandByID("you.docs")
	if err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	cmd := commandFromManifest(record, true)
	cmd.SilenceUsage = true
	cmd.Args = positionalArgsFromManifest(record)
	cmd.ValidArgs = docscli.SupportedTopicCommands()
	if err := registry.AttachRunE(cmd, record.ID); err != nil {
		return nil, fmt.Errorf("build docs command: %w", err)
	}
	return cmd, nil
}
