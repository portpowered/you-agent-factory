package cli

import (
	submitcli "github.com/portpowered/infinite-you/pkg/cli/submit"
	"github.com/spf13/cobra"
)

func newSubmitBatchCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
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
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Args = args
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return submitBatch(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.FileFlag, "file", "", "read batch JSON from path; use - to read stdin (wins over a positional path)")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "validate input and print a summary without sending HTTP")
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}
