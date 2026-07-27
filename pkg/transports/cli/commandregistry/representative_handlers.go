package commandregistry

import (
	"fmt"
	"io"
	"sort"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// RunnableRepresentativeCommandIDs returns contracted runnable command IDs for
// the representative family in stable sorted order.
func RunnableRepresentativeCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.RepresentativeFamilyCommandIDs))
	for _, commandID := range climanifestgen.RepresentativeFamilyCommandIDs {
		if err := climanifestgen.AssertRepresentativeFamilyCommandID(commandID); err != nil {
			return nil, err
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyRepresentativeRunnableCoverage fails when any contracted runnable
// representative-family command ID lacks a registered handwritten handler.
func (r *Registry) VerifyRepresentativeRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableRepresentativeCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("representative runnable command handlers missing for: %v", missing)
	}
	return nil
}

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
	return func(cmd *cobra.Command, args []string) error {
		if binding.ShowSession == nil {
			return fmt.Errorf("session show service is required")
		}
		cfg := sessioncli.ShowConfig{}
		cfg.Context = cmd.Context()
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
		return binding.ShowSession(cfg)
	}
}
