package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/cobracompletion"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	serverstopcli "github.com/portpowered/infinite-you/pkg/transports/cli/serverstop"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"github.com/spf13/cobra"
)

type systemInitializationContextKey struct{}
type homeDisclosureContextKey struct{}

func systemInitializationCompleted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	completed, _ := ctx.Value(systemInitializationContextKey{}).(bool)
	return completed
}

func homeDisclosureCompleted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	completed, _ := ctx.Value(homeDisclosureContextKey{}).(bool)
	return completed
}

func prepareRunFactoryConfig(
	cmd *cobra.Command,
	cfg runcli.RunConfig,
	promptArgs []string,
	globals *cliGlobalOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	policy terminalpolicy.Policy,
	rootOptions CommandFactory,
	defaultInvocation bool,
) (runcli.RunConfig, error) {
	cfg = applyRunScopedServerMode(cfg)
	if err := validateRunFactoryOptions(&cfg, defaultInvocation); err != nil {
		return runcli.RunConfig{}, err
	}
	logger, err := policy.BuildLogger(rootOptions.buildTerminalLogger)
	if err != nil {
		return runcli.RunConfig{}, err
	}
	cfg.Logger = logger
	cfg.Verbose = policy.VerboseEnabled()
	cfg.TerminalPolicy = policy
	cfg.ExecutionBaseDir = startupcli.WorkingDirectory(cmd.Context())
	homeDir, remote, err := prepareRunFactoryConfigInputs(
		cmd,
		&cfg,
		promptArgs,
		globals,
		policy,
		rootOptions,
		defaultInvocation,
	)
	if err != nil {
		return runcli.RunConfig{}, err
	}
	if err := prepareRunFactoryStartup(cmd, &cfg, rootOptions, remote); err != nil {
		return runcli.RunConfig{}, err
	}
	if err := prepareRunFactoryHomeDisclosure(cmd, &cfg, remote); err != nil {
		return runcli.RunConfig{}, err
	}
	if err := configureRunEnvironment(cmd, &cfg, rootOptions, homeDir); err != nil {
		return runcli.RunConfig{}, err
	}
	if err := resolveRunFactorySelection(
		cmd,
		&cfg,
		homeDir,
		rootOptions.namedFactoryCatalog,
		rootOptions.resolveNamedFactoryRoots,
		rootOptions.resolveNamedFactoryCandidatePaths,
	); err != nil {
		return runcli.RunConfig{}, err
	}

	runOperatorDefaults := *operatorDefaults
	runOperatorDefaults.providerOverride = cfg.ProviderOverride
	runOperatorDefaults.modelOverride = cfg.ModelOverride
	resolvedOperatorDefaults, err := resolveOperatorDefaults(cmd, &runOperatorDefaults, rootOptions, homeDir)
	if err != nil {
		return runcli.RunConfig{}, err
	}
	cfg.OperatorDefaults = resolvedOperatorDefaults
	cfg.Stdin = cmd.InOrStdin()
	cfg.StdinIsTTY = func() bool { return startupcli.StdinIsTTY(cmd.Context()) }
	cfg.OutputIsTTY = startupcli.StdoutIsTTY(cmd.Context())
	if err := resolveRunFactoryPrompt(cmd, &cfg, promptArgs, rootOptions.prepareInvocationInput); err != nil {
		runcli.ObserveInvocationRejection(logger, err)
		return runcli.RunConfig{}, err
	}
	configureRunFactoryOutput(cmd, &cfg, promptArgs, globals, policy)
	return cfg, nil
}

