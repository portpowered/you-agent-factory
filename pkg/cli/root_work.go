package cli

import (
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	workcli "github.com/portpowered/infinite-you/pkg/cli/work"
	workflowcli "github.com/portpowered/infinite-you/pkg/cli/workflow"
	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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
			"  pause   pause one live or durable Factory Session through POST /factory-sessions/{session_id}/pause\n" +
			"  resume  resume one paused live or durable Factory Session through POST /factory-sessions/{session_id}/resume\n" +
			"  create  open another live session from a folder path\n" +
			"  delete  close a live session by session id\n" +
			"  pause   pause automatic progression for one live session\n" +
			"  resume  resume a paused live session and drain buffered work\n\n" +
			"Durable list output uses Factory Session status, source identity, result availability, " +
			"progress, and action availability. Session commands use the same default --port as work list. " +
			"Use --json to emit API-shaped responses on stdout; diagnostics stay on stderr when --verbose " +
			"or --debug is set.",
		Example: "  # List live sessions on the default local port.\n" +
			"  " + cliBinaryName + " session list\n\n" +
			"  # Show orchestrator-aware runtime for one live session.\n" +
			"  " + cliBinaryName + " session show session-beta\n\n" +
			"  # Pause and resume the default compatibility session.\n" +
			"  " + cliBinaryName + " session pause\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Pause and resume one named or durable Factory Session.\n" +
			"  " + cliBinaryName + " session pause session-beta\n" +
			"  " + cliBinaryName + " session resume dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " session list --json\n\n" +
			"  # Open and close sessions on a non-default port.\n" +
			"  " + cliBinaryName + " session create --dir /workspace/fleet --port 9090\n" +
			"  " + cliBinaryName + " session delete session-beta --port 9090 --json\n\n" +
			"  # Pause and resume the default compatibility session.\n" +
			"  " + cliBinaryName + " session pause\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Target a different service port for list output.\n" +
			"  " + cliBinaryName + " session list --port 9090",
	}
	sessionCmd.AddCommand(
		newSessionListCommand(diagnostics),
		newSessionShowCommand(globals, diagnostics),
		newSessionPauseCommand(globals, diagnostics),
		newSessionResumeCommand(globals, diagnostics),
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

func newSessionPauseCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.LifecycleControlConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "pause [session-id]",
		Short: "Pause one live or durable Factory Session",
		Long: "Pause one live or durable Factory Session through POST /factory-sessions/{session_id}/pause.\n\n" +
			"Human output reports paused, already-paused, invalid-state, not-found, or unreachable-host " +
			"outcomes using Factory Session terminology. Omit session-id to target the default compatibility " +
			"session (~default), pass a named live session id such as session-beta, or pass a durable " +
			"session id from `you session list --scope all`. Use global --json for the API-shaped " +
			"FactorySessionLifecycleControlResponse and global --server to target the same factory API base URI " +
			"as workflow status and session show.",
		Example: "  # Pause the default compatibility Factory Session.\n" +
			"  " + cliBinaryName + " session pause\n\n" +
			"  # Pause one named live Factory Session.\n" +
			"  " + cliBinaryName + " session pause session-beta\n\n" +
			"  # Pause one running durable Factory Session.\n" +
			"  " + cliBinaryName + " session pause dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session pause session-beta",
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
			return pauseSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionResumeCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
	cfg := sessioncli.LifecycleControlConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "resume [session-id]",
		Short: "Resume one paused live or durable Factory Session",
		Long: "Resume one paused live or durable Factory Session through POST /factory-sessions/{session_id}/resume.\n\n" +
			"Human output reports resumed, already-running, invalid-state, not-found, or unreachable-host " +
			"outcomes using Factory Session terminology. Omit session-id to target the default compatibility " +
			"session (~default), pass a named live session id such as session-beta, or pass a durable " +
			"session id from `you session list --scope all`. Use global --json for the API-shaped " +
			"FactorySessionLifecycleControlResponse and global --server to target the same factory API base URI " +
			"as workflow status and session show.",
		Example: "  # Resume the default compatibility Factory Session.\n" +
			"  " + cliBinaryName + " session resume\n\n" +
			"  # Resume one named live Factory Session.\n" +
			"  " + cliBinaryName + " session resume session-beta\n\n" +
			"  # Resume one paused durable Factory Session.\n" +
			"  " + cliBinaryName + " session resume dur-sess-js-run-n-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session resume session-beta",
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
			return resumeSession(cfg)
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

func newWorkflowCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Validate JavaScript workflow sources for Factory Session execution",
		Long: "Validate JavaScript or TypeScript workflow sources before starting a Factory Session.\n\n" +
			"Subcommands:\n" +
			"  validate   primary CLI path: resolve workflow source and validate it without execution\n" +
			"  preview    compatibility alias for the Factory preview contract; prefer validate for CLI checks\n" +
			"  run        start one durable Factory Session synchronously through the mock-backed execution loop\n" +
			"  start      start one durable Factory Session asynchronously and return inspection links\n" +
			"  status     read the durable Factory Session lifecycle and progress state\n" +
			"  result     read the durable Factory Session final or partial result\n" +
			"  dispatches list durable Factory Session dispatches for inspection\n" +
			"  artifacts  list durable Factory Session artifacts for inspection\n" +
			"  events     poll ordered durable Factory Session events with optional reconnect cursors",
	}
	cmd.AddCommand(
		newWorkflowValidateCommand(globals),
		newWorkflowPreviewCommand(globals),
		newWorkflowRunCommand(globals),
		newWorkflowStartCommand(globals),
		newWorkflowStatusCommand(globals),
		newWorkflowResultCommand(globals),
		newWorkflowDispatchesCommand(globals),
		newWorkflowArtifactsCommand(globals),
		newWorkflowEventsCommand(globals),
	)
	return cmd
}

