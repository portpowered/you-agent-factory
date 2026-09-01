package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"syscall"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// maxRunInvocationStdinBytes is the inclusive byte limit for intentional
// Factory invocation stdin. The extra byte read by collectRunInvocationStdin
// is only an overflow sentinel and is never passed to Work or retained after
// rejection.
const maxRunInvocationStdinBytes = 1 << 20

type resolvedProcessHomeDirectoryContextKey struct{}

type resolvedProcessHomeDirectory struct {
	path string
}

func scalarTarget[T bool | string | int](value T) *T {
	return &value
}

func newRunServerFlagBindings() climanifestcobra.RunServerFlagBindings {
	targets := map[string]any{}
	stringInputs := []string{
		"you.run.flag.work", "you.run.flag.dir", "you.run.flag.named",
		"you.run.flag.factory", "you.run.flag.record", "you.run.flag.replay", "you.run.flag.resume",
		"you.run.flag.provider", "you.run.flag.model", "you.run.flag.worker-reasoning-effort",
		"you.run.flag.worktree", "you.run.flag.to-file",
		"you.run.flag.runtime-log-dir", "you.run.flag.runtime-metrics-dir",
		"you.run.flag.with-mock-workers", "you.run.flag.output", "you.run.flag.listen",
		"you.run.flag.session",
		"you.server.flag.listen",
	}
	boolInputs := []string{
		"you.run.flag.continuously", "you.run.flag.no-record",
		"you.run.flag.runtime-log-compress", "you.run.flag.runtime-metrics-compress",
		"you.run.flag.with-server", "you.run.flag.with-site", "you.run.flag.pprof", "you.server.flag.pprof", "you.run.flag.quiet",
		"you.run.flag.skip-permissions",
	}
	intInputs := []string{
		"you.run.flag.runtime-log-max-size-mb", "you.run.flag.runtime-log-max-backups",
		"you.run.flag.runtime-log-max-age-days", "you.run.flag.runtime-metrics-max-size-mb",
		"you.run.flag.runtime-metrics-max-backups", "you.run.flag.runtime-metrics-max-age-days",
	}
	for _, inputID := range stringInputs {
		targets[inputID] = scalarTarget("")
	}
	for _, inputID := range boolInputs {
		targets[inputID] = scalarTarget(false)
	}
	for _, inputID := range intInputs {
		targets[inputID] = scalarTarget(0)
	}
	return climanifestcobra.RunServerFlagBindings{LocalTargets: targets}
}

