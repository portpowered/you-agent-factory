package cli

import (
	"context"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	"github.com/spf13/cobra"
)

func newSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options CommandFactory) *cobra.Command {
	sessionCmd := legacySessionParentCommand()
	sessionCmd.AddCommand(handwrittenSessionSubcommands(globals, diagnostics, options, newSessionShowCommand(globals, diagnostics, options))...)
	return sessionCmd
}

func newSessionHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) (*commandregistry.Registry, climanifestcobra.SessionFamilyBindings, error) {
	configs := newSessionFamilyBindings()
	diagnosticsBinding := commandregistry.SessionDiagnosticsBinding{
		Verbose: diagnostics.verboseEnabled, Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer,
	}
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: commandregistry.SessionCreateRunE(commandregistry.SessionCreateBinding{
			Config: configs.Create, SessionDiagnosticsBinding: diagnosticsBinding, CreateSession: options.CreateSession,
		}),
		ListRunE: commandregistry.SessionListRunE(commandregistry.SessionListBinding{
			Config: configs.List, SessionDiagnosticsBinding: diagnosticsBinding,
			Server: &globals.server, Prepare: sessionListPrepare(options), ListSessions: options.ListSessions,
		}),
		ShowRunE: commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
			Server: &globals.server, JSON: &globals.json, Verbose: diagnostics.verboseEnabled,
			Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer, ShowSession: options.ShowSession,
		}),
		DeleteRunE: commandregistry.SessionDeleteRunE(commandregistry.SessionDeleteBinding{
			Config: configs.Delete, SessionDiagnosticsBinding: diagnosticsBinding, DeleteSession: options.DeleteSession,
		}),
		DispatchesRunE: commandregistry.SessionDispatchesRunE(commandregistry.SessionDispatchesBinding{
			Config: configs.Dispatches, Server: &globals.server, JSON: &globals.json,
			SessionDiagnosticsBinding: diagnosticsBinding, ListDispatches: options.ListSessionDispatches,
		}),
		PauseRunE:  sessionLifecycleRunE(configs.Pause, globals, diagnosticsBinding, options.PauseSession),
		ResumeRunE: sessionLifecycleRunE(configs.Resume, globals, diagnosticsBinding, options.ResumeSession),
	})
	return registry, configs, err
}

func sessionLifecycleRunE(
	config *sessioncli.LifecycleControlConfig,
	globals *cliGlobalOptions,
	diagnostics commandregistry.SessionDiagnosticsBinding,
	control func(sessioncli.LifecycleControlConfig) error,
) commandregistry.RunE {
	return commandregistry.SessionLifecycleRunE(commandregistry.SessionLifecycleBinding{
		Config: config, Server: &globals.server, JSON: &globals.json,
		SessionDiagnosticsBinding: diagnostics, Control: control,
	})
}

func newSessionFamilyBindings() climanifestcobra.SessionFamilyBindings {
	return climanifestcobra.SessionFamilyBindings{
		Create:     &sessioncli.CreateConfig{Port: defaultcmd.FactoryPort},
		List:       &sessioncli.ListConfig{Port: defaultcmd.FactoryPort, Scope: "live"},
		Delete:     &sessioncli.DeleteConfig{Port: defaultcmd.FactoryPort},
		Dispatches: &sessioncli.DispatchesConfig{},
		Pause:      &sessioncli.LifecycleControlConfig{},
		Resume:     &sessioncli.LifecycleControlConfig{},
		FlagUsages: sessionFamilyFlagUsages(),
	}
}

func sessionListPrepare(options CommandFactory) func(context.Context, *sessioncli.ListConfig) error {
	return func(ctx context.Context, cfg *sessioncli.ListConfig) error {
		scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
		if scope != fse.SessionListScopePersisted && scope != fse.SessionListScopeAll {
			return nil
		}
		service, err := buildWorkflowExecutionService(ctx, options, string(fse.ExecutionProviderFake), "", "", "")
		if err != nil {
			return err
		}
		cfg.DurableLister = service.ListSessions
		cfg.DurableCloser = service
		return nil
	}
}