func newWorkflowValidateCommand(globals *cliGlobalOptions) *cobra.Command {
	cfg := workflowcli.ValidateConfig{SourceConfig: workflowcli.SourceConfig{Dir: defaultcmd.FactoryDir}}
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate JavaScript workflow source",
		Long:  "Resolve workflow source and validate it without execution using the shared workflow validation contract.",
		Example: "  # Validate a project workflow by name.\n" +
			"  " + cliBinaryName + " workflow validate --kind WORKFLOW_NAME --value review\n\n" +
			"  # Validate inline workflow source.\n" +
			"  " + cliBinaryName + " workflow validate --kind INLINE_WORKFLOW --inline \"phase('setup');\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return workflowcli.Validate(cfg)
		},
	}
	addWorkflowSourceFlags(cmd, &cfg.SourceConfig)
	return cmd
}

func newWorkflowPreviewCommand(globals *cliGlobalOptions) *cobra.Command {
	cfg := workflowcli.PreviewConfig{SourceConfig: workflowcli.SourceConfig{Dir: defaultcmd.FactoryDir}}
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Compatibility preview of workflow validation and policy",
		Long: "Compatibility command for the Factory preview contract. Resolve workflow source, validate it " +
			"without execution, and print source, loader, policy, and result-shape diagnostics. Prefer " +
			cliBinaryName + " workflow validate for CLI source checks before Factory Session execution.",
		Example: "  # Preview a project workflow by name.\n" +
			"  " + cliBinaryName + " workflow preview --kind WORKFLOW_NAME --value review\n\n" +
			"  # Preview inline workflow source.\n" +
			"  " + cliBinaryName + " workflow preview --kind INLINE_WORKFLOW --inline \"phase('setup');\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			return workflowcli.Preview(cfg)
		},
	}
	addWorkflowSourceFlags(cmd, &cfg.SourceConfig)
	return cmd
}

func addWorkflowSourceFlags(command *cobra.Command, cfg *workflowcli.SourceConfig) {
	command.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "project root used for ordered workflow source lookup")
	command.Flags().StringVar(&cfg.SourceKind, "kind", string(workflowsource.KindWorkflowName), "workflow source kind")
	command.Flags().StringVar(&cfg.SourceValue, "value", "", "workflow name, file ref, or factory id")
	command.Flags().StringVar(&cfg.InlineSource, "inline", "", "inline workflow source text")
	command.Flags().StringVar(&cfg.ArtifactRoot, "artifact-root", "", "optional absolute artifact root")
	command.Flags().StringVar(&cfg.ArgsSchema, "args-schema", "", "optional orchestrator.javascript argsSchema JSON")
	command.Flags().StringVar(&cfg.RequestedPolicyJSON, "requested-policy", "", "optional requested policy override JSON")
}

