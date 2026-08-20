package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/cliversion"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/cobracompletion"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

func newRootCommandWithGeneratedRepresentativeFamily(options CommandFactory) *cobra.Command {
	globals := &cliGlobalOptions{
		server:    cliserver.DefaultBaseURI,
		placement: climanifest.ResolveRootPlacement(false),
	}
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
	factoryConfigInit := productionFactoryConfigInitCommands(diagnostics, options)
	docsCmd, err := newProductionDocsCommand(diagnostics)
	if err != nil {
		panic(fmt.Sprintf("build docs command: %v", err))
	}
	modelsCmd, err := newProductionModelsCommand(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build models command: %v", err))
	}
	providersCmd, err := newProductionProvidersCommand(diagnostics, options)
	if err != nil {
		panic(fmt.Sprintf("build providers command: %v", err))
	}
	metricsCmd := visualizationcli.NewMetricsCommand(visualizationcli.MetricsCommandConfig{
		Query: options.runtimeMetricsQuery, HomeDir: options.homeDir,
		JSON: func() bool { return globals != nil && globals.json },
	})
	b12, err := newB12ProductionFamilies(globals, diagnostics, operatorDefaults, options)
	if err != nil {
		panic(fmt.Sprintf("build B12 production families: %v", err))
	}

	return NewRootCommandFromSubcommands(root, RootSubcommands{Commands: productionRootSubcommands(
		globals, diagnostics, options, factoryConfigInit, docsCmd, modelsCmd, providersCmd, metricsCmd, b12,
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
	jsonOutput, err := inputs.Bool("you.flag.json")
	if err != nil {
		return err
	}
	server, err := inputs.String("you.flag.server")
	if err != nil {
		return err
	}
	remote, err := inputs.Bool("you.flag.remote")
	if err != nil {
		return err
	}
	verbose, err := inputs.Bool("you.flag.verbose")
	if err != nil {
		return err
	}

	diagnostics.debug = debug
	globals.json = jsonOutput
	globals.server = server
	globals.remote = remote
	globals.placement = climanifest.ResolveRootPlacement(remote)
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
	root.Version = cliversion.String()
	root.SetVersionTemplate("{{.Version}}\n")
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
	providersCmd *cobra.Command,
	metricsCmd *cobra.Command,
	b12 b12ProductionFamilies,
) []*cobra.Command {
	return []*cobra.Command{
		docsCmd,
		factoryConfigInit.Config,
		factoryConfigInit.Factory,
		factoryConfigInit.Init,
		b12.MCP,
		modelsCmd,
		providersCmd,
		metricsCmd,
		b12.Run,
		b12.Server,
		b12.Submit,
		productionServeCommand(options),
		productionWorkCommand(globals, diagnostics, options),
		productionWorkerSessionsCommand(globals, diagnostics, options),
		productionWorkersCommand(options),
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

func executeRunCommand(cmd *cobra.Command, args []string, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions CommandFactory) error {
	promptArgs, resolvedConfig, err := resolveRunCommandInvocationInput(
		cmd, args, defaultcmd.ExplicitRunConfig(rootOptions.runDefaults),
	)
	if err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if err := validateRunRemoteHostingConflict(cmd, globals); err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	if err := applyRunCommandInvocationOutputMode(cmd, &resolvedConfig); err != nil {
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
		return err
	}
	outputExplicit, changedErr := climanifestcobra.InputChanged(cmd, "you.run.flag.output")
	if changedErr != nil {
		return changedErr
	}
	if err := runcli.ValidateInvocationOutputSelection(
		resolvedConfig.SuppressDashboardRendering,
		globals.json,
		outputExplicit,
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
		return handleRunExecutionError(cmd, resolvedConfig, promptArgs, globals, basePolicy, err, currentFactorySelected)
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
		!cmd.Flags().Changed("replay") &&
		!cmd.Flags().Changed("resume")
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
	changed, err := climanifestcobra.InputChanged(cmd, "you.run.flag.output")
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	cfg.InvocationOutputExplicit = true
	normalized, err := runcli.NormalizeInvocationOutputMode(cfg.InvocationOutputMode)
	if err != nil {
		return err
	}
	cfg.InvocationOutputMode = normalized
	return nil
}

func resolveRunCommandInvocationInput(cmd *cobra.Command, args []string, base runcli.RunConfig) ([]string, runcli.RunConfig, error) {
	args, err := parseRunCommandArgs(cmd, args)
	if err != nil {
		return nil, base, err
	}
	if err := climanifestcobra.RefreshResolvedPersistentInputs(cmd); err != nil {
		return nil, base, err
	}
	if err := rejectDeprecatedPortFlag(cmd, nil); err != nil {
		return nil, base, err
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return nil, base, err
	}
	cfg, err := applyRunResolvedInputs(base, values)
	if err != nil {
		return nil, base, err
	}
	cfg.ListenExplicit, err = climanifestcobra.InputChanged(cmd, runListenInputID)
	if err != nil {
		return nil, base, err
	}
	cfg.InvocationFileExplicit, err = climanifestcobra.InputChanged(cmd, "you.run.flag.to-file")
	if err != nil {
		return nil, base, err
	}
	mockWorkersExplicit, err := climanifestcobra.InputChanged(cmd, "you.run.flag.with-mock-workers")
	if err != nil {
		return nil, base, err
	}
	cfg.MockWorkersEnabled = mockWorkersExplicit
	skipPermissionsExplicit, err := climanifestcobra.InputChanged(cmd, "you.run.flag.skip-permissions")
	if err != nil {
		return nil, base, err
	}
	if skipPermissionsExplicit {
		override := true
		cfg.InvocationSkipPermissionsOverride = &override
	}
	if cfg.MockWorkersConfigPath == defaultMockWorkersConfigPathSentinel {
		// Bare --with-mock-workers already applied its no-option default in the
		// tokenizer. Do not steal the next remainder token: it may be a
		// factory-signature flag such as --to or a positional prompt.
		cfg.MockWorkersConfigPath = ""
	}
	return args, cfg, nil
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
	registry, err := commandregistry.NewRunServerRegistry(commandregistry.RunServerHandlers{
		Run: commandregistry.CommandHandlers{
			PreRunE: func(cmd *cobra.Command, args []string) error {
				if err := validateRunServerPlacement(globals, "you.run"); err != nil {
					return err
				}
				if err := validateRunListenPlacement(cmd); err != nil {
					return err
				}
				return rejectDeprecatedPortFlag(cmd, args)
			},
			RunE: func(cmd *cobra.Command, args []string) error {
				return executeRunCommand(
					cmd, args, globals, diagnostics, operatorDefaults, rootOptions,
				)
			},
		},
		Server: commandregistry.CommandHandlers{
			PreRunE: func(cmd *cobra.Command, args []string) error {
				if err := validateRunServerPlacement(globals, "you.server"); err != nil {
					return err
				}
				return rejectDeprecatedPortFlag(cmd, args)
			},
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
	return registry, newRunServerFlagBindings(), nil
}

func validateRunServerPlacement(globals *cliGlobalOptions, commandID string) error {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return fmt.Errorf("resolve command %q placement: %w", commandID, err)
	}
	record, err := manifest.CommandByID(commandID)
	if err != nil {
		return fmt.Errorf("resolve command %q placement: %w", commandID, err)
	}
	remote := globals != nil && globals.remote
	if _, err := record.ResolvePlacement(remote); err != nil {
		return err
	}
	return nil
}

func validateRunListenPlacement(cmd *cobra.Command) error {
	changed, err := climanifestcobra.InputChanged(cmd, runListenInputID)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	withServer, err := climanifestcobra.InputChanged(cmd, "you.run.flag.with-server")
	if err != nil {
		return err
	}
	withSite, err := climanifestcobra.InputChanged(cmd, "you.run.flag.with-site")
	if err != nil {
		return err
	}
	if withServer || withSite {
		return nil
	}
	return fmt.Errorf("input relationship %q: --listen requires --with-server or --with-site", "you.run.rel.listen-server")
}

func validateRunRemoteHostingConflict(cmd *cobra.Command, globals *cliGlobalOptions) error {
	if globals == nil || !globals.remote {
		return nil
	}
	withServer, err := climanifestcobra.InputChanged(cmd, "you.run.flag.with-server")
	if err != nil {
		return err
	}
	withSite, err := climanifestcobra.InputChanged(cmd, "you.run.flag.with-site")
	if err != nil {
		return err
	}
	if !withServer && !withSite {
		return nil
	}
	return newRunRemoteLocalHostingConflictError()
}

func newRunRemoteLocalHostingConflictError() error {
	return &runcli.InvocationError{
		Code:    runcli.RemoteLocalHostingConflictCode,
		Message: "--remote selects a running server through --server and cannot be combined with --with-server or --with-site; remove --remote for local hosting and use --listen <host:port> to choose an exact local bind",
	}
}

func productionWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, injected ...CommandFactory) *cobra.Command {
	dependencies := CommandFactory{}
	if len(injected) > 0 {
		dependencies = injected[0]
	}
	// The resolved-input owner adapters are the canonical production path. Cobra
	// only forwards the generated values to the Work transport handlers.
	handlers := commandregistry.ResolvedWorkHandlers{
		ApprovalList: commandregistry.ResolvedApprovalListRunE(commandregistry.ResolvedApprovalListBinding{
			ListHumanApprovals: dependencies.ListHumanApprovals,
			DiagnosticsWriter:  diagnostics.writer,
		}),
		ApprovalShow: commandregistry.ResolvedApprovalShowRunE(commandregistry.ResolvedApprovalShowBinding{
			ShowHumanApproval: dependencies.ShowHumanApproval,
			DiagnosticsWriter: diagnostics.writer,
		}),
		List: commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
			ListWork:          dependencies.ListWork,
			DiagnosticsWriter: diagnostics.writer,
		}),
		Watch: commandregistry.ResolvedWatchRunE(commandregistry.ResolvedWatchBinding{
			WatchWork:         dependencies.WatchWork,
			DiagnosticsWriter: diagnostics.writer,
		}),
		Show: commandregistry.ResolvedShowRunE(commandregistry.ResolvedShowBinding{
			ShowWork:          dependencies.ShowWork,
			DiagnosticsWriter: diagnostics.writer,
		}),
		Move: commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{
			MoveWork:          dependencies.MoveWork,
			DiagnosticsWriter: diagnostics.writer,
		}),
		Visualize: commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{
			VisualizeWork: dependencies.VisualizeWork,
		}),
	}
	work, err := climanifestcobra.NewResolvedWorkCommand(handlers)
	if err != nil {
		panic(fmt.Sprintf("build resolved work command: %v", err))
	}
	return work
}

func commandInputValue[T any](values map[string]any, inputID string) (T, error) {
	var zero T
	value, ok := values[inputID]
	if !ok {
		return zero, fmt.Errorf("resolved CLI input %q is unavailable", inputID)
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("resolved CLI input %q has incompatible type %T", inputID, value)
	}
	return typed, nil
}

func optionalCommandInputValue[T any](values map[string]any, inputID string) (T, error) {
	if _, ok := values[inputID]; !ok {
		var zero T
		return zero, nil
	}
	return commandInputValue[T](values, inputID)
}

func generatedCommandInputs(cmd *cobra.Command) (map[string]any, error) {
	values, err := climanifestcobra.InputValues(cmd)
	if err != nil {
		return nil, fmt.Errorf("resolve generated command inputs: %w", err)
	}
	return values, nil
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
