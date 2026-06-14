package cli

import (
	defaultcmd "github.com/portpowered/infinite-you/pkg/cli/default"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	workflowcli "github.com/portpowered/infinite-you/pkg/cli/workflow"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/spf13/cobra"
)

func newWorkflowCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Validate, preview, and run JavaScript workflow sources",
		Long: "Validate, preview, and run JavaScript or TypeScript workflow sources using the shared workflow contracts.\n\n" +
			"Subcommands:\n" +
			"  preview    resolve workflow source, validate it without execution, and project policy and result constraints\n" +
			"  run        start one durable Factory Session synchronously through the mock-backed execution loop\n" +
			"  start      start one durable Factory Session asynchronously and return inspection links\n" +
			"  status     read the durable Factory Session lifecycle and progress state\n" +
			"  result     read the durable Factory Session final or partial result\n" +
			"  dispatches list durable Factory Session dispatches for inspection\n" +
			"  artifacts  list durable Factory Session artifacts for inspection\n" +
			"  events     poll ordered durable Factory Session events with optional reconnect cursors",
	}
	cmd.AddCommand(
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

func newWorkflowPreviewCommand(globals *cliGlobalOptions) *cobra.Command {
	cfg := workflowcli.PreviewConfig{Dir: defaultcmd.FactoryDir}
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Preview workflow validation and policy",
		Long:  "Resolve workflow source, validate it without execution, and print source, loader, policy, and result-shape diagnostics.",
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
	cmd.Flags().StringVar(&cfg.Dir, "dir", cfg.Dir, "project root used for ordered workflow source lookup")
	cmd.Flags().StringVar(&cfg.SourceKind, "kind", string(workflowsource.KindWorkflowName), "workflow source kind")
	cmd.Flags().StringVar(&cfg.SourceValue, "value", "", "workflow name, file ref, or factory id")
	cmd.Flags().StringVar(&cfg.InlineSource, "inline", "", "inline workflow source text")
	cmd.Flags().StringVar(&cfg.ArtifactRoot, "artifact-root", "", "optional absolute artifact root")
	return cmd
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
