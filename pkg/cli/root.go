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
	configcli "github.com/portpowered/infinite-you/pkg/cli/config"
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/spf13/cobra"
)

var runCLI = runcli.Run
var flattenFactoryConfig = configcli.FlattenFactoryConfig
var expandFactoryConfig = configcli.ExpandFactoryConfig
var initFactory = initcmd.Init
var submitWork = submitcli.Submit
var listWork = workcli.List
var listSessions = sessioncli.List
var createSession = sessioncli.Create
var deleteSession = sessioncli.Delete
var queryFactory = factorycli.Query
var listFactories = factorycli.List
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
func NewRootCommand() *cobra.Command {
	diagnostics := &cliDiagnosticsOptions{}
	root := &cobra.Command{
		Use:          cliBinaryName,
		Short:        "Run and manage CPN-based workflow factories",
		SilenceUsage: true,
		Long: "Run and manage CPN-based workflow factories.\n\n" +
			"Running " + cliBinaryName + " with no arguments starts the out-of-the-box flow: " +
			"it prepares ./factory when needed, keeps the runtime alive in continuous mode, " +
			"watches factory/inputs/task/default for Markdown or JSON task files, and reports " +
			"the local dashboard at the first available port, preferring http://localhost:7437/dashboard/ui.\n\n" +
			"Default command output is customer-facing. Use --verbose for concise troubleshooting context; " +
			"--debug enables lower-level diagnostics where supported and implies --verbose. Diagnostics " +
			"use stderr so JSON stdout remains parseable, and must not include full prompts, " +
			"full work payloads, access tokens, full model input text, full successful response bodies, or sensitive generated content.\n\n" +
			"Packaged reference topics are also available through " + cliBinaryName + " docs <topic>. " +
			"Supported docs topics: " + supportedDocsTopicsHelpText() + ".",
		Example: "  # Start the default Codex-backed factory in the current project.\n" +
			"  " + cliBinaryName + "\n\n" +
			"  # In another terminal, submit a Markdown task to the default scaffold.\n" +
			"  printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md\n\n" +
			"  # Print the packaged workstations reference page from the installed binary.\n" +
			"  " + cliBinaryName + " docs workstations\n\n" +
			"  # Explicit batch-style runs are still available when you need them.\n" +
			"  " + cliBinaryName + " run --dir factory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFactory(cmd, defaultcmd.OOTBRunConfig(), nil, diagnostics.verboseEnabled(), diagnostics.debug)
		},
	}
	root.PersistentFlags().BoolVarP(&diagnostics.verbose, "verbose", "v", false, "emit concise command diagnostics to stderr")
	root.PersistentFlags().BoolVarP(&diagnostics.debug, "debug", "d", false, "emit lower-level command diagnostics where supported (implies --verbose)")

	root.AddCommand(
		newConfigCommand(diagnostics),
		newDocsCommand(diagnostics),
		newFactoryCommand(diagnostics),
		newInitCommand(diagnostics),
		newModelsCommand(diagnostics),
		newRunCommand(diagnostics),
		newSubmitCommand(diagnostics),
		newSessionCommand(diagnostics),
		newWorkCommand(diagnostics),
	)

	return root
}

type cliDiagnosticsOptions struct {
	verbose bool
	debug   bool
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

func newFactoryCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	factoryCmd := &cobra.Command{
		Use:   "factory",
		Short: "Inspect and manage factory definitions",
		Long: "Inspect live factory runtime state and manage persisted named factories.\n\n" +
			"Subcommands:\n" +
			"  query   show the current active factory from a running service\n" +
			"  list    list persisted named factories under a factory root\n" +
			"  save    create a named factory from factory.json or persist the live current factory\n" +
			"  update  replace an existing named factory from factory.json\n" +
			"  delete  remove an unused named factory from disk\n\n" +
			"Use query against a running service. Use list, save, update, and delete for on-disk " +
			"named factories under --dir (default factory/). Live save with no name argument uses " +
			"--port and --session like query.",
		Example: "  # Show the active factory from the running service.\n" +
			"  " + cliBinaryName + " factory query\n\n" +
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
		newFactoryQueryCommand(diagnostics),
		newFactoryListCommand(diagnostics),
		newFactorySaveCommand(diagnostics),
		newFactoryUpdateFromFileCommand(diagnostics),
		newFactoryDeleteCommand(diagnostics),
	)
	return factoryCmd
}

