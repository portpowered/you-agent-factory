package cli

import (
	"context"
	"fmt"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry/workflowmcp"
	defaultcmd "github.com/portpowered/infinite-you/pkg/transports/cli/default"
	mcpcli "github.com/portpowered/infinite-you/pkg/transports/cli/mcp"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	workflowcli "github.com/portpowered/infinite-you/pkg/transports/cli/workflow"
	"github.com/spf13/cobra"
)

func newWorkflowCommand(globals *cliGlobalOptions, _ *cliDiagnosticsOptions, options RootCommandOptions) *cobra.Command {
	return newWorkflowCommandWithValidationPreview(
		globals,
		options,
		newWorkflowValidateCommand(globals),
		newWorkflowPreviewCommand(globals),
	)
}

func newWorkflowCommandWithValidationPreview(
	globals *cliGlobalOptions,
	options RootCommandOptions,
	validate *cobra.Command,
	preview *cobra.Command,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Compatibility commands for Factory Preview and Factory Session behavior",
		Long: "Compatibility-only workflow spellings for canonical Factory Preview and Factory Session behavior. " +
			"New integrations should use POST /factories/preview, POST /factory-sessions/{sync|async}, " +
			"the /factory-sessions/{session_id} inspection routes, or the canonical you session commands where available.\n\n" +
			"Subcommands:\n" +
			"  validate   compatibility validation; successor: POST /factories/preview\n" +
			"  preview    compatibility preview; successor: POST /factories/preview\n" +
			"  run        compatibility sync start; successor: POST /factory-sessions/sync\n" +
			"  start      compatibility async start; successor: POST /factory-sessions/async\n" +
			"  status     compatibility read; successor: GET /factory-sessions/{session_id}\n" +
			"  result     compatibility result read; successor: GET /factory-sessions/{session_id}/results\n" +
			"  dispatches compatibility dispatch read; successor: you session dispatches or the session API\n" +
			"  artifacts  compatibility artifact read; successor: the Factory Session artifacts API\n" +
			"  events     compatibility event read; successor: the Factory Session events API",
	}
	cmd.AddCommand(
		validate,
		preview,
		newWorkflowRunCommand(globals, options),
		newWorkflowStartCommand(globals, options),
		newWorkflowStatusCommand(globals, options),
		newWorkflowResultCommand(globals, options),
		newWorkflowDispatchesCommand(globals, options),
		newWorkflowArtifactsCommand(globals, options),
		newWorkflowEventsCommand(globals, options),
	)
	return cmd
}

const useGeneratedWorkflowMCPFamily = true

type workflowMCPBindingState struct {
	mcpFixtureCatalogPath string
	mcpRuntimeBacked      bool
	mcpProjectRoot        string
	preview               workflowcli.PreviewConfig
	validate              workflowcli.ValidateConfig
}

func newWorkflowMCPBindingState() *workflowMCPBindingState {
	return &workflowMCPBindingState{
		preview:  workflowcli.PreviewConfig{SourceConfig: workflowcli.SourceConfig{Dir: defaultcmd.FactoryDir}},
		validate: workflowcli.ValidateConfig{SourceConfig: workflowcli.SourceConfig{Dir: defaultcmd.FactoryDir}},
	}
}

func newProductionWorkflowMCPCommands(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options RootCommandOptions,
) (*cobra.Command, *cobra.Command) {
	if !useGeneratedWorkflowMCPFamily {
		return newMCPCommand(options), newWorkflowCommand(globals, diagnostics, options)
	}
	state := newWorkflowMCPBindingState()
	registries, err := workflowMCPHandlerRegistries(globals, options, state)
	if err != nil {
		panic(fmt.Sprintf("build workflow/MCP handler registries: %v", err))
	}
	components, err := climanifestcobra.NewWorkflowMCPFamilyComponents(registries, workflowMCPFlagBindings(state))
	if err != nil {
		panic(fmt.Sprintf("build workflow/MCP family commands: %v", err))
	}
	workflow := newWorkflowCommandWithValidationPreview(globals, options, components.WorkflowValidate, components.WorkflowPreview)
	return components.MCP, workflow
}

