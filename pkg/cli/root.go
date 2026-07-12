// Package cli defines Cobra commands for the agent-factory CLI.
// Commands contain only flag parsing and delegate to command-specific packages.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	configcli "github.com/portpowered/infinite-you/pkg/cli/config"
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/spf13/cobra"
)

var runCLI = runcli.Run
var flattenFactoryConfig = configcli.FlattenFactoryConfig
var expandFactoryConfig = configcli.ExpandFactoryConfig
var initFactory = initcmd.Init
var submitWork = submitcli.Submit
var submitBatch = submitcli.SubmitBatch
var listWork = workcli.List
var showWork = workcli.Show
var moveWork = workcli.Move
var visualizeWork = workcli.Visualize
var listSessions = sessioncli.List
var showSession = sessioncli.Show
var pauseSession = sessioncli.Pause
var resumeSession = sessioncli.Resume
var listSessionDispatches = sessioncli.Dispatches
var createSession = sessioncli.Create
var deleteSession = sessioncli.Delete
var queryFactory = factorycli.Query
var listFactories = factorycli.List
var validateFactory = factorycli.Validate
var saveFactoryFromFile = factorycli.SaveFromFile
var saveFactoryCurrent = factorycli.SaveCurrent
var updateFactoryFromFile = factorycli.UpdateFromFile
var deleteFactory = factorycli.Delete
var listModels = modelscli.List
var inspectModel = modelscli.Inspect
var invokeModel = modelscli.Invoke
var pullModel = modelscli.Pull

const (
	defaultMockWorkersConfigPathSentinel = "__agent_factory_default_mock_workers_config__"
)

const cliBinaryName = "you"

// NewRootCommand creates the top-level Cobra command for the you-agent-factory CLI.
type cliGlobalOptions struct {
	server string
	json   bool
}

type cliOperatorDefaultsOptions struct {
	defaultWorkerModelProvider string
	defaultWorkerModel         string
}

func NewRootCommand() *cobra.Command {
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := &cobra.Command{
		Use:          cliBinaryName,
		Short:        "Run and manage CPN-based workflow factories",
		SilenceUsage: true,
		Long: "Run and manage CPN-based workflow factories.\n\n" +
			"What:\n" +
			"CPN-based workflow factory CLI for running factories, submitting work, and inspecting live sessions.\n\n" +
			"How to use:\n" +
			"Running " + cliBinaryName + " with no args starts the out-of-the-box continuous factory and local dashboard (http://localhost:7437/dashboard/ui).\n" +
			"Use " + cliBinaryName + " run --dir factory for explicit runs. See " + cliBinaryName + " <cmd> --help for subcommand details.\n\n" +
			"Agents:\n" +
			"Start with " + cliBinaryName + " docs agents for orientation, " + cliBinaryName + " submit or " + cliBinaryName + " submit batch to enqueue work, and " + cliBinaryName + " session list to confirm a live factory.\n" +
			"Run " + cliBinaryName + " docs for all packaged reference topics. Use --verbose or --debug for stderr diagnostics; full policy in " + cliBinaryName + " docs.",
		Example: "  # Start the default Codex-backed factory in the current project.\n" +
			"  " + cliBinaryName + "\n\n" +
			"  # Agent orientation and command matrix.\n" +
			"  " + cliBinaryName + " docs agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFactory(cmd, defaultcmd.OOTBRunConfig(), nil, globals, operatorDefaults, diagnostics.verboseEnabled(), diagnostics.debug)
		},
	}
	root.PersistentFlags().BoolVarP(&diagnostics.verbose, "verbose", "v", false, "emit concise command diagnostics to stderr")
	root.PersistentFlags().BoolVarP(&diagnostics.debug, "debug", "d", false, "emit lower-level command diagnostics where supported (implies --verbose)")
	root.PersistentFlags().StringVar(&globals.server, "server", cliserver.DefaultBaseURI, "factory API base URI (http:// or https://); HTTP client commands target this URI and you run binds locally to its host and port")
	root.PersistentFlags().BoolVar(&globals.json, "json", false, "emit structured JSON on stdout for supported commands; diagnostics remain on stderr")
	root.PersistentFlags().StringVar(
		&operatorDefaults.defaultWorkerModelProvider,
		"default-worker-model-provider",
		"",
		fmt.Sprintf(
			"default worker model provider for model workers with omitted modelProvider (%s; DEFAULT resolves through lower-precedence concrete provider)",
			interfaces.AcceptedPublicWorkerModelProviderSummary(),
		),
	)
	root.PersistentFlags().StringVar(
		&operatorDefaults.defaultWorkerModel,
		"default-worker-model",
		"",
		"default worker model for model workers with omitted model",
	)

	root.AddCommand(
		newConfigCommand(diagnostics),
		newDocsCommand(diagnostics),
		newFactoryCommand(globals, diagnostics),
		newInitCommand(globals, diagnostics),
		newMCPCommand(),
		newModelsCommand(globals, diagnostics),
		newRunCommand(globals, diagnostics, operatorDefaults),
		newSubmitCommand(globals, diagnostics),
		newSessionCommand(globals, diagnostics),
		newWorkCommand(globals, diagnostics),
		newWorkflowCommand(globals, diagnostics),
	)

	return root
}