func applyRunResolvedInputs(cfg runcli.RunConfig, values map[string]any) (runcli.RunConfig, error) {
	stringFields := []struct {
		id     string
		target *string
	}{
		{"you.run.flag.work", &cfg.WorkFile},
		{"you.run.flag.dir", &cfg.Dir},
		{"you.run.flag.named", &cfg.NamedFactoryName},
		{"you.run.flag.factory", &cfg.FactoryConfigPath},
		{"you.run.flag.provider", &cfg.ProviderOverride},
		{"you.run.flag.model", &cfg.ModelOverride},
		{"you.run.flag.worker-reasoning-effort", &cfg.WorkerReasoningEffort},
		{"you.run.flag.worktree", &cfg.Worktree},
		{"you.run.flag.to-file", &cfg.InvocationFilePath},
		{"you.run.flag.record", &cfg.RecordPath},
		{"you.run.flag.replay", &cfg.ReplayPath},
		{"you.run.flag.resume", &cfg.ResumePath},
		{"you.run.flag.runtime-log-dir", &cfg.RuntimeLogDir},
		{"you.run.flag.runtime-metrics-dir", &cfg.RuntimeMetricsDir},
		{"you.run.flag.with-mock-workers", &cfg.MockWorkersConfigPath},
		{"you.run.flag.output", &cfg.InvocationOutputMode},
		{"you.run.flag.listen", &cfg.ListenAddress},
		{"you.run.flag.session", &cfg.FactorySessionID},
	}
	boolFields := []struct {
		id     string
		target *bool
	}{
		{"you.run.flag.continuously", &cfg.Continuously},
		{"you.run.flag.no-record", &cfg.DisableDefaultRecording},
		{"you.run.flag.runtime-log-compress", &cfg.RuntimeLogConfig.Compress},
		{"you.run.flag.runtime-metrics-compress", &cfg.RuntimeMetricsConfig.Compress},
		{"you.run.flag.with-server", &cfg.WithServer},
		{"you.run.flag.with-site", &cfg.WithSite},
		{"you.run.flag.pprof", &cfg.Pprof},
		{"you.run.flag.quiet", &cfg.SuppressDashboardRendering},
	}
	intFields := []struct {
		id     string
		target *int
	}{
		{"you.run.flag.runtime-log-max-size-mb", &cfg.RuntimeLogConfig.MaxSize},
		{"you.run.flag.runtime-log-max-backups", &cfg.RuntimeLogConfig.MaxBackups},
		{"you.run.flag.runtime-log-max-age-days", &cfg.RuntimeLogConfig.MaxAge},
		{"you.run.flag.runtime-metrics-max-size-mb", &cfg.RuntimeMetricsConfig.MaxSize},
		{"you.run.flag.runtime-metrics-max-backups", &cfg.RuntimeMetricsConfig.MaxBackups},
		{"you.run.flag.runtime-metrics-max-age-days", &cfg.RuntimeMetricsConfig.MaxAge},
	}
	var err error
	for _, field := range stringFields {
		*field.target, err = commandInputValue[string](values, field.id)
		if err != nil {
			return cfg, err
		}
	}
	for _, field := range boolFields {
		*field.target, err = commandInputValue[bool](values, field.id)
		if err != nil {
			return cfg, err
		}
	}
	for _, field := range intFields {
		*field.target, err = commandInputValue[int](values, field.id)
		if err != nil {
			return cfg, err
		}
	}
	return cfg, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func parseRunCommandArgs(cmd *cobra.Command, args []string) ([]string, error) {
	remainder := make([]string, 0, len(args))
	flagsByToken := indexRunCommandFlags(cmd)
	flagArgs, positional, _ := runcli.SplitFlagTerminator(args)

	for index := 0; index < len(flagArgs); index++ {
		token := flagArgs[index]
		if token == "-" || !strings.HasPrefix(token, "-") {
			remainder = append(remainder, token)
			continue
		}

		lookupToken := token
		inlineValue := ""
		hasInlineValue := false
		if strings.HasPrefix(token, "--") {
			if name, value, ok := strings.Cut(token, "="); ok {
				lookupToken = name
				inlineValue = value
				hasInlineValue = true
			}
		}

		flag, ok := flagsByToken[lookupToken]
		if !ok {
			remainder = append(remainder, token)
			continue
		}
		if flag == nil || flag.Value == nil {
			return nil, fmt.Errorf("flag %s is unavailable", lookupToken)
		}

		value, consumedNext, err := resolveRunFlagValue(flag, flagArgs, index, hasInlineValue, inlineValue)
		if err != nil {
			return nil, err
		}
		if err := flag.Value.Set(value); err != nil {
			return nil, &runcli.InvocationError{
				Code:    runcli.InvocationArgumentInvalidValueCode,
				Message: fmt.Sprintf("invalid value for %s: %s", lookupToken, err),
				Cause:   err,
			}
		}
		flag.Changed = true
		if consumedNext {
			index++
		}
	}

	remainder = append(remainder, positional...)
	if mockWorkersFlag := flagsByToken["--with-mock-workers"]; mockWorkersFlag != nil &&
		mockWorkersFlag.Changed &&
		mockWorkersFlag.Value.String() == defaultMockWorkersConfigPathSentinel &&
		len(remainder) > 0 &&
		!runFlagValueLooksLikeFlag(remainder[0]) &&
		strings.EqualFold(filepath.Ext(remainder[0]), ".json") {
		if err := mockWorkersFlag.Value.Set(remainder[0]); err != nil {
			return nil, err
		}
		remainder = remainder[1:]
	}
	return remainder, nil
}

func indexRunCommandFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	indexed := map[string]*pflag.Flag{}
	addFlags := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			indexed["--"+flag.Name] = flag
			if flag.Shorthand != "" {
				indexed["-"+flag.Shorthand] = flag
			}
		})
	}
	addFlags(cmd.InheritedFlags())
	addFlags(cmd.Flags())
	return indexed
}

