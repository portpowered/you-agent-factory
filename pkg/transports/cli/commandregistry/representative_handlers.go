package commandregistry

import (
	"fmt"
	"io"

	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
)

// RepresentativeHandlers carries handwritten RunE handlers for contracted runnable
// representative-family command IDs.
type RepresentativeHandlers struct {
	RootRunE        RunE
	SessionShowRunE RunE
}

// NewRepresentativeRegistry registers handwritten handlers for the representative
// family and verifies contracted runnable command coverage.
func NewRepresentativeRegistry(handlers RepresentativeHandlers) (*Registry, error) {
	if handlers.RootRunE == nil {
		return nil, fmt.Errorf("build representative handler registry: root handler is required")
	}
	if handlers.SessionShowRunE == nil {
		return nil, fmt.Errorf("build representative handler registry: session show handler is required")
	}

	registry := NewRegistry()
	if err := registry.Register("you", handlers.RootRunE); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	if err := registry.Register("you.session.show", handlers.SessionShowRunE); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}

	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	if err := registry.VerifyRepresentativeRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build representative handler registry: %w", err)
	}
	return registry, nil
}

// SessionShowBinding supplies handwritten session show execution dependencies.
type SessionShowBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ShowSession       func(sessioncli.ShowConfig) error
}

// SessionShowRunE returns the handwritten session show RunE used by production wiring.
func SessionShowRunE(binding SessionShowBinding) RunE {
	showSession := binding.ShowSession
	if showSession == nil {
		showSession = sessioncli.Show
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := sessioncli.ShowConfig{}
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.SessionID = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return showSession(cfg)
	}
}
