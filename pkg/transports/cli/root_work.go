package cli

import (
	"errors"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// useGeneratedRepresentativeFamily toggles production root wiring between the
// generated representative-family constructor and the legacy handwritten path.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedRepresentativeFamily = true

// useGeneratedSessionFamily toggles only the production session subtree between
// the generated metadata constructor and the retained handwritten rollback path.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedSessionFamily = true

// useGeneratedWorkFamily toggles production work wiring between the generated
// metadata constructor and the legacy handwritten path.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedWorkFamily = true

// useGeneratedRunSubmitFamily toggles production run/submit wiring between the
// generated metadata constructor and the legacy handwritten path.
// Flip this constant to false for a one-localized-change rollback.
const useGeneratedRunSubmitFamily = true

func newLegacyRootCommandWithOptions(options RootCommandOptions) *cobra.Command {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	factoryConfigInit := productionFactoryConfigInitCommands(globals, diagnostics, options)
	docsCmd := newDocsCommand(diagnostics)
	modelsCmd := newModelsCommand(globals, diagnostics, operatorDefaults, options)
	b12 := newB12ProductionFamilies(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(productionRootSubcommands(globals, diagnostics, factoryConfigInit, docsCmd, modelsCmd, b12)...)
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

	factoryConfigInit := productionFactoryConfigInitCommands(globals, diagnostics, options)
	docsCmd, modelsCmd, err := newProductionModelsDocsCommands(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build models/docs family command: %v", err))
	}
	b12 := newB12ProductionFamilies(globals, diagnostics, operatorDefaults, options)

	root := components.Root
	root.AddCommand(productionRootSubcommands(globals, diagnostics, factoryConfigInit, docsCmd, modelsCmd, b12)...)
	return root
}

// b12ProductionFamilies is the one shared production-root fan-in for the
// session, workflow/MCP, and run/submit migrations. Each field is constructed
// once through its family-local generated/legacy cutover seam.
type b12ProductionFamilies struct {
	Session  *cobra.Command
	MCP      *cobra.Command
	Workflow *cobra.Command
	Run      *cobra.Command
	Submit   *cobra.Command
}

func newB12ProductionFamilies(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
) b12ProductionFamilies {
	mcp, workflow := newProductionWorkflowMCPCommands(globals, diagnostics, options)
	runSubmit := productionRunSubmitCommands(globals, diagnostics, operatorDefaults, options)
	return b12ProductionFamilies{
		Session:  productionSessionCommand(globals, diagnostics, options),
		MCP:      mcp,
		Workflow: workflow,
		Run:      runSubmit.Run,
		Submit:   runSubmit.Submit,
	}
}

func productionSessionCommand(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options RootCommandOptions,
) *cobra.Command {
	if !useGeneratedSessionFamily {
		return newSessionCommand(globals, diagnostics, options)
	}
	registry, bindings, err := newSessionHandlerRegistry(globals, diagnostics, options)
	if err != nil {
		panic(fmt.Sprintf("build session handler registry: %v", err))
	}
	session, err := climanifestcobra.NewSessionFamilyCommand(registry, bindings)
	if err != nil {
		panic(fmt.Sprintf("build session family command: %v", err))
	}
	return session
}

func productionRootSubcommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	factoryConfigInit factoryConfigInitProductionCommands,
	docsCmd *cobra.Command,
	modelsCmd *cobra.Command,
	b12 b12ProductionFamilies,
) []*cobra.Command {
	return []*cobra.Command{
		docsCmd,
		factoryConfigInit.Config,
		factoryConfigInit.Factory,
		factoryConfigInit.Init,
		b12.MCP,
		modelsCmd,
		b12.Run,
		b12.Submit,
		b12.Session,
		productionWorkCommand(globals, diagnostics),
		b12.Workflow,
	}
}

type runSubmitProductionCommands struct {
	Run    *cobra.Command
	Submit *cobra.Command
}

func productionRunSubmitCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
) runSubmitProductionCommands {
	commands, err := buildRunSubmitProductionCommands(
		globals, diagnostics, operatorDefaults, options, useGeneratedRunSubmitFamily,
	)
	if err != nil {
		panic(fmt.Sprintf("build run/submit family commands: %v", err))
	}
	return commands
}

func buildRunSubmitProductionCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
	generatedFamily bool,
) (runSubmitProductionCommands, error) {
	if !generatedFamily {
		return runSubmitProductionCommands{
			Run:    newRunCommand(globals, diagnostics, operatorDefaults, options),
			Submit: newSubmitCommandWithHandlers(globals, diagnostics, options.SubmitWork, options.SubmitBatch),
		}, nil
	}

	registry, bindings, err := newRunSubmitHandlerRegistry(
		globals, diagnostics, operatorDefaults, options,
	)
	if err != nil {
		return runSubmitProductionCommands{}, err
	}
	components, err := climanifestcobra.NewRunSubmitFamilyComponents(registry, bindings)
	if err != nil {
		return runSubmitProductionCommands{}, err
	}
	return runSubmitProductionCommands{Run: components.Run, Submit: components.Submit}, nil
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
		newSessionListCommand(globals, diagnostics, options),
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

// NewGeneratedRunSubmitFamilyCommandForParity builds an isolated you root with
// generated run/submit metadata and the retained production handler paths.
// Production registration uses the same generated constructor and handler bindings.
func NewGeneratedRunSubmitFamilyCommandForParity() (*cobra.Command, error) {
	return newRunSubmitFamilyRootForParity(RootCommandOptions{}, true)
}

// NewRunSubmitFamilyParityRoots builds independent handwritten and generated
// run/submit roots with the same process-owned dependencies. Keeping the roots
// independent prevents Cobra flag state from leaking between parity executions.
func NewRunSubmitFamilyParityRoots(options RootCommandOptions) (legacyRoot, generatedRoot *cobra.Command, err error) {
	legacyRoot, err = newRunSubmitFamilyRootForParity(options, false)
	if err != nil {
		return nil, nil, err
	}
	generatedRoot, err = newRunSubmitFamilyRootForParity(options, true)
	if err != nil {
		return nil, nil, err
	}
	return legacyRoot, generatedRoot, nil
}

func newRunSubmitFamilyRootForParity(options RootCommandOptions, generatedFamily bool) (*cobra.Command, error) {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	if !generatedFamily {
		root.AddCommand(
			newRunCommand(globals, diagnostics, operatorDefaults, options),
			newSubmitCommandWithHandlers(globals, diagnostics, options.SubmitWork, options.SubmitBatch),
		)
		return root, nil
	}
	registry, bindings, err := newRunSubmitHandlerRegistry(
		globals,
		diagnostics,
		operatorDefaults,
		options,
	)
	if err != nil {
		return nil, err
	}
	components, err := climanifestcobra.NewRunSubmitFamilyComponents(registry, bindings)
	if err != nil {
		return nil, err
	}
	root.AddCommand(components.Run, components.Submit)
	return root, nil
}

func newRunSubmitHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions RootCommandOptions,
) (*commandregistry.Registry, climanifestcobra.RunSubmitFlagBindings, error) {
	runCfg := defaultcmd.ExplicitRunConfig()
	var invocationOutputMode string
	submitCfg := submitcli.SubmitConfig{Server: globals.server}
	batchCfg := submitcli.BatchConfig{Server: globals.server}
	registry, err := commandregistry.NewRunSubmitRegistry(commandregistry.RunSubmitHandlers{
		Run: commandregistry.CommandHandlers{
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, args []string) error {
				return executeRunCommand(
					cmd, args, &runCfg, globals, diagnostics, operatorDefaults, rootOptions,
				)
			},
		},
		Submit: commandregistry.CommandHandlers{
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return executeSubmitCommand(cmd, &submitCfg, globals, diagnostics, rootOptions.SubmitWork)
			},
		},
		SubmitBatch: commandregistry.CommandHandlers{
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, args []string) error {
				return executeSubmitBatchCommand(cmd, args, &batchCfg, globals, diagnostics, rootOptions.SubmitBatch)
			},
		},
	})
	if err != nil {
		return nil, climanifestcobra.RunSubmitFlagBindings{}, err
	}
	return registry, climanifestcobra.RunSubmitFlagBindings{
		Run:                 &runCfg,
		RunInvocationOutput: &invocationOutputMode,
		Submit:              &submitCfg,
		SubmitBatch:         &batchCfg,
		FlagUsages:          runSubmitFlagUsages(globals, diagnostics, operatorDefaults, rootOptions),
	}, nil
}

