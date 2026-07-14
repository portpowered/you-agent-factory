package commandregistry

import (
	"fmt"
	"io"

	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"github.com/spf13/cobra"
)

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
	query := binding.Query
	if query == nil {
		query = factorycli.Query
	}
	return func(cmd *cobra.Command, _ []string) error {
		cfg := factorycli.QueryConfig{}
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
		return query(cfg)
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
	list := binding.List
	if list == nil {
		list = factorycli.List
	}
	return func(cmd *cobra.Command, _ []string) error {
		cfg := factorycli.ListConfig{}
		if binding.Dir != nil {
			cfg.Dir = *binding.Dir
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return list(cfg)
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
	create := binding.Create
	if create == nil {
		create = factorycli.CreateFromFile
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := factorycli.CreateFromFileConfig{}
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
		return create(cfg)
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
	update := binding.Update
	if update == nil {
		update = factorycli.UpdateFromFile
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := factorycli.UpdateFromFileConfig{}
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
		return update(cfg)
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
	deleteFactory := binding.Delete
	if deleteFactory == nil {
		deleteFactory = factorycli.Delete
	}
	return func(cmd *cobra.Command, args []string) error {
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
		return deleteFactory(cfg)
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
	replaceCurrent := binding.ReplaceCurrent
	if replaceCurrent == nil {
		replaceCurrent = factorycli.ReplaceCurrent
	}
	return func(cmd *cobra.Command, _ []string) error {
		cfg := factorycli.ReplaceCurrentConfig{}
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
		return replaceCurrent(cfg)
	}
}

// FactoryConfigValidateBinding supplies handwritten factory config validate dependencies.
type FactoryConfigValidateBinding struct {
	JSON     *bool
	Validate func(factorycli.ValidateConfig) error
}

// FactoryConfigValidateRunE returns the handwritten factory config validate RunE.
func FactoryConfigValidateRunE(binding FactoryConfigValidateBinding) RunE {
	validate := binding.Validate
	if validate == nil {
		validate = factorycli.Validate
	}
	return func(cmd *cobra.Command, args []string) error {
		cfg := factorycli.ValidateConfig{}
		if len(args) == 1 {
			cfg.Path = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		return validate(cfg)
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
	flatten := binding.Flatten
	if flatten == nil {
		flatten = configcli.FlattenFactoryConfig
	}
	return func(cmd *cobra.Command, args []string) error {
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
		return flatten(cfg)
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
	expand := binding.Expand
	if expand == nil {
		expand = configcli.ExpandFactoryConfig
	}
	return func(cmd *cobra.Command, args []string) error {
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
		return expand(cfg)
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
	if init == nil {
		init = configinitcmd.Init
	}
	return func(cmd *cobra.Command, _ []string) error {
		cfg := configinitcmd.InitConfig{}
		if binding.HomeDir != nil {
			homeDir, err := binding.HomeDir()
			if err != nil {
				return fmt.Errorf("resolve config init home directory: %w", err)
			}
			cfg.HomeDir = homeDir
		}
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
	Init              func(initcmd.InitConfig) error
}

// InitRunE returns the handwritten you init RunE used by production wiring.
func InitRunE(binding InitBinding) RunE {
	init := binding.Init
	if init == nil {
		init = initcmd.Init
	}
	return func(cmd *cobra.Command, _ []string) error {
		cfg := initcmd.InitConfig{}
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
		return init(cfg)
	}
}
