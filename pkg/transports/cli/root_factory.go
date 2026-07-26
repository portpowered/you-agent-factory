package cli

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
)

func newRootCommandWithFactory(options CommandFactory) *cobra.Command {
	root := newRootCommandWithGeneratedRepresentativeFamily(options)
	if root == nil {
		return nil
	}
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if requiresSystemInitialization(cmd.CommandPath(), args) {
			if options.initializer == nil {
				return fmt.Errorf("system initializer is required")
			}
			homeDir, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			if err := options.initializer.InitializeSystem(cmd.Context(), homeDir); err != nil {
				return fmt.Errorf("initialize system: %w", err)
			}
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return root
}

func requiresSystemInitialization(commandPath string, args []string) bool {
	switch commandPath {
	case "you":
		return len(args) > 0
	case "you factory list", "you mcp serve", "you run":
		return true
	default:
		return false
	}
}

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
	_ *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) factoryConfigInitProductionCommands {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			QueryFactory:          options.QueryFactory,
			ListFactories:         options.ListFactories,
			CreateFactoryFromFile: options.CreateFactoryFromFile,
			UpdateFactoryFromFile: options.UpdateFactoryFromFile,
			DeleteFactory:         options.DeleteFactory,
			ReplaceFactoryCurrent: options.ReplaceFactoryCurrent,
			ValidateFactory:       options.ValidateFactory,
			FlattenFactoryConfig:  options.FlattenFactoryConfig,
			ExpandFactoryConfig:   options.ExpandFactoryConfig,
			ConfigureInit:         options.ConfigureInit,
			HomeDir:               options.homeDir,
			DiagnosticsWriter:     diagnostics.writer,
		},
	)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(handler)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}
