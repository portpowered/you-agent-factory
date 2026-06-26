package cli

import (
	"fmt"

	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/spf13/cobra"
)

func newRunCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions) *cobra.Command {
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
			if err := rejectDeprecatedPortFlag(cmd, args); err != nil {
				return err
			}
			normalized, err := runcli.NormalizeInvocationOutputMode(invocationOutputMode)
			if err != nil {
				return err
			}
			cfg.InvocationOutputMode = normalized
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeRunCommand(cmd, args, &cfg, globals, diagnostics, operatorDefaults)
		},
	}
	registerRunCommandFlags(cmd, &cfg, &invocationOutputMode)
	return cmd
}

func executeRunCommand(cmd *cobra.Command, args []string, cfg *runcli.RunConfig, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, operatorDefaults *cliOperatorDefaultsOptions) error {
	promptArgs, resolvedConfig, err := resolveRunCommandInvocationInput(cmd, args, cfg)
	if err != nil {
		return err
	}
	if helpRequested(cmd) {
		return writeRunCommandHelp(cmd, &resolvedConfig)
	}
	err = runFactory(cmd, resolvedConfig, promptArgs, globals, operatorDefaults, diagnostics.verboseEnabled(), diagnostics.debug)
	if err != nil && !runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
	}
	return err
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

func writeRunCommandHelp(cmd *cobra.Command, cfg *runcli.RunConfig) error {
	if err := resolveRunFactorySelection(cmd, cfg); err != nil {
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
	cmd.Flags().StringVar(&cfg.Workflow, "workflow", "", "workflow ID to run (default: all)")
	cmd.Flags().BoolVar(&cfg.Continuously, "continuously", false, "keep the factory alive while idle until cancelled")
	cmd.Flags().StringVar(&cfg.WorkFile, "work", "", "path to initial FACTORY_REQUEST_BATCH JSON file to submit")
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "factory base directory")
	cmd.Flags().StringVar(&cfg.NamedFactoryName, "named", "", "canonical persisted factory name resolved from ./factory before ~/.you-agent-factory/factories; built-ins materialize there on first use and remain editable")
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
}

func runCommandLongHelp() string {
	return "Load workflow and run the factory engine.\n\n" +
		"For the quickest local setup, run " + cliBinaryName + " with no arguments. " +
		"That default flow bootstraps ./factory, watches factory/inputs/task/default, " +
		"keeps the runtime alive, and reports the first available dashboard URL, preferring http://localhost:7437/dashboard/ui. " +
		"Default execution uses batch mode and exits after idle completion. " +
		"Normal live runs record by default unless you pass --no-record. " +
		"Replay artifacts are sensitive and can contain prompts, payloads, stdout, stderr, and diagnostic metadata. " +
		"Use global --default-worker-model-provider and --default-worker-model to set operator-level model defaults for omitted model-worker fields. " +
		"Use --continuously to keep the factory alive while idle until you cancel it. " +
		"Use --with-mock-workers with an optional JSON config path to test workflows with deterministic mock worker outcomes. " +
		"Use --quiet to suppress dashboard output for scripted or CI-oriented runs. " +
		"Use --named with a persisted canonical factory name to resolve project-local factories before global built-ins under ~/.you-agent-factory/factories. " +
		"Built-ins such as @you/tts and @you/goal materialize lazily into that global root on first use and stay editable on disk for later runs. " +
		"Use --factory with a factory.json file path to run a portable factory config without guessing --dir. " +
		"Selected factories can define custom invocation arguments; run " + cliBinaryName + " run --named <factory> --help or " + cliBinaryName + " run --factory <factory.json> --help to inspect signature-backed usage while keeping existing run-level flags available. " +
		"In factory invocation mode, provide either trailing positional text or piped stdin text; supplying both is rejected with INVOCATION_INPUT_SOURCE_CONFLICT. " +
		"Packaged @you/fusion, @you/goal, and @you/tts invocation details live in " + cliBinaryName + " docs packaged-fusion, " + cliBinaryName + " docs packaged-goal, and " + cliBinaryName + " docs packaged-tts. " +
		"Full invocation input and return-policy details live in " + cliBinaryName + " docs config and " + cliBinaryName + " docs sessions. " +
		"Use --output response-stream on supported one-shot factory invocations to render live internal session response-stream progress while the CLI owns the runtime; unsupported run shapes fall back to primary-result-only output or return INVOCATION_OUTPUT_UNSUPPORTED. " +
		"Runtime logs are structured JSON rolling files grouped by UTC start date under the selected log root. " +
		"Runtime metrics are a separate structured JSONL operational channel with their own rolling files and do not replace runtime logs. " +
		"Environment details are record-channel diagnostics only, and system logs include command stdout/stderr only on command failures."
}

func runCommandExamples() string {
	return "  # Start the out-of-the-box continuous factory.\n" +
		"  " + cliBinaryName + "\n\n" +
		"  # Submit a Markdown task to the default scaffold.\n" +
		"  printf \"Fix the lint issues\\n\" > factory/inputs/task/default/fix-lint.md\n\n" +
		"  # Run an existing factory once in explicit batch mode.\n" +
		"  " + cliBinaryName + " run --dir factory\n\n" +
		"  # Run a persisted named factory from any working directory.\n" +
		"  " + cliBinaryName + " run --named @you/tts\n\n" +
		"  # Run a portable factory.json with a one-shot prompt (see handlingBehavior DEFAULT).\n" +
		"  " + cliBinaryName + " run --factory ./factory.json \"Fix the lint issues\"\n\n" +
		"  # Render live internal response-stream progress for a named goal invocation.\n" +
		"  " + cliBinaryName + " run --named @you/goal --output response-stream \"Ship the login bugfix\""
}