func newFactoryDeleteCommand(_ *cliDiagnosticsOptions) *cobra.Command {
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
			cfg.Output = cmd.OutOrStdout()
			return deleteFactory(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	return cmd
}

func newFactoryUpdateFromFileCommand(_ *cliDiagnosticsOptions) *cobra.Command {
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
			"  " + cliBinaryName + " factory update staging --from ./factory.json --json",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Name = args[0]
			cfg.Output = cmd.OutOrStdout()
			return updateFactoryFromFile(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.From, "from", "", "path to an existing factory.json payload (required)")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit structured confirmation JSON")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func newFactorySaveCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	fileCfg := factorycli.SaveFromFileConfig{Dir: defaultcmd.FactoryDir}
	liveCfg := factorycli.SaveCurrentConfig{Port: defaultcmd.FactoryPort}

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
			"  " + cliBinaryName + " factory save --session session-beta --json",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				liveCfg.Output = cmd.OutOrStdout()
				liveCfg.Diagnostics = diagnostics.writer(cmd)
				liveCfg.Verbose = diagnostics.verboseEnabled()
				liveCfg.JSON = fileCfg.JSON // shared --json flag
				return saveFactoryCurrent(liveCfg)
			}
			fileCfg.Name = args[0]
			fileCfg.Output = cmd.OutOrStdout()
			return saveFactoryFromFile(fileCfg)
		},
	}

	cmd.Flags().StringVar(&fileCfg.From, "from", "", "path to an existing factory.json payload (required with <name>)")
	cmd.Flags().StringVar(&fileCfg.Dir, "dir", fileCfg.Dir, "factory root directory containing named factories")
	cmd.Flags().BoolVar(&fileCfg.SetCurrent, "set-current", false, "update .current-factory to the saved name")
	cmd.Flags().IntVar(&liveCfg.Port, "port", liveCfg.Port, "HTTP server port for live save")
	cmd.Flags().StringVar(&liveCfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	cmd.Flags().BoolVar(&fileCfg.JSON, "json", false, "emit structured confirmation JSON")
	return cmd
}

func newFactoryListCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.ListConfig{Dir: defaultcmd.FactoryDir}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List persisted named factories",
		Long: "List persisted named factories stored under a factory root.\n\n" +
			"By default the command writes a human-readable table with each factory name, " +
			"on-disk directory, and whether it is selected by .current-factory. " +
			"Use --dir to scope discovery to a different factory root and --json for scripting output.",
		Example: "  # List named factories under the default factory root.\n" +
			"  " + cliBinaryName + " factory list\n\n" +
			"  # List factories from a custom root as JSON.\n" +
			"  " + cliBinaryName + " factory list --dir my-factory --json",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			return listFactories(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory root directory containing named factories")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit persisted factories as a JSON array")
	return cmd
}

func newFactoryQueryCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := factorycli.QueryConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Show the current active factory",
		Long: "Show the current active factory from a running you-agent-factory service.\n\n" +
			"By default the command writes a human-readable table with the current factory name and " +
			"runtime-identifying fields. Use --json for the API-shaped current-factory payload, and " +
			"use --port to target the same server-port selection pattern as work list. Run " +
			cliBinaryName + " session list to discover live session ids when routing other commands with --session.",
		Example: "  # Show the current factory from the running service on the default port.\n" +
			"  " + cliBinaryName + " factory query\n\n" +
			"  # Emit API-shaped JSON for automation from the default local service.\n" +
			"  " + cliBinaryName + " factory query --json\n\n" +
			"  # Query a different service port when your runtime is not on the default port.\n" +
			"  " + cliBinaryName + " factory query --port 9090 --json",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return queryFactory(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API current-factory JSON response")
	return cmd
}

func newSessionCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "List, open, and close live factory sessions on a running host",
		Long: "Manage live factory sessions on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list    list live factory sessions from GET /factory-sessions\n" +
			"  create  open another live session from a folder path\n" +
			"  delete  close a live session by session id\n\n" +
			"Session commands use the same default --port as work list. Use --json to emit API-shaped " +
			"responses on stdout; diagnostics stay on stderr when --verbose or --debug is set.",
		Example: "  # List live sessions on the default local port.\n" +
			"  " + cliBinaryName + " session list\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " session list --json\n\n" +
			"  # Open and close sessions on a non-default port.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --port 9090\n" +
			"  " + cliBinaryName + " session delete session-beta --port 9090 --json\n\n" +
			"  # Target a different service port for list output.\n" +
			"  " + cliBinaryName + " session list --port 9090",
	}
	sessionCmd.AddCommand(
		newSessionListCommand(diagnostics),
		newSessionCreateCommand(diagnostics),
		newSessionDeleteCommand(diagnostics),
	)
	return sessionCmd
}

func newSessionListCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.ListConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List live factory sessions",
		Long: "List every live factory session that the running host currently has open.\n\n" +
			"Human output prints session id, project, folder path, factory dir, default marker, and " +
			"target kind/name. Use --json to emit ListFactorySessionsResponse on stdout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessions(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API list-factory-sessions JSON response")
	return cmd
}

func newSessionCreateCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.CreateConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Open a live factory session from a folder",
		Long: "Open another live factory session from a folder path via POST /factory-sessions.\n\n" +
			"Use --dir to provide the folder path. Optional --init-new-factory and --validate-only " +
			"map to the API request fields and are mutually exclusive. Use --target-kind and --target-name " +
			"when the folder exposes multiple runnable targets. Use --json to emit OpenFactorySessionResponse " +
			"on stdout; diagnostics stay on stderr when --verbose or --debug is set.",
		Example: "  # Open a session for an existing factory folder.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet\n\n" +
			"  # Validate a folder without opening a live session.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --validate-only\n\n" +
			"  # Open a named target inside a multi-factory folder.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --target-kind named --target-name beta",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return createSession(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", "", "folder path to open as a live factory session")
	cmd.Flags().BoolVar(&cfg.InitNewFactory, "init-new-factory", false, "write the default init scaffold at --dir and open a live session")
	cmd.Flags().BoolVar(&cfg.ValidateOnly, "validate-only", false, "validate the folder and optional target without creating a live session")
	cmd.Flags().StringVar(&cfg.TargetKind, "target-kind", "", "target kind when disambiguating runnable factories (default or named)")
	cmd.Flags().StringVar(&cfg.TargetName, "target-name", "", "named target when --target-kind is named")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API open-factory-session JSON response")
	_ = cmd.MarkFlagRequired("dir")
	cmd.MarkFlagsMutuallyExclusive("init-new-factory", "validate-only")
	return cmd
}

func newSessionDeleteCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.DeleteConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Close a live factory session",
		Long: "Close one live factory session via DELETE /factory-sessions/{session_id}.\n\n" +
			"Use the same --port selection as session list. Use --json to emit a JSON confirmation on stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.SessionID = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return deleteSession(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit a JSON confirmation after the session closes")
	return cmd
}

func newWorkCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	workCmd := &cobra.Command{
		Use:   "work",
		Short: "Inspect work from a running factory",
	}
	workCmd.AddCommand(newWorkListCommand(diagnostics))
	return workCmd
}

func newModelsCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "Inspect discovered models from a running service",
		Long: "Inspect discovered models from a running infinite-you service.\n\n" +
			"Use list to discover model identifiers, inspect to view one model's readiness and capabilities, " +
			"invoke to call a discovered model directly through the same /models contract exposed by the API, " +
			"and pull to populate the managed local-model cache for supported local assets.",
	}
	modelsCmd.AddCommand(newModelsListCommand(diagnostics), newModelsInspectCommand(diagnostics), newModelsInvokeCommand(diagnostics), newModelsPullCommand(diagnostics))
	return modelsCmd
}

func newModelsListCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.ListConfig{Port: defaultcmd.FactoryPort}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List discovered models",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listModels(cfg)
		},
	}
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API list-models JSON response")
	return cmd
}

func newModelsInspectCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.InspectConfig{Port: defaultcmd.FactoryPort}
	cmd := &cobra.Command{
		Use:   "inspect <model-name>",
		Short: "Inspect one discovered model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ModelName = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return inspectModel(cfg)
		},
	}
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API model-detail JSON response")
	return cmd
}

func newModelsInvokeCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.InvokeConfig{Port: defaultcmd.FactoryPort, Operation: "TTS"}
	cmd := &cobra.Command{
		Use:   "invoke <model-name>",
		Short: "Invoke one discovered model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ModelName = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return invokeModel(cfg)
		},
	}
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.Operation, "operation", cfg.Operation, "uppercase provider-agnostic operation name")
	cmd.Flags().StringVar(&cfg.Text, "text", "", "text input for direct invocation")
	cmd.Flags().StringVar(&cfg.OutputPath, "output", "", "output file path for streamed audio responses")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API model-invocation JSON metadata response")
	return cmd
}

func newModelsPullCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := modelscli.PullConfig{Port: defaultcmd.FactoryPort}
	cmd := &cobra.Command{
		Use:   "pull <model-name>",
		Short: "Pull one discovered local model into the managed cache",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.ModelName = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return pullModel(cfg)
		},
	}
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API model-pull JSON response")
	return cmd
}

func newWorkListCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := workcli.ListConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List work from a running factory",
		Long: "List work from a running you-agent-factory service.\n\n" +
			"By default the command targets the default compatibility session. " +
			"Use --session to route the request to one specific live factory session instead. " +
			"Run " + cliBinaryName + " session list to discover live session ids.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listWork(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.StateName, "state-name", "", "filter by current state name")
	cmd.Flags().StringVar(&cfg.StateType, "state-type", "", "filter by current state type (INITIAL, PROCESSING, TERMINAL, FAILED)")
	cmd.Flags().StringVar(&cfg.SortBy, "sort-by", "", "sort returned work by field (state.type)")
	cmd.Flags().IntVar(&cfg.MaxResults, "max-results", 0, "maximum work items to return")
	cmd.Flags().StringVar(&cfg.NextToken, "next-token", "", "pagination cursor returned by a previous work list response")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API list-work JSON response")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newDocsCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	docsCmd := &cobra.Command{
		Use:          "docs [topic]",
		Short:        "Print packaged markdown reference topics",
		SilenceUsage: true,
		Long: "Print packaged markdown reference topics from the installed binary.\n\n" +
			"Run without a topic to print the packaged docs index. Use one supported topic argument to print the authored markdown page with no wrapper formatting.",
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

func supportedDocsTopicsHelpText() string {
	return strings.Join(docscli.SupportedTopics(), ", ")
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

func newInitCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
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

func newRunCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := defaultcmd.ExplicitRunConfig()

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Load workflow and run the factory engine",
		Long: "Load workflow and run the factory engine.\n\n" +
			"For the quickest local setup, run " + cliBinaryName + " with no arguments. " +
			"That default flow bootstraps ./factory, watches factory/inputs/task/default, " +
			"keeps the runtime alive, and reports the first available dashboard URL, preferring http://localhost:7437/dashboard/ui. " +
			"Default execution uses batch mode and exits after idle completion. " +
			"Normal live runs record by default unless you pass --no-record. " +
			"Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata. " +
			"Use --runner to override the factory-level runner for this run while still allowing workstation-specific runner overrides to win. " +
			"Use --continuously to keep the factory alive while idle until you cancel it. " +
			"Use --with-mock-workers with an optional JSON config path to test workflows with deterministic mock worker outcomes. " +
			"Use --quiet to suppress dashboard output for scripted or CI-oriented runs. " +
			"Use --factory with a factory.json file path to run a portable factory config without guessing --dir. " +
			"Runtime logs are structured JSON rolling files grouped by UTC start date under the selected log root; environment details are record-channel diagnostics only, and system logs include command stdout/stderr only on command failures.",
		Example: "  # Start the out-of-the-box continuous factory.\n" +
			"  " + cliBinaryName + "\n\n" +
			"  # Submit a Markdown task to the default scaffold.\n" +
			"  printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md\n\n" +
			"  # Run an existing factory once in explicit batch mode.\n" +
			"  " + cliBinaryName + " run --dir factory\n\n" +
			"  # Run a portable factory.json with a one-shot prompt (see handlingBehavior DEFAULT).\n" +
			"  " + cliBinaryName + " run --factory ./factory.json \"Fix the lint issues\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.MockWorkersEnabled = cmd.Flags().Changed("with-mock-workers")
			if cmd.Flags().Changed("port") {
				cfg.AutoPort = false
			}
			promptArgs := args
			if cfg.MockWorkersConfigPath == defaultMockWorkersConfigPathSentinel {
				if len(args) > 0 {
					cfg.MockWorkersConfigPath = args[0]
					promptArgs = args[1:]
				} else {
					cfg.MockWorkersConfigPath = ""
				}
			}
			return runFactory(cmd, cfg, promptArgs, diagnostics.verboseEnabled(), diagnostics.debug)
		},
	}

	cmd.Flags().StringVar(&cfg.Workflow, "workflow", "", "workflow ID to run (default: all)")
	cmd.Flags().BoolVar(&cfg.Continuously, "continuously", false, "keep the factory alive while idle until cancelled")
	cmd.Flags().StringVar(&cfg.WorkFile, "work", "", "path to initial FACTORY_REQUEST_BATCH JSON file to submit")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory base directory")
	cmd.Flags().StringVar(&cfg.FactoryConfigPath, "factory", "", "path to factory.json for portable one-shot runs")
	cmd.Flags().StringVar(&cfg.RunnerID, "runner", "", fmt.Sprintf("factory-level runner override (%s)", strings.Join([]string{
		interfaces.RunnerIDCodex,
		interfaces.RunnerIDGemini,
		interfaces.RunnerIDKiro,
		interfaces.RunnerIDCursorCLI,
		interfaces.RunnerIDOpenCode,
	}, ", ")))
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port; specifying this flag disables automatic fallback")
	cmd.Flags().StringVar(&cfg.RecordPath, "record", "", "path to write a replay artifact for this run; replay artifacts are sensitive, and default live runs record automatically unless --no-record is used")
	cmd.Flags().BoolVar(&cfg.DisableDefaultRecording, "no-record", false, "disable the default replay artifact for this invocation")
	cmd.Flags().StringVar(&cfg.ReplayPath, "replay", "", "path to replay an existing sensitive replay artifact")
	cmd.Flags().StringVar(&cfg.RuntimeLogDir, "runtime-log-dir", "", "root directory for structured runtime log files grouped by UTC start date (default: ~/.you-agent-factory/logs)")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxSize, "runtime-log-max-size-mb", cfg.RuntimeLogConfig.MaxSize, "rotate each runtime log file after this many megabytes")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxBackups, "runtime-log-max-backups", cfg.RuntimeLogConfig.MaxBackups, "maximum rotated runtime log files to retain")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxAge, "runtime-log-max-age-days", cfg.RuntimeLogConfig.MaxAge, "maximum days to retain rotated runtime log files")
	cmd.Flags().BoolVar(&cfg.RuntimeLogConfig.Compress, "runtime-log-compress", false, "compress rotated runtime log files")
	cmd.Flags().StringVar(&cfg.MockWorkersConfigPath, "with-mock-workers", "", "enable mock-worker execution with an optional mock-workers JSON config path")
	cmd.Flags().Lookup("with-mock-workers").NoOptDefVal = defaultMockWorkersConfigPathSentinel
	cmd.Flags().BoolVar(&cfg.SuppressDashboardRendering, "quiet", false, "suppress dashboard output for quiet or CI-oriented runs")
	return cmd
}