type cliDiagnosticsOptions struct {
	verbose bool
	debug   bool
}

const deprecatedPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"

func rejectDeprecatedPortFlag(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Lookup("port") != nil && cmd.Flags().Changed("port") {
		return fmt.Errorf("%s", deprecatedPortFlagMessage)
	}
	return nil
}

func registerDeprecatedPortFlag(cmd *cobra.Command) {
	var deprecatedPort int
	cmd.Flags().IntVar(&deprecatedPort, "port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
}

func (opts *cliDiagnosticsOptions) verboseEnabled() bool {
	return opts.verbose || opts.debug
}

func (opts *cliDiagnosticsOptions) writer(cmd *cobra.Command) io.Writer {
	if !opts.verboseEnabled() {
		return nil
	}
	return cmd.ErrOrStderr()
}

func newFactoryCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	factoryCmd := &cobra.Command{
		Use:   "factory",
		Short: "Inspect and manage factory definitions",
		Long: "Inspect live factory runtime state and manage persisted named factories.\n\n" +
			"Subcommands:\n" +
			"  query    show the current active factory from a running service\n" +
			"  list     list persisted named factories under a factory root\n" +
			"  validate validate a factory.json payload or factory directory\n" +
			"  save     create a named factory from factory.json or persist the live current factory\n" +
			"  update   replace an existing named factory from factory.json\n" +
			"  delete   remove an unused named factory from disk\n\n" +
			"Use query against a running service. Use list, save, update, and delete for on-disk " +
			"named factories under --dir (default factory/). Live save with no name argument uses " +
			"global --server and --session like query.",
		Example: "  # Show the active factory from the running service.\n" +
			"  " + cliBinaryName + " factory query\n\n" +
			"  # Validate a factory config before saving it.\n" +
			"  " + cliBinaryName + " factory validate ./factory.json\n\n" +
			"  # List persisted named factories and which one is current.\n" +
			"  " + cliBinaryName + " factory list\n\n" +
			"  # Save a new named factory from a config file.\n" +
			"  " + cliBinaryName + " factory save staging --from ./factory.json --set-current\n\n" +
			"  # Replace an existing named factory definition.\n" +
			"  " + cliBinaryName + " factory update staging --from ./factory.json\n\n" +
			"  # Delete an unused named factory.\n" +
			"  " + cliBinaryName + " factory delete staging\n\n" +
			"  # Persist the live current factory back to durable storage.\n" +
			"  " + cliBinaryName + " factory save",
	}
	factoryCmd.AddCommand(
		newFactoryQueryCommand(globals, diagnostics),
		newFactoryListCommand(globals, diagnostics),
		newFactoryValidateCommand(globals, diagnostics),
		newFactorySaveCommand(globals, diagnostics),
		newFactoryUpdateFromFileCommand(globals, diagnostics),
		newFactoryDeleteCommand(globals, diagnostics),
	)
	return factoryCmd
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

func newFactorySaveCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	fileCfg := factorycli.SaveFromFileConfig{Dir: defaultcmd.FactoryDir}
	liveCfg := factorycli.SaveCurrentConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "save [name]",
		Short: "Save a named factory from disk or persist the live current factory",
		Long: "Save factory definitions from disk or persist the live current factory from a running service.\n\n" +
			"With a name argument, the command validates a factory.json payload and materializes a new " +
			"named factory layout under the selected factory root. Without a name, the command reads the " +
			"session current factory from the running service and persists it with PUT.",
		Example: "  # Save a new named factory from a config file.\n" +
			"  " + cliBinaryName + " factory save staging --from ./factory.json\n\n" +
			"  # Save and select the new factory as current.\n" +
			"  " + cliBinaryName + " factory save staging --from ./factory.json --set-current\n\n" +
			"  # Persist the live current factory from the running service.\n" +
			"  " + cliBinaryName + " factory save\n\n" +
			"  # Persist the live current factory for one session as JSON.\n" +
			"  " + cliBinaryName + " --json factory save --session session-beta",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		PreRunE:      rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				liveCfg.Server = globals.server
				liveCfg.JSON = globals.json
				liveCfg.Output = cmd.OutOrStdout()
				liveCfg.Diagnostics = diagnostics.writer(cmd)
				liveCfg.Verbose = diagnostics.verboseEnabled()
				return saveFactoryCurrent(liveCfg)
			}
			fileCfg.Name = args[0]
			fileCfg.JSON = globals.json
			fileCfg.Output = cmd.OutOrStdout()
			return saveFactoryFromFile(fileCfg)
		},
	}

	cmd.Flags().StringVar(&fileCfg.From, "from", "", "path to an existing factory.json payload (required with <name>)")
	cmd.Flags().StringVar(&fileCfg.Dir, "dir", fileCfg.Dir, "factory root directory containing named factories")
	cmd.Flags().BoolVar(&fileCfg.SetCurrent, "set-current", false, "update .current-factory to the saved name")
	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&liveCfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
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
			"~/.you-agent-factory/you-agent-factories and are listed only when you point --dir there explicitly. " +
			"The command lists exactly one root at a time and never merges project-local and global entries. " +
			"Use global --json for scripting output.",
		Example: "  # List named factories under the default factory root.\n" +
			"  " + cliBinaryName + " factory list\n\n" +
			"  # List global built-ins and shared factories.\n" +
			"  " + cliBinaryName + " factory list --dir ~/.you-agent-factory/you-agent-factories\n\n" +
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

func newFactoryValidateCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
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
			"  " + cliBinaryName + " factory validate ./factory.json\n\n" +
			"  # Validate a split-layout factory directory.\n" +
			"  " + cliBinaryName + " factory validate ./factory\n\n" +
			"  # Emit structured validation output for automation.\n" +
			"  " + cliBinaryName + " --json factory validate ./factory.json",
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

func newModelsCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "Inspect discovered models from a running service",
		Long: "Inspect discovered models from a running infinite-you service.\n\n" +
			"Use list to discover model identifiers, inspect to view one model's readiness and capabilities, " +
			"invoke to call a discovered model directly through the same /models contract exposed by the API, " +
			"and pull to populate the managed local-model cache for supported local assets.",
	}
	modelsCmd.AddCommand(
		newModelsListCommand(globals, diagnostics),
		newModelsInspectCommand(globals, diagnostics),
		newModelsInvokeCommand(globals, diagnostics),
		newModelsPullCommand(globals, diagnostics),
	)
	return modelsCmd
}

func newModelsListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.ListConfig{Server: globals.server}
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List discovered models",
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listModels(cfg)
		},
	}
	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newModelsInspectCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.InspectConfig{Server: globals.server}
	cmd := &cobra.Command{
		Use:     "inspect <model-name>",
		Short:   "Inspect one discovered model",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.ModelName = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return inspectModel(cfg)
		},
	}
	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newModelsInvokeCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.InvokeConfig{Server: globals.server, Operation: "TTS"}
	cmd := &cobra.Command{
		Use:     "invoke <model-name>",
		Short:   "Invoke one discovered model",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.ModelName = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return invokeModel(cfg)
		},
	}
	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.Operation, "operation", cfg.Operation, "uppercase provider-agnostic operation name")
	cmd.Flags().StringVar(&cfg.Text, "text", "", "text input for direct invocation")
	cmd.Flags().StringVar(&cfg.OutputPath, "output", "", "output file path for streamed audio responses")
	return cmd
}

func newModelsPullCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.PullConfig{Server: globals.server}
	cmd := &cobra.Command{
		Use:     "pull <model-name>",
		Short:   "Pull one discovered local model into the managed cache",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.ModelName = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return pullModel(cfg)
		},
	}
	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newMCPCommand() *cobra.Command {
	return mcpcli.NewCommand()
}

func newDocsCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	docsCmd := &cobra.Command{
		Use:          "docs [topic]",
		Short:        "Print packaged markdown reference topics",
		SilenceUsage: true,
		Long: "Print packaged markdown reference topics from the installed binary.\n\n" +
			"Run without a topic to print the quick-start blurb and packaged docs index. Use one supported topic argument to print the authored markdown page with no wrapper formatting.",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: docscli.SupportedTopicCommands(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				_, err := io.WriteString(cmd.OutOrStdout(), docscli.IndexMarkdown(cliBinaryName))
				return err
			}

			topic := args[0]
			diagnosticsOutput := diagnostics.writer(cmd)
			clidiag.Printf(diagnosticsOutput, diagnostics.verboseEnabled(), "docs request topic=%s", topic)
			markdown, err := docscli.Markdown(topic)
			if err != nil {
				clidiag.Printf(diagnosticsOutput, diagnostics.verboseEnabled(), "docs failed topic=%s phase=resolve-topic", topic)
				return err
			}
			clidiag.Printf(diagnosticsOutput, diagnostics.verboseEnabled(), "docs resolved topic=%s contentBytes=%d", topic, len(markdown))
			_, err = io.WriteString(cmd.OutOrStdout(), markdown)
			if err != nil {
				clidiag.Printf(diagnosticsOutput, diagnostics.verboseEnabled(), "docs failed topic=%s phase=write-output", topic)
			}
			return err
		},
	}

	return docsCmd
}

func newConfigCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and transform factory configuration",
	}
	configCmd.AddCommand(
		newConfigExpandCommand(diagnostics),
		newConfigFlattenCommand(diagnostics),
	)
	return configCmd
}

func newConfigFlattenCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
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

func newConfigExpandCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
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

// Execute runs the root command.
func Execute() {
	if err := NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newInitCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := initcmd.InitConfig{
		Dir:      defaultcmd.FactoryDir,
		Type:     string(initcmd.DefaultScaffoldType),
		Executor: initcmd.DefaultStarterExecutor,
	}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create factory directory structure",
		Long: "Create factory directory structure.\n\n" +
			"Supported scaffold types:\n" +
			"  default - single-step task-processing scaffold\n" +
			"  ralph   - minimal PRD-to-execution scaffold\n\n" +
			"Omitting --executor preserves the default Codex-backed starter scaffold. " +
			"Supported starter scaffold values are codex and claude. " +
			"Omitting --type keeps the current default scaffold behavior. " +
			"For the default scaffold, --executor chooses which starter worker scaffold is generated.",
		Example: "  # Create the default Codex-backed scaffold in ./factory.\n" +
			"  " + cliBinaryName + " init\n\n" +
			"  # Create a Claude-backed default scaffold in a custom directory.\n" +
			"  " + cliBinaryName + " init --dir my-factory --executor claude\n\n" +
			"  # Create the minimal Ralph PRD-to-execution scaffold.\n" +
			"  " + cliBinaryName + " init --type ralph --dir ralph-factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return initFactory(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "base directory to create")
	cmd.Flags().StringVar(&cfg.Type, "type", cfg.Type, "scaffold type to generate (supported: default, ralph)")
	cmd.Flags().StringVar(
		&cfg.Executor,
		"executor",
		cfg.Executor,
		fmt.Sprintf(
			"starter scaffold to generate (%s)",
			strings.Join(initcmd.SupportedStarterExecutors(), ", "),
		),
	)
	return cmd
}

