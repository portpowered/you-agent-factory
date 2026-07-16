package cli

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	"github.com/spf13/cobra"
)

func rejectUnknownSubcommandArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return cmd.Help()
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}

func configureGroupCommandUnknownSubcommandGuard(cmd *cobra.Command) {
	cmd.DisableFlagParsing = true
	cmd.Args = rejectUnknownSubcommandArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return rejectUnknownSubcommandArgs(cmd, args)
	}
}

func newFactoryCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	factoryCmd := &cobra.Command{
		Use:   "factory",
		Short: "Inspect and manage factory definitions",
		Long: "Inspect live factory runtime state and manage persisted named factories.\n\n" +
			"Subcommands:\n" +
			"  query    show the current active factory from a running service\n" +
			"  list     list persisted named factories under a factory root\n" +
			"  config   inspect and transform factory configuration\n" +
			"  create   create a named factory from factory.json\n" +
			"  update   replace an existing named factory from factory.json\n" +
			"  replace-current  persist the live current factory from a running service\n" +
			"  delete   remove an unused named factory from disk\n\n" +
			"Use query against a running service. Use config validate, flatten, and expand for " +
			"factory configuration inspection and transformation. Use list, create, update, and delete " +
			"for on-disk named factories under --dir (default factory/). Use replace-current with " +
			"global --server and --session like query to persist the live current factory.",
		Example: "  # Show the active factory from the running service.\n" +
			"  " + cliBinaryName + " factory query\n\n" +
			"  # Validate a factory config before creating or updating it.\n" +
			"  " + cliBinaryName + " factory config validate ./factory.json\n\n" +
			"  # List persisted named factories and which one is current.\n" +
			"  " + cliBinaryName + " factory list\n\n" +
			"  # Create a new named factory from a config file.\n" +
			"  " + cliBinaryName + " factory create staging --from ./factory.json --set-current\n\n" +
			"  # Replace an existing named factory definition.\n" +
			"  " + cliBinaryName + " factory update staging --from ./factory.json\n\n" +
			"  # Delete an unused named factory.\n" +
			"  " + cliBinaryName + " factory delete staging\n\n" +
			"  # Persist the live current factory back to durable storage.\n" +
			"  " + cliBinaryName + " factory replace-current",
	}
	factoryCmd.AddCommand(
		newFactoryQueryCommand(globals, diagnostics),
		newFactoryListCommand(globals, diagnostics),
		newFactoryConfigCommand(globals, diagnostics),
		newFactoryCreateCommand(globals, diagnostics),
		newFactoryUpdateFromFileCommand(globals, diagnostics),
		newFactoryReplaceCurrentCommand(globals, diagnostics),
		newFactoryDeleteCommand(globals, diagnostics),
	)
	configureGroupCommandUnknownSubcommandGuard(factoryCmd)
	return factoryCmd
}

func newFactoryConfigCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and transform factory configuration",
		Long: "Inspect and transform factory configuration.\n\n" +
			"Subcommands:\n" +
			"  validate validate a factory.json payload or factory directory\n" +
			"  flatten  write canonical single-file factory config to stdout\n" +
			"  expand   write split factory config layout beside the input file\n\n" +
			"Use validate before create or update. Use flatten and expand to move between " +
			"single-file and split-layout factory directories.",
		Example: "  # Validate a single-file factory config.\n" +
			"  " + cliBinaryName + " factory config validate ./factory.json\n\n" +
			"  # Flatten a split-layout factory directory.\n" +
			"  " + cliBinaryName + " factory config flatten ./factory\n\n" +
			"  # Expand a canonical factory.json into split layout.\n" +
			"  " + cliBinaryName + " factory config expand ./factory.json",
	}
	configCmd.AddCommand(
		newFactoryConfigValidateCommand(globals, diagnostics),
		newFactoryConfigFlattenCommand(diagnostics),
		newFactoryConfigExpandCommand(diagnostics),
	)
	configureGroupCommandUnknownSubcommandGuard(configCmd)
	return configCmd
}

func newFactoryDeleteCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.DeleteConfig{Dir: defaultcmd.FactoryDir}

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a persisted named factory",
		Long: "Delete a persisted named factory from disk.\n\n" +
			"The command removes the named factory directory under the selected factory root " +
			"after validation. It refuses to delete the factory currently selected by " +
			".current-factory; switch the current pointer to another factory first.",
		Example: "  # Delete an unused named factory.\n" +
			"  " + cliBinaryName + " factory delete staging\n\n" +
			"  # Delete from a custom factory root.\n" +
			"  " + cliBinaryName + " factory delete staging --dir my-factory",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return deleteFactory(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	return cmd
}

func newFactoryUpdateFromFileCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.UpdateFromFileConfig{Dir: defaultcmd.FactoryDir}

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing named factory from a config file",
		Long: "Replace an existing named factory from an existing factory.json file.\n\n" +
			"The command validates the payload, atomically replaces the named factory layout under " +
			"the selected factory root, and leaves .current-factory unchanged when it already " +
			"points at the updated name.",
		Example: "  # Replace an existing named factory from a config file.\n" +
			"  " + cliBinaryName + " factory update staging --from ./factory.json\n\n" +
			"  # Emit structured confirmation for scripting.\n" +
			"  " + cliBinaryName + " --json factory update staging --from ./factory.json",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return updateFactoryFromFile(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.From, "from", "", "path to an existing factory.json payload (required)")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func newFactoryCreateCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.CreateFromFileConfig{Dir: defaultcmd.FactoryDir}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a named factory from a config file",
		Long: "Create a new named factory from an existing factory.json file.\n\n" +
			"The command validates the payload, materializes a new named factory layout under " +
			"the selected factory root, and refuses to overwrite an existing factory name.",
		Example: "  # Create a new named factory from a config file.\n" +
			"  " + cliBinaryName + " factory create staging --from ./factory.json\n\n" +
			"  # Create and select the new factory as current.\n" +
			"  " + cliBinaryName + " factory create staging --from ./factory.json --set-current\n\n" +
			"  # Emit structured confirmation for scripting.\n" +
			"  " + cliBinaryName + " --json factory create staging --from ./factory.json",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return createFactoryFromFile(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.From, "from", "", "path to an existing factory.json payload (required)")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	cmd.Flags().BoolVar(&cfg.SetCurrent, "set-current", false, "update .current-factory to the created name")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func newFactoryReplaceCurrentCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.ReplaceCurrentConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "replace-current",
		Short: "Persist the live current factory from a running service",
		Long: "Read the session current factory from a running service and persist it with PUT.\n\n" +
			"The command uses global --server and --session like factory query.",
		Example: "  # Persist the live current factory from the running service.\n" +
			"  " + cliBinaryName + " factory replace-current\n\n" +
			"  # Persist the live current factory for one session as JSON.\n" +
			"  " + cliBinaryName + " --json factory replace-current --session session-beta",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		PreRunE:      rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			return replaceFactoryCurrent(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newFactoryListCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.ListConfig{Dir: defaultcmd.FactoryDir}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List persisted named factories",
		Long: "List persisted named factories stored under a factory root.\n\n" +
			"By default the command lists project-local named factories from ./factory and writes a " +
			"human-readable table with each factory name, on-disk directory, and whether it is selected " +
			"by .current-factory. Global built-ins and customer-edited shared factories live under " +
			"~/.you-agent-factory/factories and are listed only when you point --dir there explicitly. " +
			"The command lists exactly one root at a time and never merges project-local and global entries. " +
			"Use global --json for scripting output.",
		Example: "  # List named factories under the default factory root.\n" +
			"  " + cliBinaryName + " factory list\n\n" +
			"  # List global built-ins and shared factories.\n" +
			"  " + cliBinaryName + " factory list --dir ~/.you-agent-factory/factories\n\n" +
			"  # List factories from a custom root as JSON.\n" +
			"  " + cliBinaryName + " --json factory list --dir my-factory",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return listFactories(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	return cmd
}

func newFactoryConfigValidateCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.ValidateConfig{}

	cmd := &cobra.Command{
		Use:   "validate <factory-path>",
		Short: "Validate a factory config without persisting it",
		Long: "Validate a factory.json payload or factory directory through the shared " +
			"validate-only factory contract used by POST /factory-validations.\n\n" +
			"Human output lists authored worker and workstation runtime taxonomy values and " +
			"prints blocking validation targets with inference, agent, script, or poller terminology " +
			"when worker/workstation pairings are incompatible.",
		Example: "  # Validate a single-file factory config.\n" +
			"  " + cliBinaryName + " factory config validate ./factory.json\n\n" +
			"  # Validate a split-layout factory directory.\n" +
			"  " + cliBinaryName + " factory config validate ./factory\n\n" +
			"  # Emit structured validation output for automation.\n" +
			"  " + cliBinaryName + " --json factory config validate ./factory.json",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Path = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return validateFactory(cfg)
		},
	}

	return cmd
}

func newFactoryQueryCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.QueryConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Show the current active factory",
		Long: "Show the current active factory from a running you-agent-factory service.\n\n" +
			"By default the command writes a human-readable table with the current factory name and " +
			"runtime-identifying fields. Use global --json for the API-shaped current-factory payload, and " +
			"use global --server to target the same factory API base URI as work list and submit. Run " +
			cliBinaryName + " session list to discover live session ids when routing other commands with --session.",
		Example: "  # Show the current factory from the default local service.\n" +
			"  " + cliBinaryName + " factory query\n\n" +
			"  # Emit API-shaped JSON for automation from the default local service.\n" +
			"  " + cliBinaryName + " --json factory query\n\n" +
			"  # Query a factory API on a non-default host or port.\n" +
			"  " + cliBinaryName + " --server http://localhost:9090 --json factory query",
		SilenceUsage: true,
		PreRunE:      rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return queryFactory(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}
func newFactoryConfigFlattenCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := configcli.FactoryConfigFlattenConfig{}

	cmd := &cobra.Command{
		Use:   "flatten <factory-path>",
		Short: "Write canonical single-file factory config",
		Long: "Write canonical single-file factory config.\n\n" +
			"The path may be a factory directory containing factory.json or a standalone factory.json file. " +
			"The command writes camelCase canonical JSON to stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Path = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return flattenFactoryConfig(cfg)
		},
	}

	return cmd
}

func newFactoryConfigExpandCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := configcli.FactoryConfigExpandConfig{}

	cmd := &cobra.Command{
		Use:   "expand <factory.json>",
		Short: "Write split factory config layout",
		Long: "Write split factory config layout.\n\n" +
			"The path may be a standalone factory.json file or a factory directory containing factory.json. " +
			"The command writes canonical factory.json plus workers and workstations directories beside the input file.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Path = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return expandFactoryConfig(cfg)
		},
	}

	return cmd
}

// useGeneratedFactoryConfigInitFamily toggles production wiring for the factory,
// system config, and init families between generated metadata and legacy constructors.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedFactoryConfigInitFamily = true

// NewLegacyFactoryConfigInitFamilyCommand builds the isolated handwritten
// you → factory/config/init tree used by the generator-vs-legacy parity matrix.
func NewLegacyFactoryConfigInitFamilyCommand() *cobra.Command {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(
		configinitcmd.NewSystemConfigCommand(cliBinaryName, configinitcmd.CommandGlobals{
			JSON:    func() bool { return globals.json },
			HomeDir: options.HomeDir,
		}, configinitcmd.CommandDiagnostics{
			Writer:  diagnostics.writer,
			Verbose: diagnostics.verboseEnabled,
		}),
		newFactoryCommand(globals, diagnostics),
		newInitCommand(globals, diagnostics),
	)
	return root
}

