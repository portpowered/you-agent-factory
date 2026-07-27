package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/factoryload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

func newRootCommandWithGeneratedRepresentativeFamily(options CommandFactory) *cobra.Command {
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}

	registry, err := newRepresentativeHandlerRegistry(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build representative handler registry: %v", err))
	}
	sessionRegistry, err := newSessionHandlerRegistry(diagnostics, options)
	if err != nil {
		panic(fmt.Sprintf("build session handler registry: %v", err))
	}
	root, err := newGenericRepresentativeFamily(
		registry,
		sessionRegistry,
		representativeSourceValues(options),
		func(inputs resolvedinput.Inputs) error {
			return applyRepresentativeResolvedInputs(
				inputs,
				globals,
				diagnostics,
				operatorDefaults,
			)
		},
	)
	if err != nil {
		panic(fmt.Sprintf("build representative family command: %v", err))
	}

	factoryConfigInit := productionFactoryConfigInitCommands(globals, diagnostics, options)
	docsCmd, err := newProductionDocsCommand(diagnostics)
	if err != nil {
		panic(fmt.Sprintf("build docs command: %v", err))
	}
	modelsCmd, err := newProductionModelsCommand(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build models command: %v", err))
	}
	b12, err := newB12ProductionFamilies(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build B12 production families: %v", err))
	}

	return NewRootCommandFromSubcommands(root, RootSubcommands{Commands: productionRootSubcommands(
		globals, diagnostics, options, factoryConfigInit, docsCmd, modelsCmd, b12,
	)})
}

func newGenericRepresentativeFamily(
	representativeRegistry *commandregistry.Registry,
	sessionRegistry *commandregistry.Registry,
	sourceValues climanifestcobra.SourceCandidateProvider,
	rootInputs climanifestcobra.ResolvedInputsBinding,
) (*cobra.Command, error) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, err
	}
	sessionManifest, err := generated.SessionFamilyManifest()
	if err != nil {
		return nil, err
	}
	for commandID, record := range sessionManifest.Commands {
		manifest.Commands[commandID] = record
	}
	handlers := make(climanifestcobra.CobraHandlerRegistry)
	rootRecord, err := manifest.CommandByID("you")
	if err != nil {
		return nil, err
	}
	rootHandlers, err := representativeRegistry.LookupHandlers(rootRecord.ID)
	if err != nil {
		return nil, err
	}
	handlers[rootRecord.Handler.ID] = productionGenericCobraHandler(rootHandlers)

	resolvedHandlers := make(climanifestcobra.ResolvedCobraHandlerRegistry)
	for _, record := range sessionManifest.Commands {
		if !record.Runnable {
			continue
		}
		registered, lookupErr := sessionRegistry.LookupHandlers(record.Handler.ID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		resolvedHandlers[record.Handler.ID] = registered.ResolvedRunE
	}
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		CobraHandlers:         handlers,
		ResolvedCobraHandlers: resolvedHandlers,
		SourceValues:          sourceValues,
		RootInputs:            rootInputs,
	})
	if err != nil {
		return nil, err
	}
	root.SilenceUsage = true
	return root, nil
}

func productionGenericCobraHandler(
	handlers commandregistry.CommandHandlers,
) climanifestcobra.CobraHandler {
	return func(
		cmd *cobra.Command,
		args []string,
		values map[string]any,
		_ resolvedinput.Inputs,
	) error {
		if handlers.PreRunE != nil {
			if err := handlers.PreRunE(cmd, args); err != nil {
				return err
			}
		}
		return handlers.RunE(cmd, args)
	}
}

func applyRepresentativeResolvedInputs(
	inputs resolvedinput.Inputs,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
) error {
	debug, err := inputs.Bool("you.flag.debug")
	if err != nil {
		return err
	}
	defaultWorkerModel, err := inputs.String("you.flag.default-worker-model")
	if err != nil {
		return err
	}
	defaultWorkerModelProvider, err := inputs.String("you.flag.default-worker-model-provider")
	if err != nil {
		return err
	}
	jsonOutput, err := inputs.Bool("you.flag.json")
	if err != nil {
		return err
	}
	server, err := inputs.String("you.flag.server")
	if err != nil {
		return err
	}
	verbose, err := inputs.Bool("you.flag.verbose")
	if err != nil {
		return err
	}

	diagnostics.debug = debug
	operatorDefaults.defaultWorkerModel = defaultWorkerModel
	operatorDefaults.defaultWorkerModelProvider = defaultWorkerModelProvider
	globals.json = jsonOutput
	globals.server = server
	diagnostics.verbose = verbose
	return nil
}

