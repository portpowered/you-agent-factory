package cli

import (
	"errors"
	"fmt"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	serverstopcli "github.com/portpowered/infinite-you/pkg/transports/cli/serverstop"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
			writeHomeDirectoryDisclosure(cmd, args, homeDir)
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

func writeHomeDirectoryDisclosure(cmd *cobra.Command, args []string, homeDir string) {
	if !shouldDiscloseHomeDirectory(cmd, args) {
		return
	}
	output := cmd.OutOrStdout()
	if homeDirectoryDisclosureUsesDiagnostics(cmd, args) {
		output = cmd.ErrOrStderr()
	}
	if output == nil {
		return
	}
	_, _ = fmt.Fprintf(output, "Home directory: %s\n", homeDir)
}

func shouldDiscloseHomeDirectory(cmd *cobra.Command, args []string) bool {
	if cmd == nil || commandHelpRequested(cmd, args) {
		return false
	}
	// JSON and NDJSON invocations reserve stdout for their machine-readable
	// result and stderr for the single structured error response on failure.
	// A human startup line cannot be emitted on either stream without changing
	// those public contracts.
	if commandInputEnabled(cmd, args, "you.flag.json", "json") {
		return false
	}
	switch cmd.CommandPath() {
	case "you server", "you server acp", "you server mcp":
		return true
	case "you run":
		if commandInputEnabled(cmd, args, "you.flag.remote", "remote") ||
			commandInputEnabled(cmd, args, "you.run.flag.quiet", "quiet") {
			return false
		}
		return !runInvocationOutputIsClean(cmd, args)
	default:
		return false
	}
}

func homeDirectoryDisclosureUsesDiagnostics(cmd *cobra.Command, args []string) bool {
	if cmd == nil {
		return true
	}
	switch cmd.CommandPath() {
	case "you server acp", "you server mcp":
		return true
	case "you server":
		return commandInputEnabled(cmd, args, "you.flag.json", "json") ||
			commandInputEnabled(cmd, args, "you.flag.verbose", "verbose") ||
			commandInputEnabled(cmd, args, "you.flag.debug", "debug")
	case "you run":
		if commandInputEnabled(cmd, args, "you.flag.json", "json") ||
			commandInputEnabled(cmd, args, "you.flag.verbose", "verbose") ||
			commandInputEnabled(cmd, args, "you.flag.debug", "debug") ||
			rawFlagSupplied(args, "output") || commandInputChanged(cmd, "you.run.flag.output", "output") {
			return true
		}
	}
	return false
}

func commandHelpRequested(cmd *cobra.Command, args []string) bool {
	if rawFlagEnabled(args, "help") {
		return true
	}
	flag := cmd.Flag("help")
	return flag != nil && flag.Changed
}

func commandFlagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	for _, flagSet := range []*pflag.FlagSet{
		cmd.Flags(), cmd.InheritedFlags(), cmd.PersistentFlags(), cmd.Root().PersistentFlags(),
	} {
		if flag := flagSet.Lookup(name); flag != nil && flag.Changed {
			return true
		}
	}
	return false
}

func commandInputChanged(cmd *cobra.Command, inputID, fallbackFlagName string) bool {
	changed, err := climanifestcobra.InputChanged(cmd, inputID)
	if err == nil && changed {
		return changed
	}
	return commandFlagChanged(cmd, fallbackFlagName)
}

func commandInputEnabled(cmd *cobra.Command, args []string, inputID, fallbackFlagName string) bool {
	return rawFlagEnabled(args, fallbackFlagName) || commandInputChanged(cmd, inputID, fallbackFlagName)
}

func runInvocationOutputIsClean(cmd *cobra.Command, args []string) bool {
	if rawFlagSupplied(args, "output") || commandInputChanged(cmd, "you.run.flag.output", "output") {
		return true
	}
	selected := rawFlagSupplied(args, "factory") || rawFlagSupplied(args, "named") ||
		commandInputChanged(cmd, "you.run.flag.factory", "factory") || commandInputChanged(cmd, "you.run.flag.named", "named")
	if !selected {
		return false
	}
	return len(cmd.Flags().Args()) > 0 || !startupcli.StdinIsTTY(cmd.Context()) ||
		rawFlagSupplied(args, "to-file") || rawFlagSupplied(args, "work") || commandInputChanged(cmd, "you.run.flag.work", "work")
}

func rawFlagSupplied(args []string, name string) bool {
	prefix := "--" + name
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}

func rawFlagEnabled(args []string, name string) bool {
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