func runFactory(cmd *cobra.Command, cfg runcli.RunConfig, promptArgs []string, verbose, debug bool) error {
	if err := resolveRunFactorySelection(cmd, &cfg); err != nil {
		return err
	}
	if err := resolveRunFactoryPrompt(cmd, &cfg, promptArgs); err != nil {
		return err
	}

	logger, err := logging.BuildLogger(verbose, debug)
	if err != nil {
		return err
	}
	cfg.Logger = logger
	cfg.Verbose = verbose || debug
	cfg.StartupOutput = cmd.OutOrStdout()
	cfg.Diagnostics = cmd.ErrOrStderr()

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

func resolveRunFactorySelection(cmd *cobra.Command, cfg *runcli.RunConfig) error {
	factoryChanged := cmd.Flags().Changed("factory")
	dirChanged := cmd.Flags().Changed("dir")
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

func resolveRunFactoryPrompt(cmd *cobra.Command, cfg *runcli.RunConfig, promptArgs []string) error {
	factoryChanged := cmd.Flags().Changed("factory")
	workChanged := cmd.Flags().Changed("work")
	prompt := strings.TrimSpace(strings.Join(promptArgs, " "))

	if !factoryChanged {
		if prompt != "" {
			return fmt.Errorf("positional prompt arguments require --factory")
		}
		return nil
	}
	if workChanged && prompt != "" {
		return fmt.Errorf("positional prompt arguments cannot be used with --work")
	}
	if len(promptArgs) > 0 && prompt == "" {
		return fmt.Errorf("prompt is required for you run --factory")
	}
	if prompt == "" {
		return nil
	}

	workFile, err := runcli.PrepareFactoryPromptWorkFile(cfg.FactoryConfigPath, prompt)
	if err != nil {
		return err
	}
	cfg.WorkFile = workFile
	return nil
}

func newSubmitCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := submitcli.SubmitConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit work to a running factory",
		Long: "Submit work to a running you-agent-factory service.\n\n" +
			"By default the command submits to the default compatibility session. " +
			"Use --session to submit to one specific live factory session instead.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return submitWork(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Name, "name", "", "authored request name for the submitted work (required)")
	cmd.Flags().StringVar(&cfg.WorkTypeName, "work-type-name", "", "work type name to submit to (required)")
	cmd.Flags().StringVar(&cfg.Payload, "payload", "", "path to payload file (.json or .md) (required)")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}
