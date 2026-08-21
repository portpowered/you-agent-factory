// Package cli defines Cobra commands for the agent-factory CLI.
// Commands contain only flag parsing and delegate to command-specific packages.
// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/cobracompletion"
	configcli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/config"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli/factoryload"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli/initsetup"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	submitcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli/worker_sessions"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	mcpcli "github.com/portpowered/infinite-you/pkg/transports/cli/mcp"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"github.com/spf13/cobra"
)

const (
	defaultMockWorkersConfigPathSentinel = "__agent_factory_default_mock_workers_config__"
	runListenInputID                     = "you.run.flag.listen"
	serverListenInputID                  = "you.server.flag.listen"
)

const cliBinaryName = "you"

type cliGlobalOptions struct {
	server    string
	json      bool
	remote    bool
	placement climanifest.ExecutionPlacement
}

type cliOperatorDefaultsOptions struct {
	providerOverride string
	modelOverride    string
}

type SubmitWorkOperation func(submitcli.SubmitConfig) error
type SubmitBatchOperation func(submitcli.BatchConfig) error

// LocalSessionsCLIService is the distinct Wire binding for local lifecycle
// controls. It embeds the CLI service contract while keeping the local and
// remote adapters distinguishable in the inert command-operation graph.
type LocalSessionsCLIService interface {
	sessioncli.Service
}

// OwnedExecutionService adds execution-local cleanup to the Factory
// Sessions-owned durable execution and scoped inventory capabilities. The CLI
// builder owns this cleanup boundary, while the Factory Sessions root remains
// the sole owner of durable operations and session inventory.
type OwnedExecutionService interface {
	factorysessions.DurableExecutionService
	factorysessions.SessionInventoryService
	Close() error
}
type ExecutionServiceBuilder func(context.Context, string, string, string, string) (OwnedExecutionService, error)
type FlattenFactoryConfigOperation func(configcli.FactoryConfigFlattenConfig) error
type ExpandFactoryConfigOperation func(configcli.FactoryConfigExpandConfig) error
type ConfigureInitOperation func(initsetup.Config) error
type InstallPackagedFactoryOperation func(factorydefinitionscli.InstallPackagedFactoryConfig) error
type QueryFactoryOperation func(factorycli.QueryConfig) error
type ListFactoriesOperation func(factorycli.ListConfig) error
type ValidateFactoryOperation func(factorycli.ValidateConfig) error
type CreateFactoryFromFileOperation func(factorycli.CreateFromFileConfig) error
type ReplaceFactoryCurrentOperation func(factorycli.ReplaceCurrentConfig) error
type UpdateFactoryFromFileOperation func(factorycli.UpdateFromFileConfig) error
type DeleteFactoryOperation func(factorycli.DeleteConfig) error
type ListWorkOperation func(workcli.ListConfig) error
type WatchWorkOperation func(workcli.WatchConfig) error
type ShowWorkOperation func(workcli.ShowConfig) error
type MoveWorkOperation func(workcli.MoveConfig) error
type VisualizeWorkOperation func(workcli.VisualizeConfig) error
type ListHumanApprovalsOperation func(workcli.ListHumanApprovalsConfig) error
type ShowHumanApprovalOperation func(workcli.ShowHumanApprovalConfig) error

type ListWorkerSessionsOperation = workersessionscli.ListOperation
type ShowWorkerSessionsOperation = workersessionscli.ShowOperation
type ReadWorkerSessionOperation = workersessionscli.ReadOperation
type StreamWorkerSessionOperation = workersessionscli.StreamOperation
type InvokeWorkerSessionOperation = workersessionscli.InvokeOperation
type ContinueWorkerSessionOperation = workersessionscli.ContinueOperation
type InterruptWorkerSessionOperation = workersessionscli.InterruptOperation
type PauseWorkerSessionOperation func(workersessionscli.ControlConfig) error
type ResumeWorkerSessionOperation func(workersessionscli.ControlConfig) error
type CancelWorkerSessionOperation func(workersessionscli.ControlConfig) error
type TerminateWorkerSessionOperation func(workersessionscli.ControlConfig) error
type NamedFactoryRootsResolver func(homeDir, workingDir string) (interfaces.NamedFactoryRoots, error)

