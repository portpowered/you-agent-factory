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
	configinitcmd "github.com/portpowered/infinite-you/pkg/cli/configinit"
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	docscli "github.com/portpowered/infinite-you/pkg/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/cli/init"
	mcpcli "github.com/portpowered/infinite-you/pkg/cli/mcp"
	modelscli "github.com/portpowered/infinite-you/pkg/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	startupcli "github.com/portpowered/infinite-you/pkg/cli/startup"
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/portpowered/infinite-you/pkg/cli/terminalpolicy"
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
var createFactoryFromFile = factorycli.CreateFromFile
var replaceFactoryCurrent = factorycli.ReplaceCurrent
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

// RootCommandOptions supplies process-owned values used while executing a
// command. Zero values preserve the legacy process environment behavior.
type RootCommandOptions struct {
	HomeDir    func() (string, error)
	LookupEnv  func(string) (string, bool)
	Startup    startupcli.Handler
	RunFactory func(context.Context, runcli.RunConfig) error
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(RootCommandOptions{})
}

// NewRootCommandWithOptions constructs the command tree with explicit process
// inputs so callers can execute independent command instances deterministically.
func NewRootCommandWithOptions(options RootCommandOptions) *cobra.Command {
	options = normalizeRootCommandOptions(options)
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

	root.AddCommand(
		newDocsCommand(diagnostics),
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
		newModelsCommand(globals, diagnostics, operatorDefaults, options),
		newRunCommand(globals, diagnostics, operatorDefaults, options),
		newSubmitCommand(globals, diagnostics),
		newSessionCommand(globals, diagnostics),
		newWorkCommand(globals, diagnostics),
		newWorkflowCommand(globals, diagnostics),
	)

	return root
}