func resolveRunFlagValue(flag *pflag.Flag, args []string, index int, hasInlineValue bool, inlineValue string) (string, bool, error) {
	if hasInlineValue {
		return inlineValue, false, nil
	}
	if flag.Value.Type() == "bool" {
		if flag.NoOptDefVal != "" {
			return flag.NoOptDefVal, false, nil
		}
		return "true", false, nil
	}
	if index+1 < len(args) && !runFlagValueLooksLikeFlag(args[index+1]) {
		return args[index+1], true, nil
	}
	if flag.NoOptDefVal != "" {
		return flag.NoOptDefVal, false, nil
	}
	return "", false, &runcli.InvocationError{
		Code:    runcli.InvocationArgumentMissingValueCode,
		Message: fmt.Sprintf("flag needs an argument: %s", "--"+flag.Name),
	}
}

// runFlagValueLooksLikeFlag reports whether token would be parsed as a flag
// token rather than an optional flag value. Bare "-" remains a value so stdin
// path conventions stay available.
func runFlagValueLooksLikeFlag(token string) bool {
	return token != "-" && strings.HasPrefix(token, "-")
}

// ParseArgvForCLIInputsInventory parses argv on the caller-supplied canonical
// process command through the target command's flag and positional validators
// without invoking RunE.
// Run uses its custom tokenizer because the command sets DisableFlagParsing.
func ParseArgvForCLIInputsInventory(root *cobra.Command, argv []string) (*cobra.Command, []string, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("parse CLI inputs inventory: root command is required")
	}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	cmd, flagArgs, err := root.Find(argv)
	if err != nil {
		return cmd, nil, err
	}

	cmd.InitDefaultHelpFlag()

	if cmd.DisableFlagParsing {
		remainder, err := parseRunCommandArgs(cmd, flagArgs)
		if err != nil {
			return cmd, nil, err
		}
		if err := cmd.ValidateArgs(remainder); err != nil {
			return cmd, remainder, err
		}
		if err := climanifestcobra.ValidateRequiredFlags(cmd); err != nil {
			return cmd, remainder, err
		}
		if err := cmd.ValidateFlagGroups(); err != nil {
			return cmd, remainder, err
		}
		return cmd, remainder, nil
	}

	if err := cmd.ParseFlags(flagArgs); err != nil {
		return cmd, nil, err
	}

	positionals := cmd.Flags().Args()
	if err := cmd.ValidateArgs(positionals); err != nil {
		return cmd, positionals, err
	}
	if err := climanifestcobra.ValidateRequiredFlags(cmd); err != nil {
		return cmd, positionals, err
	}
	if err := cmd.ValidateFlagGroups(); err != nil {
		return cmd, positionals, err
	}
	return cmd, positionals, nil
}

func resolveOperatorDefaults(_ *cobra.Command, options *cliOperatorDefaultsOptions, rootOptions CommandFactory, homeDir string) (operatorconfig.ResolvedDefaults, error) {
	if rootOptions.resolveOperatorDefaults == nil {
		return operatorconfig.ResolvedDefaults{}, fmt.Errorf("Operator Settings defaults resolver is required")
	}
	environment := operatorconfig.Defaults{}
	var err error
	environment.WorkerModelProvider, _, err = lookupProcessEnvironment(
		rootOptions,
		operatorconfig.EnvDefaultWorkerModelProvider,
	)
	if err != nil {
		return operatorconfig.ResolvedDefaults{}, err
	}
	environment.WorkerModel, _, err = lookupProcessEnvironment(
		rootOptions,
		operatorconfig.EnvDefaultWorkerModel,
	)
	if err != nil {
		return operatorconfig.ResolvedDefaults{}, err
	}
	flags := operatorconfig.FlagOverrides{}
	if options != nil {
		flags.WorkerModelProvider = strings.TrimSpace(options.providerOverride)
		flags.WorkerModel = strings.TrimSpace(options.modelOverride)
	}
	return rootOptions.resolveOperatorDefaults(homeDir, environment, flags)
}

