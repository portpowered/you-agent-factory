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

	factoryConfigInit := productionFactoryConfigInitCommands(diagnostics, options)
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

func executeRunCommand(cmd *cobra.Command, args []string, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions, rootOptions CommandFactory) error {
	promptArgs, resolvedConfig, err := resolveRunCommandInvocationInput(
		cmd, args, defaultcmd.ExplicitRunConfig(rootOptions.runDefaults),
	)
	if err != nil {
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
	changed, err := climanifestcobra.InputChanged(cmd, "you.run.flag.output")
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
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
	promptArgs := args
	if cfg.MockWorkersConfigPath != defaultMockWorkersConfigPathSentinel {
		return promptArgs, cfg, nil
	}
	if len(args) == 0 {
		cfg.MockWorkersConfigPath = ""
		return promptArgs, cfg, nil
	}
	cfg.MockWorkersConfigPath = args[0]
	return args[1:], cfg, nil
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
			PreRunE: rejectDeprecatedPortFlag,
			RunE: func(cmd *cobra.Command, args []string) error {
				return executeRunCommand(
					cmd, args, globals, diagnostics, operatorDefaults, rootOptions,
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
	return registry, newRunServerFlagBindings(), nil
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

func newWorkFamilyBindings() climanifestcobra.WorkFamilyBindings {
	format := scalarTarget("mermaid")
	return climanifestcobra.WorkFamilyBindings{LocalTargets: map[string]any{
		"you.work.list.flag.state-name":     scalarTarget(""),
		"you.work.list.flag.state-type":     scalarTarget(""),
		"you.work.list.flag.name":           scalarTarget(""),
		"you.work.list.flag.work-type-name": scalarTarget(""),
		"you.work.list.flag.trace-id":       scalarTarget(""),
		"you.work.list.flag.sort-by":        scalarTarget(""),
		"you.work.list.flag.max-results":    scalarTarget(0),
		"you.work.list.flag.next-token":     scalarTarget(""),
		"you.work.list.flag.session":        scalarTarget(""),
		"you.work.show.flag.session":        scalarTarget(""),
		"you.work.move.flag.session":        scalarTarget(""),
		"you.work.move.flag.request-id":     scalarTarget(""),
		"you.work.visualize.flag.format":    format,
	}}
}

func newWorkHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	dependencies CommandFactory,
) (*commandregistry.Registry, climanifestcobra.WorkFamilyBindings, error) {
	bindings := newWorkFamilyBindings()
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: func(cmd *cobra.Command, _ []string) error {
			return executeGeneratedWorkList(cmd, globals, diagnostics, dependencies.ListWork)
		},
		ShowRunE: func(cmd *cobra.Command, args []string) error {
			return executeGeneratedWorkShow(cmd, args, globals, diagnostics, dependencies.ShowWork)
		},
		MoveRunE: func(cmd *cobra.Command, args []string) error {
			return executeGeneratedWorkMove(cmd, args, globals, diagnostics, dependencies.MoveWork)
		},
		VisualizeRunE: func(cmd *cobra.Command, args []string) error {
			return executeGeneratedWorkVisualize(cmd, args, dependencies.VisualizeWork)
		},
	})
	if err != nil {
		return nil, climanifestcobra.WorkFamilyBindings{}, err
	}
	return registry, bindings, nil
}

func scalarTarget[T bool | string | int](value T) *T {
	return &value
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

func generatedCommandInputs(cmd *cobra.Command) (map[string]any, error) {
	values, err := climanifestcobra.InputValues(cmd)
	if err != nil {
		return nil, fmt.Errorf("resolve generated command inputs: %w", err)
	}
	return values, nil
}

func executeGeneratedWorkList(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	list func(workcli.ListConfig) error,
) error {
	if list == nil {
		return fmt.Errorf("work list service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	cfg := workcli.ListConfig{
		Context: cmd.Context(), Server: globals.server, JSON: globals.json,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	}
	fields := []struct {
		id     string
		target *string
	}{
		{"you.work.list.flag.state-name", &cfg.StateName},
		{"you.work.list.flag.state-type", &cfg.StateType},
		{"you.work.list.flag.name", &cfg.Name},
		{"you.work.list.flag.work-type-name", &cfg.WorkTypeName},
		{"you.work.list.flag.trace-id", &cfg.TraceID},
		{"you.work.list.flag.sort-by", &cfg.SortBy},
		{"you.work.list.flag.next-token", &cfg.NextToken},
		{"you.work.list.flag.session", &cfg.SessionID},
	}
	for _, field := range fields {
		*field.target, err = commandInputValue[string](values, field.id)
		if err != nil {
			return err
		}
	}
	cfg.MaxResults, err = commandInputValue[int](values, "you.work.list.flag.max-results")
	if err != nil {
		return err
	}
	return list(cfg)
}

func executeGeneratedWorkShow(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	show func(workcli.ShowConfig) error,
) error {
	if show == nil {
		return fmt.Errorf("work show service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.work.show.flag.session")
	if err != nil {
		return err
	}
	return show(workcli.ShowConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkID: args[0], JSON: globals.json, Output: cmd.OutOrStdout(),
		Diagnostics: diagnostics.writer(cmd), Verbose: diagnostics.verboseEnabled(),
		Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkMove(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	move func(workcli.MoveConfig) error,
) error {
	if move == nil {
		return fmt.Errorf("work move service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.work.move.flag.session")
	if err != nil {
		return err
	}
	requestID, err := commandInputValue[string](values, "you.work.move.flag.request-id")
	if err != nil {
		return err
	}
	return move(workcli.MoveConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkID: args[0], StateName: args[1], RequestID: requestID,
		JSON: globals.json, Output: cmd.OutOrStdout(),
		Diagnostics: diagnostics.writer(cmd), Verbose: diagnostics.verboseEnabled(),
		Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkVisualize(
	cmd *cobra.Command,
	args []string,
	visualize func(workcli.VisualizeConfig) error,
) error {
	if visualize == nil {
		return fmt.Errorf("work visualize service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	format, err := commandInputValue[string](values, "you.work.visualize.flag.format")
	if err != nil {
		return err
	}
	return visualize(workcli.VisualizeConfig{
		BatchFile: args[0], Format: format, Output: cmd.OutOrStdout(),
	})
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
