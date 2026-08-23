package cli

import (
	"errors"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	serverstopcli "github.com/portpowered/infinite-you/pkg/transports/cli/serverstop"
	"github.com/spf13/cobra"
)

func newRootCommandWithFactory(options CommandFactory) *cobra.Command {
	root := newRootCommandWithGeneratedRepresentativeFamily(options)
	if root == nil {
		return nil
	}
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := validateRunRemoteHostingBeforeInitialization(cmd, args); err != nil {
			_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, false)
			return err
		}
		if requiresSystemInitialization(cmd.CommandPath(), args) {
			if options.initializer == nil {
				return fmt.Errorf("system initializer is required")
			}
			homeDir, err := resolveProcessHomeDirForCommand(cmd, options)
			if err != nil {
				return err
			}
			if err := options.initializer.InitializeSystem(cmd.Context(), homeDir); err != nil {
				wrapped := fmt.Errorf("initialize system: %w", err)
				if errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
					diagnostic := wrapped
					if cause := errors.Unwrap(err); cause != nil {
						diagnostic = fmt.Errorf("%s: %v", wrapped, cause)
					}
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), diagnostic)
				}
				return wrapped
			}
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return root
}

func validateRunRemoteHostingBeforeInitialization(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.CommandPath() != "you run" {
		return nil
	}
	remote := runFlagEnabled(cmd, "remote") || rawRunFlagEnabled(args, "remote")
	if !remote {
		return nil
	}
	if !rawRunFlagEnabled(args, "with-server") && !rawRunFlagEnabled(args, "with-site") {
		return nil
	}
	return newRunRemoteLocalHostingConflictError()
}

func runFlagEnabled(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag(name)
	return flag != nil && flag.Changed && flag.Value != nil && flag.Value.String() == "true"
}

func rawRunFlagEnabled(args []string, name string) bool {
	enabled := false
	prefix := "--" + name
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case prefix:
			enabled = true
		case prefix + "=true":
			enabled = true
		case prefix + "=false":
			enabled = false
		}
	}
	return enabled
}

func requiresSystemInitialization(commandPath string, args []string) bool {
	switch commandPath {
	case "you":
		return len(args) > 0
	case "you server", "you server acp", "you server mcp", "you run":
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
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	cfg.ListenAddress, err = commandInputValue[string](values, serverListenInputID)
	if err != nil {
		return err
	}
	cfg.Pprof, err = commandInputValue[bool](values, serverPprofInputID)
	if err != nil {
		return err
	}
	cfg.ListenExplicit, err = climanifestcobra.InputChanged(cmd, serverListenInputID)
	if err != nil {
		return err
	}
	if err := selectCurrentFactoryFromWorkingDirectory(cmd, &cfg); err != nil {
		mapped := runcli.MapCurrentFactoryFailure(err)
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
		return mapped
	}
	policy := diagnostics.resolvePolicy(false)
	err = runFactoryWithOptions(
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

func executeServerStopCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	rootOptions CommandFactory,
) error {
	if rootOptions.serverStopCLI == nil {
		return fmt.Errorf("server stop operation is required")
	}
	server := ""
	jsonOutput := false
	if globals != nil {
		server = globals.server
		jsonOutput = globals.json
	}
	return rootOptions.serverStopCLI(cmd.Context(), serverstopcli.Config{
		Server: server, JSON: jsonOutput, Output: cmd.OutOrStdout(),
	})
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
			QueryFactory:           options.QueryFactory,
			ListFactories:          options.ListFactories,
			CreateFactoryFromFile:  options.CreateFactoryFromFile,
			UpdateFactoryFromFile:  options.UpdateFactoryFromFile,
			DeleteFactory:          options.DeleteFactory,
			ReplaceFactoryCurrent:  options.ReplaceFactoryCurrent,
			ValidateFactory:        options.ValidateFactory,
			FlattenFactoryConfig:   options.FlattenFactoryConfig,
			ExpandFactoryConfig:    options.ExpandFactoryConfig,
			ConfigureInit:          options.ConfigureInit,
			InstallPackagedFactory: options.InstallPackagedFactory,
			HomeDir:                options.homeDir,
			ResolveFactoryRoots:    options.resolveNamedFactoryRoots,
			DiagnosticsWriter:      diagnostics.writer,
		},
	)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(handler)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	if options.completePackagedFactoryNames != nil {
		if err := cobracompletion.RegisterPackagedFactoryNames(
			components.Init,
			options.completePackagedFactoryNames,
		); err != nil {
			panic(fmt.Sprintf("register packaged factory init completion: %v", err))
		}
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}