// CommandOperations is the complete inert CLI operation graph assembled by Wire.
type CommandOperations struct {
	ObserveCLI                        platformprocess.CLIObserver
	NamedFactoryCatalog               interfaces.NamedFactoryCatalog
	CompleteFactoryNames              cobracompletion.FactoryNamesOperation
	CompletePackagedFactoryNames      cobracompletion.PackagedFactoryNamesOperation
	CompleteSelectedFactorySignature  cobracompletion.SelectedFactorySignatureOperation
	ResolveNamedFactoryRoots          NamedFactoryRootsResolver
	ResolveNamedFactoryCandidatePaths interfaces.NamedFactoryCandidatePathsResolver
	ResolveCurrentFactoryDir          interfaces.CurrentFactoryDirectoryResolver
	ResolveFactoryConfigRoot          interfaces.FactoryConfigRootResolver
	LoadFactoryConfigFile             interfaces.FactoryConfigFileLoader
	WorkRequestFileLoader             work.RequestFileLoader
	PrepareSingleWorkTarget           work.SingleWorkTargetPreparation
	PrepareInvocationInput            work.InvocationInputPreparation
	BuildTerminalLogger               terminalpolicy.LoggerBuilder
	RunDefaults                       runcli.RunConfig
	BatchInputFileSystem              submitcli.BatchInputFileSystem
	RunDirectoryCreator               platformfilesystem.DirectoryCreator
	BrowserOpener                     platformbrowser.Opener
	ResolveOperatorDefaults           operatorconfig.DefaultsResolver
	LoadOperatorConfig                operatorconfig.ConfigLoader
	BuildExecution                    ExecutionServiceBuilder
	ModelsCLI                         modelscli.Service
	ProvidersCLI                      providerscli.Service
	SessionsCLI                       sessioncli.Service
	LocalSessionsCLI                  LocalSessionsCLIService
	SubmitWork                        SubmitWorkOperation
	SubmitBatch                       SubmitBatchOperation
	FlattenFactoryConfig              FlattenFactoryConfigOperation
	ExpandFactoryConfig               ExpandFactoryConfigOperation
	InitFactory                       interfaces.ScaffoldInitializer
	ConfigureInit                     ConfigureInitOperation
	InstallPackagedFactory            InstallPackagedFactoryOperation
	QueryFactory                      QueryFactoryOperation
	ListFactories                     ListFactoriesOperation
	ValidateFactory                   ValidateFactoryOperation
	CreateFactoryFromFile             CreateFactoryFromFileOperation
	ReplaceFactoryCurrent             ReplaceFactoryCurrentOperation
	UpdateFactoryFromFile             UpdateFactoryFromFileOperation
	DeleteFactory                     DeleteFactoryOperation
	ListWork                          ListWorkOperation
	ListHumanApprovals                ListHumanApprovalsOperation
	ShowHumanApproval                 ShowHumanApprovalOperation
	WatchWork                         WatchWorkOperation
	ShowWork                          ShowWorkOperation
	MoveWork                          MoveWorkOperation
	VisualizeWork                     VisualizeWorkOperation
	ListWorkerSessions                ListWorkerSessionsOperation
	ShowWorkerSession                 ShowWorkerSessionsOperation
	ReadWorkerSession                 ReadWorkerSessionOperation
	StreamWorkerSession               StreamWorkerSessionOperation
	InvokeWorkerSession               InvokeWorkerSessionOperation
	ContinueWorkerSession             ContinueWorkerSessionOperation
	InterruptWorkerSession            InterruptWorkerSessionOperation
	PauseWorkerSession                PauseWorkerSessionOperation
	ResumeWorkerSession               ResumeWorkerSessionOperation
	CancelWorkerSession               CancelWorkerSessionOperation
	TerminateWorkerSession            TerminateWorkerSessionOperation
	LocalWorkerSessions               workersessionscli.LocalInvokeBoundary
	LocalWorkerSessionControls        workersessionscli.LocalControlBoundary
	OpenRunSelection                  runcli.SelectionFactory
	RemoteInvocation                  runcli.RemoteInvocationOperation
	ResponsePresentation              factoryvisualization.ResponsePresentation
	ACP                               acpcli.Operations
	ACPServer                         acp.Server
	RuntimeMetricsQuery               factoryvisualization.RuntimeMetricsQuery
}