// NewWorkflowMCPFamilyParityRoots builds detached handwritten and generated
// roots for observable constructor parity checks.
func NewWorkflowMCPFamilyParityRoots() (legacyRoot, generatedRoot *cobra.Command, err error) {
	options := normalizeRootCommandOptions(RootCommandOptions{})
	legacyGlobals := &cliGlobalOptions{server: "http://localhost:7437"}
	legacyDiagnostics := &cliDiagnosticsOptions{}
	legacyRoot = newLegacyRootCommandShell(legacyGlobals, legacyDiagnostics, &cliOperatorDefaultsOptions{}, options)
	legacyRoot.AddCommand(newMCPCommand(options), newWorkflowCommand(legacyGlobals, legacyDiagnostics, options))

	generatedGlobals := &cliGlobalOptions{server: "http://localhost:7437"}
	generatedDiagnostics := &cliDiagnosticsOptions{}
	generatedRoot = newLegacyRootCommandShell(generatedGlobals, generatedDiagnostics, &cliOperatorDefaultsOptions{}, options)
	state := newWorkflowMCPBindingState()
	registries, err := workflowMCPHandlerRegistries(generatedGlobals, options, state)
	if err != nil {
		return nil, nil, fmt.Errorf("build workflow/MCP parity registries: %w", err)
	}
	components, err := climanifestcobra.NewWorkflowMCPFamilyComponents(registries, workflowMCPFlagBindings(state))
	if err != nil {
		return nil, nil, fmt.Errorf("build workflow/MCP parity commands: %w", err)
	}
	workflow := newWorkflowCommandWithValidationPreview(generatedGlobals, options, components.WorkflowValidate, components.WorkflowPreview)
	generatedRoot.AddCommand(components.MCP, workflow)
	return legacyRoot, generatedRoot, nil
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
		RunE: workflowcli.ValidateRunE(&cfg, &globals.json),
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
		RunE: workflowcli.PreviewRunE(&cfg, &globals.json),
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

func newWorkflowMCPHandlerRegistries(
	globals *cliGlobalOptions,
	options RootCommandOptions,
) (workflowmcp.Registries, error) {
	return workflowMCPHandlerRegistries(globals, options, newWorkflowMCPBindingState())
}

func workflowMCPHandlerRegistries(
	globals *cliGlobalOptions,
	options RootCommandOptions,
	state *workflowMCPBindingState,
) (workflowmcp.Registries, error) {
	return workflowmcp.NewRegistries(workflowmcp.Handlers{
		MCPServe: mcpcli.ServeRunE(mcpcli.ServeBinding{
			FixtureCatalogPath: &state.mcpFixtureCatalogPath,
			RuntimeBacked:      &state.mcpRuntimeBacked,
			ProjectRoot:        &state.mcpProjectRoot,
			Startup:            options.Startup,
		}),
		WorkflowPreview:  workflowcli.PreviewRunE(&state.preview, &globals.json),
		WorkflowValidate: workflowcli.ValidateRunE(&state.validate, &globals.json),
	})
}

func workflowMCPFlagBindings(state *workflowMCPBindingState) climanifestcobra.WorkflowMCPFlagBindings {
	workflowUsages := map[string]string{
		"dir":              "project root used for ordered workflow source lookup",
		"kind":             "workflow source kind",
		"value":            "workflow name, file ref, or factory id",
		"inline":           "inline workflow source text",
		"artifact-root":    "optional absolute artifact root",
		"args-schema":      "optional orchestrator.javascript argsSchema JSON",
		"requested-policy": "optional requested policy override JSON",
	}
	sourceBindings := func(cfg *workflowcli.SourceConfig) climanifestcobra.WorkflowSourceFlagBindings {
		return climanifestcobra.WorkflowSourceFlagBindings{
			Dir: &cfg.Dir, SourceKind: &cfg.SourceKind, SourceValue: &cfg.SourceValue,
			InlineSource: &cfg.InlineSource, ArtifactRoot: &cfg.ArtifactRoot,
			ArgsSchema: &cfg.ArgsSchema, RequestedPolicyJSON: &cfg.RequestedPolicyJSON,
			FlagUsages: workflowUsages,
		}
	}
	return climanifestcobra.WorkflowMCPFlagBindings{
		MCPServe: climanifestcobra.MCPServeFlagBindings{
			FixtureCatalogPath: &state.mcpFixtureCatalogPath,
			RuntimeBacked:      &state.mcpRuntimeBacked,
			ProjectRoot:        &state.mcpProjectRoot,
			FlagUsages: map[string]string{
				"fixture-catalog": "optional path to durable-session contract fixtures; defaults to the catalog discovered from the current working directory",
				"runtime":         "select the shared durable JavaScript runtime execution service instead of the fixture catalog",
				"project-root":    "project root for workflow source resolution in --runtime mode; defaults to the current working directory",
			},
		},
		WorkflowPreview:  sourceBindings(&state.preview.SourceConfig),
		WorkflowValidate: sourceBindings(&state.validate.SourceConfig),
	}
}

func newWorkflowRunCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, runCfg.ExecutionBackendConfig, runCfg.FixtureCatalogPath, runCfg.StartConfig.ChildExecutorMode, func(service fse.Service) error {
				runCfg.Service = service
				runCfg.JSON = globals.json
				runCfg.Output = cmd.OutOrStdout()
				runCfg.StartConfig.PositionalArgs = args
				runCfg.StartConfig.Stdin = cmd.InOrStdin()
				if cmd.Flags().Changed("wait-timeout-millis") {
					runCfg.StartConfig.WaitTimeoutMillis = &waitTimeoutMillis
				}
				return sessionexecutioncli.RunSync(cmd.Context(), runCfg)
			})
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

func newWorkflowStartCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, startCfg.ExecutionBackendConfig, startCfg.FixtureCatalogPath, startCfg.StartConfig.ChildExecutorMode, func(service fse.Service) error {
				startCfg.Service = service
				startCfg.JSON = globals.json
				startCfg.Output = cmd.OutOrStdout()
				startCfg.StartConfig.PositionalArgs = args
				startCfg.StartConfig.Stdin = cmd.InOrStdin()
				return sessionexecutioncli.RunAsync(cmd.Context(), startCfg)
			})
		},
	}
	addWorkflowStartFlags(cmd, &startCfg.StartConfig, &startCfg.FixtureCatalogPath)
	addWorkflowExecutionBackendFlags(cmd, &startCfg.ExecutionBackendConfig, &startCfg.StartConfig.ChildExecutorMode)
	return cmd
}

func newWorkflowStatusCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, statusCfg.ExecutionBackendConfig, statusCfg.FixtureCatalogPath, "", func(service fse.Service) error {
				statusCfg.Service = service
				statusCfg.JSON = globals.json
				statusCfg.Output = cmd.OutOrStdout()
				statusCfg.SessionID = args[0]
				return sessionexecutioncli.RunStatus(cmd.Context(), statusCfg)
			})
		},
	}
	cmd.Flags().StringVar(&statusCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed status reads")
	addWorkflowExecutionBackendFlags(cmd, &statusCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowResultCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, resultCfg.ExecutionBackendConfig, resultCfg.FixtureCatalogPath, "", func(service fse.Service) error {
				resultCfg.Service = service
				resultCfg.JSON = globals.json
				resultCfg.Output = cmd.OutOrStdout()
				resultCfg.SessionID = args[0]
				return sessionexecutioncli.RunResult(cmd.Context(), resultCfg)
			})
		},
	}
	cmd.Flags().StringVar(&resultCfg.Mode, "mode", "", "result read mode: final or partial")
	cmd.Flags().BoolVar(&resultCfg.IncludeArtifacts, "include-artifacts", false, "include artifact refs in the result read")
	cmd.Flags().StringVar(&resultCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed result reads")
	addWorkflowExecutionBackendFlags(cmd, &resultCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowDispatchesCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, dispatchesCfg.ExecutionBackendConfig, dispatchesCfg.FixtureCatalogPath, "", func(service fse.Service) error {
				dispatchesCfg.Service = service
				dispatchesCfg.JSON = globals.json
				dispatchesCfg.Output = cmd.OutOrStdout()
				dispatchesCfg.SessionID = args[0]
				return sessionexecutioncli.RunDispatches(cmd.Context(), dispatchesCfg)
			})
		},
	}
	cmd.Flags().StringVar(&dispatchesCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed dispatch reads")
	cmd.Flags().StringVar(&dispatchesCfg.Phase, "phase", "", "filter by exact Dispatch phase")
	cmd.Flags().StringVar(&dispatchesCfg.Status, "status", "", "filter by canonical Dispatch status")
	addWorkflowExecutionBackendFlags(cmd, &dispatchesCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowArtifactsCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, artifactsCfg.ExecutionBackendConfig, artifactsCfg.FixtureCatalogPath, "", func(service fse.Service) error {
				artifactsCfg.Service = service
				artifactsCfg.JSON = globals.json
				artifactsCfg.Output = cmd.OutOrStdout()
				artifactsCfg.SessionID = args[0]
				return sessionexecutioncli.RunArtifacts(cmd.Context(), artifactsCfg)
			})
		},
	}
	cmd.Flags().StringVar(&artifactsCfg.FixtureCatalogPath, "fixture-catalog", "", "path to durable session contract fixtures for mock-backed artifact reads")
	addWorkflowExecutionBackendFlags(cmd, &artifactsCfg.ExecutionBackendConfig, nil)
	return cmd
}