func prepareRunFactoryConfigInputs(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	globals *cliGlobalOptions,
	policy terminalpolicy.Policy,
	rootOptions CommandFactory,
	defaultInvocation bool,
) (string, bool, error) {
	remote := remotePlacementSelected(globals)
	if !remote {
		if err := resolveRunBindFromServer(cmd, globals.server, cfg); err != nil {
			return "", false, err
		}
		cfg.DeferHomeDisclosureUntilHostReady = cfg.ListenExplicit ||
			persistentInputWasCLI(cmd, "you.flag.server", "server")
		warnLegacyListenerBinding(cmd, *cfg, defaultInvocation, persistentInputWasCLI(cmd, "you.flag.server", "server"))
	}
	homeDir, err := resolveProcessHomeDirForCommand(cmd, rootOptions)
	if err != nil {
		return "", false, err
	}
	cfg.HomeDir = homeDir
	cfg.JSON = globals.json
	cfg.JSONOutput = globals.json
	cfg.CleanInvocation = preliminaryRunInvocationOutputIsClean(cmd, *cfg, promptArgs)
	if defaultInvocation {
		// The default server invocation is a hosted human run even when Cobra
		// receives non-TTY streams. Keep the preliminary gate from classifying
		// its selected factory as a clean one-shot invocation.
		cfg.CleanInvocation = false
	}
	if !remote && !cfg.CleanInvocation && !cfg.InvocationOutputExplicit {
		cfg.StartupOutput = policy.HumanTerminalWriter(cmd.OutOrStdout())
	}
	return homeDir, remote, nil
}

func prepareRunFactoryStartup(cmd *cobra.Command, cfg *runcli.RunConfig, options CommandFactory, remote bool) error {
	if remote {
		return nil
	}
	return prepareRunStartup(cmd, cfg, options)
}

func prepareRunFactoryHomeDisclosure(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	remote bool,
) error {
	if remote || cfg == nil || cfg.StartupPreparation == nil {
		return nil
	}
	if !runStartupDisclosureEnabled(*cfg) {
		return cfg.StartupPreparation(cmd.Context(), false, nil)
	}
	if !cfg.DeferHomeDisclosureUntilHostReady && !runHasRecordingInput(*cfg) {
		return cfg.StartupPreparation(cmd.Context(), true, cfg.StartupOutput)
	}

	staged := &bytes.Buffer{}
	if err := cfg.StartupPreparation(cmd.Context(), true, staged); err != nil {
		return err
	}
	if !cfg.DeferHomeDisclosureUntilHostReady && !cfg.StartupPreflightBlocked {
		_, _ = io.Copy(cfg.StartupOutput, staged)
		return nil
	}
	committed := false
	cfg.StartupDisclosureCommit = func() {
		if committed {
			return
		}
		committed = true
		if cfg.StartupOutput != nil {
			_, _ = io.Copy(cfg.StartupOutput, staged)
		}
	}
	return nil
}

func validateRunFactoryOptions(cfg *runcli.RunConfig, defaultInvocation bool) error {
	if cfg == nil {
		return fmt.Errorf("run configuration is required")
	}
	if cfg.Pprof && !defaultInvocation && !cfg.WithServer && !cfg.WithSite {
		return fmt.Errorf("input relationship %q: --pprof requires --with-server or --with-site", "you.run.rel.pprof-server")
	}
	if cfg.ListenExplicit || strings.TrimSpace(cfg.ListenAddress) != "" {
		cfg.ListenExplicit = true
		if !defaultInvocation && !cfg.WithServer && !cfg.WithSite {
			return fmt.Errorf("--listen requires --with-server or --with-site on you run")
		}
	}
	return nil
}

func configureRunFactoryOutput(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	globals *cliGlobalOptions,
	basePolicy terminalpolicy.Policy,
) {
	cleanInvocation, textInvocation := runInvocationModes(cmd, *cfg)
	configureDefaultRunInvocationOutput(cmd, cfg, promptArgs, textInvocation)
	cfg.CleanInvocation = cleanInvocation
	cfg.JSON = globals.json
	runPolicy := resolveEffectiveRunPolicy(cmd, *cfg, basePolicy)
	cfg.TerminalPolicy = runPolicy
	cfg.Verbose = runPolicy.VerboseEnabled()
	cfg.SuppressDashboardRendering = runPolicy.Mode() == terminalpolicy.ModeQuiet
	configureRunProgressOutput(cmd, cfg, basePolicy)
	configureRunFactoryStreams(cmd, cfg, cleanInvocation, textInvocation, runPolicy)
	cfg.Diagnostics = runPolicy.DiagnosticsWriter(cmd.ErrOrStderr())
	cfg.ReplayMetadataOutput = cmd.OutOrStdout()
	cfg.JSONOutput = globals.json
	if !cleanInvocation && strings.TrimSpace(cfg.WorkFile) != "" {
		// A finite --work run has a customer-facing result even when it uses
		// --dir rather than a named/current Factory selection. JSON batch
		// results must own stdout exclusively so startup and dashboard output
		// cannot make the document invalid.
		cfg.Output = cmd.OutOrStdout()
		if globals.json {
			cfg.StartupOutput = nil
			cfg.SuppressDashboardRendering = true
		}
	}
}