// CommandFactory constructs a fresh Cobra tree for each invocation from
// immutable Wire-supplied entrypoints and invocation-local process edges.
type CommandFactory struct {
	observeCLI                        platformprocess.CLIObserver
	homeDir                           func() (string, error)
	lookupEnv                         func(string) (string, bool)
	initializer                       startupcli.Initializer
	namedFactoryCatalog               interfaces.NamedFactoryCatalog
	completeFactoryNames              cobracompletion.FactoryNamesOperation
	completePackagedFactoryNames      cobracompletion.PackagedFactoryNamesOperation
	completeSelectedFactorySignature  cobracompletion.SelectedFactorySignatureOperation
	resolveNamedFactoryRoots          NamedFactoryRootsResolver
	resolveNamedFactoryCandidatePaths interfaces.NamedFactoryCandidatePathsResolver
	resolveCurrentFactoryDir          interfaces.CurrentFactoryDirectoryResolver
	resolveFactoryConfigRoot          interfaces.FactoryConfigRootResolver
	loadFactoryConfigFile             interfaces.FactoryConfigFileLoader
	workRequestFileLoader             work.RequestFileLoader
	prepareSingleWorkTarget           work.SingleWorkTargetPreparation
	prepareInvocationInput            work.InvocationInputPreparation
	buildTerminalLogger               terminalpolicy.LoggerBuilder
	runDefaults                       runcli.RunConfig
	batchInputFileSystem              submitcli.BatchInputFileSystem
	runDirectoryCreator               platformfilesystem.DirectoryCreator
	browserOpener                     platformbrowser.Opener
	resolveOperatorDefaults           operatorconfig.DefaultsResolver
	loadOperatorConfig                operatorconfig.ConfigLoader

	SubmitWork                 func(submitcli.SubmitConfig) error
	SubmitBatch                func(submitcli.BatchConfig) error
	SessionsCLI                sessioncli.Service
	LocalSessionsCLI           sessioncli.Service
	BuildExecution             ExecutionServiceBuilder
	ModelsCLI                  modelscli.Service
	ProvidersCLI               providerscli.Service
	FlattenFactoryConfig       func(configcli.FactoryConfigFlattenConfig) error
	ExpandFactoryConfig        func(configcli.FactoryConfigExpandConfig) error
	InitFactory                interfaces.ScaffoldInitializer
	ConfigureInit              func(initsetup.Config) error
	InstallPackagedFactory     func(factorydefinitionscli.InstallPackagedFactoryConfig) error
	QueryFactory               func(factorycli.QueryConfig) error
	ListFactories              func(factorycli.ListConfig) error
	ValidateFactory            func(factorycli.ValidateConfig) error
	CreateFactoryFromFile      func(factorycli.CreateFromFileConfig) error
	ReplaceFactoryCurrent      func(factorycli.ReplaceCurrentConfig) error
	UpdateFactoryFromFile      func(factorycli.UpdateFromFileConfig) error
	DeleteFactory              func(factorycli.DeleteConfig) error
	ListWork                   func(workcli.ListConfig) error
	ListHumanApprovals         func(workcli.ListHumanApprovalsConfig) error
	ShowHumanApproval          func(workcli.ShowHumanApprovalConfig) error
	WatchWork                  func(workcli.WatchConfig) error
	ShowWork                   func(workcli.ShowConfig) error
	MoveWork                   func(workcli.MoveConfig) error
	VisualizeWork              func(workcli.VisualizeConfig) error
	ListWorkerSessions         workersessionscli.ListOperation
	ShowWorkerSession          workersessionscli.ShowOperation
	ReadWorkerSession          workersessionscli.ReadOperation
	StreamWorkerSession        workersessionscli.StreamOperation
	InvokeWorkerSession        workersessionscli.InvokeOperation
	ContinueWorkerSession      workersessionscli.ContinueOperation
	InterruptWorkerSession     workersessionscli.InterruptOperation
	PauseWorkerSession         PauseWorkerSessionOperation
	ResumeWorkerSession        ResumeWorkerSessionOperation
	CancelWorkerSession        CancelWorkerSessionOperation
	TerminateWorkerSession     TerminateWorkerSessionOperation
	LocalWorkerSessions        workersessionscli.LocalInvokeBoundary
	LocalWorkerSessionControls workersessionscli.LocalControlBoundary
	openRunSelection           runcli.SelectionFactory
	remoteInvocation           runcli.RemoteInvocationOperation
	responsePresentation       factoryvisualization.ResponsePresentation
	acp                        acpcli.Operations
	acpServer                  acp.Server
	runtimeMetricsQuery        factoryvisualization.RuntimeMetricsQuery
}

