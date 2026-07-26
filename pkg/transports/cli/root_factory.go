package cli

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/spf13/cobra"
)

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
			InitSystemConfig:      options.InitSystemConfig,
			InitFactory:           options.InitFactory,
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