func configureDefaultRunInvocationOutput(cmd *cobra.Command, cfg *runcli.RunConfig, promptArgs []string, textInvocation bool) {
	invocationFactorySelected := cmd.Flags().Changed("factory") || cmd.Flags().Changed("named") || cfg.InvocationFileExplicit
	defaultResponseStream := !cfg.SuppressDashboardRendering &&
		(textInvocation || (invocationFactorySelected && !cfg.Continuously && !cmd.Flags().Changed("work") && len(promptArgs) > 0))
	if defaultResponseStream && strings.TrimSpace(cfg.InvocationOutputMode) == "" && !cfg.InvocationOutputExplicit {
		cfg.InvocationOutputMode = runcli.InvocationOutputResponseStream
	}
}

func configureRunFactoryStreams(cmd *cobra.Command, cfg *runcli.RunConfig, cleanInvocation bool, textInvocation bool, policy terminalpolicy.Policy) {
	humanTerminal := policy.HumanTerminalWriter(cmd.OutOrStdout())
	if cleanInvocation || textInvocation {
		cfg.Output = cmd.OutOrStdout()
		cfg.StartupOutput = nil
		return
	}
	if strings.TrimSpace(cfg.FactoryConfigPath) != "" ||
		(strings.TrimSpace(cfg.ReplayPath) != "" && !cfg.SuppressDashboardRendering) {
		cfg.Output = cmd.OutOrStdout()
	}
	cfg.StartupOutput = humanTerminal
}

func writeRunCommandInvocationError(cmd *cobra.Command, globals *cliGlobalOptions, err error) error {
	_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json)
	return err
}

func validateRunCommandInputs(cmd *cobra.Command, cfg *runcli.RunConfig, globals *cliGlobalOptions) (error, bool) {
	if err := validateRunRemoteHostingConflict(cmd, globals); err != nil {
		return err, true
	}
	if err := applyRunCommandInvocationOutputMode(cmd, cfg); err != nil {
		return err, true
	}
	outputExplicit, err := climanifestcobra.InputChanged(cmd, "you.run.flag.output")
	if err != nil {
		return err, false
	}
	if err := runcli.ValidateInvocationOutputSelection(
		cfg.SuppressDashboardRendering,
		globals.json,
		outputExplicit,
	); err != nil {
		return err, true
	}
	return nil, false
}

func executeResolvedRunCommand(
	cmd *cobra.Command,
	promptArgs []string,
	resolvedConfig runcli.RunConfig,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions CommandFactory,
) error {
	if helpRequested(cmd) {
		return executeRunCommandHelp(cmd, &resolvedConfig, globals, rootOptions)
	}
	if runCommandUsesNamedFactory(cmd, resolvedConfig) {
		namedPolicy := diagnostics.resolvePolicy(resolvedConfig.SuppressDashboardRendering)
		if err := prepareNamedRunSystemInitialization(
			cmd, &resolvedConfig, promptArgs, globals, namedPolicy, rootOptions,
		); err != nil {
			return writeRunCommandInvocationError(cmd, globals, err)
		}
	}
	currentFactorySelected := runUsesCurrentFactory(cmd)
	if currentFactorySelected {
		if err := selectCurrentFactoryFromWorkingDirectory(cmd, &resolvedConfig); err != nil {
			mapped := runcli.MapCurrentFactoryFailure(err)
			return writeRunCommandInvocationError(cmd, globals, mapped)
		}
	}
	basePolicy := diagnostics.resolvePolicy(resolvedConfig.SuppressDashboardRendering)
	err := runFactoryWithOptions(cmd, resolvedConfig, promptArgs, globals, operatorDefaults, basePolicy, rootOptions, false)
	if err == nil {
		return nil
	}
	return handleRunExecutionError(cmd, resolvedConfig, promptArgs, globals, basePolicy, err, currentFactorySelected)
}

