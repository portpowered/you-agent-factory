package cli

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
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
	case "you mcp serve", "you run":
		return true
	default:
		return false
	}
}

func executeServerCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions CommandFactory,
) error {
	cfg := defaultcmd.ServerRunConfig(rootOptions.runDefaults)
	if err := selectCurrentFactoryFromWorkingDirectory(cmd, &cfg); err != nil {
		mapped := runcli.MapCurrentFactoryFailure(err)
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
		return mapped
	}
	policy := diagnostics.resolvePolicy(false)
	err := runFactoryWithOptions(
		cmd, cfg, nil, globals, operatorDefaults, policy, rootOptions, true,
	)
	if err == nil {
		return nil
	}
	mapped := runcli.MapServerFailure(err)
	mapped = runcli.MapCurrentFactoryFailure(factoryload.MaybeFormatOperatorError(mapped, cfg.Dir))
	_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
	return mapped
}

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
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
			InstallPackagedFactory: options.InstallPackagedFactory,
			HomeDir:               options.homeDir,
			ResolveFactoryRoots:   options.resolveNamedFactoryRoots,
			DiagnosticsWriter:     diagnostics.writer,
		},
	)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(handler)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	if err := cobracompletion.RegisterPackagedFactoryNames(
		components.Init,
		options.completePackagedFactoryNames,
	); err != nil {
		panic(fmt.Sprintf("register packaged factory init completion: %v", err))
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}
