package commandregistry

import (
	"fmt"
	"io"
	"sort"

	initcmd "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// RunnableFactoryConfigInitCommandIDs returns contracted runnable command IDs for
// the factory/config/init family in stable sorted order.
func RunnableFactoryConfigInitCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	for _, commandID := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID(commandID); err != nil {
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

// VerifyFactoryConfigInitRunnableCoverage fails when any contracted runnable
// factory/config/init command ID lacks a registered handwritten handler.
func (r *Registry) VerifyFactoryConfigInitRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableFactoryConfigInitCommandIDs(manifest)
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
			"factory/config/init runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}

// FactoryConfigInitHandlers carries handwritten RunE handlers for contracted runnable
// factory/config/init command IDs.
type FactoryConfigInitHandlers struct {
	FactoryQueryRunE          RunE
	FactoryListRunE           RunE
	FactoryCreateRunE         RunE
	FactoryUpdateRunE         RunE
	FactoryDeleteRunE         RunE
	FactoryReplaceCurrentRunE RunE
	FactoryConfigValidateRunE RunE
	FactoryConfigFlattenRunE  RunE
	FactoryConfigExpandRunE   RunE
	ConfigInitRunE            RunE
	InitRunE                  RunE
}

// NewFactoryConfigInitRegistry registers handwritten handlers for the
// factory/config/init family and verifies contracted runnable command coverage.
func NewFactoryConfigInitRegistry(handlers FactoryConfigInitHandlers) (*Registry, error) {
	bindings := []struct {
		commandID string
		handler   RunE
	}{
		{commandID: "you.factory.query", handler: handlers.FactoryQueryRunE},
		{commandID: "you.factory.list", handler: handlers.FactoryListRunE},
		{commandID: "you.factory.create", handler: handlers.FactoryCreateRunE},
		{commandID: "you.factory.update", handler: handlers.FactoryUpdateRunE},
		{commandID: "you.factory.delete", handler: handlers.FactoryDeleteRunE},
		{commandID: "you.factory.replace-current", handler: handlers.FactoryReplaceCurrentRunE},
		{commandID: "you.factory.config.validate", handler: handlers.FactoryConfigValidateRunE},
		{commandID: "you.factory.config.flatten", handler: handlers.FactoryConfigFlattenRunE},
		{commandID: "you.factory.config.expand", handler: handlers.FactoryConfigExpandRunE},
		{commandID: "you.config.init", handler: handlers.ConfigInitRunE},
		{commandID: "you.init", handler: handlers.InitRunE},
	}

	for _, binding := range bindings {
		if binding.handler == nil {
			return nil, fmt.Errorf(
				"build factory/config/init handler registry: %s handler is required",
				binding.commandID,
			)
		}
	}

	registry := NewRegistry()
	for _, binding := range bindings {
		if err := registry.Register(binding.commandID, binding.handler); err != nil {
			return nil, fmt.Errorf("build factory/config/init handler registry: %w", err)
		}
	}

	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build factory/config/init handler registry: %w", err)
	}
	if err := registry.VerifyFactoryConfigInitRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build factory/config/init handler registry: %w", err)
	}
	return registry, nil
}

// FactoryQueryBinding supplies handwritten factory query execution dependencies.
type FactoryQueryBinding struct {
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Query             func(factorycli.QueryConfig) error
}

// FactoryQueryRunE returns the handwritten factory query RunE used by production wiring.
func FactoryQueryRunE(binding FactoryQueryBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.Query == nil {
			return fmt.Errorf("factory query service is required")
		}
		cfg := factorycli.QueryConfig{}
		cfg.Context = cmd.Context()
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
		return binding.Query(cfg)
	}
}

// FactoryListBinding supplies handwritten factory list execution dependencies.
type FactoryListBinding struct {
	Dir  *string
	JSON *bool
	List func(factorycli.ListConfig) error
}

// FactoryListRunE returns the handwritten factory list RunE used by production wiring.
func FactoryListRunE(binding FactoryListBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.List == nil {
			return fmt.Errorf("factory list service is required")
		}
		cfg := factorycli.ListConfig{}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return binding.List(cfg)
	}
}

// FactoryCreateBinding supplies handwritten factory create execution dependencies.
type FactoryCreateBinding struct {
	Dir        *string
	From       *string
	SetCurrent *bool
	JSON       *bool
	Create     func(factorycli.CreateFromFileConfig) error
}

// FactoryCreateRunE returns the handwritten factory create RunE used by production wiring.
func FactoryCreateRunE(binding FactoryCreateBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Create == nil {
			return fmt.Errorf("factory create service is required")
		}
		cfg := factorycli.CreateFromFileConfig{}
		cfg.Context = cmd.Context()
		if len(args) == 1 {
			cfg.Name = args[0]
		}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.From != nil {
			cfg.From = *binding.From
		}
		if binding.SetCurrent != nil {
			cfg.SetCurrent = *binding.SetCurrent
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return binding.Create(cfg)
	}
}

// FactoryUpdateBinding supplies handwritten factory update execution dependencies.
type FactoryUpdateBinding struct {
	Dir    *string
	From   *string
	JSON   *bool
	Update func(factorycli.UpdateFromFileConfig) error
}