func executeRunCommandHelp(cmd *cobra.Command, cfg *runcli.RunConfig, globals *cliGlobalOptions, rootOptions CommandFactory) error {
	if runCommandUsesNamedFactory(cmd, *cfg) {
		if err := prepareNamedFactoryHelpInitialization(cmd, cfg, globals, rootOptions); err != nil {
			return writeRunCommandInvocationError(cmd, globals, err)
		}
	}
	return writeRunCommandHelp(cmd, cfg, rootOptions)
}

func runCommandUsesNamedFactory(cmd *cobra.Command, cfg runcli.RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) != "" &&
		!cmd.Flags().Changed("factory") && !cmd.Flags().Changed("dir")
}

func prepareRunStartup(cmd *cobra.Command, cfg *runcli.RunConfig, options CommandFactory) error {
	startupAllowed, err := prepareRunSystemInitialization(cmd, cfg, options)
	if err != nil {
		return err
	}
	installRunStartupPreparation(cmd, cfg, options, startupAllowed)
	return nil
}

func installRunStartupPreparation(cmd *cobra.Command, cfg *runcli.RunConfig, options CommandFactory, startupAllowed bool) {
	if cfg == nil || cfg.StartupPreparation != nil {
		return
	}
	if !startupAllowed {
		cfg.StartupPreparation = func(context.Context, bool, io.Writer) error { return nil }
		return
	}
	preparation := newRunStartupPreparation(cmd, cfg, options)
	cfg.StartupPreparation = preparation.Prepare
}

type runStartupPreparation struct {
	cmd                   *cobra.Command
	cfg                   *runcli.RunConfig
	options               CommandFactory
	homeDir               string
	initialized           bool
	homeDisclosed         bool
	recordingInputChecked bool
	recordingInputBlocked bool
	activationChecked     bool
	activationAllowed     bool
}

func newRunStartupPreparation(cmd *cobra.Command, cfg *runcli.RunConfig, options CommandFactory) *runStartupPreparation {
	return &runStartupPreparation{
		cmd:               cmd,
		cfg:               cfg,
		options:           options,
		homeDir:           cfg.HomeDir,
		initialized:       systemInitializationCompleted(cmd.Context()),
		homeDisclosed:     homeDisclosureCompleted(cmd.Context()),
		activationAllowed: true,
	}
}

func (preparation *runStartupPreparation) Prepare(ctx context.Context, discloseHome bool, disclosureOutput io.Writer) error {
	if preparation == nil || preparation.cfg == nil {
		return errors.New("run startup preparation is required")
	}
	if preparation.recordingInputBlocked {
		preparation.cfg.StartupPreflightBlocked = true
		return nil
	}
	preparation.checkRecordingInput(discloseHome)
	if preparation.recordingInputBlocked {
		preparation.cfg.StartupPreflightBlocked = true
		return nil
	}
	preparation.checkFactoryActivation()
	if !preparation.activationAllowed {
		preparation.cfg.StartupPreflightBlocked = true
		return nil
	}
	preparation.discloseHome(discloseHome, disclosureOutput)
	return preparation.initialize(ctx)
}

func (preparation *runStartupPreparation) checkRecordingInput(discloseHome bool) {
	if preparation.recordingInputChecked ||
		(runHasRecordingInput(*preparation.cfg) && !discloseHome && runStartupDisclosureEnabled(*preparation.cfg)) {
		return
	}
	preparation.recordingInputChecked = true
	preparation.recordingInputBlocked = !inspectRunRecordingInput(preparation.cmd, *preparation.cfg, preparation.options)
}

