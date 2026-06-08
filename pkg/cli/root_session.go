package cli

import (
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	"github.com/spf13/cobra"
)

func newSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "List, open, and close live factory sessions on a running host",
		Long: "Manage live factory sessions on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list    list live factory sessions from GET /factory-sessions\n" +
			"  show    show one live factory session from GET /factory-sessions/{session_id}\n" +
			"  create  open another live session from a folder path\n" +
			"  delete  close a live session by session id\n\n" +
			"Session commands use the same default --port as work list. Use --json to emit API-shaped " +
			"responses on stdout; diagnostics stay on stderr when --verbose or --debug is set.",
		Example: "  # List live sessions on the default local port.\n" +
			"  " + cliBinaryName + " session list\n\n" +
			"  # Show orchestrator-aware runtime for one live session.\n" +
			"  " + cliBinaryName + " session show session-beta\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " session list --json\n\n" +
			"  # Open and close sessions on a non-default port.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --port 9090\n" +
			"  " + cliBinaryName + " session delete session-beta --port 9090 --json\n\n" +
			"  # Target a different service port for list output.\n" +
			"  " + cliBinaryName + " session list --port 9090",
	}
	sessionCmd.AddCommand(
		newSessionListCommand(diagnostics),
		newSessionShowCommand(globals, diagnostics),
		newSessionCreateCommand(diagnostics),
		newSessionDeleteCommand(diagnostics),
	)
	return sessionCmd
}

func newSessionShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.ShowConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "show [session-id]",
		Short: "Show one live factory session",
		Long: "Show one live factory session from GET /factory-sessions/{session_id}.\n\n" +
			"Human output uses FactorySession as the canonical runtime noun and prints orchestrator " +
			"kind plus Petri or JavaScript runtime projections. Dynamic workflow wording appears only " +
			"as JavaScript shorthand. Omit session-id to target the default compatibility session " +
			"(~default). Use global --json for the API-shaped FactorySession payload and global " +
			"--server to target the same factory API base URI as work list and factory query.",
		Example: "  # Show the default compatibility factory session.\n" +
			"  " + cliBinaryName + " session show\n\n" +
			"  # Show one named live session with orchestrator-aware runtime fields.\n" +
			"  " + cliBinaryName + " session show session-beta\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session show session-beta",
		Args:    cobra.MaximumNArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return showSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionListCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.ListConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List live factory sessions",
		Long: "List every live factory session that the running host currently has open.\n\n" +
			"Human output prints session id, project, folder path, factory dir, default marker, and " +
			"target kind/name. Use --json to emit ListFactorySessionsResponse on stdout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessions(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API list-factory-sessions JSON response")
	return cmd
}

func newSessionCreateCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.CreateConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Open a live factory session from a folder",
		Long: "Open another live factory session from a folder path via POST /factory-sessions.\n\n" +
			"Use --dir to provide the folder path. Optional --init-new-factory and --validate-only " +
			"map to the API request fields and are mutually exclusive. Use --target-kind and --target-name " +
			"when the folder exposes multiple runnable targets. Use --json to emit OpenFactorySessionResponse " +
			"on stdout; diagnostics stay on stderr when --verbose or --debug is set.",
		Example: "  # Open a session for an existing factory folder.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet\n\n" +
			"  # Validate a folder without opening a live session.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --validate-only\n\n" +
			"  # Open a named target inside a multi-factory folder.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --target-kind named --target-name beta",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return createSession(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Dir, "dir", "", "folder path to open as a live factory session")
	cmd.Flags().BoolVar(&cfg.InitNewFactory, "init-new-factory", false, "write the default init scaffold at --dir and open a live session")
	cmd.Flags().BoolVar(&cfg.ValidateOnly, "validate-only", false, "validate the folder and optional target without creating a live session")
	cmd.Flags().StringVar(&cfg.TargetKind, "target-kind", "", "target kind when disambiguating runnable factories (default or named)")
	cmd.Flags().StringVar(&cfg.TargetName, "target-name", "", "named target when --target-kind is named")
	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API open-factory-session JSON response")
	_ = cmd.MarkFlagRequired("dir")
	cmd.MarkFlagsMutuallyExclusive("init-new-factory", "validate-only")
	return cmd
}

func newSessionDeleteCommand(diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.DeleteConfig{Port: defaultcmd.FactoryPort}

	cmd := &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Close a live factory session",
		Long: "Close one live factory session via DELETE /factory-sessions/{session_id}.\n\n" +
			"Use the same --port selection as session list. Use --json to emit a JSON confirmation on stdout.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.SessionID = args[0]
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return deleteSession(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit a JSON confirmation after the session closes")
	return cmd
}