// FactoryUpdateRunE returns the handwritten factory update RunE used by production wiring.
func FactoryUpdateRunE(binding FactoryUpdateBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Update == nil {
			return fmt.Errorf("factory update service is required")
		}
		cfg := factorycli.UpdateFromFileConfig{}
		cfg.Context = cmd.Context()
		if len(args) == 1 {
			cfg.Name = args[0]
		}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.From != nil {
			cfg.From = *binding.From
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return binding.Update(cfg)
	}
}

// FactoryDeleteBinding supplies handwritten factory delete execution dependencies.
type FactoryDeleteBinding struct {
	Dir    *string
	JSON   *bool
	Delete func(factorycli.DeleteConfig) error
}

// FactoryDeleteRunE returns the handwritten factory delete RunE used by production wiring.
func FactoryDeleteRunE(binding FactoryDeleteBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Delete == nil {
			return fmt.Errorf("factory delete service is required")
		}
		cfg := factorycli.DeleteConfig{}
		if len(args) == 1 {
			cfg.Name = args[0]
		}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return binding.Delete(cfg)
	}
}

// FactoryReplaceCurrentBinding supplies handwritten replace-current execution dependencies.
type FactoryReplaceCurrentBinding struct {
	Server            *string
	SessionID         *string
	JSON              *bool
	Verbose           func() bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ReplaceCurrent    func(factorycli.ReplaceCurrentConfig) error
}

// FactoryReplaceCurrentRunE returns the handwritten replace-current RunE used by production wiring.
func FactoryReplaceCurrentRunE(binding FactoryReplaceCurrentBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.ReplaceCurrent == nil {
			return fmt.Errorf("factory replace-current service is required")
		}
		cfg := factorycli.ReplaceCurrentConfig{}
		cfg.Context = cmd.Context()
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if binding.SessionID != nil {
			cfg.SessionID = *binding.SessionID
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
		return binding.ReplaceCurrent(cfg)
	}
}

// FactoryConfigValidateBinding supplies handwritten factory config validate dependencies.
type FactoryConfigValidateBinding struct {
	JSON     *bool
	Validate func(factorycli.ValidateConfig) error
}

// FactoryConfigValidateRunE returns the handwritten factory config validate RunE.
func FactoryConfigValidateRunE(binding FactoryConfigValidateBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Validate == nil {
			return fmt.Errorf("factory validate service is required")
		}
		cfg := factorycli.ValidateConfig{}
		cfg.Context = cmd.Context()
		if len(args) == 1 {
			cfg.Path = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return binding.Validate(cfg)
	}
}

// FactoryConfigFlattenBinding supplies handwritten factory config flatten dependencies.
type FactoryConfigFlattenBinding struct {
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Flatten           func(configcli.FactoryConfigFlattenConfig) error
}

// FactoryConfigFlattenRunE returns the handwritten factory config flatten RunE.
func FactoryConfigFlattenRunE(binding FactoryConfigFlattenBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Flatten == nil {
			return fmt.Errorf("factory flatten service is required")
		}
		cfg := configcli.FactoryConfigFlattenConfig{}
		if len(args) == 1 {
			cfg.Path = args[0]
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
		return binding.Flatten(cfg)
	}
}

// FactoryConfigExpandBinding supplies handwritten factory config expand dependencies.
type FactoryConfigExpandBinding struct {
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Expand            func(configcli.FactoryConfigExpandConfig) error
}

// FactoryConfigExpandRunE returns the handwritten factory config expand RunE.
func FactoryConfigExpandRunE(binding FactoryConfigExpandBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Expand == nil {
			return fmt.Errorf("factory expand service is required")
		}
		cfg := configcli.FactoryConfigExpandConfig{}
		if len(args) == 1 {
			cfg.Path = args[0]
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
		return binding.Expand(cfg)
	}
}

// ConfigInitBinding supplies handwritten you config init execution dependencies.
type ConfigInitBinding struct {
	HomeDir           func() (string, error)
	JSON              func() bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Verbose           func() bool
	Init              func(configinitcmd.InitConfig) error
}

// ConfigInitRunE returns the handwritten you config init RunE used by production wiring.
func ConfigInitRunE(binding ConfigInitBinding) RunE {
	init := binding.Init
	return func(cmd *cobra.Command, _ []string) error {
		if init == nil {
			return fmt.Errorf("system initialization service is required")
		}
		if binding.HomeDir == nil {
			return fmt.Errorf("config init home directory resolver is required")
		}
		cfg := configinitcmd.InitConfig{Context: cmd.Context()}
		homeDir, err := binding.HomeDir()
		if err != nil {
			return fmt.Errorf("resolve config init home directory: %w", err)
		}
		cfg.HomeDir = homeDir
		if binding.JSON != nil {
			cfg.JSON = binding.JSON()
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		return init(cfg)
	}
}

// InitBinding supplies handwritten you init execution dependencies.
type InitBinding struct {
	Dir               *string
	Type              *string
	Executor          *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	Init              func(initcmd.ScaffoldConfig) error
}

// InitRunE returns the handwritten you init RunE used by production wiring.
func InitRunE(binding InitBinding) RunE {
	return func(cmd *cobra.Command, _ []string) error {
		if binding.Init == nil {
			return fmt.Errorf("init service is required")
		}
		cfg := initcmd.ScaffoldConfig{}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.Type != nil {
			cfg.Type = *binding.Type
		}
		if binding.Executor != nil {
			cfg.Executor = *binding.Executor
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
		return binding.Init(cfg)
	}
}