func (preparation *runStartupPreparation) checkFactoryActivation() {
	if preparation.activationChecked {
		return
	}
	preparation.activationChecked = true
	configPath := runFactoryConfigPath(preparation.cmd, *preparation.cfg)
	if strings.TrimSpace(configPath) == "" || runFactorySourceUsesJavaScript(configPath) {
		return
	}
	preparation.activationAllowed = inspectRunFactoryActivation(
		preparation.cmd,
		*preparation.cfg,
		preparation.options,
		configPath,
	)
}

func (preparation *runStartupPreparation) discloseHome(discloseHome bool, disclosureOutput io.Writer) {
	if !discloseHome || preparation.homeDisclosed {
		return
	}
	disclosureCfg := *preparation.cfg
	if disclosureOutput != nil {
		disclosureCfg.StartupOutput = disclosureOutput
	}
	runcli.DiscloseHomeDirectory(disclosureCfg)
	preparation.homeDisclosed = true
}

func (preparation *runStartupPreparation) initialize(ctx context.Context) error {
	if preparation.initialized {
		return nil
	}
	if preparation.options.initializer == nil {
		return errors.New("run service initializer is required")
	}
	if err := preparation.options.initializer.InitializeSystem(ctx, preparation.homeDir); err != nil {
		return reportSystemInitializationFailure(preparation.cmd, err)
	}
	preparation.initialized = true
	return nil
}

func prepareRunSystemInitialization(cmd *cobra.Command, cfg *runcli.RunConfig, options CommandFactory) (bool, error) {
	if cmd == nil || cfg == nil {
		return false, fmt.Errorf("prepare run system initialization: command and config are required")
	}
	if runcli.ValidateRecordingInvocationFlags(*cfg) != nil {
		// Recordings flag conflicts remain visible to the run transport's
		// deterministic validator. They must not activate the system while the
		// invalid invocation is waiting to be rejected.
		return false, nil
	}
	if systemInitializationCompleted(cmd.Context()) {
		return true, nil
	}
	if options.initializer == nil {
		return false, fmt.Errorf("system initializer is required")
	}
	if deferBatchSystemInitialization(cmd, *cfg) {
		return false, nil
	}
	return true, nil
}

// deferBatchSystemInitialization keeps the profile-selected packaged-Factory
// bootstrap work out of the finite mock/no-record batch path. That path has no
// system-bootstrap demand: its Factory and Work inputs are local, its worker
// behavior is supplied by the selected mock configuration, and its recording
// request explicitly disables durable recording. Server, recording/replay,
// named/bootstrap, continuous, and real-worker invocations retain the normal
// initialization boundary.
func deferBatchSystemInitialization(cmd *cobra.Command, cfg runcli.RunConfig) bool {
	if cmd != nil && (cmd.Flags().Changed("dir") || cmd.Flags().Changed("factory") || cmd.Flags().Changed("named")) {
		return false
	}
	return strings.TrimSpace(cfg.WorkFile) != "" &&
		!cfg.Continuously &&
		cfg.MockWorkersEnabled &&
		cfg.DisableDefaultRecording &&
		strings.TrimSpace(cfg.RecordPath) == "" &&
		strings.TrimSpace(cfg.ReplayPath) == "" &&
		strings.TrimSpace(cfg.ResumePath) == "" &&
		!cfg.WithServer &&
		!cfg.WithSite &&
		!cfg.ListenExplicit &&
		!cfg.Bootstrap &&
		strings.TrimSpace(cfg.NamedFactoryName) == ""
}

func inspectRunRecordingInput(cmd *cobra.Command, cfg runcli.RunConfig, options CommandFactory) bool {
	path := strings.TrimSpace(cfg.ReplayPath)
	if path == "" {
		path = strings.TrimSpace(cfg.ResumePath)
	}
	if path == "" || options.runInputPathInspector == nil {
		return true
	}
	info, err := options.runInputPathInspector.Stat(resolveRunPath(cmd, path))
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return info != nil && info.Mode().IsRegular()
}