func runFactory(cmd *cobra.Command, cfg runcli.RunConfig, promptArgs []string, globals *cliGlobalOptions, operatorDefaults *cliOperatorDefaultsOptions, verbose, debug bool) error {
	logger, err := logging.BuildLogger(verbose, debug)
	if err != nil {
		return err
	}
	cfg.Logger = logger
	cfg.Verbose = verbose || debug

	if err := resolveRunBindFromServer(cmd, globals.server, &cfg); err != nil {
		return err
	}
	if err := resolveRunFactorySelection(cmd, &cfg); err != nil {
		return err
	}

	resolvedOperatorDefaults, err := resolveOperatorDefaults(cmd, operatorDefaults)
	if err != nil {
		return err
	}
	cfg.OperatorDefaults = resolvedOperatorDefaults
	if err := resolveRunFactoryPrompt(cmd, &cfg, promptArgs); err != nil {
		runcli.ObserveInvocationRejection(logger, err)
		return err
	}
	cleanInvocation, textInvocation := runInvocationModes(cmd, cfg)
	cfg.CleanInvocation = cleanInvocation
	cfg.JSON = globals.json
	cfg.SuppressDashboardRendering = cfg.SuppressDashboardRendering || cleanInvocation || textInvocation
	if cleanInvocation || textInvocation {
		cfg.Output = cmd.OutOrStdout()
	} else if strings.TrimSpace(cfg.FactoryConfigPath) != "" {
		cfg.Output = cmd.OutOrStdout()
		cfg.StartupOutput = cmd.OutOrStdout()
	} else {
		cfg.StartupOutput = cmd.OutOrStdout()
	}
	cfg.Diagnostics = cmd.ErrOrStderr()
	cfg.JSONOutput = globals.json

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			logger.Info("received signal, shutting down")
			cancel()
		case <-ctx.Done():
		}
	}()

	return runCLI(ctx, cfg)
}

func runInvocationModes(cmd *cobra.Command, cfg runcli.RunConfig) (cleanInvocation bool, textInvocation bool) {
	invocationFactorySelected := cmd.Flags().Changed("factory") || cmd.Flags().Changed("named")
	cleanInvocation = invocationFactorySelected &&
		cmd.Flags().Changed("work") &&
		strings.TrimSpace(cfg.WorkFile) != "" &&
		!cfg.Continuously
	textInvocation = invocationFactorySelected &&
		!cmd.Flags().Changed("work") &&
		!cfg.Continuously &&
		(cfg.InvocationPositionalText != nil || cfg.InvocationStdinText != nil)
	return cleanInvocation, textInvocation
}

func resolveOperatorDefaults(cmd *cobra.Command, operatorDefaults *cliOperatorDefaultsOptions) (operatorconfig.ResolvedDefaults, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return operatorconfig.ResolvedDefaults{}, fmt.Errorf("resolve operator home directory: %w", err)
	}
	return operatorconfig.ResolveFromHome(homeDir, operatorconfig.FlagOverrides{
		WorkerModelProvider: persistentFlagValueIfChanged(cmd, "default-worker-model-provider", operatorDefaults.defaultWorkerModelProvider),
		WorkerModel:         persistentFlagValueIfChanged(cmd, "default-worker-model", operatorDefaults.defaultWorkerModel),
	})
}

func persistentFlagValueIfChanged(cmd *cobra.Command, name, value string) string {
	if cmd.Root().PersistentFlags().Changed(name) {
		return value
	}
	return ""
}

func resolveRunBindFromServer(cmd *cobra.Command, server string, cfg *runcli.RunConfig) error {
	target, err := cliserver.LocalBindTargetFromServer(server)
	if err != nil {
		return err
	}
	cfg.BindHost = target.Host
	cfg.Port = target.Port
	if cmd.Root().PersistentFlags().Changed("server") {
		cfg.AutoPort = false
	} else {
		cfg.AutoPort = true
	}
	return nil
}

func resolveRunFactorySelection(cmd *cobra.Command, cfg *runcli.RunConfig) error {
	factoryChanged := cmd.Flags().Changed("factory")
	dirChanged := cmd.Flags().Changed("dir")
	namedChanged := cmd.Flags().Changed("named")
	if namedChanged {
		switch {
		case factoryChanged:
			return fmt.Errorf("--named cannot be used with --factory")
		case dirChanged:
			return fmt.Errorf("--named cannot be used with --dir")
		}
		return resolveRunNamedFactorySelection(cfg)
	}
	if factoryChanged && dirChanged {
		return fmt.Errorf("--factory cannot be used with --dir")
	}
	if !factoryChanged {
		return nil
	}

	factoryRoot, err := factoryrun.ResolveFactoryRootFromConfigFile(cfg.FactoryConfigPath)
	if err != nil {
		return err
	}
	cfg.Dir = factoryRoot
	return nil
}