func newWorkflowRunCommand(globals *cliGlobalOptions) *cobra.Command {
	runCfg := sessionexecutioncli.RunConfig{
		StartConfig: sessionexecutioncli.StartConfig{
			Mode: sessionexecutioncli.ExecutionModeSync,
		},
	}
	var waitTimeoutMillis int64
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one durable Factory Session synchronously",
		Long: "Start one durable Factory Session synchronously through the shared execution request contract.\n\n" +
			"The mock-backed provider path resolves fixture-backed request ids to deterministic session, " +
			"status, result, and inspection-link outcomes. Use global --json to emit FactorySessionSyncExecutionResponse " +
			"on stdout; timed-out sync runs also include requestId, cancelOnTimeout, and resultAvailability.",
		Example: "  # Run the published sync-success fixture by factory id and request id.\n" +
			"  " + cliBinaryName + " workflow run --request-id req-petri-success-001 --factory customer-support-triage\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow run --request-id req-petri-success-001 --factory customer-support-triage",
		RunE: func(cmd *cobra.Command, args []string) error {
			runCfg.JSON = globals.json
			runCfg.Output = cmd.OutOrStdout()
			runCfg.StartConfig.PositionalArgs = args
			runCfg.StartConfig.Stdin = cmd.InOrStdin()
			if cmd.Flags().Changed("wait-timeout-millis") {
				runCfg.StartConfig.WaitTimeoutMillis = &waitTimeoutMillis
			}
			return sessionexecutioncli.RunSync(cmd.Context(), runCfg)
		},
	}
	cmd.Flags().StringVar(&runCfg.StartConfig.RequestID, "request-id", "", "durable execution request id and idempotency key")
	cmd.Flags().StringVar(&runCfg.StartConfig.FactoryID, "factory", "", "factory id source selector")
	cmd.Flags().StringVar(&runCfg.StartConfig.WorkflowName, "workflow", "", "workflow name source selector")
	cmd.Flags().StringVar(&runCfg.StartConfig.WorkflowFile, "workflow-file", "", "workflow file source selector")
	cmd.Flags().StringVar(&runCfg.StartConfig.ArgsJSON, "args", "", "execution args JSON object")
	cmd.Flags().StringVar(&runCfg.StartConfig.PolicyJSON, "policy", "", "requested policy JSON object")
	cmd.Flags().StringVar(&runCfg.StartConfig.PolicyHash, "policy-hash", "", "requested policy hash selector")
	cmd.Flags().Int64Var(&waitTimeoutMillis, "wait-timeout-millis", 0, "sync wait timeout in milliseconds")
	cmd.Flags().BoolVar(&runCfg.StartConfig.CancelOnTimeout, "cancel-on-timeout", false, "request session cancel when sync wait times out")
	cmd.Flags().StringVar(&runCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed runs")
	addWorkflowExecutionBackendFlags(cmd, &runCfg.ExecutionBackendConfig, &runCfg.StartConfig.ChildExecutorMode)
	return cmd
}

func newWorkflowStartCommand(globals *cliGlobalOptions) *cobra.Command {
	startCfg := sessionexecutioncli.RunConfig{
		StartConfig: sessionexecutioncli.StartConfig{
			Mode: sessionexecutioncli.ExecutionModeAsync,
		},
	}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start one durable Factory Session asynchronously",
		Long: "Start one durable Factory Session asynchronously through the shared execution request contract.\n\n" +
			"The mock-backed provider path resolves fixture-backed request ids to deterministic session, " +
			"status, result, and inspection-link outcomes. Use global --json to emit FactorySessionExecutionResponse " +
			"fields plus requestId and resultAvailability on stdout.",
		Example: "  # Start the published async-running fixture by workflow name and request id.\n" +
			"  " + cliBinaryName + " workflow start --request-id req-js-run-n-001 --workflow release-train\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow start --request-id req-js-run-n-001 --workflow release-train",
		RunE: func(cmd *cobra.Command, args []string) error {
			startCfg.JSON = globals.json
			startCfg.Output = cmd.OutOrStdout()
			startCfg.StartConfig.PositionalArgs = args
			startCfg.StartConfig.Stdin = cmd.InOrStdin()
			return sessionexecutioncli.RunAsync(cmd.Context(), startCfg)
		},
	}
	addWorkflowStartFlags(cmd, &startCfg.StartConfig, &startCfg.FixtureCatalogPath)
	addWorkflowExecutionBackendFlags(cmd, &startCfg.ExecutionBackendConfig, &startCfg.StartConfig.ChildExecutorMode)
	return cmd
}