// NewGeneratedFactoryConfigInitFamilyCommandForParity builds the generated
// you → factory/config/init tree used by the generator-vs-legacy parity matrix.
func NewGeneratedFactoryConfigInitFamilyCommandForParity(
	registry *commandregistry.Registry,
	bindings climanifestcobra.FactoryConfigInitFlagBindings,
) (*cobra.Command, error) {
	if registry == nil {
		return nil, fmt.Errorf("build factory/config/init parity command: registry is required")
	}
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(registry, bindings)
	if err != nil {
		return nil, fmt.Errorf("build factory/config/init parity command: %w", err)
	}
	root.AddCommand(components.Config, components.Factory, components.Init)
	return root, nil
}

// LegacyFactoryConfigInitFamilyCommands holds isolated handwritten factory/config/init
// trees used by the generator-vs-legacy parity matrix and rollback comparisons.
type LegacyFactoryConfigInitFamilyCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

// NewLegacyFactoryConfigInitFamilyCommands builds detached handwritten factory,
// system config, and init commands for parity comparisons.
func NewLegacyFactoryConfigInitFamilyCommands(options RootCommandOptions) LegacyFactoryConfigInitFamilyCommands {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	return LegacyFactoryConfigInitFamilyCommands{
		Factory: newFactoryCommand(globals, diagnostics),
		Config: configinitcmd.NewSystemConfigCommand(cliBinaryName, configinitcmd.CommandGlobals{
			JSON:    func() bool { return globals.json },
			HomeDir: options.HomeDir,
		}, configinitcmd.CommandDiagnostics{
			Writer:  diagnostics.writer,
			Verbose: diagnostics.verboseEnabled,
		}),
		Init: newInitCommand(globals, diagnostics),
	}
}

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options RootCommandOptions,
) factoryConfigInitProductionCommands {
	if !useGeneratedFactoryConfigInitFamily {
		legacy := NewLegacyFactoryConfigInitFamilyCommands(options)
		return factoryConfigInitProductionCommands{
			Factory: legacy.Factory,
			Config:  legacy.Config,
			Init:    legacy.Init,
		}
	}

	registry, bindings, err := newFactoryConfigInitWiring(globals, diagnostics, options)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init handler registry: %v", err))
	}
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(registry, bindings)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}

func newFactoryConfigInitWiring(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	rootOptions RootCommandOptions,
) (*commandregistry.Registry, climanifestcobra.FactoryConfigInitFlagBindings, error) {
	state := factoryConfigInitBindingState{
		listDir:      defaultcmd.FactoryDir,
		createDir:    defaultcmd.FactoryDir,
		updateDir:    defaultcmd.FactoryDir,
		deleteDir:    defaultcmd.FactoryDir,
		initDir:      defaultcmd.FactoryDir,
		initType:     string(initcmd.DefaultScaffoldType),
		initExecutor: initcmd.DefaultStarterExecutor,
	}
	bindings := factoryConfigInitFlagBindingsFromState(&state)
	registry, err := newFactoryConfigInitHandlerRegistry(globals, diagnostics, rootOptions, &state)
	if err != nil {
		return nil, climanifestcobra.FactoryConfigInitFlagBindings{}, err
	}
	return registry, bindings, nil
}

type factoryConfigInitBindingState struct {
	listDir          string
	createDir        string
	updateDir        string
	deleteDir        string
	createFrom       string
	createSetCurrent bool
	updateFrom       string
	replaceSessionID string
	initDir          string
	initType         string
	initExecutor     string
}

func factoryConfigInitFlagBindingsFromState(state *factoryConfigInitBindingState) climanifestcobra.FactoryConfigInitFlagBindings {
	return climanifestcobra.FactoryConfigInitFlagBindings{
		FactoryListDir:          &state.listDir,
		FactoryCreateDir:        &state.createDir,
		FactoryUpdateDir:        &state.updateDir,
		FactoryDeleteDir:        &state.deleteDir,
		FactoryCreateFrom:       &state.createFrom,
		FactoryCreateSetCurrent: &state.createSetCurrent,
		FactoryUpdateFrom:       &state.updateFrom,
		FactoryReplaceSessionID: &state.replaceSessionID,
		InitDir:                 &state.initDir,
		InitType:                &state.initType,
		InitExecutor:            &state.initExecutor,
		FlagUsages:              factoryConfigInitFlagUsages(),
	}
}

