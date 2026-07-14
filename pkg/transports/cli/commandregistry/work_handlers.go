package commandregistry

import (
	"fmt"
	"io"
	"sort"

	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// RunnableWorkCommandIDs returns contracted runnable command IDs for the work
// family in stable sorted order.
func RunnableWorkCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.WorkFamilyCommandIDs))
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
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

// VerifyWorkRunnableCoverage fails when any contracted runnable work-family
// command ID lacks a registered handwritten handler.
func (r *Registry) VerifyWorkRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableWorkCommandIDs(manifest)
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
		return fmt.Errorf(
			"work runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}

// WorkHandlers carries handwritten RunE handlers for contracted runnable
// work-family command IDs.
type WorkHandlers struct {
	ListRunE      RunE
	ShowRunE      RunE
	MoveRunE      RunE
	VisualizeRunE RunE
}

// NewWorkRegistry registers handwritten handlers for the work family and
// verifies contracted runnable command coverage.
func NewWorkRegistry(handlers WorkHandlers) (*Registry, error) {
	if handlers.ListRunE == nil {
		return nil, fmt.Errorf("build work handler registry: list handler is required")
	}
	if handlers.ShowRunE == nil {
		return nil, fmt.Errorf("build work handler registry: show handler is required")
	}
	if handlers.MoveRunE == nil {
		return nil, fmt.Errorf("build work handler registry: move handler is required")
	}
	if handlers.VisualizeRunE == nil {
		return nil, fmt.Errorf("build work handler registry: visualize handler is required")
	}

	registry := NewRegistry()
	registrations := []struct {
		commandID string
		handler   RunE
	}{
		{commandID: "you.work.list", handler: handlers.ListRunE},
		{commandID: "you.work.show", handler: handlers.ShowRunE},
		{commandID: "you.work.move", handler: handlers.MoveRunE},
		{commandID: "you.work.visualize", handler: handlers.VisualizeRunE},
	}
	for _, registration := range registrations {
		if err := registry.Register(registration.commandID, registration.handler); err != nil {
			return nil, fmt.Errorf("build work handler registry: %w", err)
		}
	}

	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	return registry, nil
}

// ListBinding supplies handwritten work list execution dependencies.
type ListBinding struct {
	Config            *workcli.ListConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ListWork          func(workcli.ListConfig) error
}

// ListRunE returns the handwritten work list RunE used by production wiring.
func ListRunE(binding ListBinding) RunE {
	listWork := binding.ListWork
	if listWork == nil {
		listWork = workcli.List
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := *binding.Config
		if binding.Server != nil {
			cfg.Server = *binding.Server
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
		return listWork(cfg)
	}
}

// ShowBinding supplies handwritten work show execution dependencies.
type ShowBinding struct {
	Config            *workcli.ShowConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ShowWork          func(workcli.ShowConfig) error
}

// ShowRunE returns the handwritten work show RunE used by production wiring.
func ShowRunE(binding ShowBinding) RunE {
	showWork := binding.ShowWork
	if showWork == nil {
		showWork = workcli.Show
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := *binding.Config
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.WorkID = args[0]
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
		return showWork(cfg)
	}
}

// MoveBinding supplies handwritten work move execution dependencies.
type MoveBinding struct {
	Config            *workcli.MoveConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	MoveWork          func(workcli.MoveConfig) error
}

// MoveRunE returns the handwritten work move RunE used by production wiring.
func MoveRunE(binding MoveBinding) RunE {
	moveWork := binding.MoveWork
	if moveWork == nil {
		moveWork = workcli.Move
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := *binding.Config
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) >= 1 {
			cfg.WorkID = args[0]
		}
		if len(args) >= 2 {
			cfg.StateName = args[1]
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
		return moveWork(cfg)
	}
}

// VisualizeBinding supplies handwritten work visualize execution dependencies.
type VisualizeBinding struct {
	Format     *string
	Visualize  func(workcli.VisualizeConfig) error
}

// VisualizeRunE returns the handwritten work visualize RunE used by production wiring.
func VisualizeRunE(binding VisualizeBinding) RunE {
	visualizeWork := binding.Visualize
	if visualizeWork == nil {
		visualizeWork = workcli.Visualize
	}
	return func(cmd *cobra.Command, args []string) error {
		format := ""
		if binding.Format != nil {
			format = *binding.Format
		}
		return visualizeWork(workcli.VisualizeConfig{
			BatchFile: args[0],
			Format:    format,
			Output:    cmd.OutOrStdout(),
		})
	}
}