func runHasRecordingInput(cfg runcli.RunConfig) bool {
	return strings.TrimSpace(cfg.ReplayPath) != "" || strings.TrimSpace(cfg.ResumePath) != ""
}

func inspectRunFactoryActivation(cmd *cobra.Command, cfg runcli.RunConfig, options CommandFactory, configPath string) bool {
	if options.runInputPathInspector == nil {
		return true
	}
	resolvedPath := resolveRunPath(cmd, configPath)
	info, statErr := options.runInputPathInspector.Stat(resolvedPath)
	if statErr != nil {
		return allowRunFactoryActivationAfterStatError(cmd, statErr)
	}
	if info == nil || !info.Mode().IsRegular() {
		return false
	}
	if options.ValidateFactory != nil {
		if err := options.ValidateFactory(factorycli.ValidateConfig{
			Context: cmd.Context(), Path: resolvedPath, JSON: true, Output: io.Discard,
		}); err != nil {
			// Leave the authoritative runtime opening to report its existing
			// operator-facing validation error; this is only an activation gate.
			return false
		}
	}
	return true
}

func allowRunFactoryActivationAfterStatError(cmd *cobra.Command, statErr error) bool {
	if errors.Is(statErr, fs.ErrNotExist) && runUsesCurrentFactory(cmd) {
		// Current Factory discovery owns this clean failure. Do not create
		// global state while the selected local asset is absent.
		return false
	}
	// Explicit missing factory paths still run system initialization. The
	// bootstrap probe relies on that call to materialize packaged roots,
	// but its eventual missing-input failure remains output-clean.
	return true
}

func runStartupDisclosureEnabled(cfg runcli.RunConfig) bool {
	return cfg.StartupOutput != nil && !cfg.JSON && !cfg.JSONOutput &&
		!cfg.CleanInvocation && !cfg.SuppressDashboardRendering && !cfg.InvocationOutputExplicit
}

func runFactoryConfigPath(cmd *cobra.Command, cfg runcli.RunConfig) string {
	if strings.TrimSpace(cfg.FactoryConfigPath) != "" {
		return cfg.FactoryConfigPath
	}
	if strings.TrimSpace(cfg.ReplayPath) != "" || strings.TrimSpace(cfg.ResumePath) != "" {
		return ""
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		if cmd == nil || !runUsesCurrentFactory(cmd) {
			return ""
		}
		workingDirectory := startupcli.WorkingDirectory(cmd.Context())
		if strings.TrimSpace(workingDirectory) == "" {
			return ""
		}
		return filepath.Join(workingDirectory, defaultcmd.FactoryDir, factorydefinitions.FactoryConfigFile)
	}
	return filepath.Join(cfg.Dir, factorydefinitions.FactoryConfigFile)
}

func resolveRunPath(cmd *cobra.Command, path string) string {
	if filepath.IsAbs(path) || cmd == nil {
		return path
	}
	workingDirectory := startupcli.WorkingDirectory(cmd.Context())
	if strings.TrimSpace(workingDirectory) == "" {
		return path
	}
	return filepath.Join(workingDirectory, path)
}

func initializeSystemAtStartupBoundary(cmd *cobra.Command, cfg runcli.RunConfig, options CommandFactory, discloseHome bool) error {
	if cmd == nil || options.initializer == nil {
		return fmt.Errorf("system initializer is required")
	}
	if discloseHome && !homeDisclosureCompleted(cmd.Context()) {
		runcli.DiscloseHomeDirectory(cfg)
		cmd.SetContext(context.WithValue(cmd.Context(), homeDisclosureContextKey{}, true))
	}
	if systemInitializationCompleted(cmd.Context()) {
		return nil
	}
	if err := options.initializer.InitializeSystem(cmd.Context(), cfg.HomeDir); err != nil {
		return reportSystemInitializationFailure(cmd, err)
	}
	cmd.SetContext(context.WithValue(cmd.Context(), systemInitializationContextKey{}, true))
	return nil
}