// RootSubcommands contains the already-constructed top-level command families
// injected into the root command constructor.
type RootSubcommands struct {
	Commands []*cobra.Command
}

// NewRootCommandFromSubcommands is the root command constructor boundary. The
// root owns only its persistent behavior and receives its top-level commands
// from the command composition graph.
func NewRootCommandFromSubcommands(root *cobra.Command, subcommands RootSubcommands) *cobra.Command {
	root.AddCommand(subcommands.Commands...)
	return root
}

// b12ProductionFamilies is the one shared production-root fan-in for the
// session, workflow/MCP, run/server, and submit migrations. Each field is
// constructed once through its family-local generated seam.
type b12ProductionFamilies struct {
	MCP    *cobra.Command
	Run    *cobra.Command
	Server *cobra.Command
	Submit *cobra.Command
}

func newB12ProductionFamilies(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options CommandFactory,
) (b12ProductionFamilies, error) {
	runServer := productionRunServerCommands(globals, diagnostics, operatorDefaults, options)
	submitRegistry, err := commandregistry.NewSubmitRegistry(commandregistry.SubmitHandlers{
		Submit: commandregistry.UnarySubmitHandler(options.SubmitWork),
		SubmitBatch: commandregistry.BatchSubmitHandler(
			options.SubmitBatch,
			commandregistry.BatchSubmitEffects{
				FileSystem: options.batchInputFileSystem,
				StdinIsTTY: startupcli.StdinIsTTY,
			},
		),
	})
	if err != nil {
		return b12ProductionFamilies{}, err
	}
	submitCommand, err := climanifestcobra.NewSubmitFamilyCommand(submitRegistry)
	if err != nil {
		return b12ProductionFamilies{}, err
	}
	if err := preserveSubmitArgumentCompatibility(submitCommand); err != nil {
		return b12ProductionFamilies{}, err
	}
	mcpCommand, err := newMCPCommand(options)
	if err != nil {
		return b12ProductionFamilies{}, err
	}
	return b12ProductionFamilies{
		MCP:    mcpCommand,
		Run:    runServer.Run,
		Server: runServer.Server,
		Submit: submitCommand,
	}, nil
}

// preserveSubmitArgumentCompatibility keeps the established public Cobra
// argument shape while the isolated Submit constructor retains manifest-owned
// argument resolution. The canonical batch resolver must run before its
// relationship validation, even though the production root leaves Args unset.
func preserveSubmitArgumentCompatibility(submit *cobra.Command) error {
	if submit == nil {
		return fmt.Errorf("preserve submit argument compatibility: submit command is required")
	}
	submit.Args = nil
	submit.Long = strings.TrimPrefix(submit.Long, submit.Short+"\n\n")
	submit.Example = ""
	for _, flagName := range []string{"name", "payload", "work-type-name"} {
		flag := submit.Flags().Lookup(flagName)
		if flag == nil {
			return fmt.Errorf("preserve submit argument compatibility: flag %q is required", flagName)
		}
		delete(flag.Annotations, cobra.BashCompOneRequiredFlag)
		delete(flag.Annotations, "infinite-you/required")
	}

	batch, _, err := submit.Find([]string{"batch"})
	if err != nil {
		return fmt.Errorf("preserve submit argument compatibility: find batch command: %w", err)
	}
	if batch == nil {
		return fmt.Errorf("preserve submit argument compatibility: batch command is required")
	}
	batch.Long = strings.TrimPrefix(batch.Long, batch.Short+"\n\n")
	resolveArguments := batch.Args
	if resolveArguments == nil {
		return fmt.Errorf("preserve submit argument compatibility: batch argument resolver is required")
	}
	validateInputs := batch.PreRunE
	batch.Args = nil
	batch.PreRunE = func(cmd *cobra.Command, args []string) error {
		if err := resolveArguments(cmd, args); err != nil {
			return err
		}
		if validateInputs != nil {
			return validateInputs(cmd, args)
		}
		return nil
	}
	return nil
}

func productionRootSubcommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
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
		b12.Server,
		b12.Submit,
		productionWorkCommand(globals, diagnostics, options),
	}
}