func resolveRunNamedFactorySelection(cfg *runcli.RunConfig) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current working directory for --named: %w", err)
	}
	projectRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(cwd)
	if err != nil {
		return fmt.Errorf("resolve project named-factory root: %w", err)
	}
	globalRoot, err := factoryconfig.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		return fmt.Errorf("resolve global named-factory root: %w", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(projectRoot, globalRoot, cfg.NamedFactoryName)
	if err != nil {
		return factoryconfig.MaybeFormatBlockingFactoryLoadOperatorErrorForNamedFactory(err, projectRoot, globalRoot, cfg.NamedFactoryName)
	}
	cfg.Dir = resolution.FactoryDir
	cfg.NamedFactoryResolution = resolution
	return nil
}

func resolveRunFactoryPrompt(cmd *cobra.Command, cfg *runcli.RunConfig, promptArgs []string) error {
	factoryChanged := cmd.Flags().Changed("factory")
	namedChanged := cmd.Flags().Changed("named")
	workChanged := cmd.Flags().Changed("work")

	if !factoryChanged && !namedChanged {
		return resolveLegacyRunFactoryPrompt(cmd, promptArgs)
	}
	if len(promptArgs) == 0 && runCommandInputIsTTY(cmd.InOrStdin()) {
		return nil
	}

	signature, err := runcli.ResolveFactoryInvocationSignature(cfg.Dir)
	if err != nil {
		return err
	}
	if signature != nil {
		return resolveSignatureRunFactoryPrompt(cmd, cfg, promptArgs, signature)
	}
	return resolveCompatibilityRunFactoryPrompt(cmd, cfg, promptArgs, workChanged)
}

func resolveLegacyRunFactoryPrompt(cmd *cobra.Command, promptArgs []string) error {
	for _, arg := range promptArgs {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}
	input, err := runcli.ResolveFactoryInvocationInput(runcli.FactoryInvocationInputConfig{
		PromptArgs: promptArgs,
		Stdin:      cmd.InOrStdin(),
	})
	if err != nil {
		return err
	}
	if input.Payload != "" {
		return fmt.Errorf("positional prompt arguments require --factory or --named")
	}
	return nil
}

func resolveSignatureRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	signature *interfaces.InvocationSignatureConfig,
) error {
	normalized, err := runcli.ResolveSignatureFactoryInvocationInput(runcli.SignatureFactoryInvocationInputConfig{
		PromptArgs: promptArgs,
		Signature:  signature,
		Stdin:      cmd.InOrStdin(),
	})
	if err != nil {
		return err
	}
	cfg.InvocationNormalizedArguments = &normalized
	return nil
}

func resolveCompatibilityRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	workChanged bool,
) error {
	input, err := runcli.ResolveFactoryInvocationInput(runcli.FactoryInvocationInputConfig{
		PromptArgs: promptArgs,
		Stdin:      cmd.InOrStdin(),
	})
	if err != nil {
		return err
	}
	if workChanged && input.Payload != "" {
		return fmt.Errorf("%s cannot be used with --work", input.Source)
	}
	if workChanged {
		cfg.CleanInvocationInputSource = runcli.InvocationInputSourceWorkFile
	}
	if input.Payload == "" {
		return nil
	}
	assignCompatibilityInvocationInput(cfg, input)
	return nil
}

func assignCompatibilityInvocationInput(cfg *runcli.RunConfig, input runcli.FactoryInvocationInput) {
	payload := input.Payload
	switch input.Source {
	case runcli.InvocationInputSourcePositional:
		cfg.InvocationPositionalText = &payload
	case runcli.InvocationInputSourceStdin:
		cfg.InvocationStdinText = &payload
	}
	cfg.CleanInvocationInputSource = input.Source
}

func runCommandInputIsTTY(stdin io.Reader) bool {
	if stdin != nil && stdin != os.Stdin {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