func reportSystemInitializationFailure(cmd *cobra.Command, err error) error {
	wrapped := fmt.Errorf("initialize system: %w", err)
	if errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) && cmd != nil {
		diagnostic := wrapped
		if cause := errors.Unwrap(err); cause != nil {
			diagnostic = fmt.Errorf("%s: %v", wrapped, cause)
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), diagnostic)
		clidiag.MarkDiagnosticRendered(cmd.ErrOrStderr())
	}
	return wrapped
}

func preliminaryRunInvocationOutputIsClean(cmd *cobra.Command, cfg runcli.RunConfig, promptArgs []string) bool {
	if cfg.InvocationOutputExplicit || cfg.SuppressDashboardRendering || cfg.InvocationFileExplicit ||
		cfg.WorkFile != "" || len(promptArgs) > 0 {
		return true
	}
	if cmd == nil || startupcli.StdinIsTTY(cmd.Context()) {
		return false
	}
	return strings.TrimSpace(cfg.NamedFactoryName) != "" ||
		strings.TrimSpace(cfg.FactoryConfigPath) != ""
}

func prepareNamedRunSystemInitialization(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	globals *cliGlobalOptions,
	policy terminalpolicy.Policy,
	options CommandFactory,
) error {
	if cmd == nil || cfg == nil || globals == nil || remotePlacementSelected(globals) {
		return nil
	}
	homeDir, err := resolveProcessHomeDirForCommand(cmd, options)
	if err != nil {
		return err
	}
	cfg.HomeDir = homeDir
	cfg.JSON = globals.json
	cfg.JSONOutput = globals.json
	cfg.CleanInvocation = preliminaryRunInvocationOutputIsClean(cmd, *cfg, promptArgs)
	if !cfg.CleanInvocation && !cfg.InvocationOutputExplicit {
		cfg.StartupOutput = policy.HumanTerminalWriter(cmd.OutOrStdout())
	}
	return initializeSystemAtStartupBoundary(cmd, *cfg, options, runStartupDisclosureEnabled(*cfg))
}

func prepareNamedFactoryHelpInitialization(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	globals *cliGlobalOptions,
	options CommandFactory,
) error {
	if cmd == nil || cfg == nil || globals == nil || remotePlacementSelected(globals) {
		return nil
	}
	homeDir, err := resolveProcessHomeDirForCommand(cmd, options)
	if err != nil {
		return err
	}
	cfg.HomeDir = homeDir
	return initializeSystemAtStartupBoundary(cmd, *cfg, options, false)
}

func newRootCommandWithFactory(options CommandFactory) *cobra.Command {
	root := newRootCommandWithGeneratedRepresentativeFamily(options)
	if root == nil {
		return nil
	}
	previous := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := validateRunRemoteHostingBeforeInitialization(cmd, args); err != nil {
			_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), err, false)
			return err
		}
		if requiresSystemInitialization(cmd, args) {
			if options.initializer == nil {
				return fmt.Errorf("system initializer is required")
			}
			homeDir, err := resolveProcessHomeDirForCommand(cmd, options)
			if err != nil {
				return err
			}
			if err := options.initializer.InitializeSystem(cmd.Context(), homeDir); err != nil {
				return reportSystemInitializationFailure(cmd, err)
			}
			cmd.SetContext(context.WithValue(cmd.Context(), systemInitializationContextKey{}, true))
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return root
}

func validateRunRemoteHostingBeforeInitialization(cmd *cobra.Command, args []string) error {
	if cmd == nil || cmd.CommandPath() != "you run" {
		return nil
	}
	remote := runFlagEnabled(cmd, "remote") || rawRunFlagEnabled(args, "remote")
	if !remote {
		return nil
	}
	if !rawRunFlagEnabled(args, "with-server") && !rawRunFlagEnabled(args, "with-site") {
		return nil
	}
	return newRunRemoteLocalHostingConflictError()
}

func runFlagEnabled(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag(name)
	return flag != nil && flag.Changed && flag.Value != nil && flag.Value.String() == "true"
}

func rawRunFlagEnabled(args []string, name string) bool {
	enabled := false
	prefix := "--" + name
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case prefix:
			enabled = true
		case prefix + "=true":
			enabled = true
		case prefix + "=false":
			enabled = false
		}
	}
	return enabled
}