func sessionFamilyFlagUsages() map[string]string {
	return map[string]string{
		"you.session.create.dir":              "folder path to open as a live factory session",
		"you.session.create.init-new-factory": "write the default init scaffold at --dir and open a live session",
		"you.session.create.validate-only":    "validate the folder and optional target without creating a live session",
		"you.session.create.target-kind":      "target kind when disambiguating runnable factories (default or named)",
		"you.session.create.target-name":      "named target when --target-kind is named",

		"you.session.create.json":       "emit the API open-factory-session JSON response",
		"you.session.list.scope":        "session list scope: live, persisted, or all",
		"you.session.list.json":         "emit the API list-factory-sessions JSON response",
		"you.session.delete.json":       "emit a JSON confirmation after the session closes",
		"you.session.dispatches.phase":  "filter by exact Dispatch phase",
		"you.session.dispatches.status": "filter by canonical Dispatch status",
		"you.session.show.port":         "deprecated; use --server",
		"you.session.dispatches.port":   "deprecated; use --server",
		"you.session.pause.port":        "deprecated; use --server",
		"you.session.resume.port":       "deprecated; use --server",
		"port":                          "HTTP server port",
	}
}

func legacySessionParentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "session",
		Short: "List, open, and close factory sessions on a running host",
		Long: "Manage factory sessions on a running you-agent-factory service.\n\n" +
			"Subcommands:\n" +
			"  list       list live workspace sessions or durable Factory Sessions with --scope live|persisted|all\n" +
			"  show       show one live or durable Factory Session from GET /factory-sessions/{session_id}\n" +
			"  dispatches list durable Factory Session dispatches from GET /factory-sessions/{session_id}/dispatches\n" +
			"  pause      pause one live or durable Factory Session through POST /factory-sessions/{session_id}/pause\n" +
			"  resume     resume one paused live or durable Factory Session through POST /factory-sessions/{session_id}/resume\n" +
			"  create     open another live session from a folder path\n" +
			"  delete     close a live session by session id\n\n" +
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
}

func newSessionShowCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.ShowSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionDispatchesCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
	cfg := sessioncli.DispatchesConfig{Server: globals.server}

	cmd := &cobra.Command{
		Use:   "dispatches [session-id]",
		Short: "List durable Factory Session dispatches",
		Long: "List durable Factory Session dispatches from GET /factory-sessions/{session_id}/dispatches.\n\n" +
			"Human output uses FactorySession, Dispatch, and FactoryArtifact vocabulary for dispatch id, status, " +
			"kind, label, and output artifact ids. The command requires a durable dur-sess-* session id. Use global " +
			"--json for the API-shaped ListFactorySessionDispatchesResponse and global --server to target the same " +
			"factory API base URI as session show and session resume.",
		Example: "  # List dispatches for one interrupted durable Factory Session.\n" +
			"  " + cliBinaryName + " session dispatches dur-sess-js-interrupted-001\n\n" +
			"  # Emit API-shaped JSON for automation.\n" +
			"  " + cliBinaryName + " --json session dispatches dur-sess-js-interrupted-001",
		Args:    cobra.ExactArgs(1),
		PreRunE: rejectDeprecatedPortFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Context = cmd.Context()
			cfg.SessionID = args[0]
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.ListSessionDispatches(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.Phase, "phase", "", "filter by exact Dispatch phase")
	cmd.Flags().StringVar(&cfg.Status, "status", "", "filter by canonical Dispatch status")
	return cmd
}

func newSessionPauseCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.PauseSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionResumeCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			if len(args) == 1 {
				cfg.SessionID = args[0]
			}
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return dependencies.ResumeSession(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	return cmd
}

func newSessionListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options CommandFactory) *cobra.Command {
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
			cfg.Context = cmd.Context()
			scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
			if scope == fse.SessionListScopePersisted || scope == fse.SessionListScopeAll {
				service, err := buildWorkflowExecutionService(cmd.Context(), options, string(fse.ExecutionProviderFake), "", "", "")
				if err != nil {
					return err
				}
				cfg.DurableLister = service.ListSessions
				cfg.DurableCloser = service
			}
			if cmd.Root().PersistentFlags().Changed("server") {
				cfg.Server = globals.server
			}
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)

			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return options.ListSessions(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().StringVar(&cfg.Scope, "scope", cfg.Scope, "session list scope: live, persisted, or all")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit the API list-factory-sessions JSON response")
	return cmd
}

func newSessionCreateCommand(diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			return dependencies.CreateSession(cfg)
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

func newSessionDeleteCommand(diagnostics *cliDiagnosticsOptions, dependencies CommandFactory) *cobra.Command {
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
			return dependencies.DeleteSession(cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.Port, "port", cfg.Port, "HTTP server port")
	cmd.Flags().BoolVar(&cfg.JSON, "json", false, "emit a JSON confirmation after the session closes")
	return cmd
}