func normalizeRootCommandOptions(options RootCommandOptions) RootCommandOptions {
	if options.HomeDir == nil {
		options.HomeDir = os.UserHomeDir
	}
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	if options.RunFactory == nil {
		options.RunFactory = func(ctx context.Context, cfg runcli.RunConfig) error {
			return runCLI(ctx, cfg)
		}
	}
	return options
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

func (opts *cliDiagnosticsOptions) resolvePolicy(quiet bool) terminalpolicy.Policy {
	return terminalpolicy.Resolve(terminalpolicy.Options{
		Quiet:   quiet,
		Verbose: opts.verbose,
		Debug:   opts.debug,
	})
}

func (opts *cliDiagnosticsOptions) verboseEnabled() bool {
	return opts.resolvePolicy(false).VerboseEnabled()
}

func (opts *cliDiagnosticsOptions) writer(cmd *cobra.Command) io.Writer {
	return opts.resolvePolicy(false).DiagnosticsWriter(cmd.ErrOrStderr())
}

func newModelsCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions RootCommandOptions) *cobra.Command {
	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "Inspect discovered models from a running service",
		Long: "Inspect discovered models from a running infinite-you service.\n\n" +
			"Use list to discover model identifiers, inspect to view one model's readiness and capabilities, " +
			"invoke to call a discovered model directly through the shared in-process bootstrap, " +
			"and pull to populate the managed local-model cache for supported local assets.",
	}
	modelsCmd.AddCommand(
		newModelsListCommand(globals, diagnostics),
		newModelsInspectCommand(globals, diagnostics),
		newModelsInvokeCommand(globals, diagnostics, operatorDefaults, rootOptions),
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

func newModelsInvokeCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions RootCommandOptions) *cobra.Command {
	cfg := modelscli.InvokeConfig{Server: globals.server, Operation: "TTS"}
	cmd := &cobra.Command{
		Use:     "invoke <model-name>",
		Short:   "Invoke one discovered model",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			policy := diagnostics.resolvePolicy(false)
			logger, err := policy.BuildLogger(logging.BuildLogger)
			if err != nil {
				return err
			}
			homeDir, err := rootOptions.HomeDir()
			if err != nil {
				return fmt.Errorf("resolve process home directory: %w", err)
			}
			resolvedOperatorDefaults, err := resolveOperatorDefaults(cmd, operatorDefaults, rootOptions, homeDir)
			if err != nil {
				return err
			}
			cfg.Server = globals.server
			cfg.ModelName = args[0]
			cfg.JSON = globals.json
			cfg.HomeDir = homeDir
			cfg.OperatorDefaults = resolvedOperatorDefaults
			cfg.Logger = logger
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

func newMCPCommand(options RootCommandOptions) *cobra.Command {
	if options.Startup == nil {
		return mcpcli.NewCommand()
	}
	return mcpcli.NewCommandWithStartup(options.Startup)
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

func runFactoryWithOptions(cmd *cobra.Command, cfg runcli.RunConfig, promptArgs []string, globals *cliGlobalOptions, operatorDefaults *cliOperatorDefaultsOptions, policy terminalpolicy.Policy, rootOptions RootCommandOptions, defaultInvocation bool) error {
	logger, err := policy.BuildLogger(logging.BuildLogger)
	if err != nil {
		return err
	}
	cfg.Logger = logger
	cfg.Verbose = policy.VerboseEnabled()
	cfg.TerminalPolicy = policy

	if err := resolveRunBindFromServer(cmd, globals.server, &cfg); err != nil {
		return err
	}
	homeDir, err := rootOptions.HomeDir()
	if err != nil {
		return fmt.Errorf("resolve process home directory: %w", err)
	}
	cfg.HomeDir = homeDir
	if err := resolveRunFactorySelection(cmd, &cfg, homeDir); err != nil {
		return err
	}

	resolvedOperatorDefaults, err := resolveOperatorDefaults(cmd, operatorDefaults, rootOptions, homeDir)
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
	runPolicy := resolveEffectiveRunPolicy(cmd, cfg, policy)
	cfg.TerminalPolicy = runPolicy
	cfg.Verbose = runPolicy.VerboseEnabled()
	cfg.SuppressDashboardRendering = runPolicy.Mode() == terminalpolicy.ModeQuiet
	humanTerminal := runPolicy.HumanTerminalWriter(cmd.OutOrStdout())
	if cleanInvocation || textInvocation {
		cfg.Output = cmd.OutOrStdout()
	} else if strings.TrimSpace(cfg.FactoryConfigPath) != "" {
		cfg.Output = cmd.OutOrStdout()
		cfg.StartupOutput = humanTerminal
	} else {
		cfg.StartupOutput = humanTerminal
	}
	cfg.Diagnostics = runPolicy.DiagnosticsWriter(cmd.ErrOrStderr())
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

	if rootOptions.Startup == nil {
		return rootOptions.RunFactory(ctx, cfg)
	}
	return delegateRunStartup(ctx, cfg, defaultInvocation, rootOptions)
}

func delegateRunStartup(ctx context.Context, cfg runcli.RunConfig, defaultInvocation bool, options RootCommandOptions) error {
	invocationOnly := cfg.CleanInvocation || cfg.InvocationPositionalText != nil || cfg.InvocationStdinText != nil || cfg.InvocationNormalizedArguments != nil
	request := startupcli.Request{
		Kind: startupcli.KindRun,
		Run: startupcli.RunIntent{
			DefaultInvocation:     defaultInvocation,
			Continuous:            cfg.Continuously,
			APIEnabled:            cfg.Port > 0 && !invocationOnly,
			DashboardEnabled:      cfg.Port > 0 && !cfg.SuppressDashboardRendering && !invocationOnly,
			WorkerSidecarsEnabled: true,
		},
		RunConfig: &cfg,
	}
	return options.Startup(ctx, request)
}

func resolveEffectiveRunPolicy(cmd *cobra.Command, cfg runcli.RunConfig, basePolicy terminalpolicy.Policy) terminalpolicy.Policy {
	cleanInvocation, textInvocation := runInvocationModes(cmd, cfg)
	if cfg.SuppressDashboardRendering || cleanInvocation || textInvocation {
		return terminalpolicy.Resolve(terminalpolicy.Options{
			Quiet:   true,
			Verbose: basePolicy.VerboseEnabled(),
			Debug:   basePolicy.DebugEnabled(),
		})
	}
	return basePolicy
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

func resolveOperatorDefaults(cmd *cobra.Command, operatorDefaults *cliOperatorDefaultsOptions, rootOptions RootCommandOptions, homeDir string) (operatorconfig.ResolvedDefaults, error) {
	environment := operatorconfig.Defaults{}
	environment.WorkerModelProvider, _ = rootOptions.LookupEnv(operatorconfig.EnvDefaultWorkerModelProvider)
	environment.WorkerModel, _ = rootOptions.LookupEnv(operatorconfig.EnvDefaultWorkerModel)
	return operatorconfig.ResolveFromHomeWithEnvironment(homeDir, environment, operatorconfig.FlagOverrides{
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

func resolveRunFactorySelection(cmd *cobra.Command, cfg *runcli.RunConfig, homeDir string) error {
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
		return resolveRunNamedFactorySelection(cfg, homeDir)
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

func resolveRunNamedFactorySelection(cfg *runcli.RunConfig, homeDir string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current working directory for --named: %w", err)
	}
	projectRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(cwd)
	if err != nil {
		return fmt.Errorf("resolve project named-factory root: %w", err)
	}
	globalRoot, err := factoryconfig.GlobalNamedFactoryRootForHome(homeDir)
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
