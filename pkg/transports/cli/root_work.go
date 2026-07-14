package cli

import (
	"errors"
	"fmt"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

// useGeneratedRepresentativeFamily toggles production root wiring between the
// generated representative-family constructor and the legacy handwritten path.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedRepresentativeFamily = true

func newLegacyRootCommandWithOptions(options RootCommandOptions) *cobra.Command {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	docsCmd := newDocsCommand(diagnostics)
	modelsCmd := newModelsCommand(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(productionRootSubcommands(globals, diagnostics, operatorDefaults, options, newSessionCommand(globals, diagnostics, options), docsCmd, modelsCmd)...)
	return root
}

// NewLegacyRepresentativeFamilyCommand builds the isolated handwritten
// you → session → show tree used by the generator-vs-legacy parity matrix.
func NewLegacyRepresentativeFamilyCommand() *cobra.Command {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(newLegacyRepresentativeSessionCommand(globals, diagnostics))
	return root
}

func newLegacyRootCommandShell(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
) *cobra.Command {
	root := &cobra.Command{
		Use:          cliBinaryName,
		Short:        "Run and manage CPN-based workflow factories",
		SilenceUsage: true,
		Long: "Run and manage CPN-based workflow factories.\n\n" +
			"What:\n" +
			"CPN-based workflow factory CLI for running factories, submitting work, and inspecting live sessions.\n\n" +
			"How to use:\n" +
			"Run " + cliBinaryName + " run --work ./docs/examples/startup-work.json to start the current Factory with explicit Work and the local dashboard (http://localhost:7437/dashboard/ui).\n" +
			"Use " + cliBinaryName + " run --dir factory --work ./docs/examples/startup-work.json for an explicit Factory directory. See " + cliBinaryName + " <cmd> --help for subcommand details.\n\n" +
			"Agents:\n" +
			"Start with " + cliBinaryName + " docs agents for orientation, " + cliBinaryName + " submit or " + cliBinaryName + " submit batch to enqueue work, and " + cliBinaryName + " session list to confirm a live factory.\n" +
			"Run " + cliBinaryName + " docs for all packaged reference topics. Use --verbose or --debug for stderr diagnostics; full policy in " + cliBinaryName + " docs.",
		Example: "  # Start the default Codex-backed Factory with explicit Work.\n" +
			"  " + cliBinaryName + " run --work ./docs/examples/startup-work.json\n\n" +
			"  # Agent orientation and command matrix.\n" +
			"  " + cliBinaryName + " docs agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			policy := diagnostics.resolvePolicy(false)
			return runFactoryWithOptions(cmd, defaultcmd.OOTBRunConfig(), nil, globals, operatorDefaults, policy, options, true)
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
	return root
}

func newLegacyRepresentativeSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	sessionCmd := legacySessionParentCommand()
	sessionCmd.AddCommand(newSessionShowCommand(globals, diagnostics))
	return sessionCmd
}

func newRootCommandWithGeneratedRepresentativeFamily(options RootCommandOptions) *cobra.Command {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}

	registry, err := newRepresentativeHandlerRegistry(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build representative handler registry: %v", err))
	}
	components, err := climanifestcobra.NewRepresentativeFamilyComponents(
		registry,
		representativePersistentFlagBindings(globals, diagnostics, operatorDefaults),
	)
	if err != nil {
		panic(fmt.Sprintf("build representative family command: %v", err))
	}

	session := components.Session
	session.AddCommand(handwrittenSessionSubcommands(globals, diagnostics, options, components.Show)...)

	docsCmd, modelsCmd, err := newProductionModelsDocsCommands(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build models/docs family command: %v", err))
	}

	root := components.Root
	root.AddCommand(productionRootSubcommands(globals, diagnostics, operatorDefaults, options, session, docsCmd, modelsCmd)...)
	return root
}

func productionRootSubcommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
	session *cobra.Command,
	docsCmd *cobra.Command,
	modelsCmd *cobra.Command,
) []*cobra.Command {
	return []*cobra.Command{
		docsCmd,
		configinitcmd.NewSystemConfigCommand(cliBinaryName, configinitcmd.CommandGlobals{
			JSON:    func() bool { return globals.json },
			HomeDir: options.HomeDir,
		}, configinitcmd.CommandDiagnostics{
			Writer:  diagnostics.writer,
			Verbose: diagnostics.verboseEnabled,
		}),
		newFactoryCommand(globals, diagnostics),
		newInitCommand(globals, diagnostics),
		newMCPCommand(options),
		modelsCmd,
		newRunCommand(globals, diagnostics, operatorDefaults, options),
		newSubmitCommand(globals, diagnostics),
		session,
		newWorkCommand(globals, diagnostics),
		newWorkflowCommand(globals, diagnostics, options),
	}
}