// NewCommandFactory copies the Wire-built graph without installing defaults.
func NewCommandFactory(operations CommandOperations) CommandFactory {
	return CommandFactory{
		observeCLI:                        operations.ObserveCLI,
		namedFactoryCatalog:               operations.NamedFactoryCatalog,
		completeFactoryNames:              operations.CompleteFactoryNames,
		completePackagedFactoryNames:      operations.CompletePackagedFactoryNames,
		completeSelectedFactorySignature:  operations.CompleteSelectedFactorySignature,
		resolveNamedFactoryRoots:          operations.ResolveNamedFactoryRoots,
		resolveNamedFactoryCandidatePaths: operations.ResolveNamedFactoryCandidatePaths,
		resolveCurrentFactoryDir:          operations.ResolveCurrentFactoryDir,
		resolveFactoryConfigRoot:          operations.ResolveFactoryConfigRoot,
		loadFactoryConfigFile:             operations.LoadFactoryConfigFile,
		workRequestFileLoader:             operations.WorkRequestFileLoader,
		prepareSingleWorkTarget:           operations.PrepareSingleWorkTarget,
		prepareInvocationInput:            operations.PrepareInvocationInput,
		buildTerminalLogger:               operations.BuildTerminalLogger,
		runDefaults:                       operations.RunDefaults,
		batchInputFileSystem:              operations.BatchInputFileSystem,
		runDirectoryCreator:               operations.RunDirectoryCreator,
		browserOpener:                     operations.BrowserOpener,
		resolveOperatorDefaults:           operations.ResolveOperatorDefaults,
		loadOperatorConfig:                operations.LoadOperatorConfig,
		SubmitWork:                        operations.SubmitWork,
		SubmitBatch:                       operations.SubmitBatch,
		SessionsCLI:                       operations.SessionsCLI,
		LocalSessionsCLI:                  operations.LocalSessionsCLI,
		BuildExecution:                    operations.BuildExecution,
		ModelsCLI:                         operations.ModelsCLI,
		ProvidersCLI:                      operations.ProvidersCLI,
		FlattenFactoryConfig:              operations.FlattenFactoryConfig,
		ExpandFactoryConfig:               operations.ExpandFactoryConfig,
		InitFactory:                       operations.InitFactory,
		ConfigureInit:                     operations.ConfigureInit,
		InstallPackagedFactory:            operations.InstallPackagedFactory,
		QueryFactory:                      operations.QueryFactory,
		ListFactories:                     operations.ListFactories,
		ValidateFactory:                   operations.ValidateFactory,
		CreateFactoryFromFile:             operations.CreateFactoryFromFile,
		ReplaceFactoryCurrent:             operations.ReplaceFactoryCurrent,
		UpdateFactoryFromFile:             operations.UpdateFactoryFromFile,
		DeleteFactory:                     operations.DeleteFactory,
		ListWork:                          operations.ListWork,
		ListHumanApprovals:                operations.ListHumanApprovals,
		ShowHumanApproval:                 operations.ShowHumanApproval,
		WatchWork:                         operations.WatchWork,
		ShowWork:                          operations.ShowWork,
		MoveWork:                          operations.MoveWork,
		VisualizeWork:                     operations.VisualizeWork,
		ListWorkerSessions:                operations.ListWorkerSessions,
		ShowWorkerSession:                 operations.ShowWorkerSession,
		ReadWorkerSession:                 operations.ReadWorkerSession,
		StreamWorkerSession:               operations.StreamWorkerSession,
		InvokeWorkerSession:               operations.InvokeWorkerSession,
		ContinueWorkerSession:             operations.ContinueWorkerSession,
		InterruptWorkerSession:            operations.InterruptWorkerSession,
		PauseWorkerSession:                operations.PauseWorkerSession,
		ResumeWorkerSession:               operations.ResumeWorkerSession,
		CancelWorkerSession:               operations.CancelWorkerSession,
		TerminateWorkerSession:            operations.TerminateWorkerSession,
		LocalWorkerSessions:               operations.LocalWorkerSessions,
		LocalWorkerSessionControls:        operations.LocalWorkerSessionControls,
		openRunSelection:                  operations.OpenRunSelection,
		remoteInvocation:                  operations.RemoteInvocation,
		responsePresentation:              operations.ResponsePresentation,
		acp:                               operations.ACP,
		acpServer:                         operations.ACPServer,
		runtimeMetricsQuery:               operations.RuntimeMetricsQuery,
	}
}

