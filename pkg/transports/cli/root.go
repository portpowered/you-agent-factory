// Package cli defines Cobra commands for the agent-factory CLI.
// Commands contain only flag parsing and delegate to command-specific packages.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	configcli "github.com/portpowered/infinite-you/pkg/transports/cli/config"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	docscli "github.com/portpowered/infinite-you/pkg/transports/cli/docs"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	mcpcli "github.com/portpowered/infinite-you/pkg/transports/cli/mcp"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
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
	HomeDir               func() (string, error)
	LookupEnv             func(string) (string, bool)
	Startup               startupcli.Handler
	RunFactory            func(context.Context, runcli.RunConfig) error
	SubmitWork            func(submitcli.SubmitConfig) error
	SubmitBatch           func(submitcli.BatchConfig) error
	BuildSessionExecution sessionexecutioncli.ServiceBuilder
	BuildModelInvocation  modelscli.InvocationBuilder
}

func NewRootCommand() *cobra.Command {
	return NewRootCommandWithOptions(RootCommandOptions{})
}

// NewRootCommandWithOptions constructs the command tree with explicit process
// inputs so callers can execute independent command instances deterministically.
func NewRootCommandWithOptions(options RootCommandOptions) *cobra.Command {
	if useGeneratedRepresentativeFamily {
		return newRootCommandWithGeneratedRepresentativeFamily(options)
	}
	return newLegacyRootCommandWithOptions(options)
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
	if options.SubmitWork == nil {
		options.SubmitWork = submitWork
	}
	if options.SubmitBatch == nil {
		options.SubmitBatch = submitBatch
	}
	return options
}

func buildWorkflowExecutionService(
	ctx context.Context,
	options RootCommandOptions,
	backend sessionexecutioncli.ExecutionBackendConfig,
	fixtureCatalogPath string,
	childExecutorMode string,
) (sessionexecutioncli.ServiceOwner, error) {
	if options.BuildSessionExecution == nil {
		return nil, fmt.Errorf("construct workflow execution: durable execution builder is required")
	}
	service, err := options.BuildSessionExecution(ctx, sessionexecutioncli.ServiceRequest{
		ExecutionBackendConfig: backend,
		FixtureCatalogPath:     fixtureCatalogPath,
		ChildExecutorMode:      childExecutorMode,
	})
	if err != nil {
		return nil, fmt.Errorf("construct workflow execution: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("construct workflow execution: builder returned nil service")
	}
	return service, nil
}

func withWorkflowExecutionService(
	ctx context.Context,
	options RootCommandOptions,
	backend sessionexecutioncli.ExecutionBackendConfig,
	fixtureCatalogPath string,
	childExecutorMode string,
	run func(fse.Service) error,
) (err error) {
	owner, err := buildWorkflowExecutionService(ctx, options, backend, fixtureCatalogPath, childExecutorMode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := owner.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close workflow execution: %w", closeErr))
		}
	}()
	return run(owner)
}