func representativePersistentFlagBindings(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
) climanifestcobra.PersistentFlagBindings {
	return climanifestcobra.PersistentFlagBindings{
		Verbose:                    &diagnostics.verbose,
		Debug:                      &diagnostics.debug,
		Server:                     &globals.server,
		JSON:                       &globals.json,
		DefaultWorkerModelProvider: &operatorDefaults.defaultWorkerModelProvider,
		DefaultWorkerModel:         &operatorDefaults.defaultWorkerModel,
		FlagUsages: map[string]string{
			"verbose": "emit concise command diagnostics to stderr",
			"debug":   "emit lower-level command diagnostics where supported (implies --verbose)",
			"server":  "factory API base URI (http:// or https://); HTTP client commands target this URI and you run binds locally to its host and port",
			"json":    "emit structured JSON on stdout for supported commands; diagnostics remain on stderr",
			"default-worker-model-provider": fmt.Sprintf(
				"default worker model provider for model workers with omitted modelProvider (%s; DEFAULT resolves through lower-precedence concrete provider)",
				interfaces.AcceptedPublicWorkerModelProviderSummary(),
			),
			"default-worker-model": "default worker model for model workers with omitted model",
		},
	}
}

func handwrittenSessionSubcommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options RootCommandOptions,
	generatedShow *cobra.Command,
) []*cobra.Command {
	return []*cobra.Command{
		newSessionListCommand(diagnostics, options),
		generatedShow,
		newSessionDispatchesCommand(globals, diagnostics),
		newSessionPauseCommand(globals, diagnostics),
		newSessionResumeCommand(globals, diagnostics),
		newSessionCreateCommand(diagnostics),
		newSessionDeleteCommand(diagnostics),
	}
}

func newRunCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions RootCommandOptions) *cobra.Command {
	cfg := defaultcmd.ExplicitRunConfig()
	var invocationOutputMode string
	cmd := &cobra.Command{
		Use:                "run",
		Short:              "Load workflow and run the factory engine",
		DisableFlagParsing: true,
		SilenceErrors:      true,
		Long:               runCommandLongHelp(),
		Example:            runCommandExamples(),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return rejectDeprecatedPortFlag(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeRunCommand(cmd, args, &cfg, globals, diagnostics, operatorDefaults, rootOptions)
		},
	}
	registerRunCommandFlags(cmd, &cfg, &invocationOutputMode)
	return cmd
}

func executeRunCommand(cmd *cobra.Command, args []string, cfg *runcli.RunConfig, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions RootCommandOptions) error {
	promptArgs, resolvedConfig, err := resolveRunCommandInvocationInput(cmd, args, cfg)
	if err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if err := applyRunCommandInvocationOutputMode(cmd, &resolvedConfig); err != nil {
		return err
	}
	if helpRequested(cmd) {
		return writeRunCommandHelp(cmd, &resolvedConfig, rootOptions)
	}
	basePolicy := diagnostics.resolvePolicy(resolvedConfig.SuppressDashboardRendering)
	err = runFactoryWithOptions(cmd, resolvedConfig, promptArgs, globals, operatorDefaults, basePolicy, rootOptions, false)
	if err != nil {
		err = factoryconfig.MaybeFormatBlockingFactoryLoadOperatorError(err, resolvedConfig.Dir)
		errorWriter := resolveEffectiveRunPolicy(cmd, resolvedConfig, basePolicy).HumanTerminalWriter(cmd.ErrOrStderr())
		var ambiguousInputErr *runcli.AmbiguousInvocationInputError
		if errors.As(err, &ambiguousInputErr) {
			errorWriter = cmd.ErrOrStderr()
		}
		if !runcli.WriteInvocationError(errorWriter, err, globals.json) {
			if errorWriter != nil {
				_, _ = fmt.Fprintln(errorWriter, err)
			}
		}
	}
	return err
}