func newWorkflowEventsCommand(globals *cliGlobalOptions, options RootCommandOptions) *cobra.Command {
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
			return withWorkflowExecutionService(cmd.Context(), options, eventsCfg.ExecutionBackendConfig, eventsCfg.FixtureCatalogPath, "", func(service fse.Service) error {
				eventsCfg.Service = service
				eventsCfg.JSON = globals.json
				eventsCfg.Output = cmd.OutOrStdout()
				eventsCfg.SessionID = args[0]
				if cmd.Flags().Changed("after-sequence") {
					eventsCfg.AfterSequence = &afterSequence
				}
				return sessionexecutioncli.RunEvents(cmd.Context(), eventsCfg)
			})
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

func newSessionCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options RootCommandOptions) *cobra.Command {
	sessionCmd := legacySessionParentCommand()
	sessionCmd.AddCommand(handwrittenSessionSubcommands(globals, diagnostics, options, newSessionShowCommand(globals, diagnostics))...)
	return sessionCmd
}

// NewLegacySessionFamilyCommand builds the isolated handwritten session tree
// retained as the generated-family parity and rollback reference.
func NewLegacySessionFamilyCommand(options RootCommandOptions) *cobra.Command {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	root.AddCommand(newSessionCommand(globals, diagnostics, options))
	return root
}

// NewGeneratedSessionFamilyCommand builds an isolated root/session tree from
// generated metadata with all execution paths bound to handwritten handlers.
// Production uses the same constructor through its session-local cutover seam.
func NewGeneratedSessionFamilyCommand(options RootCommandOptions) (*cobra.Command, error) {
	options = normalizeRootCommandOptions(options)
	globals := &cliGlobalOptions{server: cliserver.DefaultBaseURI}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	root := newLegacyRootCommandShell(globals, diagnostics, operatorDefaults, options)
	registry, bindings, err := newSessionHandlerRegistry(globals, diagnostics, options)
	if err != nil {
		return nil, err
	}
	session, err := climanifestcobra.NewSessionFamilyCommand(registry, bindings)
	if err != nil {
		return nil, err
	}
	root.AddCommand(session)
	return root, nil
}