type runServerProductionCommands struct {
	Run    *cobra.Command
	Server *cobra.Command
}

func productionRunServerCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options CommandFactory,
) runServerProductionCommands {
	commands, err := buildRunServerProductionCommands(
		globals, diagnostics, operatorDefaults, options,
	)
	if err != nil {
		panic(fmt.Sprintf("build run/server family commands: %v", err))
	}
	return commands
}

func buildRunServerProductionCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	options CommandFactory,
) (runServerProductionCommands, error) {
	registry, bindings, err := newRunServerHandlerRegistry(
		globals, diagnostics, operatorDefaults, options,
	)
	if err != nil {
		return runServerProductionCommands{}, err
	}
	components, err := climanifestcobra.NewRunServerFamilyComponents(registry, bindings)
	if err != nil {
		return runServerProductionCommands{}, err
	}
	if err := registerSelectedFactoryNameCompletion(components.Run, options); err != nil {
		return runServerProductionCommands{}, err
	}
	if err := registerSelectedFactorySignatureCompletion(components.Run, options); err != nil {
		return runServerProductionCommands{}, err
	}
	return runServerProductionCommands{
		Run: components.Run, Server: components.Server,
	}, nil
}

func registerSelectedFactoryNameCompletion(run *cobra.Command, options CommandFactory) error {
	return cobracompletion.RegisterFactoryNames(
		run, options.completeFactoryNames, selectedFactoryCompletionRootsResolver(options),
	)
}

func registerSelectedFactorySignatureCompletion(run *cobra.Command, options CommandFactory) error {
	return cobracompletion.RegisterSelectedFactorySignature(
		run,
		options.completeSelectedFactorySignature,
		selectedFactoryCompletionRootsResolver(options),
	)
}

func selectedFactoryCompletionRootsResolver(options CommandFactory) cobracompletion.FactoryNamesRequestResolver {
	return func(cmd *cobra.Command, enteredPrefix string) (cobracompletion.FactoryNamesRequest, bool) {
		if cmd == nil || options.resolveNamedFactoryRoots == nil {
			return cobracompletion.FactoryNamesRequest{}, false
		}
		workingDirectory := startupcli.WorkingDirectory(cmd.Context())
		if strings.TrimSpace(workingDirectory) == "" {
			return cobracompletion.FactoryNamesRequest{}, false
		}
		home, err := resolveProcessHomeDir(options)
		if err != nil {
			return cobracompletion.FactoryNamesRequest{}, false
		}
		roots, err := options.resolveNamedFactoryRoots(home, workingDirectory)
		if err != nil {
			return cobracompletion.FactoryNamesRequest{}, false
		}
		return cobracompletion.FactoryNamesRequest{
			ProjectRoot:   roots.Project,
			GlobalRoot:    roots.Global,
			EnteredPrefix: enteredPrefix,
		}, true
	}
}

func executeRunCommand(cmd *cobra.Command, args []string, cfg *runcli.RunConfig, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions CommandFactory) error {
	promptArgs, resolvedConfig, err := resolveRunCommandInvocationInput(cmd, args, cfg)
	if err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if err := applyRunCommandInvocationOutputMode(cmd, &resolvedConfig); err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if err := runcli.ValidateInvocationOutputSelection(
		resolvedConfig.SuppressDashboardRendering,
		globals.json,
		cmd.Flags().Changed("output"),
	); err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if helpRequested(cmd) {
		return writeRunCommandHelp(cmd, &resolvedConfig, rootOptions)
	}
	currentFactorySelected := runUsesCurrentFactory(cmd)
	if currentFactorySelected {
		if err := selectCurrentFactoryFromWorkingDirectory(cmd, &resolvedConfig); err != nil {
			mapped := runcli.MapCurrentFactoryFailure(err)
			_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
			return mapped
		}
	}
	basePolicy := diagnostics.resolvePolicy(resolvedConfig.SuppressDashboardRendering)
	err = runFactoryWithOptions(cmd, resolvedConfig, promptArgs, globals, operatorDefaults, basePolicy, rootOptions, false)
	if err != nil {
		err = factoryload.MaybeFormatOperatorError(err, resolvedConfig.Dir)
		err = runcli.MapServerFailure(err)
		if currentFactorySelected {
			err = runcli.MapCurrentFactoryFailure(err)
		}
		if len(promptArgs) > 0 {
			err = runcli.MapInvocationFailure(err)
		}
		if runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json) {
			return err
		}
		errorWriter := resolveEffectiveRunPolicy(cmd, resolvedConfig, basePolicy).HumanTerminalWriter(cmd.ErrOrStderr())
		var ambiguousInputErr *runcli.AmbiguousInvocationInputError
		if errors.As(err, &ambiguousInputErr) {
			errorWriter = cmd.ErrOrStderr()
		}
		if errorWriter != nil {
			_, _ = fmt.Fprintln(errorWriter, err)
		}
	}
	return err
}

