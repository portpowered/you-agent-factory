package cli

import (
	"fmt"
	"io"
	"strings"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newSubmitCommand(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operations CommandFactory,
) *cobra.Command {
	return newSubmitCommandWithHandlers(
		globals, diagnostics, operations.SubmitWork, operations.SubmitBatch,
	)
}

func newSubmitCommandWithHandlers(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	submitHandler func(submitcli.SubmitConfig) error,
	batchHandler func(submitcli.BatchConfig) error,
) *cobra.Command {
	cfg := submitcli.SubmitConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit work to a running factory",
		Long: "Submit work to a running you-agent-factory service.\n\n" +
			"Unary submit (this command) posts one work item with --name, --work-type-name, and --payload. " +
			"For multi-work FACTORY_REQUEST_BATCH ingress to an already-running session, use " +
			cliBinaryName + " submit batch. See " + cliBinaryName + " submit batch --help and " +
			cliBinaryName + " docs batch-inputs.\n\n" +
			"By default unary submit targets the default compatibility session. " +
			"Use --session to submit to one specific live factory session instead.",
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeSubmitCommand(cmd, &cfg, globals, diagnostics, submitHandler)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.Name, "name", "", "authored request name for the submitted work (required)")
	cmd.Flags().StringVar(&cfg.WorkTypeName, "work-type-name", "", "work type name to submit to (required)")
	cmd.Flags().StringVar(&cfg.Payload, "payload", "", "path to payload file (.json or .md) (required)")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	cmd.AddCommand(newSubmitBatchCommandWithHandler(globals, diagnostics, batchHandler))
	return cmd
}

func executeSubmitCommand(
	cmd *cobra.Command,
	cfg *submitcli.SubmitConfig,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	submitHandler func(submitcli.SubmitConfig) error,
) error {
	cfg.Context = cmd.Context()
	cfg.Server = globals.server
	cfg.JSON = globals.json
	cfg.Output = cmd.OutOrStdout()
	cfg.Diagnostics = diagnostics.writer(cmd)
	cfg.Verbose = diagnostics.verboseEnabled()
	cfg.Debug = diagnostics.debug
	return submitHandler(*cfg)
}

func newSubmitBatchCommandWithHandler(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	batchHandler func(submitcli.BatchConfig) error,
) *cobra.Command {
	cfg := submitcli.BatchConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "batch [path|-|<inline-json>]",
		Short: "Upsert a FACTORY_REQUEST_BATCH to a running factory",
		Long: "Upsert canonical FACTORY_REQUEST_BATCH JSON to a running you-agent-factory session.\n\n" +
			"The document must use type FACTORY_REQUEST_BATCH with at least one works entry. " +
			"See " + cliBinaryName + " docs batch-inputs for the batch schema, relation rules, and examples.\n\n" +
			"Batch input modes (first match wins):\n" +
			"  --file <path>   read batch JSON from path; use --file - to read stdin\n" +
			"  <path>          read batch JSON from an existing filesystem path (primary form)\n" +
			"  -               read batch JSON from stdin\n" +
			"  <inline-json>   positional whose first non-whitespace byte is { is parsed as JSON\n" +
			"  (no args)       when stdin is not a TTY, read the full piped document from stdin\n\n" +
			"When both --file and a positional path are provided, --file wins.\n" +
			"When a file path or --file is set, stdin is ignored.\n\n" +
			"Inline JSON is convenient for small batches; shell argument length limits apply—use a file or pipe for large documents.\n\n" +
			"Use --dry-run to parse and validate locally, print a summary, and perform no HTTP. " +
			"Valid --dry-run exits 0 even when the factory is unreachable.\n\n" +
			"By default the command targets the default compatibility session (~default). " +
			"Use --session to upsert into one specific live factory session instead.\n\n" +
			"Global flags (place before the subcommand): --server selects the factory API base URI; " +
			"--json emits structured success output on stdout; --verbose and --debug emit concise " +
			"request metadata on stderr without logging batch payload content.",
		Example: "  # Upsert a batch file to the default session.\n" +
			"  " + cliBinaryName + " submit batch ./factory/inputs/BATCH/my-batch.json\n\n" +
			"  # Explicit file flag (wins over a positional path when both are set).\n" +
			"  " + cliBinaryName + " submit batch --file ./batches/deploy.json\n\n" +
			"  # Pipe batch JSON without a temp file.\n" +
			"  cat batch.json | " + cliBinaryName + " submit batch\n\n" +
			"  # Read batch JSON from stdin explicitly.\n" +
			"  " + cliBinaryName + " submit batch - < batch.json\n\n" +
			"  # Validate locally without contacting the server.\n" +
			"  " + cliBinaryName + " submit batch --dry-run ./batch.json\n\n" +
			"  # Target a non-default live session with structured output.\n" +
			"  " + cliBinaryName + " --server http://localhost:9090 --json submit batch --session session-beta ./batch.json",
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeSubmitBatchCommand(cmd, args, &cfg, globals, diagnostics, batchHandler)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.FileFlag, "file", "", "read batch JSON from path; use - to read stdin (wins over a positional path)")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "validate input and print a summary without sending HTTP")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func executeSubmitBatchCommand(
	cmd *cobra.Command,
	args []string,
	cfg *submitcli.BatchConfig,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	batchHandler func(submitcli.BatchConfig) error,
) error {
	cfg.Context = cmd.Context()
	cfg.Server = globals.server
	cfg.JSON = globals.json
	cfg.Args = args
	cfg.Stdin = cmd.InOrStdin()
	cfg.StdinIsTTY = func() bool { return startupcli.StdinIsTTY(cmd.Context()) }
	cfg.Output = cmd.OutOrStdout()
	cfg.Verbose = diagnostics.verboseEnabled()
	cfg.Debug = diagnostics.debug
	return batchHandler(*cfg)
}

func parseRunCommandArgs(cmd *cobra.Command, args []string) ([]string, error) {
	remainder := make([]string, 0, len(args))
	flagsByToken := indexRunCommandFlags(cmd)

	for index := 0; index < len(args); index++ {
		token := args[index]
		if token == "--" {
			remainder = append(remainder, args[index+1:]...)
			break
		}
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

		value, consumedNext, err := resolveRunFlagValue(flag, args, index, hasInlineValue, inlineValue)
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