func addWorkflowExecutionBackendFlags(
	cmd *cobra.Command,
	backend *sessionexecutioncli.ExecutionBackendConfig,
	childExecutorMode *string,
) {
	cmd.Flags().StringVar(&backend.Provider, "execution-provider", string(fse.ExecutionProviderFake), "durable execution backend: fake or javascript-runtime")
	cmd.Flags().StringVar(&backend.ProjectRoot, "project-root", "", "project root for javascript-runtime workflow source lookup")
	if childExecutorMode != nil {
		cmd.Flags().StringVar(childExecutorMode, "child-executor-mode", "", "javascript child executor mode: fake or live-provider")
	}
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
			cfg.BuildInvocation = rootOptions.BuildModelInvocation
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

// ShowSessionAccessor returns the current session show delegate.
func ShowSessionAccessor() func(sessioncli.ShowConfig) error {
	return showSession
}

// SetShowSessionAccessor replaces the session show delegate for tests.
func SetShowSessionAccessor(fn func(sessioncli.ShowConfig) error) {
	showSession = fn
}

// ListModelsAccessor returns the current models list delegate.
func ListModelsAccessor() func(modelscli.ListConfig) error {
	return listModels
}

// SetListModelsAccessor replaces the models list delegate for tests.
func SetListModelsAccessor(fn func(modelscli.ListConfig) error) {
	listModels = fn
}

// InspectModelAccessor returns the current models inspect delegate.
func InspectModelAccessor() func(modelscli.InspectConfig) error {
	return inspectModel
}

// SetInspectModelAccessor replaces the models inspect delegate for tests.
func SetInspectModelAccessor(fn func(modelscli.InspectConfig) error) {
	inspectModel = fn
}

// InvokeModelAccessor returns the current models invoke delegate.
func InvokeModelAccessor() func(modelscli.InvokeConfig) error {
	return invokeModel
}

// SetInvokeModelAccessor replaces the models invoke delegate for tests.
func SetInvokeModelAccessor(fn func(modelscli.InvokeConfig) error) {
	invokeModel = fn
}

// PullModelAccessor returns the current models pull delegate.
func PullModelAccessor() func(modelscli.PullConfig) error {
	return pullModel
}

// SetPullModelAccessor replaces the models pull delegate for tests.
func SetPullModelAccessor(fn func(modelscli.PullConfig) error) {
	pullModel = fn
}

// useGeneratedModelsDocsFamily toggles production models/docs wiring between the
// generated metadata constructor and the legacy handwritten path.
const useGeneratedModelsDocsFamily = true

// NewLegacyModelsFamilyCommand builds the isolated handwritten you → models tree
// used by the generator-vs-legacy parity matrix.
func NewLegacyModelsFamilyCommand() *cobra.Command {
	root, globals, diagnostics, operatorDefaults, options := newModelsFamilyParityShell()
	root.AddCommand(newModelsCommand(globals, diagnostics, operatorDefaults, options))
	return root
}

// NewGeneratedModelsFamilyParityCommand builds you → models from generated metadata
// and attaches handwritten handlers by stable command ID for parity tests.
func NewGeneratedModelsFamilyParityCommand(
	registry *commandregistry.Registry,
	invokeFlags climanifestcobra.ModelsInvokeFlagBindings,
) (*cobra.Command, error) {
	root, _, _, _, _ := newModelsFamilyParityShell()
	components, err := climanifestcobra.NewModelsDocsFamilyComponents(registry, invokeFlags)
	if err != nil {
		return nil, err
	}
	root.AddCommand(components.Models)
	return root, nil
}

// NewLegacyDocsFamilyCommand builds the isolated handwritten you → docs tree used
// by the generator-vs-legacy parity matrix.
func NewLegacyDocsFamilyCommand() *cobra.Command {
	root, diagnostics, _, _, _ := newDocsFamilyParityShell()
	root.AddCommand(newDocsCommand(diagnostics))
	return root
}

// NewGeneratedDocsFamilyParityCommand builds you → docs from generated metadata
// and attaches handwritten handlers by stable command ID for parity tests.
func NewGeneratedDocsFamilyParityCommand(
	registry *commandregistry.Registry,
	invokeFlags climanifestcobra.ModelsInvokeFlagBindings,
) (*cobra.Command, error) {
	root, _, _, _, _ := newDocsFamilyParityShell()
	components, err := climanifestcobra.NewModelsDocsFamilyComponents(registry, invokeFlags)
	if err != nil {
		return nil, err
	}
	root.AddCommand(components.Docs)
	return root, nil
}

func newDocsFamilyParityShell() (*cobra.Command, *cliDiagnosticsOptions, *cliGlobalOptions, *cliOperatorDefaultsOptions, RootCommandOptions) {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	return root, diagnostics, globals, operatorDefaults, options
}

func newModelsFamilyParityShell() (*cobra.Command, *cliGlobalOptions, *cliDiagnosticsOptions, *cliOperatorDefaultsOptions, RootCommandOptions) {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	return root, globals, diagnostics, operatorDefaults, options
}

func newProductionModelsDocsCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options RootCommandOptions,
) (*cobra.Command, *cobra.Command, error) {
	if !useGeneratedModelsDocsFamily {
		return newDocsCommand(diagnostics), newModelsCommand(globals, diagnostics, operatorDefaults, options), nil
	}

	registry, invokeFlags, err := newModelsDocsHandlerRegistry(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		return nil, nil, err
	}
	components, err := climanifestcobra.NewModelsDocsFamilyComponents(registry, invokeFlags)
	if err != nil {
		return nil, nil, err
	}
	return components.Docs, components.Models, nil
}

func newModelsDocsHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions RootCommandOptions,
) (*commandregistry.Registry, climanifestcobra.ModelsInvokeFlagBindings, error) {
	if operatorDefaults == nil {
		operatorDefaults = &cliOperatorDefaultsOptions{}
	}
	invokeCfg := modelscli.InvokeConfig{Server: globals.server, Operation: "TTS"}
	registry, err := commandregistry.NewModelsDocsRegistry(commandregistry.ModelsDocsHandlers{
		DocsRunE: commandregistry.DocsRunE(commandregistry.DocsBinding{
			BinaryName:        cliBinaryName,
			DiagnosticsWriter: diagnostics.writer,
			Verbose:           diagnostics.verboseEnabled,
		}),
		ModelsListRunE: commandregistry.ModelsListRunE(commandregistry.ModelsListBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			ListModels:        listModels,
		}),
		ModelsInspectRunE: commandregistry.ModelsInspectRunE(commandregistry.ModelsInspectBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			InspectModel:      inspectModel,
		}),
		ModelsInvokeRunE: commandregistry.ModelsInvokeRunE(commandregistry.ModelsInvokeBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Operation:         &invokeCfg.Operation,
			Text:              &invokeCfg.Text,
			OutputPath:        &invokeCfg.OutputPath,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			HomeDir:           rootOptions.HomeDir,
			ResolveOperatorDefaults: func(cmd *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
				return resolveOperatorDefaults(cmd, operatorDefaults, rootOptions, homeDir)
			},
			BuildLogger: func() (*zap.Logger, error) {
				policy := diagnostics.resolvePolicy(false)
				return policy.BuildLogger(logging.BuildLogger)
			},
			BuildModelInvocation: rootOptions.BuildModelInvocation,
			InvokeModel:          invokeModel,
		}),
		ModelsPullRunE: commandregistry.ModelsPullRunE(commandregistry.ModelsPullBinding{
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			PullModel:         pullModel,
		}),
	})
	if err != nil {
		return nil, climanifestcobra.ModelsInvokeFlagBindings{}, err
	}
	return registry, climanifestcobra.ModelsInvokeFlagBindings{
		Operation:  &invokeCfg.Operation,
		Text:       &invokeCfg.Text,
		OutputPath: &invokeCfg.OutputPath,
		FlagUsages: map[string]string{
			"operation": "uppercase provider-agnostic operation name",
			"text":      "text input for direct invocation",
			"output":    "output file path for streamed audio responses",
		},
	}, nil
}