func resolveProcessHomeDir(options CommandFactory) (string, error) {
	if options.homeDir == nil {
		return "", fmt.Errorf("process home directory resolver is required")
	}
	homeDir, err := options.homeDir()
	if err != nil {
		return "", fmt.Errorf("resolve process home directory: %w", err)
	}
	return homeDir, nil
}

func resolveProcessHomeDirForCommand(cmd *cobra.Command, options CommandFactory) (string, error) {
	if cmd != nil {
		ctx := cmd.Context()
		if ctx != nil {
			if resolved, ok := ctx.Value(resolvedProcessHomeDirectoryContextKey{}).(resolvedProcessHomeDirectory); ok {
				return resolved.path, nil
			}
		}
	}
	homeDir, err := resolveProcessHomeDir(options)
	if err != nil {
		return "", err
	}
	if cmd != nil {
		ctx := cmd.Context()
		if ctx == nil {
			return "", fmt.Errorf("resolve process home directory: command context is required")
		}
		cmd.SetContext(context.WithValue(ctx, resolvedProcessHomeDirectoryContextKey{}, resolvedProcessHomeDirectory{path: homeDir}))
	}
	return homeDir, nil
}

func lookupProcessEnvironment(
	options CommandFactory,
	name string,
) (string, bool, error) {
	if options.lookupEnv == nil {
		return "", false, fmt.Errorf("process environment lookup is required")
	}
	value, ok := options.lookupEnv(name)
	return value, ok, nil
}

func persistentInputWasCLI(cmd *cobra.Command, inputID, legacyName string) bool {
	if cmd == nil {
		return false
	}
	inputs, err := climanifestcobra.ResolvedPersistentInputs(cmd)
	if err == nil {
		state, found := inputs.State(inputID)
		if found {
			return state.Provenance == resolvedinput.SourceCLIFlag
		}
	}
	return cmd.Root().PersistentFlags().Changed(legacyName)
}

func representativeSourceValues(options CommandFactory) climanifestcobra.SourceCandidateProvider {
	return func(
		_ context.Context,
		binding climanifest.SourceBinding,
		kind resolvedinput.ValueKind,
	) (resolvedinput.Value, bool, error) {
		if kind != resolvedinput.ValueKindString {
			return resolvedinput.Value{}, false, fmt.Errorf(
				"source binding %q requires unsupported value kind %q",
				binding.ID,
				kind,
			)
		}
		if binding.Source == climanifest.SourceEnvironment {
			if options.lookupEnv == nil {
				return resolvedinput.Value{}, false, nil
			}
			value, present, err := lookupProcessEnvironment(options, binding.ExternalKey)
			if err != nil || !present || strings.TrimSpace(value) == "" {
				return resolvedinput.Value{}, false, err
			}
			return resolvedinput.StringValue(strings.TrimSpace(value)), true, nil
		}
		if binding.Source != climanifest.SourceOperatorConfig {
			return resolvedinput.Value{}, false, nil
		}
		return operatorConfigSourceValue(options, binding)
	}
}