func applyRunCommandInvocationOutputMode(cmd *cobra.Command, cfg *runcli.RunConfig) error {
	if !cmd.Flags().Changed("output") {
		return nil
	}
	normalized, err := runcli.NormalizeInvocationOutputMode(cmd.Flag("output").Value.String())
	if err != nil {
		return err
	}
	cfg.InvocationOutputMode = normalized
	return nil
}

func resolveRunCommandInvocationInput(cmd *cobra.Command, args []string, cfg *runcli.RunConfig) ([]string, runcli.RunConfig, error) {
	args, err := parseRunCommandArgs(cmd, args)
	if err != nil {
		return nil, *cfg, err
	}
	if err := rejectDeprecatedPortFlag(cmd, nil); err != nil {
		return nil, *cfg, err
	}
	cfg.MockWorkersEnabled = cmd.Flags().Changed("with-mock-workers")
	if cmd.Flags().Changed("skip-permissions") {
		override := true
		cfg.InvocationSkipPermissionsOverride = &override
	}
	promptArgs := args
	if cfg.MockWorkersConfigPath != defaultMockWorkersConfigPathSentinel {
		return promptArgs, *cfg, nil
	}
	if len(args) == 0 {
		cfg.MockWorkersConfigPath = ""
		return promptArgs, *cfg, nil
	}
	cfg.MockWorkersConfigPath = args[0]
	return args[1:], *cfg, nil
}

func helpRequested(cmd *cobra.Command) bool {
	helpFlag := cmd.Flags().Lookup("help")
	return helpFlag != nil && helpFlag.Changed
}

func writeRunCommandHelp(cmd *cobra.Command, cfg *runcli.RunConfig, rootOptions RootCommandOptions) error {
	homeDir, err := rootOptions.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve process home directory: %w", err)
	}
	cfg.HomeDir = homeDir
	if err := resolveRunFactorySelection(cmd, cfg, homeDir); err != nil {
		return err
	}
	wroteFactoryHelp, err := runcli.WriteFactoryInvocationHelp(cmd.OutOrStdout(), cliBinaryName, *cfg)
	if err != nil {
		return err
	}
	if wroteFactoryHelp {
		return nil
	}
	return cmd.Help()
}