func applyRunScopedServerMode(cfg runcli.RunConfig) runcli.RunConfig {
	if cfg.WithSite {
		cfg.WithServer = true
		cfg.OpenDashboard = true
	}
	return cfg
}

func runUsesCurrentFactory(cmd *cobra.Command) bool {
	return cmd != nil &&
		!cmd.Flags().Changed("dir") &&
		!cmd.Flags().Changed("factory") &&
		!cmd.Flags().Changed("named") &&
		!cmd.Flags().Changed("replay")
}

func selectCurrentFactoryFromWorkingDirectory(cmd *cobra.Command, cfg *runcli.RunConfig) error {
	if cmd == nil || cfg == nil {
		return fmt.Errorf("select Current Factory: run command and config are required")
	}
	workingDirectory := strings.TrimSpace(startupcli.WorkingDirectory(cmd.Context()))
	if workingDirectory == "" {
		return fmt.Errorf("select Current Factory: process working directory is required")
	}
	factoryDir := filepath.Join(workingDirectory, defaultcmd.FactoryDir)
	cfg.Dir = factoryDir
	cfg.FactoryConfigPath = filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	return nil
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
	if err := climanifestcobra.RefreshResolvedPersistentInputs(cmd); err != nil {
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

func writeRunCommandHelp(cmd *cobra.Command, cfg *runcli.RunConfig, rootOptions CommandFactory) error {
	cfg.ResolveFactoryConfigRoot = rootOptions.resolveFactoryConfigRoot
	cfg.LoadFactoryConfigFile = rootOptions.loadFactoryConfigFile
	cfg.WorkRequestFileLoader = rootOptions.workRequestFileLoader
	homeDir, err := resolveProcessHomeDir(rootOptions)
	if err != nil {
		return err
	}
	cfg.HomeDir = homeDir
	if err := resolveRunFactorySelection(
		cmd,
		cfg,
		homeDir,
		rootOptions.namedFactoryCatalog,
		rootOptions.resolveNamedFactoryRoots,
		rootOptions.resolveNamedFactoryCandidatePaths,
	); err != nil {
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

func newRunServerHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions CommandFactory,
) (*commandregistry.Registry, climanifestcobra.RunServerFlagBindings, error) {
	runCfg := defaultcmd.ExplicitRunConfig(rootOptions.runDefaults)
	var invocationOutputMode string
	registry, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run: commandregistry.CommandHandlers{
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, args []string) error {
				return executeRunCommand(
					cmd, args, &runCfg, globals, diagnostics, operatorDefaults, rootOptions,
				)
			},
		},
		Server: commandregistry.CommandHandlers{
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return executeServerCommand(
					cmd, globals, diagnostics, operatorDefaults, rootOptions,
				)
			},
		},
	})
	if err != nil {
		return nil, climanifestcobra.RunServerFlagBindings{}, err
	}
	return registry, climanifestcobra.RunServerFlagBindings{
		Run:                 &runCfg,
		RunInvocationOutput: &invocationOutputMode,
		RunLocalTargets:     runLocalTargets(&runCfg, &invocationOutputMode),
	}, nil
}