func operatorConfigSourceValue(
	options CommandFactory,
	binding climanifest.SourceBinding,
) (resolvedinput.Value, bool, error) {
	if options.loadOperatorConfig == nil {
		return resolvedinput.Value{}, false, nil
	}
	homeDir, err := resolveProcessHomeDir(options)
	if err != nil {
		return resolvedinput.Value{}, false, err
	}
	config, err := options.loadOperatorConfig(
		operatorconfig.DefaultConfigPath(homeDir),
	)
	if err != nil {
		// A config path below a non-directory ancestor is unavailable in
		// the same way as a missing optional config. Commands such as
		// initializer-owned startup still owns the later, actionable creation error.
		if errors.Is(err, syscall.ENOTDIR) {
			return resolvedinput.Value{}, false, nil
		}
		return resolvedinput.Value{}, false, err
	}
	value := ""
	switch binding.ExternalKey {
	case "defaults.workerModelProvider":
		value = config.Defaults.WorkerModelProvider
	case "defaults.workerModel":
		value = config.Defaults.WorkerModel
	default:
		return resolvedinput.Value{}, false, fmt.Errorf(
			"source binding %q has unsupported operator-config key %q",
			binding.ID,
			binding.ExternalKey,
		)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return resolvedinput.Value{}, false, nil
	}
	return resolvedinput.StringValue(value), true, nil
}

func resolveRunBindFromServer(cmd *cobra.Command, server string, cfg *runcli.RunConfig) error {
	if cfg == nil {
		return fmt.Errorf("resolve local listener: run config is required")
	}
	if cfg.ListenExplicit || strings.TrimSpace(cfg.ListenAddress) != "" {
		target, err := cliserver.LocalBindTargetFromListen(cfg.ListenAddress)
		if err != nil {
			return err
		}
		cfg.BindHost = target.Host
		cfg.Port = target.Port
		cfg.AutoPort = false
		cfg.ListenExplicit = true
		return nil
	}
	target, err := cliserver.LocalBindTargetFromServer(server)
	if err != nil {
		return err
	}
	cfg.BindHost = target.Host
	cfg.Port = target.Port
	cfg.AutoPort = true
	return nil
}

func listenerOwner(defaultInvocation bool, cfg runcli.RunConfig) bool {
	return defaultInvocation || cfg.WithServer || cfg.WithSite
}

func warnLegacyListenerBinding(cmd *cobra.Command, cfg runcli.RunConfig, defaultInvocation, explicitServer bool) {
	if !explicitServer || !listenerOwner(defaultInvocation, cfg) {
		return
	}
	if cmd == nil || cmd.ErrOrStderr() == nil {
		return
	}
	if cfg.ListenExplicit {
		_, _ = fmt.Fprintln(
			cmd.ErrOrStderr(),
			"warning: --listen takes precedence over --server for the local listener; use --listen for the listener and reserve --server for the factory API endpoint",
		)
		return
	}
	_, _ = fmt.Fprintln(
		cmd.ErrOrStderr(),
		"warning: --server is deprecated for local listener binding; use --listen <host:port> instead",
	)
}

func resolveRunFactorySelection(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	homeDir string,
	namedFactoryCatalog interfaces.NamedFactoryCatalog,
	resolveNamedFactoryRoots NamedFactoryRootsResolver,
	resolveNamedFactoryCandidatePaths interfaces.NamedFactoryCandidatePathsResolver,
) error {
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
		return resolveRunNamedFactorySelection(
			cmd.Context(),
			cfg,
			homeDir,
			namedFactoryCatalog,
			resolveNamedFactoryRoots,
			resolveNamedFactoryCandidatePaths,
		)
	}
	if factoryChanged && dirChanged {
		return fmt.Errorf("--factory cannot be used with --dir")
	}
	if !factoryChanged {
		return nil
	}
	if cfg.ResolveFactoryConfigRoot == nil {
		return fmt.Errorf("Factory Definitions config root resolver is required")
	}
	factoryRoot, err := cfg.ResolveFactoryConfigRoot(cfg.FactoryConfigPath)
	if contextErr := cmd.Context().Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return err
	}
	cfg.Dir = factoryRoot
	return nil
}