func registerRunCommandFlags(cmd *cobra.Command, cfg *runcli.RunConfig, invocationOutputMode *string) {
	registerDeprecatedPortFlag(cmd)
	cmd.Flags().BoolVar(&cfg.Continuously, "continuously", false, "keep the factory alive while idle until cancelled")
	cmd.Flags().StringVar(&cfg.WorkFile, "work", "", "path to initial FACTORY_REQUEST_BATCH JSON file to submit")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory base directory")
	cmd.Flags().StringVar(&cfg.NamedFactoryName, "named", "", "canonical persisted factory name resolved from ./factory before ~/.you-agent-factory/you-agent-factories; built-ins materialize there on first use and remain editable")
	cmd.Flags().StringVar(&cfg.FactoryConfigPath, "factory", "", "path to factory.json for portable one-shot runs; use positional text or piped stdin for the invocation input")
	cmd.Flags().StringVar(&cfg.RecordPath, "record", "", "path to write a replay artifact for this run; replay artifacts are sensitive, and default live runs record automatically unless --no-record is used")
	cmd.Flags().BoolVar(&cfg.DisableDefaultRecording, "no-record", false, "disable the default replay artifact for this invocation")
	cmd.Flags().StringVar(&cfg.ReplayPath, "replay", "", "path to replay an existing sensitive replay artifact")
	cmd.Flags().StringVar(&cfg.RuntimeLogDir, "runtime-log-dir", "", "root directory for structured runtime log files grouped by UTC start date (default: ~/.you-agent-factory/logs)")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxSize, "runtime-log-max-size-mb", cfg.RuntimeLogConfig.MaxSize, "rotate each runtime log file after this many megabytes")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxBackups, "runtime-log-max-backups", cfg.RuntimeLogConfig.MaxBackups, "maximum rotated runtime log files to retain")
	cmd.Flags().IntVar(&cfg.RuntimeLogConfig.MaxAge, "runtime-log-max-age-days", cfg.RuntimeLogConfig.MaxAge, "maximum days to retain rotated runtime log files")
	cmd.Flags().BoolVar(&cfg.RuntimeLogConfig.Compress, "runtime-log-compress", false, "compress rotated runtime log files")
	cmd.Flags().StringVar(&cfg.RuntimeMetricsDir, "runtime-metrics-dir", "", "root directory for structured runtime metrics JSONL files grouped by UTC start date (default: ~/.you-agent-factory/metrics)")
	cmd.Flags().IntVar(&cfg.RuntimeMetricsConfig.MaxSize, "runtime-metrics-max-size-mb", cfg.RuntimeMetricsConfig.MaxSize, "rotate each runtime metrics file after this many megabytes")
	cmd.Flags().IntVar(&cfg.RuntimeMetricsConfig.MaxBackups, "runtime-metrics-max-backups", cfg.RuntimeMetricsConfig.MaxBackups, "maximum rotated runtime metrics files to retain")
	cmd.Flags().IntVar(&cfg.RuntimeMetricsConfig.MaxAge, "runtime-metrics-max-age-days", cfg.RuntimeMetricsConfig.MaxAge, "maximum days to retain rotated runtime metrics files")
	cmd.Flags().BoolVar(&cfg.RuntimeMetricsConfig.Compress, "runtime-metrics-compress", false, "compress rotated runtime metrics files")
	cmd.Flags().StringVar(&cfg.MockWorkersConfigPath, "with-mock-workers", "", "enable mock-worker execution with an optional mock-workers JSON config path")
	cmd.Flags().Lookup("with-mock-workers").NoOptDefVal = defaultMockWorkersConfigPathSentinel
	cmd.Flags().BoolVar(&cfg.SuppressDashboardRendering, "quiet", false, "suppress dashboard output for quiet or CI-oriented runs")
	cmd.Flags().StringVar(invocationOutputMode, "output", "", "invocation stdout mode: primary (default) or response-stream for live internal session progress on supported one-shot factory runs")
	var skipPermissions bool
	cmd.Flags().BoolVar(&skipPermissions, "skip-permissions", false, "request an invocation-only unsafe permission bypass for agent workers without changing persisted factory configuration")
}