// NewCommand constructs one fresh command tree from invocation-local process
// boundaries and the factory's injected command collaborators.
func (factory CommandFactory) NewCommand(
	homeDir func() (string, error),
	lookupEnv func(string) (string, bool),
	initializer startupcli.Initializer,
) *cobra.Command {
	factory.homeDir = homeDir
	factory.lookupEnv = lookupEnv
	factory.initializer = initializer
	return newRootCommandWithFactory(factory)
}

// ExecuteCommand constructs one private tree for the invocation. Observation
// callers receive detached contracts and parser state; ordinary callers run
// the selected command handler.
func (factory CommandFactory) ExecuteCommand(input startupcli.CommandInvocation) error {
	factory.homeDir = input.HomeDir
	factory.lookupEnv = input.LookupEnv
	factory.initializer = input.Initializer
	diagnostics := clidiag.NewDiagnosticWriter(input.Stderr)
	root := newRootCommandWithFactory(factory)
	if root == nil {
		return executeCommandFailure(diagnostics, fmt.Errorf("execute CLI command: command is required"))
	}
	root.SetArgs(append([]string(nil), input.Arguments...))
	root.SetIn(input.Stdin)
	root.SetOut(input.Stdout)
	root.SetErr(diagnostics)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetContext(clidiag.WithCentralDiagnostics(input.Context, true))
	if factory.observeCLI == nil {
		return executeCommandResult(diagnostics, cobracompletion.ExecuteWithPowerShellFilesystemDelegation(root))
	}
	snapshot, err := cliobservation.CaptureSnapshot(root)
	if err != nil {
		return executeCommandFailure(diagnostics, err)
	}
	command, positionals, parseErr := ParseArgvForCLIInputsInventory(root, input.Arguments)
	result := cliobservation.Result{
		Snapshot: snapshot,
		Parse:    cliobservation.CaptureParseResult(command, positionals),
	}
	if parseErr == nil {
		resolved, resolveErr := climanifestcobra.ResolvePersistentInputsForObservation(command, positionals)
		if resolveErr != nil {
			return executeCommandFailure(diagnostics, resolveErr)
		}
		result.ResolvedInputs = resolved.Observations()
	}
	edgeObservation, err := cliobservation.Encode(result)
	if err != nil {
		return executeCommandFailure(diagnostics, err)
	}
	if err := factory.observeCLI(edgeObservation); err != nil {
		return executeCommandFailure(diagnostics, fmt.Errorf("observe CLI command: %w", err))
	}
	return executeCommandResult(diagnostics, parseErr)
}

func executeCommandResult(diagnostics *clidiag.DiagnosticWriter, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return executeCommandFailure(diagnostics, err)
	}
	if diagnostics != nil && diagnostics.DiagnosticRendered() {
		return err
	}
	return executeCommandFailure(diagnostics, err)
}

func executeCommandFailure(diagnostics io.Writer, err error) error {
	if err == nil {
		return nil
	}
	// Cancellation is a process-control sentinel, not a command failure. Keep
	// its identity independent of whether a command already rendered a
	// diagnostic before returning it.
	if errors.Is(err, context.Canceled) {
		if !clidiag.DiagnosticRendered(diagnostics) {
			_, _ = fmt.Fprintln(diagnostics, "Error: context canceled")
			clidiag.MarkDiagnosticRendered(diagnostics)
		}
		return context.Canceled
	}
	normalized := clidiag.Normalize(err)
	if clidiag.DiagnosticRendered(diagnostics) {
		return err
	}
	clidiag.WriteFailure(diagnostics, normalized)
	return normalized
}