func resolveRunNamedFactorySelection(
	ctx context.Context,
	cfg *runcli.RunConfig,
	homeDir string,
	namedFactoryCatalog interfaces.NamedFactoryCatalog,
	resolveNamedFactoryRoots NamedFactoryRootsResolver,
	resolveNamedFactoryCandidatePaths interfaces.NamedFactoryCandidatePathsResolver,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if namedFactoryCatalog == nil {
		return fmt.Errorf("Factory Definitions named-factory catalog is required")
	}
	if resolveNamedFactoryRoots == nil {
		return fmt.Errorf("Factory Definitions named-factory root resolver is required")
	}
	if resolveNamedFactoryCandidatePaths == nil {
		return fmt.Errorf("Factory Definitions named-factory candidate-path resolver is required")
	}
	cwd := startupcli.WorkingDirectory(ctx)
	if strings.TrimSpace(cwd) == "" {
		return fmt.Errorf("resolve current working directory for --named: process working directory is required")
	}
	roots, err := resolveNamedFactoryRoots(homeDir, cwd)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return fmt.Errorf("resolve named-Factory roots: %w", err)
	}
	resolution, err := namedFactoryCatalog.ResolveNamedFactoryAcrossRoots(
		roots.Project,
		roots.Global,
		cfg.NamedFactoryName,
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		candidates, candidateErr := resolveNamedFactoryCandidatePaths(
			roots.Project,
			roots.Global,
			cfg.NamedFactoryName,
		)
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if candidateErr != nil {
			return err
		}
		return factoryload.MaybeFormatOperatorErrorForNamedFactory(err, candidates)
	}
	cfg.Dir = resolution.FactoryDir
	cfg.NamedFactoryResolution = resolution
	return nil
}

func resolveRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	preparation work.InvocationInputPreparation,
) error {
	factoryChanged := cmd.Flags().Changed("factory")
	namedChanged := cmd.Flags().Changed("named")
	workChanged := cmd.Flags().Changed("work")
	if err := rejectFileBackedRunShape(cfg, workChanged); err != nil {
		return err
	}

	if !factoryChanged && !namedChanged {
		return resolveUnselectedRunFactoryPrompt(cmd, cfg, promptArgs, workChanged, preparation)
	}
	if factoryChanged && runFactorySourceUsesJavaScript(cfg.FactoryConfigPath) {
		return rejectJavaScriptFilePrompt(cfg)
	}
	return resolveSelectedRunFactoryPrompt(cmd, cfg, promptArgs, workChanged, preparation)
}

func rejectFileBackedRunShape(cfg *runcli.RunConfig, workChanged bool) error {
	if !cfg.InvocationFileExplicit {
		return nil
	}
	switch {
	case workChanged:
		return fmt.Errorf("--to-file cannot be used with --work")
	case cfg.Continuously:
		return fmt.Errorf("--to-file cannot be used with --continuously")
	case strings.TrimSpace(cfg.ReplayPath) != "":
		return fmt.Errorf("--to-file cannot be used with --replay")
	default:
		return nil
	}
}

func resolveUnselectedRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	workChanged bool,
	preparation work.InvocationInputPreparation,
) error {
	if cfg.InvocationFileExplicit {
		return resolveCompatibilityRunFactoryPrompt(cmd, cfg, promptArgs, workChanged, preparation)
	}
	return resolveLegacyRunFactoryPrompt(cmd, promptArgs, preparation)
}

func rejectJavaScriptFilePrompt(cfg *runcli.RunConfig) error {
	if cfg.InvocationFileExplicit {
		return fmt.Errorf("--to-file is not supported for JavaScript workflow invocation")
	}
	return nil
}

func resolveSelectedRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	workChanged bool,
	preparation work.InvocationInputPreparation,
) error {
	signatureSource := filepath.Join(cfg.Dir, interfaces.FactoryConfigFile)
	if strings.TrimSpace(cfg.FactoryConfigPath) != "" {
		signatureSource = cfg.FactoryConfigPath
	}
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return fmt.Errorf("load run CLI manifest: %w", err)
	}
	schema, diagnostics, err := runcli.ResolveFactoryInvocationInputSchema(
		cmd.Context(),
		manifest,
		"you.run",
		cfg.LoadFactoryConfigFile,
		signatureSource,
	)
	if err != nil {
		return err
	}
	if err := runcli.MapCompositionDiagnostics(diagnostics); err != nil {
		return err
	}
	signature := runcli.InvocationSignatureFromEffectiveSchema(schema)
	if signature != nil {
		return resolveSignatureRunFactoryPrompt(cmd, cfg, promptArgs, signature, preparation)
	}
	return resolveCompatibilityRunFactoryPrompt(cmd, cfg, promptArgs, workChanged, preparation)
}