func newSessionHandlerRegistry(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options RootCommandOptions,
) (*commandregistry.Registry, climanifestcobra.SessionFamilyBindings, error) {
	configs := newSessionFamilyBindings()
	diagnosticsBinding := commandregistry.SessionDiagnosticsBinding{
		Verbose: diagnostics.verboseEnabled, Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer,
	}
	registry, err := commandregistry.NewSessionRegistry(commandregistry.SessionHandlers{
		CreateRunE: commandregistry.SessionCreateRunE(commandregistry.SessionCreateBinding{
			Config: configs.Create, SessionDiagnosticsBinding: diagnosticsBinding, CreateSession: createSession,
		}),
		ListRunE: commandregistry.SessionListRunE(commandregistry.SessionListBinding{
			Config: configs.List, SessionDiagnosticsBinding: diagnosticsBinding,
			Server: &globals.server, Prepare: sessionListPrepare(options), ListSessions: listSessions,
		}),
		ShowRunE: commandregistry.SessionShowRunE(commandregistry.SessionShowBinding{
			Server: &globals.server, JSON: &globals.json, Verbose: diagnostics.verboseEnabled,
			Debug: &diagnostics.debug, DiagnosticsWriter: diagnostics.writer, ShowSession: showSession,
		}),
		DeleteRunE: commandregistry.SessionDeleteRunE(commandregistry.SessionDeleteBinding{
			Config: configs.Delete, SessionDiagnosticsBinding: diagnosticsBinding, DeleteSession: deleteSession,
		}),
		DispatchesRunE: commandregistry.SessionDispatchesRunE(commandregistry.SessionDispatchesBinding{
			Config: configs.Dispatches, Server: &globals.server, JSON: &globals.json,
			SessionDiagnosticsBinding: diagnosticsBinding, ListDispatches: listSessionDispatches,
		}),
		PauseRunE:  sessionLifecycleRunE(configs.Pause, globals, diagnosticsBinding, pauseSession),
		ResumeRunE: sessionLifecycleRunE(configs.Resume, globals, diagnosticsBinding, resumeSession),
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

func sessionListPrepare(options RootCommandOptions) func(context.Context, *sessioncli.ListConfig) error {
	return func(ctx context.Context, cfg *sessioncli.ListConfig) error {
		scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
		if scope != fse.SessionListScopePersisted && scope != fse.SessionListScopeAll {
			return nil
		}
		service, err := buildWorkflowExecutionService(ctx, options, sessionexecutioncli.ExecutionBackendConfig{Provider: string(fse.ExecutionProviderFake)}, "", "")
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
		"you.session.create.json":             "emit the API open-factory-session JSON response",
		"you.session.list.scope":              "session list scope: live, persisted, or all",
		"you.session.list.json":               "emit the API list-factory-sessions JSON response",
		"you.session.delete.json":             "emit a JSON confirmation after the session closes",
		"you.session.dispatches.phase":        "filter by exact Dispatch phase",
		"you.session.dispatches.status":       "filter by canonical Dispatch status",
		"you.session.show.port":               "deprecated; use --server",
		"you.session.dispatches.port":         "deprecated; use --server",
		"you.session.pause.port":              "deprecated; use --server",
		"you.session.resume.port":             "deprecated; use --server",
		"port":                                "HTTP server port",
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

func newSessionDispatchesCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions) *cobra.Command {
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
			cfg.SessionID = args[0]
			cfg.Server = globals.server
			cfg.JSON = globals.json
			cfg.Output = cmd.OutOrStdout()
			cfg.Diagnostics = diagnostics.writer(cmd)
			cfg.Verbose = diagnostics.verboseEnabled()
			cfg.Debug = diagnostics.debug
			return listSessionDispatches(cfg)
		},
	}

	registerDeprecatedPortFlag(cmd)
	cmd.Flags().StringVar(&cfg.Phase, "phase", "", "filter by exact Dispatch phase")
	cmd.Flags().StringVar(&cfg.Status, "status", "", "filter by canonical Dispatch status")
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

func newSessionListCommand(globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options RootCommandOptions) *cobra.Command {
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
			scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
			if scope == fse.SessionListScopePersisted || scope == fse.SessionListScopeAll {
				service, err := buildWorkflowExecutionService(cmd.Context(), options, sessionexecutioncli.ExecutionBackendConfig{Provider: string(fse.ExecutionProviderFake)}, "", "")
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
			return sessioncli.List(cfg)
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