func runCommandLongHelp() string {
	return "Load workflow and run the factory engine.\n\n" +
		"For the quickest local setup, run " + cliBinaryName + " run --work ./docs/examples/startup-work.json. " +
		"That default flow bootstraps ./factory, watches factory/inputs/task/default, " +
		"keeps the runtime alive, and reports the first available dashboard URL, preferring http://localhost:7437/dashboard/ui. " +
		"Default execution uses batch mode and exits after idle completion. " +
		"Normal live runs record by default unless you pass --no-record. " +
		"Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata. " +
		"Use global --default-worker-model-provider and --default-worker-model to set operator-level model defaults for omitted model-worker fields. " +
		"Use --continuously to keep the factory alive while idle until you cancel it. " +
		"Use --with-mock-workers with an optional JSON config path to test workflows with deterministic mock worker outcomes. " +
		"Use --quiet to suppress dashboard output for scripted or CI-oriented runs. " +
		"Use --skip-permissions to request an invocation-only unsafe permission bypass for agent workers without changing persisted factory configuration. " +
		"Use --named with a persisted canonical factory name to resolve project-local factories before global built-ins under ~/.you-agent-factory/you-agent-factories. " +
		"Built-ins such as @you/tts and @you/goal materialize lazily into that global root on first use and stay editable on disk for later runs. " +
		"Use --factory with a factory.json file path to run a portable factory config without guessing --dir. " +
		"Supported run factory selectors are --dir, --named, and --factory; dynamic workflow source selection stays under " + cliBinaryName + " workflow. " +
		"Selected factories can define custom invocation arguments; run " + cliBinaryName + " run --named <factory> --help or " + cliBinaryName + " run --factory <factory.json> --help to inspect signature-backed usage while keeping existing run-level flags available. " +
		"In factory invocation mode, provide either trailing positional text or piped stdin text; supplying both is rejected with INVOCATION_INPUT_SOURCE_CONFLICT. " +
		"Named-Factory selection and materialization live in " + cliBinaryName + " docs authoring-factories; invocation inputs and output modes live in " + cliBinaryName + " docs run and " + cliBinaryName + " docs sessions. " +
		"Model readiness, direct TTS invocation, and audio or JSON result choices live in " + cliBinaryName + " docs models. " +
		"Supported one-shot factory invocations use primary-result-only stdout by default; use --output response-stream to render live internal session response-stream progress while the CLI owns the runtime; unsupported run shapes fall back to primary-result-only output or return INVOCATION_OUTPUT_UNSUPPORTED. " +
		"Runtime logs are structured JSON rolling files grouped by UTC start date under the selected log root. " +
		"Runtime metrics are a separate structured JSONL operational channel with their own rolling files and do not replace runtime logs. " +
		"Environment details are record-channel diagnostics only, and system logs include command stdout/stderr only on command failures."
}

func runCommandExamples() string {
	return "  # Start the current Factory with explicit Work.\n" +
		"  " + cliBinaryName + " run --work ./docs/examples/startup-work.json\n\n" +
		"  # Run an existing factory once in explicit batch mode.\n" +
		"  " + cliBinaryName + " run --dir factory --work ./docs/examples/startup-work.json\n\n" +
		"  # Run a persisted named factory from any working directory.\n" +
		"  " + cliBinaryName + " run --named @you/tts --output primary \"Read the release summary.\"\n\n" +
		"  # Run a portable factory.json with a one-shot prompt (see handlingBehavior DEFAULT).\n" +
		"  " + cliBinaryName + " run --factory ./factory.json \"Fix the lint issues\"\n\n" +
		"  # Pipe invocation input via stdin (default primary-result stdout).\n" +
		"  echo \"Ship the login bugfix\" | " + cliBinaryName + " run --named @you/goal\n\n" +
		"  # Opt into live internal response-stream progress instead of primary-result-only stdout.\n" +
		"  " + cliBinaryName + " run --named @you/goal --output response-stream \"Ship the login bugfix\""
}

func newWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	workCmd := &cobra.Command{
		Use:   "work",
		Short: "Inspect work from a running factory",
	}
	workCmd.AddCommand(newWorkListCommand(globals, diagnostics))
	workCmd.AddCommand(newWorkShowCommand(globals, diagnostics))
	workCmd.AddCommand(newWorkMoveCommand(globals, diagnostics))
	workCmd.AddCommand(newWorkVisualizeCommand())
	return workCmd
}

func newWorkVisualizeCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "visualize <batch-file.json>",
		Short: "Render a work batch dependency graph as Mermaid",
		Long: "Read a local FACTORY_REQUEST_BATCH JSON file from disk and render its declared work dependencies as a diagram.\n\n" +
			"The required <batch-file.json> argument is a path to any readable batch file. " +
			"Graph nodes represent work items; directed edges represent declared dependency relations from the batch.\n\n" +
			"Output format (default: mermaid):\n" +
			"  mermaid           Raw Mermaid flowchart syntax written to stdout.\n" +
			"  markdown-mermaid  Markdown with a title and one fenced mermaid code block.\n\n" +
			"This command is read-only. It does not submit work, contact a running factory, or render diagram images.\n\n" +
			"Redirect stdout to save output, for example:\n" +
			"  " + cliBinaryName + " work visualize batch.json > my-graph.mermaid\n" +
			"  " + cliBinaryName + " work visualize --format markdown-mermaid batch.json > graph.md",
		Example: "  # Default: raw Mermaid flowchart to stdout.\n" +
			"  " + cliBinaryName + " work visualize batch.json > my-graph.mermaid\n\n" +
			"  # Markdown with embedded Mermaid diagram.\n" +
			"  " + cliBinaryName + " work visualize --format markdown-mermaid batch.json > graph.md",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			return visualizeWork(workcli.VisualizeConfig{
				BatchFile: args[0],
				Format:    format,
				Output:    cmd.OutOrStdout(),
			})
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&format, "format", "mermaid", "output format: mermaid or markdown-mermaid")
	return cmd
}

func newWorkListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := workcli.ListConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List work from a running factory",
		Long: "List work from a running you-agent-factory service.\n\n" +
			"By default the command targets the default compatibility session. " +
			"Use --session to route the request to one specific live factory session instead. " +
			"Run " + cliBinaryName + " session list to discover live session ids.\n\n" +
			"Optional filters are applied on the server before pagination. " +
			"--name matches a case-insensitive substring of the work name. " +
			"--work-type-name matches workTypeName exactly. " +
			"--trace-id matches traceId or currentChainingTraceId exactly. " +
			"Combine filters with --max-results and --next-token to page through the filtered result set.",
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.StateName, "state-name", "", "filter by current state name")
	cmd.Flags().StringVar(&cfg.StateType, "state-type", "", "filter by current state type (INITIAL, PROCESSING, TERMINAL, FAILED)")
	cmd.Flags().StringVar(&cfg.Name, "name", "", "filter by case-insensitive substring of work name (applied before pagination)")
	cmd.Flags().StringVar(&cfg.WorkTypeName, "work-type-name", "", "filter by exact workTypeName (applied before pagination)")
	cmd.Flags().StringVar(&cfg.TraceID, "trace-id", "", "filter by exact traceId or currentChainingTraceId (applied before pagination)")
	cmd.Flags().StringVar(&cfg.SortBy, "sort-by", "", "sort returned work by field (state.type)")
	cmd.Flags().IntVar(&cfg.MaxResults, "max-results", 0, "maximum work items to return per page after server-side filters")
	cmd.Flags().StringVar(&cfg.NextToken, "next-token", "", "pagination cursor returned by a previous work list response")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newWorkShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := workcli.ShowConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "show <work-id>",
		Short: "Show one work item from a running factory",
		Long: "Show one work item from a running you-agent-factory service.\n\n" +
			"By default the command targets the default compatibility session. " +
			"Use --session to route the request to one specific live factory session instead. " +
			"Run " + cliBinaryName + " session list to discover live session ids.\n\n" +
			"After submit, use " + cliBinaryName + " work list --name <name> to find work ids, " +
			"then " + cliBinaryName + " work show <work-id> to verify one item without JSON pagination.",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return showWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newWorkMoveCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := workcli.MoveConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "move <work-id> <state-name>",
		Short: "Move one work item to another authored state",
		Long: "Move one work item to another authored marking state on a running you-agent-factory service.\n\n" +
			"By default the command targets the default compatibility session. " +
			"Use --session to route the request to one specific live factory session instead. " +
			"Run " + cliBinaryName + " session list to discover live session ids.\n\n" +
			"Moves are rejected while the work item is in an active dispatch. " +
			"Repeating the same --request-id after a successful move returns 409 without a second mutation.",
		Args:    cobra.ExactArgs(2),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.StateName = args[1]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return moveWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	cmd.Flags().StringVar(&cfg.RequestID, "request-id", "", "optional client idempotency key for operator moves")
	return cmd
}

func newSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options RootCommandOptions) *cobra.Command {
	sessionCmd := legacySessionParentCommand()
	sessionCmd.AddCommand(handwrittenSessionSubcommands(globals, diagnostics, options, newSessionShowCommand(globals, diagnostics))...)
	return sessionCmd
}

func legacySessionParentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "session",
		Short: "List, open, and close factory sessions on a running host",
		Long: "Manage factory sessions on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list       list live workspace sessions or durable Factory Sessions with --scope live|persisted|all\n" +
			"  show       show one live or durable Factory Session from GET /factory-sessions/{session_id}\n" +
			"  dispatches list durable Factory Session dispatches from GET /factory-sessions/{session_id}/dispatches\n" +
			"  pause      pause one live or durable Factory Session through POST /factory-sessions/{session_id}/pause\n" +
			"  resume     resume one paused live or durable Factory Session through POST /factory-sessions/{session_id}/resume\n" +
			"  create     open another live session from a folder path\n" +
			"  delete     close a live session by session id\n\n" +
			"Durable list output uses Factory Session status, source identity, result availability, " +
			"progress, and action availability. Session commands use the same default --port as work list. " +
			"Use --json to emit API-shaped responses on stdout; diagnostics stay on stderr when --verbose " +
			"or --debug is set.",
		Example: "  # List live sessions on the default local port.\n" +
			"  " + cliBinaryName + " session list\n\n" +
			"  # Show orchestrator-aware runtime for one live session.\n" +
			"  " + cliBinaryName + " session show session-beta\n\n" +
			"  # Pause and resume the default compatibility session.\n" +
			"  " + cliBinaryName + " session pause\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Pause and resume one named or durable Factory Session.\n" +
			"  " + cliBinaryName + " session pause session-beta\n" +
			"  " + cliBinaryName + " session resume dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " session list --json\n\n" +
			"  # Open and close sessions on a non-default port.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --port 9090\n" +
			"  " + cliBinaryName + " session delete session-beta --port 9090 --json\n\n" +
			"  # Pause and resume the default compatibility session.\n" +
			"  " + cliBinaryName + " session pause\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Target a different service port for list output.\n" +
			"  " + cliBinaryName + " session list --port 9090",
	}
}

func newSessionShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.ShowConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "show [session-id]",
		Short: "Show one live factory session",
		Long: "Show one live factory session from GET /factory-sessions/{session_id}.\n\n" +
			"Human output uses FactorySession as the canonical runtime noun and prints orchestrator " +
			"kind plus Petri or JavaScript runtime projections. Dynamic workflow wording appears only " +
			"as JavaScript shorthand. Omit session-id to target the default compatibility session " +
			"(~default). Use global --json for the API-shaped FactorySession payload and global " +
			"--server to target the same factory API base URI as work list and factory query.",
		Example: "  # Show the default compatibility factory session.\n" +
			"  " + cliBinaryName + " session show\n\n" +
			"  # Show one named live session with orchestrator-aware runtime fields.\n" +
			"  " + cliBinaryName + " session show session-beta\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session show session-beta",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return showSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newRepresentativeHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions RootCommandOptions,
) (*commandregistry.Registry, error) {
	if operatorDefaults == nil {
		operatorDefaults = &cliOperatorDefaultsOptions{}
	}
	return commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE: func(cmd *cobra.Command, args []string) error {
			policy := diagnostics.resolvePolicy(false)
			return runFactoryWithOptions(cmd, defaultcmd.OOTBRunConfig(), nil, globals, operatorDefaults, policy, rootOptions, true)
		},
		SessionShowRunE: commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			ShowSession:       showSession,
		}),
	})
}

func newSessionDispatchesCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.DispatchesConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "dispatches [session-id]",
		Short: "List durable Factory Session dispatches",
		Long: "List durable Factory Session dispatches from GET /factory-sessions/{session_id}/dispatches.\n\n" +
			"Human output uses FactorySession, Dispatch, and FactoryArtifact vocabulary for dispatch id, status, " +
			"kind, label, and output artifact ids. The command requires a durable dur-sess-* session id. Use global " +
			"--json for the API-shaped ListFactorySessionDispatchesResponse and global --server to target the same " +
			"factory API base URI as session show and session resume.",
		Example: "  # List dispatches for one interrupted durable Factory Session.\n" +
			"  " + cliBinaryName + " session dispatches dur-sess-js-interrupted-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session dispatches dur-sess-js-interrupted-001",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.SessionID = args[0]
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessionDispatches(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.Phase, "phase", "", "filter by exact Dispatch phase")
	cmd.Flags().StringVar(&cfg.Status, "status", "", "filter by canonical Dispatch status")
	return cmd
}

func newSessionPauseCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.LifecycleControlConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "pause [session-id]",
		Short: "Pause one live or durable Factory Session",
		Long: "Pause one live or durable Factory Session through POST /factory-sessions/{session_id}/pause.\n\n" +
			"Human output reports paused, already-paused, invalid-state, not-found, or unreachable-host " +
			"outcomes using Factory Session terminology. Omit session-id to target the default compatibility " +
			"session (~default), pass a named live session id such as session-beta, or pass a durable " +
			"session id from `you session list --scope all`. Use global --json for the API-shaped " +
			"FactorySessionLifecycleControlResponse and global --server to target the same factory API base URI " +
			"as workflow status and session show.",
		Example: "  # Pause the default compatibility Factory Session.\n" +
			"  " + cliBinaryName + " session pause\n\n" +
			"  # Pause one named live Factory Session.\n" +
			"  " + cliBinaryName + " session pause session-beta\n\n" +
			"  # Pause one running durable Factory Session.\n" +
			"  " + cliBinaryName + " session pause dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session pause session-beta",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return pauseSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionResumeCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.LifecycleControlConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "resume [session-id]",
		Short: "Resume one paused live or durable Factory Session",
		Long: "Resume one paused live or durable Factory Session through POST /factory-sessions/{session_id}/resume.\n\n" +
			"Human output reports resumed, already-running, invalid-state, not-found, or unreachable-host " +
			"outcomes using Factory Session terminology. Omit session-id to target the default compatibility " +
			"session (~default), pass a named live session id such as session-beta, or pass a durable " +
			"session id from `you session list --scope all`. Use global --json for the API-shaped " +
			"FactorySessionLifecycleControlResponse and global --server to target the same factory API base URI " +
			"as workflow status and session show.",
		Example: "  # Resume the default compatibility Factory Session.\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Resume one named live Factory Session.\n" +
			"  " + cliBinaryName + " session resume session-beta\n\n" +
			"  # Resume one paused durable Factory Session.\n" +
			"  " + cliBinaryName + " session resume dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session resume session-beta",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return resumeSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionListCommand(diagnostics *cliDiagnosticsOptions, options RootCommandOptions) *cobra.Command {
	cfg := sessioncli.ListConfig{Port: defaultcmd.FactoryPort, Scope: "live"}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List live and durable factory sessions",
		Long: "List factory sessions for the requested scope.\n\n" +
			"live returns workspace sessions kept open by the running host. persisted returns durable " +
			"Factory Sessions from the deterministic provider loopback. all returns both live workspace " +
			"sessions and durable Factory Sessions.\n\n" +
			"Human output prints the legacy live-session table for workspace rows and a durable Factory " +
			"Session table with status, source identity, result availability, progress, and actions. " +
			"Use --json to emit ListFactorySessionsResponse on stdout.",
		Example: "  " + cliBinaryName + " session list\n\n" +
			"  " + cliBinaryName + " session list --scope persisted\n\n" +
			"  " + cliBinaryName + " session list --scope all --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
			if scope == fse.SessionListScopePersisted || scope == fse.SessionListScopeAll {
				service, err := buildWorkflowExecutionService(cmd.Context(), options, sessionexecutioncli.ExecutionBackendConfig{Provider: string(fse.ExecutionProviderFake)}, "", "")
				if err != nil {
					return err
				}
				cfg.DurableLister = service.ListSessions
			}
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessions(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.Scope, "scope", cfg.Scope, "session list scope: live, persisted, or all")
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