func runFactorySourceUsesJavaScript(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func resolveLegacyRunFactoryPrompt(cmd *cobra.Command, promptArgs []string, preparation work.InvocationInputPreparation) error {
	for _, arg := range promptArgs {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}
	// Directory-selected runs do not have a documented implicit stdin
	// invocation. Only an explicit positional argument or a `-` token asks
	// this compatibility path to inspect process stdin; otherwise a quiet pipe
	// must not delay Factory startup.
	if len(promptArgs) == 0 {
		return nil
	}
	input, err := prepareRunInvocationInputWithFile(cmd, promptArgs, nil, nil, preparation)
	if err != nil {
		return mapRunInvocationInputError(err, "")
	}
	if input.ResolvedInput != nil {
		return fmt.Errorf("positional prompt arguments require --factory or --named")
	}
	return nil
}

func resolveSignatureRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	signature *interfaces.InvocationSignatureConfig,
	preparation work.InvocationInputPreparation,
) error {
	if preparation == nil {
		return fmt.Errorf("Work invocation-input preparation is required")
	}
	stdinText, err := collectRunInvocationStdin(
		promptArgs,
		cmd.InOrStdin(),
		func() bool { return startupcli.StdinIsTTY(cmd.Context()) },
	)
	if err != nil {
		return mapRunInvocationInputError(err, cfg.NamedFactoryName)
	}
	// Service mode is a long-lived host, so signature defaults must not turn an
	// absent command input into an invocation. Explicit raw input remains on
	// the invocation path; WorkFile is handled later by the service runtime.
	if cfg.Continuously && !continuousSignatureInvocationRequested(cfg, promptArgs, stdinText) {
		return nil
	}
	prepared, err := prepareRunInvocationInputWithStdin(
		cmd.Context(),
		promptArgs,
		signature,
		invocationFilePath(cfg),
		preparation,
		stdinText,
	)
	if err != nil {
		return mapRunInvocationInputError(err, cfg.NamedFactoryName)
	}
	cfg.InvocationNormalizedArguments = prepared.NormalizedArguments
	cfg.PreparedInvocationInput = &prepared
	cfg.InvocationArguments = work.RuntimeInvocationArguments(signature, prepared.NormalizedArguments)
	return nil
}

func continuousSignatureInvocationRequested(
	cfg *runcli.RunConfig,
	promptArgs []string,
	stdinText *string,
) bool {
	if cfg != nil && cfg.InvocationFileExplicit {
		return true
	}
	return len(promptArgs) > 0 || stdinText != nil
}

func resolveCompatibilityRunFactoryPrompt(
	cmd *cobra.Command,
	cfg *runcli.RunConfig,
	promptArgs []string,
	workChanged bool,
	preparation work.InvocationInputPreparation,
) error {
	input, err := prepareRunInvocationInputWithFile(cmd, promptArgs, nil, invocationFilePath(cfg), preparation)
	if err != nil {
		return mapRunInvocationInputError(err, cfg.NamedFactoryName)
	}
	if workChanged && input.ResolvedInput != nil {
		return fmt.Errorf("%s cannot be used with --work", runcli.InvocationInputSourceFromWork(input.Source))
	}
	if workChanged {
		cfg.CleanInvocationInputSource = runcli.InvocationInputSourceWorkFile
	}
	if input.ResolvedInput == nil {
		return nil
	}
	cfg.PreparedInvocationInput = &input
	// Keep a non-nil, empty runtime argument set for compatibility input. It
	// marks this as a one-shot invocation so Factory Definitions can resolve
	// omitted exact placeholders to their normal runtime defaults, while the
	// compatibility text itself remains in PreparedInvocationInput.
	cfg.InvocationArguments = &work.InvocationArguments{
		Arguments: map[string]work.InvocationArgument{},
	}
	assignCompatibilityInvocationInput(cfg, input)
	return nil
}

