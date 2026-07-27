package cli

import (
	"fmt"
	"io"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func executeSubmitCommand(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	submitHandler func(submitcli.SubmitConfig) error,
) error {
	if submitHandler == nil {
		return fmt.Errorf("submit service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	name, err := commandInputValue[string](values, "you.submit.flag.name")
	if err != nil {
		return err
	}
	workTypeName, err := commandInputValue[string](values, "you.submit.flag.work-type-name")
	if err != nil {
		return err
	}
	payload, err := commandInputValue[string](values, "you.submit.flag.payload")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.submit.flag.session")
	if err != nil {
		return err
	}
	return submitHandler(submitcli.SubmitConfig{
		Context: cmd.Context(), Server: globals.server, JSON: globals.json,
		Name: name, WorkTypeName: workTypeName, Payload: payload, SessionID: sessionID,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

func executeSubmitBatchCommand(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	fileSystem submitcli.BatchInputFileSystem,
	batchHandler func(submitcli.BatchConfig) error,
) error {
	if batchHandler == nil {
		return fmt.Errorf("submit batch service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	fileFlag, err := commandInputValue[string](values, "you.submit.batch.flag.file")
	if err != nil {
		return err
	}
	dryRun, err := commandInputValue[bool](values, "you.submit.batch.flag.dry-run")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.submit.batch.flag.session")
	if err != nil {
		return err
	}
	return batchHandler(submitcli.BatchConfig{
		Context: cmd.Context(), Server: globals.server, JSON: globals.json,
		FileFlag: fileFlag, DryRun: dryRun, SessionID: sessionID,
		Args: args, Stdin: cmd.InOrStdin(), FileSystem: fileSystem,
		StdinIsTTY: func() bool { return startupcli.StdinIsTTY(cmd.Context()) },
		Output:     cmd.OutOrStdout(), Verbose: diagnostics.verboseEnabled(),
		Debug: diagnostics.debug,
	})
}

func newRunSubmitFlagBindings() climanifestcobra.RunSubmitFlagBindings {
	targets := map[string]any{}
	stringInputs := []string{
		"you.run.flag.work", "you.run.flag.dir", "you.run.flag.named",
		"you.run.flag.factory", "you.run.flag.record", "you.run.flag.replay",
		"you.run.flag.runtime-log-dir", "you.run.flag.runtime-metrics-dir",
		"you.run.flag.with-mock-workers", "you.run.flag.output",
		"you.submit.flag.name", "you.submit.flag.work-type-name",
		"you.submit.flag.payload", "you.submit.flag.session",
		"you.submit.batch.flag.file", "you.submit.batch.flag.session",
	}
	boolInputs := []string{
		"you.run.flag.continuously", "you.run.flag.no-record",
		"you.run.flag.runtime-log-compress", "you.run.flag.runtime-metrics-compress",
		"you.run.flag.with-server", "you.run.flag.with-site", "you.run.flag.quiet",
		"you.run.flag.skip-permissions", "you.submit.batch.flag.dry-run",
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
	return climanifestcobra.RunSubmitFlagBindings{LocalTargets: targets}
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
		{"you.run.flag.record", &cfg.RecordPath},
		{"you.run.flag.replay", &cfg.ReplayPath},
		{"you.run.flag.runtime-log-dir", &cfg.RuntimeLogDir},
		{"you.run.flag.runtime-metrics-dir", &cfg.RuntimeMetricsDir},
		{"you.run.flag.with-mock-workers", &cfg.MockWorkersConfigPath},
		{"you.run.flag.output", &cfg.InvocationOutputMode},
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
			return nil, err
		}
		flag.Changed = true
		if consumedNext {
			index++
		}
	}

	remainder = append(remainder, positional...)
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
	if flag.NoOptDefVal != "" {
		return flag.NoOptDefVal, false, nil
	}
	if index+1 >= len(args) {
		return "", false, fmt.Errorf("flag needs an argument: %s", "--"+flag.Name)
	}
	return args[index+1], true, nil
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
		if err := cmd.ValidateRequiredFlags(); err != nil {
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
	if err := cmd.ValidateRequiredFlags(); err != nil {
		return cmd, positionals, err
	}
	if err := cmd.ValidateFlagGroups(); err != nil {
		return cmd, positionals, err
	}
	return cmd, positionals, nil
}