func requiresSystemInitialization(cmd *cobra.Command, args []string) bool {
	commandPath := ""
	if cmd != nil {
		commandPath = cmd.CommandPath()
	}
	switch commandPath {
	case "you server acp", "you server mcp":
		return true
	default:
		return false
	}
}

func executeServerCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operatorDefaults *cliOperatorDefaultsOptions,
	rootOptions CommandFactory,
) error {
	cfg := defaultcmd.ServerRunConfig(rootOptions.runDefaults)
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	cfg.ListenAddress, err = commandInputValue[string](values, serverListenInputID)
	if err != nil {
		return err
	}
	cfg.Pprof, err = commandInputValue[bool](values, serverPprofInputID)
	if err != nil {
		return err
	}
	cfg.ListenExplicit, err = climanifestcobra.InputChanged(cmd, serverListenInputID)
	if err != nil {
		return err
	}
	if err := selectCurrentFactoryFromWorkingDirectory(cmd, &cfg); err != nil {
		mapped := runcli.MapCurrentFactoryFailure(err)
		_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
		return mapped
	}
	policy := diagnostics.resolvePolicy(false)
	err = runFactoryWithOptions(
		cmd, cfg, nil, globals, operatorDefaults, policy, rootOptions, true,
	)
	if err == nil {
		return nil
	}
	mapped := runcli.MapServerFailure(err)
	mapped = runcli.MapCurrentFactoryFailure(factoryload.MaybeFormatOperatorError(mapped, cfg.Dir))
	_ = runcli.WriteInvocationError(cmd.ErrOrStderr(), mapped, globals.json)
	return mapped
}

func executeServerStopCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	rootOptions CommandFactory,
) error {
	if rootOptions.serverStopCLI == nil {
		return fmt.Errorf("server stop operation is required")
	}
	server := ""
	jsonOutput := false
	if globals != nil {
		server = globals.server
		jsonOutput = globals.json
	}
	return rootOptions.serverStopCLI(cmd.Context(), serverstopcli.Config{
		Server: server, JSON: jsonOutput, Output: cmd.OutOrStdout(),
	})
}

type factoryConfigInitProductionCommands struct {
	Factory *cobra.Command
	Config  *cobra.Command
	Init    *cobra.Command
}

func productionFactoryConfigInitCommands(
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) factoryConfigInitProductionCommands {
	handler := commandregistry.NewFactoryConfigInitCommandHandler(
		commandregistry.FactoryConfigInitServices{
			QueryFactory:           options.QueryFactory,
			ListFactories:          options.ListFactories,
			CreateFactoryFromFile:  options.CreateFactoryFromFile,
			UpdateFactoryFromFile:  options.UpdateFactoryFromFile,
			DeleteFactory:          options.DeleteFactory,
			ReplaceFactoryCurrent:  options.ReplaceFactoryCurrent,
			ValidateFactory:        options.ValidateFactory,
			FlattenFactoryConfig:   options.FlattenFactoryConfig,
			ExpandFactoryConfig:    options.ExpandFactoryConfig,
			ConfigureInit:          options.ConfigureInit,
			InstallPackagedFactory: options.InstallPackagedFactory,
			HomeDir:                options.homeDir,
			ResolveFactoryRoots:    options.resolveNamedFactoryRoots,
			DiagnosticsWriter:      diagnostics.writer,
		},
	)
	components, err := climanifestcobra.NewFactoryConfigInitFamilyComponents(handler)
	if err != nil {
		panic(fmt.Sprintf("build factory/config/init family commands: %v", err))
	}
	if options.completePackagedFactoryNames != nil {
		if err := cobracompletion.RegisterPackagedFactoryNames(
			components.Init,
			options.completePackagedFactoryNames,
		); err != nil {
			panic(fmt.Sprintf("register packaged factory init completion: %v", err))
		}
	}
	return factoryConfigInitProductionCommands{
		Factory: components.Factory,
		Config:  components.Config,
		Init:    components.Init,
	}
}