func prepareRunInvocationInput(
	cmd *cobra.Command,
	promptArgs []string,
	signature *interfaces.InvocationSignatureConfig,
	preparation work.InvocationInputPreparation,
) (work.PreparedInvocationInput, error) {
	return prepareRunInvocationInputWithFile(cmd, promptArgs, signature, nil, preparation)
}

func prepareRunInvocationInputWithFile(
	cmd *cobra.Command,
	promptArgs []string,
	signature *interfaces.InvocationSignatureConfig,
	filePath *string,
	preparation work.InvocationInputPreparation,
) (work.PreparedInvocationInput, error) {
	if preparation == nil {
		return work.PreparedInvocationInput{}, fmt.Errorf("Work invocation-input preparation is required")
	}
	stdinText, err := collectRunInvocationStdin(
		promptArgs,
		cmd.InOrStdin(),
		func() bool { return startupcli.StdinIsTTY(cmd.Context()) },
	)
	if err != nil {
		return work.PreparedInvocationInput{}, err
	}
	return prepareRunInvocationInputWithStdin(
		cmd.Context(),
		promptArgs,
		signature,
		filePath,
		preparation,
		stdinText,
	)
}

func prepareRunInvocationInputWithStdin(
	ctx context.Context,
	promptArgs []string,
	signature *interfaces.InvocationSignatureConfig,
	filePath *string,
	preparation work.InvocationInputPreparation,
	stdinText *string,
) (work.PreparedInvocationInput, error) {
	if preparation == nil {
		return work.PreparedInvocationInput{}, fmt.Errorf("Work invocation-input preparation is required")
	}
	return preparation.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Arguments: append([]string(nil), promptArgs...),
		Signature: signature,
		StdinText: stdinText,
		FilePath:  filePath,
	})
}

func invocationFilePath(cfg *runcli.RunConfig) *string {
	if cfg == nil || !cfg.InvocationFileExplicit {
		return nil
	}
	path := cfg.InvocationFilePath
	return &path
}

func collectRunInvocationStdin(
	arguments []string,
	stdin io.Reader,
	stdinIsTTY func() bool,
) (*string, error) {
	explicitStdin := false
	for _, argument := range arguments {
		if strings.TrimSpace(argument) == "-" {
			explicitStdin = true
			break
		}
	}
	if stdinIsTTY == nil {
		if stdin == nil {
			return nil, fmt.Errorf("classify invocation stdin: process terminal metadata is required")
		}
	} else if !explicitStdin && stdinIsTTY() {
		return nil, nil
	}
	if stdin == nil {
		return nil, fmt.Errorf("read invocation stdin: process stdin is required")
	}
	data, err := io.ReadAll(io.LimitReader(stdin, maxRunInvocationStdinBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read invocation stdin: %w", err)
	}
	if len(data) > maxRunInvocationStdinBytes {
		return nil, fmt.Errorf(
			"invocation stdin exceeds the %d-byte limit; use --to-file for larger input",
			maxRunInvocationStdinBytes,
		)
	}
	if len(data) == 0 && !explicitStdin {
		return nil, nil
	}
	text := string(data)
	return &text, nil
}

func assignCompatibilityInvocationInput(cfg *runcli.RunConfig, input work.PreparedInvocationInput) {
	payload := input.ResolvedInput.Text
	source := runcli.InvocationInputSourceFromWork(input.Source)
	switch source {
	case runcli.InvocationInputSourcePositional:
		cfg.InvocationPositionalText = &payload
	case runcli.InvocationInputSourceStdin:
		cfg.InvocationStdinText = &payload
	}
	cfg.CleanInvocationInputSource = source
}

func mapRunInvocationInputError(err error, factoryName string) error {
	return runcli.MapInvocationInputError(work.QualifyInvocationArgumentError(err, factoryName))
}