func buildWorkflowExecutionService(
	ctx context.Context,
	options CommandFactory,
	provider string,
	projectRoot string,
	fixtureCatalogPath string,
	childExecutorMode string,
) (OwnedExecutionService, error) {
	if options.BuildExecution == nil {
		return nil, fmt.Errorf("construct workflow execution: durable execution builder is required")
	}
	service, err := options.BuildExecution(
		ctx,
		provider,
		projectRoot,
		fixtureCatalogPath,
		childExecutorMode,
	)
	if err != nil {
		return nil, fmt.Errorf("construct workflow execution: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("construct workflow execution: builder returned nil service")
	}
	return service, nil
}

type cliDiagnosticsOptions struct {
	verbose bool
	debug   bool
}

const deprecatedPortFlagMessage = "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"

func rejectDeprecatedPortFlag(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Lookup("port") != nil && cmd.Flags().Changed("port") {
		return fmt.Errorf("%s", deprecatedPortFlagMessage)
	}
	return nil
}

func (opts *cliDiagnosticsOptions) resolvePolicy(quiet bool) terminalpolicy.Policy {
	return terminalpolicy.Resolve(terminalpolicy.Options{
		Quiet:   quiet,
		Verbose: opts.verbose,
		Debug:   opts.debug,
	})
}

func (opts *cliDiagnosticsOptions) verboseEnabled() bool {
	return opts.resolvePolicy(false).VerboseEnabled()
}

func (opts *cliDiagnosticsOptions) writer(cmd *cobra.Command) io.Writer {
	if cmd != nil && clidiag.CentralDiagnosticsEnabled(cmd.Context()) {
		return cmd.ErrOrStderr()
	}
	return opts.resolvePolicy(false).DiagnosticsWriter(cmd.ErrOrStderr())
}

func newMCPCommand(options CommandFactory) (*cobra.Command, error) {
	var initializeStdio startupcli.StdioHandler
	if options.initializer != nil {
		initializeStdio = options.initializer.Stdio
	}
	return climanifestcobra.NewMCPCommand(mcpcli.ResolvedServeHandler(mcpcli.ServeBinding{
		HomeDir:         options.homeDir,
		InitializeStdio: initializeStdio,
	}))
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func runFactoryWithOptions(cmd *cobra.Command, cfg runcli.RunConfig, promptArgs []string, globals *cliGlobalOptions, operatorDefaults *cliOperatorDefaultsOptions, policy terminalpolicy.Policy, rootOptions CommandFactory, defaultInvocation bool) error {
	cfg = applyRunScopedServerMode(cfg)
	if cfg.ListenExplicit || strings.TrimSpace(cfg.ListenAddress) != "" {
		cfg.ListenExplicit = true
		if !defaultInvocation && !cfg.WithServer && !cfg.WithSite {
			return fmt.Errorf("--listen requires --with-server or --with-site on you run")
		}
	}
	logger, err := policy.BuildLogger(rootOptions.buildTerminalLogger)
	if err != nil {
		return err
	}
	cfg.Logger = logger
	cfg.Verbose = policy.VerboseEnabled()
	cfg.TerminalPolicy = policy
	cfg.ExecutionBaseDir = startupcli.WorkingDirectory(cmd.Context())

	if !remotePlacementSelected(globals) {
		if err := resolveRunBindFromServer(cmd, globals.server, &cfg); err != nil {
			return err
		}
		warnLegacyListenerBinding(cmd, cfg, defaultInvocation, persistentInputWasCLI(cmd, "you.flag.server", "server"))
	}
	homeDir, err := resolveProcessHomeDir(rootOptions)
	if err != nil {
		return err
	}
	cfg.HomeDir = homeDir
	if err := configureRunEnvironment(cmd, &cfg, rootOptions, homeDir); err != nil {
		return err
	}
	if err := resolveRunFactorySelection(
		cmd,
		&cfg,
		homeDir,
		rootOptions.namedFactoryCatalog,
		rootOptions.resolveNamedFactoryRoots,
		rootOptions.resolveNamedFactoryCandidatePaths,
	); err != nil {
		return err
	}

	runOperatorDefaults := *operatorDefaults
	runOperatorDefaults.providerOverride = cfg.ProviderOverride
	runOperatorDefaults.modelOverride = cfg.ModelOverride
	resolvedOperatorDefaults, err := resolveOperatorDefaults(cmd, &runOperatorDefaults, rootOptions, homeDir)
	if err != nil {
		return err
	}
	cfg.OperatorDefaults = resolvedOperatorDefaults
	cfg.Stdin = cmd.InOrStdin()
	cfg.StdinIsTTY = func() bool { return startupcli.StdinIsTTY(cmd.Context()) }
	cfg.OutputIsTTY = startupcli.StdoutIsTTY(cmd.Context())
	if err := resolveRunFactoryPrompt(cmd, &cfg, promptArgs, rootOptions.prepareInvocationInput); err != nil {
		runcli.ObserveInvocationRejection(logger, err)
		return err
	}
	cleanInvocation, textInvocation := runInvocationModes(cmd, cfg)
	invocationFactorySelected := cmd.Flags().Changed("factory") || cmd.Flags().Changed("named") || cfg.InvocationFileExplicit
	defaultResponseStream := !cfg.SuppressDashboardRendering &&
		(textInvocation || (invocationFactorySelected && !cfg.Continuously && !cmd.Flags().Changed("work") && len(promptArgs) > 0))
	if defaultResponseStream && strings.TrimSpace(cfg.InvocationOutputMode) == "" && !cfg.InvocationOutputExplicit {
		cfg.InvocationOutputMode = runcli.InvocationOutputResponseStream
	}
	cfg.CleanInvocation = cleanInvocation
	cfg.JSON = globals.json
	runPolicy := resolveEffectiveRunPolicy(cmd, cfg, policy)
	cfg.TerminalPolicy = runPolicy
	cfg.Verbose = runPolicy.VerboseEnabled()
	cfg.SuppressDashboardRendering = runPolicy.Mode() == terminalpolicy.ModeQuiet
	configureRunProgressOutput(cmd, &cfg, policy)
	humanTerminal := runPolicy.HumanTerminalWriter(cmd.OutOrStdout())
	if cleanInvocation || textInvocation {
		cfg.Output = cmd.OutOrStdout()
	} else if strings.TrimSpace(cfg.FactoryConfigPath) != "" ||
		(strings.TrimSpace(cfg.ReplayPath) != "" && !cfg.SuppressDashboardRendering) {
		cfg.Output = cmd.OutOrStdout()
		cfg.StartupOutput = humanTerminal
	} else {
		cfg.StartupOutput = humanTerminal
	}
	cfg.Diagnostics = runPolicy.DiagnosticsWriter(cmd.ErrOrStderr())
	cfg.JSONOutput = globals.json
	if remotePlacementSelected(globals) {
		return runcli.RunRemoteInvocationWithWorkTarget(
			cmd.Context(), cfg, globals.server, rootOptions.remoteInvocation,
			rootOptions.prepareSingleWorkTarget, rootOptions.responsePresentation,
		)
	}
	if rootOptions.initializer == nil {
		return errors.New("run service initializer is required")
	}
	return delegateRunInitialization(cmd.Context(), cfg, defaultInvocation, rootOptions)
}

func configureRunProgressOutput(cmd *cobra.Command, cfg *runcli.RunConfig, policy terminalpolicy.Policy) {
	if cmd == nil || cfg == nil {
		return
	}
	// The effective policy intentionally suppresses operator/dashboard output
	// for one-shot text invocations while their explicit response stream remains
	// a customer result. Progress therefore follows the base policy resolved
	// from the user's explicit quiet/verbose flags, and is always routed to
	// stderr when that policy permits human terminal output.
	cfg.ProgressOutput = nil
	cfg.ProgressIsTTY = false
	if policy.AllowsHumanTerminalOutput() {
		cfg.ProgressOutput = cmd.ErrOrStderr()
		cfg.ProgressIsTTY = startupcli.StderrIsTTY(cmd.Context())
	}
}

func remotePlacementSelected(globals *cliGlobalOptions) bool {
	return globals != nil && (globals.remote || globals.placement == climanifest.ExecutionPlacementRemote)
}

func handleRunExecutionError(cmd *cobra.Command, resolvedConfig runcli.RunConfig, promptArgs []string, globals *cliGlobalOptions, basePolicy terminalpolicy.Policy, err error, currentFactorySelected bool) error {
	err = factoryload.MaybeFormatOperatorError(err, resolvedConfig.Dir)
	err = runcli.MapServerFailure(err)
	if currentFactorySelected {
		err = runcli.MapCurrentFactoryFailure(err)
	}
	if len(promptArgs) > 0 {
		err = runcli.MapInvocationFailure(err)
	}
	if writeRunIncompleteDrainError(cmd, err) {
		return err
	}
	if runcli.WriteInvocationError(cmd.ErrOrStderr(), err, globals.json) {
		return err
	}
	errorWriter := resolveEffectiveRunPolicy(cmd, resolvedConfig, basePolicy).HumanTerminalWriter(cmd.ErrOrStderr())
	var ambiguousInputErr *runcli.AmbiguousInvocationInputError
	if errors.As(err, &ambiguousInputErr) {
		errorWriter = cmd.ErrOrStderr()
	}
	if errorWriter != nil {
		writeRunHumanError(cmd, errorWriter, err)
	}
	return err
}

func writeRunIncompleteDrainError(cmd *cobra.Command, err error) bool {
	if clidiag.CentralDiagnosticsEnabled(cmd.Context()) {
		return false
	}
	return runcli.WriteIncompleteDrainError(cmd.ErrOrStderr(), err)
}

func writeRunHumanError(cmd *cobra.Command, output io.Writer, err error) {
	if clidiag.CentralDiagnosticsEnabled(cmd.Context()) {
		return
	}
	_, _ = fmt.Fprintln(output, err)
}

func configureRunEnvironment(cmd *cobra.Command, cfg *runcli.RunConfig, rootOptions CommandFactory, homeDir string) error {
	if rootOptions.loadOperatorConfig != nil {
		operatorConfig, loadErr := rootOptions.loadOperatorConfig(operatorconfig.DefaultConfigPath(homeDir))
		if loadErr != nil && !errors.Is(loadErr, syscall.ENOTDIR) {
			return loadErr
		}
		if loadErr == nil {
			cfg.ACPIntegrations = append([]operatorconfig.ACPIntegration(nil), operatorConfig.Workers.ACP.Integrations...)
			if err := rootOptions.acp.Configure(cmd.Context(), operatorConfig.Workers.ACP.Integrations); err != nil {
				return err
			}
		}
	}
	modelCacheDir, _, err := lookupProcessEnvironment(
		rootOptions,
		runcli.ModelCacheDirEnvironment,
	)
	if err != nil {
		return err
	}
	cfg.ModelCacheDir = modelCacheDir
	cfg.FactoryScaffoldInitializer = rootOptions.InitFactory
	cfg.ResolveCurrentFactoryDir = rootOptions.resolveCurrentFactoryDir
	cfg.ResolveFactoryConfigRoot = rootOptions.resolveFactoryConfigRoot
	cfg.LoadFactoryConfigFile = rootOptions.loadFactoryConfigFile
	cfg.WorkRequestFileLoader = rootOptions.workRequestFileLoader
	cfg.DirectoryCreator = rootOptions.runDirectoryCreator
	cfg.BrowserOpener = rootOptions.browserOpener
	return nil
}

func delegateRunInitialization(ctx context.Context, cfg runcli.RunConfig, defaultInvocation bool, options CommandFactory) error {
	intent := startupcli.RunIntent{
		DefaultInvocation:     defaultInvocation,
		Continuous:            cfg.Continuously,
		APIEnabled:            (defaultInvocation || cfg.WithServer) && cfg.Port > 0,
		DashboardEnabled:      (defaultInvocation || cfg.WithSite) && cfg.Port > 0,
		WorkerSidecarsEnabled: true,
	}
	if options.openRunSelection == nil {
		return fmt.Errorf("run selection operation is required")
	}
	return options.initializer.Run(ctx, intent, options.openRunSelection(cfg))
}

func resolveEffectiveRunPolicy(cmd *cobra.Command, cfg runcli.RunConfig, basePolicy terminalpolicy.Policy) terminalpolicy.Policy {
	cleanInvocation, textInvocation := runInvocationModes(cmd, cfg)
	if cfg.SuppressDashboardRendering || cleanInvocation || textInvocation {
		return terminalpolicy.Resolve(terminalpolicy.Options{
			Quiet:   true,
			Verbose: basePolicy.VerboseEnabled(),
			Debug:   basePolicy.DebugEnabled(),
		})
	}
	return basePolicy
}

func runInvocationModes(cmd *cobra.Command, cfg runcli.RunConfig) (cleanInvocation bool, textInvocation bool) {
	invocationFactorySelected := cmd.Flags().Changed("factory") || cmd.Flags().Changed("named") || cfg.InvocationFileExplicit
	cleanInvocation = invocationFactorySelected &&
		cmd.Flags().Changed("work") &&
		strings.TrimSpace(cfg.WorkFile) != "" &&
		!cfg.Continuously
	textInvocation = invocationFactorySelected &&
		!cmd.Flags().Changed("work") &&
		!cfg.Continuously &&
		(cfg.InvocationPositionalText != nil ||
			cfg.InvocationStdinText != nil ||
			cfg.InvocationFileExplicit ||
			cfg.InvocationNormalizedArguments != nil ||
			cfg.PreparedInvocationInput != nil)
	return cleanInvocation, textInvocation
}