func newWorkflowStatusCommand(globals *cliGlobalOptions) *cobra.Command {
	statusCfg := sessionexecutioncli.StatusConfig{}
	cmd := &cobra.Command{
		Use:   "status [session-id]",
		Short: "Read one durable Factory Session status",
		Long: "Read one durable Factory Session lifecycle, progress, result availability, and inspection links " +
			"through the shared execution service. Use global --json to emit FactorySessionDurableReadModel on stdout.",
		Args: cobra.ExactArgs(1),
		Example: "  # Poll the async-running fixture session started earlier.\n" +
			"  " + cliBinaryName + " workflow status dur-sess-js-run-n-001\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow status dur-sess-js-run-n-001",
		RunE: func(cmd *cobra.Command, args []string) error {
			statusCfg.JSON = globals.json
			statusCfg.Output = cmd.OutOrStdout()
			statusCfg.SessionID = args[0]
			return sessionexecutioncli.RunStatus(cmd.Context(), statusCfg)
		},
	}
	cmd.Flags().StringVar(&statusCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed status reads")
	addWorkflowExecutionBackendFlags(cmd, &statusCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowResultCommand(globals *cliGlobalOptions) *cobra.Command {
	resultCfg := sessionexecutioncli.ResultConfig{}
	cmd := &cobra.Command{
		Use:   "result [session-id]",
		Short: "Read one durable Factory Session result",
		Long: "Read one durable Factory Session final or partial result through the shared execution service.\n\n" +
			"Use --mode partial for in-progress partial reads and --mode final for terminal or not-ready final reads. " +
			"Use global --json to emit FactorySessionResult on stdout.",
		Args: cobra.ExactArgs(1),
		Example: "  # Read the final result for a completed fixture session.\n" +
			"  " + cliBinaryName + " workflow result dur-sess-petri-success-001\n\n" +
			"  # Read partial progress for a running fixture session.\n" +
			"  " + cliBinaryName + " workflow result dur-sess-js-run-n-001 --mode partial\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow result dur-sess-petri-success-001",
		RunE: func(cmd *cobra.Command, args []string) error {
			resultCfg.JSON = globals.json
			resultCfg.Output = cmd.OutOrStdout()
			resultCfg.SessionID = args[0]
			return sessionexecutioncli.RunResult(cmd.Context(), resultCfg)
		},
	}
	cmd.Flags().StringVar(&resultCfg.Mode, "mode", "", "result read mode: final or partial")
	cmd.Flags().BoolVar(&resultCfg.IncludeArtifacts, "include-artifacts", false, "include artifact refs in the result read")
	cmd.Flags().StringVar(&resultCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed result reads")
	addWorkflowExecutionBackendFlags(cmd, &resultCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowDispatchesCommand(globals *cliGlobalOptions) *cobra.Command {
	dispatchesCfg := sessionexecutioncli.DispatchesConfig{}
	cmd := &cobra.Command{
		Use:   "dispatches [session-id]",
		Short: "List durable Factory Session dispatches",
		Long: "List durable Factory Session dispatches through the shared execution service. " +
			"Use global --json to emit ListFactorySessionDispatchesResponse on stdout.",
		Args: cobra.ExactArgs(1),
		Example: "  # List dispatches for the sync-success fixture session.\n" +
			"  " + cliBinaryName + " workflow dispatches dur-sess-petri-success-001\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow dispatches dur-sess-petri-success-001",
		RunE: func(cmd *cobra.Command, args []string) error {
			dispatchesCfg.JSON = globals.json
			dispatchesCfg.Output = cmd.OutOrStdout()
			dispatchesCfg.SessionID = args[0]
			return sessionexecutioncli.RunDispatches(cmd.Context(), dispatchesCfg)
		},
	}
	cmd.Flags().StringVar(&dispatchesCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed dispatch reads")
	addWorkflowExecutionBackendFlags(cmd, &dispatchesCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowArtifactsCommand(globals *cliGlobalOptions) *cobra.Command {
	artifactsCfg := sessionexecutioncli.ArtifactsConfig{}
	cmd := &cobra.Command{
		Use:   "artifacts [session-id]",
		Short: "List durable Factory Session artifacts",
		Long: "List durable Factory Session artifacts through the shared execution service. " +
			"Use global --json to emit ListFactorySessionArtifactsResponse on stdout.",
		Args: cobra.ExactArgs(1),
		Example: "  # List artifacts for the sync-success fixture session.\n" +
			"  " + cliBinaryName + " workflow artifacts dur-sess-petri-success-001\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow artifacts dur-sess-petri-success-001",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifactsCfg.JSON = globals.json
			artifactsCfg.Output = cmd.OutOrStdout()
			artifactsCfg.SessionID = args[0]
			return sessionexecutioncli.RunArtifacts(cmd.Context(), artifactsCfg)
		},
	}
	cmd.Flags().StringVar(&artifactsCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed artifact reads")
	addWorkflowExecutionBackendFlags(cmd, &artifactsCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowEventsCommand(globals *cliGlobalOptions) *cobra.Command {
	eventsCfg := sessionexecutioncli.EventsConfig{}
	var afterSequence int
	cmd := &cobra.Command{
		Use:   "events [session-id]",
		Short: "Poll durable Factory Session events",
		Long: "Poll ordered durable Factory Session events through the shared execution service. " +
			"Use --after-event-id or --after-sequence to reconnect after a prior cursor. " +
			"Use global --json to emit a FactoryEvent array on stdout.",
		Args: cobra.ExactArgs(1),
		Example: "  # Poll events for the async-running fixture session.\n" +
			"  " + cliBinaryName + " workflow events dur-sess-js-run-n-001\n\n" +
			"  # Reconnect after the session-started event.\n" +
			"  " + cliBinaryName + " workflow events dur-sess-js-run-n-001 --after-event-id session-started/dur-sess-js-run-n-001\n\n" +
			"  # Emit deterministic JSON for automation.\n" +
			"  " + cliBinaryName + " --json workflow events dur-sess-js-run-n-001",
		RunE: func(cmd *cobra.Command, args []string) error {
			eventsCfg.JSON = globals.json
			eventsCfg.Output = cmd.OutOrStdout()
			eventsCfg.SessionID = args[0]
			if cmd.Flags().Changed("after-sequence") {
				eventsCfg.AfterSequence = &afterSequence
			}
			return sessionexecutioncli.RunEvents(cmd.Context(), eventsCfg)
		},
	}
	cmd.Flags().StringVar(&eventsCfg.AfterEventID, "after-event-id", "", "reconnect cursor event id")
	cmd.Flags().IntVar(&afterSequence, "after-sequence", 0, "reconnect cursor session sequence")
	cmd.Flags().StringVar(&eventsCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed event reads")
	addWorkflowExecutionBackendFlags(cmd, &eventsCfg.ExecutionBackendConfig, nil)
	return cmd
}

func addWorkflowStartFlags(cmd *cobra.Command, startCfg *sessionexecutioncli.StartConfig, fixtureCatalogPath *string) {
	cmd.Flags().StringVar(&startCfg.RequestID, "request-id", "", "durable execution request id and idempotency key")
	cmd.Flags().StringVar(&startCfg.FactoryID, "factory", "", "factory id source selector")
	cmd.Flags().StringVar(&startCfg.WorkflowName, "workflow", "", "workflow name source selector")
	cmd.Flags().StringVar(&startCfg.WorkflowFile, "workflow-file", "", "workflow file source selector")
	cmd.Flags().StringVar(&startCfg.ArgsJSON, "args", "", "execution args JSON object")
	cmd.Flags().StringVar(&startCfg.PolicyJSON, "policy", "", "requested policy JSON object")
	cmd.Flags().StringVar(&startCfg.PolicyHash, "policy-hash", "", "requested policy hash selector")
	cmd.Flags().StringVar(fixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed starts")
}

func addWorkflowExecutionBackendFlags(
	cmd *cobra.Command,
	backend *sessionexecutioncli.ExecutionBackendConfig,
	childExecutorMode *string,
) {
	cmd.Flags().StringVar(&backend.Provider, "execution-provider", string(fse.ExecutionProviderFake), "durable execution backend: fake or javascript-runtime")
	cmd.Flags().StringVar(&backend.ProjectRoot, "project-root", "", "project root for javascript-runtime workflow source lookup")
	if childExecutorMode != nil {
		cmd.Flags().StringVar(childExecutorMode, "child-executor-mode", "", "javascript child executor mode: fake or live-provider")
	}
}