func runLocalTargets(cfg *runcli.RunConfig, invocationOutput *string) map[string]any {
	var skipPermissions bool
	return map[string]any{
		"continuously": &cfg.Continuously, "work": &cfg.WorkFile, "dir": &cfg.Dir,
		"named": &cfg.NamedFactoryName, "factory": &cfg.FactoryConfigPath,
		"record": &cfg.RecordPath, "no-record": &cfg.DisableDefaultRecording,
		"replay": &cfg.ReplayPath, "runtime-log-dir": &cfg.RuntimeLogDir,
		"runtime-log-max-size-mb":      &cfg.RuntimeLogConfig.MaxSize,
		"runtime-log-max-backups":      &cfg.RuntimeLogConfig.MaxBackups,
		"runtime-log-max-age-days":     &cfg.RuntimeLogConfig.MaxAge,
		"runtime-log-compress":         &cfg.RuntimeLogConfig.Compress,
		"runtime-metrics-dir":          &cfg.RuntimeMetricsDir,
		"runtime-metrics-max-size-mb":  &cfg.RuntimeMetricsConfig.MaxSize,
		"runtime-metrics-max-backups":  &cfg.RuntimeMetricsConfig.MaxBackups,
		"runtime-metrics-max-age-days": &cfg.RuntimeMetricsConfig.MaxAge,
		"runtime-metrics-compress":     &cfg.RuntimeMetricsConfig.Compress,
		"with-mock-workers":            &cfg.MockWorkersConfigPath,
		"with-server":                  &cfg.WithServer, "with-site": &cfg.WithSite,
		"quiet": &cfg.SuppressDashboardRendering, "output": invocationOutput,
		"skip-permissions": &skipPermissions,
	}
}

func productionWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, injected ...CommandFactory) *cobra.Command {
	dependencies := CommandFactory{}
	if len(injected) > 0 {
		dependencies = injected[0]
	}
	registry, bindings, err := newWorkHandlerRegistry(globals, diagnostics, dependencies)
	if err != nil {
		panic(fmt.Sprintf("build work handler registry: %v", err))
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		panic(fmt.Sprintf("build work family command: %v", err))
	}
	return work
}

func newWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, injected ...CommandFactory) *cobra.Command {
	dependencies := CommandFactory{}
	if len(injected) > 0 {
		dependencies = injected[0]
	}
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
	workCmd.AddCommand(newWorkListCommand(globals, diagnostics, dependencies))
	workCmd.AddCommand(newWorkShowCommand(globals, diagnostics, dependencies))
	workCmd.AddCommand(newWorkMoveCommand(globals, diagnostics, dependencies))
	workCmd.AddCommand(newWorkVisualizeCommand(dependencies))
	return workCmd
}

func newWorkVisualizeCommand(dependencies CommandFactory) *cobra.Command {
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
			return dependencies.VisualizeWork(workcli.VisualizeConfig{
				BatchFile: args[0],
				Format:    format,
				Output:    cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().Int("port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
	cmd.Flags().StringVar(&format, "format", "mermaid", "output format: mermaid or markdown-mermaid")
	return cmd
}

func newWorkListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.ListWork(cfg)
		},
	}

	cmd.Flags().Int("port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
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

func newWorkShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.ShowWork(cfg)
		},
	}

	cmd.Flags().Int("port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newWorkMoveCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.StateName = args[1]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.MoveWork(cfg)
		},
	}

	cmd.Flags().Int("port", 0, "deprecated; use --server")
	_ = cmd.Flags().MarkHidden("port")
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
	dependencies CommandFactory,
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
			ListWork:          dependencies.ListWork,
		}),
		ShowRunE: commandregistry.ShowRunE(commandregistry.ShowBinding{
			Config:            showCfg,
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			ShowWork:          dependencies.ShowWork,
		}),
		MoveRunE: commandregistry.MoveRunE(commandregistry.MoveBinding{
			Config:            moveCfg,
			Server:            &globals.server,
			JSON:              &globals.json,
			Verbose:           diagnostics.verboseEnabled,
			Debug:             &diagnostics.debug,
			DiagnosticsWriter: diagnostics.writer,
			MoveWork:          dependencies.MoveWork,
		}),
		VisualizeRunE: commandregistry.VisualizeRunE(commandregistry.VisualizeBinding{
			Format:    visualizeFormatPtr,
			Visualize: dependencies.VisualizeWork,
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
	rootOptions CommandFactory,
) (*commandregistry.Registry, error) {
	if operatorDefaults == nil {
		operatorDefaults = &cliOperatorDefaultsOptions{}
	}
	return commandregistry.NewRepresentativeRegistry(commandregistry.RepresentativeHandlers{
		RootRunE: func(cmd *cobra.Command, args []string) error {
			policy := diagnostics.resolvePolicy(false)
			return runFactoryWithOptions(cmd, defaultcmd.OOTBRunConfig(rootOptions.runDefaults), nil, globals, operatorDefaults, policy, rootOptions, true)
		},
	})
}
