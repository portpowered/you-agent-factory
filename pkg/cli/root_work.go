package cli

import (
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
	"github.com/spf13/cobra"
)

func newWorkCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	workCmd := &cobra.Command{
		Use:   "work",
		Short: "Inspect work from a running factory",
	}
	workCmd.AddCommand(newWorkListCommand(globals, diagnostics))
	workCmd.AddCommand(newWorkShowCommand(globals, diagnostics))
	workCmd.AddCommand(newWorkMoveCommand(globals, diagnostics))
	return workCmd
}

func newWorkListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
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
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
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

func newWorkShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
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
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return showWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	return cmd
}

func newWorkMoveCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
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
		Args:    cobra.ExactArgs(2),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Server = globals.server
			cfg.WorkID = args[0]
			cfg.StateName = args[1]
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return moveWork(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.SessionID, "session", "", "target one live factory session; omit to use the default compatibility session")
	cmd.Flags().StringVar(&cfg.RequestID, "request-id", "", "optional client idempotency key for operator moves")
	return cmd
}


func newSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "List, open, and close factory sessions on a running host",
		Long: "Manage factory sessions on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list    list live workspace sessions or durable Factory Sessions with --scope live|persisted|all\n" +
			"  show    show one live factory session from GET /factory-sessions/{session_id}\n" +
			"  create  open another live session from a folder path\n" +
			"  delete  close a live session by session id\n\n" +
			"Durable list output uses Factory Session status, source identity, result availability, " +
			"progress, and action availability. Session commands use the same default --port as work list. " +
			"Use --json to emit API-shaped responses on stdout; diagnostics stay on stderr when --verbose " +
			"or --debug is set.",
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
	cfg := sessioncli.ListConfig{Port: defaultcmd.FactoryPort, Scope: "live"}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List live and durable factory sessions",
		Long: "List factory sessions for the requested scope.\n\n" +
			"live returns workspace sessions kept open by the running host. persisted returns durable " +
			"Factory Sessions from the deterministic provider loopback. all returns both live workspace " +
			"sessions and durable Factory Sessions.\n\n" +
			"Human output prints the legacy live-session table for workspace rows and a durable Factory " +
			"Session table with status, source identity, result availability, progress, and actions. " +
			"Use --json to emit ListFactorySessionsResponse on stdout.",
		Example: "  " + cliBinaryName + " session list\n\n" +
			"  " + cliBinaryName + " session list --scope persisted\n\n" +
			"  " + cliBinaryName + " session list --scope all --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessions(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.Scope, "scope", cfg.Scope, "session list scope: live, persisted, or all")
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