func factoryConfigInitFlagUsages() map[string]string {
	return map[string]string{
		"dir":         "factory root directory containing named factories",
		"from":        "path to an existing factory.json payload (required)",
		"set-current": "update .current-factory to the created name",
		"session":     "target one live factory session; omit to use the default compatibility session",
		"type":        "scaffold type to generate (supported: default, ralph)",
		"executor": fmt.Sprintf(
			"starter scaffold to generate (%s)",
			strings.Join(initcmd.SupportedStarterExecutors(), ", "),
		),
	}
}

func newFactoryConfigInitHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	rootOptions RootCommandOptions,
	state *factoryConfigInitBindingState,
) (*commandregistry.Registry, error) {
	if state == nil {
		fallback := factoryConfigInitBindingState{
			listDir:      defaultcmd.FactoryDir,
			createDir:    defaultcmd.FactoryDir,
			updateDir:    defaultcmd.FactoryDir,
			deleteDir:    defaultcmd.FactoryDir,
			initDir:      defaultcmd.FactoryDir,
			initType:     string(initcmd.DefaultScaffoldType),
			initExecutor: initcmd.DefaultStarterExecutor,
		}
		state = &fallback
	}

	return commandregistry.NewFactoryConfigInitRegistry(commandregistry.FactoryConfigInitHandlers{
		FactoryQueryRunE: commandregistry.FactoryQueryRunE(commandregistry.FactoryQueryBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Query:             queryFactory,
		}),
		FactoryListRunE: commandregistry.FactoryListRunE(commandregistry.FactoryListBinding{
			Dir:  &state.listDir,
			JSON: &globals.json,
			List: listFactories,
		}),
		FactoryCreateRunE: commandregistry.FactoryCreateRunE(commandregistry.FactoryCreateBinding{
			Dir:        &state.createDir,
			From:       &state.createFrom,
			SetCurrent: &state.createSetCurrent,
			JSON:       &globals.json,
			Create:     createFactoryFromFile,
		}),
		FactoryUpdateRunE: commandregistry.FactoryUpdateRunE(commandregistry.FactoryUpdateBinding{
			Dir:    &state.updateDir,
			From:   &state.updateFrom,
			JSON:   &globals.json,
			Update: updateFactoryFromFile,
		}),
		FactoryDeleteRunE: commandregistry.FactoryDeleteRunE(commandregistry.FactoryDeleteBinding{
			Dir:    &state.deleteDir,
			JSON:   &globals.json,
			Delete: deleteFactory,
		}),
		FactoryReplaceCurrentRunE: commandregistry.FactoryReplaceCurrentRunE(commandregistry.FactoryReplaceCurrentBinding{
			Server:            &globals.server,
			SessionID:         &state.replaceSessionID,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			DiagnosticsWriter: diagnostics.writer,
			ReplaceCurrent:    replaceFactoryCurrent,
		}),
		FactoryConfigValidateRunE: commandregistry.FactoryConfigValidateRunE(commandregistry.FactoryConfigValidateBinding{
			JSON:     &globals.json,
			Validate: validateFactory,
		}),
		FactoryConfigFlattenRunE: commandregistry.FactoryConfigFlattenRunE(commandregistry.FactoryConfigFlattenBinding{
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Flatten:           flattenFactoryConfig,
		}),
		FactoryConfigExpandRunE: commandregistry.FactoryConfigExpandRunE(commandregistry.FactoryConfigExpandBinding{
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Expand:            expandFactoryConfig,
		}),
		ConfigInitRunE: commandregistry.ConfigInitRunE(commandregistry.ConfigInitBinding{
			HomeDir:           rootOptions.HomeDir,
			JSON:              func() bool { return globals.json },
			DiagnosticsWriter: diagnostics.writer,
			Verbose:           diagnostics.verboseEnabled,
			Init:              configinitcmd.RunInit,
		}),
		InitRunE: commandregistry.InitRunE(commandregistry.InitBinding{
			Dir:               &state.initDir,
			Type:              &state.initType,
			Executor:          &state.initExecutor,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			Init:              initFactory,
		}),
	})
}