func runSubmitFlagUsages(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions RootCommandOptions,
) map[string]string {
	run := newRunCommand(globals, diagnostics, operatorDefaults, rootOptions)
	submit := newSubmitCommand(globals, diagnostics)
	commands := []*cobra.Command{run, submit}
	commands = append(commands, submit.Commands()...)
	usages := make(map[string]string)
	for _, cmd := range commands {
		cmd.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
			if _, exists := usages[flag.Name]; !exists {
				usages[flag.Name] = flag.Usage
			}
		})
	}
	return usages
}

func productionWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	if !useGeneratedWorkFamily {
		return newWorkCommand(globals, diagnostics)
	}

	registry, bindings, err := newWorkHandlerRegistry(globals, diagnostics)
	if err != nil {
		panic(fmt.Sprintf("build work handler registry: %v", err))
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		panic(fmt.Sprintf("build work family command: %v", err))
	}
	return work
}

// NewLegacyWorkFamilyCommand builds the isolated handwritten
// you work → list/show/move/visualize tree used by generator-vs-legacy parity.
func NewLegacyWorkFamilyCommand() *cobra.Command {
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	return newWorkCommand(globals, diagnostics)
}

// NewLegacyWorkFamilyRootForParity builds an isolated you → work tree with shared
// persistent globals for generator-vs-legacy parity tests.
func NewLegacyWorkFamilyRootForParity() *cobra.Command {
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	options := normalizeRootCommandOptions(RootCommandOptions{})
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(newWorkCommand(globals, diagnostics))
	return root
}

// NewWorkFamilyParityRoots builds matching legacy and generated you → work parity
// roots that share one persistent globals shell.
func NewWorkFamilyParityRoots(
	registry *commandregistry.Registry,
	bindings climanifestcobra.WorkFamilyBindings,
) (legacyRoot, generatedRoot *cobra.Command, err error) {
	if registry == nil {
		return nil, nil, fmt.Errorf("build work family parity roots: registry is required")
	}
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	options := normalizeRootCommandOptions(RootCommandOptions{})

	legacyRoot = newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	legacyRoot.AddCommand(newWorkCommand(globals, diagnostics))

	generatedWork, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		return nil, nil, fmt.Errorf("build work family parity roots: %w", err)
	}
	generatedRoot = newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	generatedRoot.AddCommand(generatedWork)
	return legacyRoot, generatedRoot, nil
}

// NewWorkFamilyParityRootsWithProductionHandlers builds legacy and generated
// parity roots wired with production work handler bindings.
func NewWorkFamilyParityRootsWithProductionHandlers() (legacyRoot, generatedRoot *cobra.Command, err error) {
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	options := normalizeRootCommandOptions(RootCommandOptions{})

	legacyRoot = newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	legacyRoot.AddCommand(newWorkCommand(globals, diagnostics))

	registry, bindings, err := newWorkHandlerRegistry(globals, diagnostics)
	if err != nil {
		return nil, nil, err
	}
	generatedWork, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		return nil, nil, err
	}
	generatedRoot = newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	generatedRoot.AddCommand(generatedWork)
	return legacyRoot, generatedRoot, nil
}

func newWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	workCmd := &cobra.Command{
		Use:   "work",
		Short: "Inspect work from a running factory",
		Long: "Inspect and control work on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list       list work from GET /factory-sessions/{session_id}/work with optional filters and pagination\n" +
			"  show       show one work item from GET /factory-sessions/{session_id}/work/{work_id}\n" +
			"  move       move one work item to another authored state through POST /factory-sessions/{session_id}/work/{work_id}/move\n" +
			"  visualize  read a local FACTORY_REQUEST_BATCH JSON file and render its dependency graph to stdout\n\n" +
			"API-backed list, show, and move commands target the default compatibility session unless --session is set. " +
			"Use global --json to emit API-shaped responses on stdout; diagnostics stay on stderr when --verbose or --debug is set.",
		Example: "  # List work on the default compatibility session.\n" +
			"  " + cliBinaryName + " work list\n\n" +
			"  # Show one work item after submit.\n" +
			"  " + cliBinaryName + " work show work-123\n\n" +
			"  # Render a local batch dependency graph.\n" +
			"  " + cliBinaryName + " work visualize batch.json > graph.mermaid",
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
		Example: "  " + cliBinaryName + " work list\n\n" +
			"  " + cliBinaryName + " work list --name startup --max-results 25",
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
		Example: "  " + cliBinaryName + " work show work-123\n\n" +
			"  " + cliBinaryName + " --json work show work-123",
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
		Example: "  " + cliBinaryName + " work move work-123 ready\n\n" +
			"  " + cliBinaryName + " work move work-123 ready --request-id op-1",
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

func newWorkFamilyBindings(globals *cliGlobalOptions) (climanifestcobra.WorkFamilyBindings, *string) {
	listCfg := workcli.ListConfig{Server: globals.server}
	showCfg := workcli.ShowConfig{Server: globals.server}
	moveCfg := workcli.MoveConfig{Server: globals.server}
	var visualizeFormat string
	return climanifestcobra.WorkFamilyBindings{
		ListConfig:      &listCfg,
		ShowConfig:      &showCfg,
		MoveConfig:      &moveCfg,
		VisualizeFormat: &visualizeFormat,
		FlagUsages:      workFamilyFlagUsages(),
	}, &visualizeFormat
}

func workFamilyFlagUsages() map[string]string {
	return map[string]string{
		"state-name":     "filter by current state name",
		"state-type":     "filter by current state type (INITIAL, PROCESSING, TERMINAL, FAILED)",
		"name":           "filter by case-insensitive substring of work name (applied before pagination)",
		"work-type-name": "filter by exact workTypeName (applied before pagination)",
		"trace-id":       "filter by exact traceId or currentChainingTraceId (applied before pagination)",
		"sort-by":        "sort returned work by field (state.type)",
		"max-results":    "maximum work items to return per page after server-side filters",
		"next-token":     "pagination cursor returned by a previous work list response",
		"session":        "target one live factory session; omit to use the default compatibility session",
		"request-id":     "optional client idempotency key for operator moves",
		"format":         "output format: mermaid or markdown-mermaid",
	}
}

func newWorkHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
) (*commandregistry.Registry, climanifestcobra.WorkFamilyBindings, error) {
	bindings, visualizeFormatPtr := newWorkFamilyBindings(globals)
	listCfg := bindings.ListConfig
	showCfg := bindings.ShowConfig
	moveCfg := bindings.MoveConfig
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: commandregistry.ListRunE(commandregistry.ListBinding{
			Config:            listCfg,
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			ListWork:          listWork,
		}),
		ShowRunE: commandregistry.ShowRunE(commandregistry.ShowBinding{
			Config:            showCfg,
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			ShowWork:          showWork,
		}),
		MoveRunE: commandregistry.MoveRunE(commandregistry.MoveBinding{
			Config:            moveCfg,
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			MoveWork:          moveWork,
		}),
		VisualizeRunE: commandregistry.VisualizeRunE(commandregistry.VisualizeBinding{
			Format:    visualizeFormatPtr,
			Visualize: visualizeWork,
		}),
	})
	if err != nil {
		return nil, climanifestcobra.WorkFamilyBindings{}, err
	}
	return registry, bindings, nil
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
