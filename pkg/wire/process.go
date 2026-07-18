package wire

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// CLICommandBuilder constructs a fresh Cobra command tree from process-owned
// transport options.
type CLICommandBuilder func(cli.RootCommandOptions) *cobra.Command

// ProcessGraphBuilder constructs an inert process graph with the MCP execution
// builder selected for the process.
type ProcessGraphBuilder func(
	context.Context,
	startupcli.Request,
	initializer.ProcessPolicy,
	ProcessGraphDependencies,
) (*initializer.ProcessGraph, error)

// ProcessInitializer starts and owns an already-constructed process graph.
type ProcessInitializer func(context.Context, *initializer.ProcessGraph) error

// WireCore is the process composition surface injected once by pkg/root. Its
// builders retain request-specific construction boundaries without requiring
// root to assemble each production dependency independently.
type WireCore struct {
	BuildCLICommand          CLICommandBuilder
	BuildProcessGraph        ProcessGraphBuilder
	InitializeProcess        ProcessInitializer
	BuildMCPExecution        MCPExecutionBuilder
	BuildSessionExecution    sessionexecutioncli.ServiceBuilder
	BuildModelInvocation     modelscli.InvocationBuilder
	BuildWorkerApplication   WorkerApplicationBuilder
	BuildRunSessionExecution RunSessionExecutionBuilder
}

func provideCLICommandBuilder() CLICommandBuilder {
	return cli.NewRootCommandWithOptions
}

func provideProcessGraphBuilder() ProcessGraphBuilder {
	return buildProcessGraphFromDependencies
}

func buildProcessGraphFromDependencies(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	dependencies ProcessGraphDependencies,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraphWithDependencies(ctx, request, policy, dependencies, buildProcessApplicationRunner)
}

func provideProcessInitializer() ProcessInitializer {
	return initializer.RunProcess
}

func provideMCPExecutionBuilder() MCPExecutionBuilder {
	return BuildMCPExecutionService
}

func provideSessionExecutionBuilder() sessionexecutioncli.ServiceBuilder {
	return SessionExecutionBuilderWithEdges(InjectWorkerApplication, FunctionalEdges{})
}

func provideModelInvocationBuilder() modelscli.InvocationBuilder {
	return BuildModelInvocation
}

// WorkerApplicationBuilder constructs the process-wide worker factories and
// side-effect edges selected before runtime construction.
type WorkerApplicationBuilder func(*zap.Logger, FunctionalEdges) (workerapplication.Components, error)

// RunSessionExecutionBuilder constructs JavaScript Factory Session execution
// from the same worker application used by the surrounding process graph.
type RunSessionExecutionBuilder func(
	context.Context,
	sessionexecutioncli.ServiceRequest,
	workerapplication.Components,
) (sessionexecutioncli.ServiceOwner, error)

// ProcessGraphDependencies are the construction capabilities and functional
// edges selected by the startup root for one inert process graph.
type ProcessGraphDependencies struct {
	BuildMCPExecution      MCPExecutionBuilder
	BuildWorkerApplication WorkerApplicationBuilder
	BuildSessionExecution  RunSessionExecutionBuilder
	FunctionalEdges        FunctionalEdges
	RuntimeMCPGraph        bool
}

func provideWorkerApplicationBuilder() WorkerApplicationBuilder { return InjectWorkerApplication }

func provideRunSessionExecutionBuilder() RunSessionExecutionBuilder {
	return BuildSessionExecutionWithApplication
}

// FunctionalEdges contains the process-owned side-effect boundaries that a
// production-shaped functional graph may replace. Its zero value preserves
// every production edge.
type FunctionalEdges struct {
	APIServerListener     net.Listener
	ProviderCommandRunner workers.CommandRunner
	ScriptCommandRunner   workers.CommandRunner
	AgyPTYAllocator       agypty.PTYAllocator
	HostedHTTPClient      *http.Client
	HostedLinearEndpoint  string
	HostedSecretResolver  hostedworkers.SecretResolver
	HostedClock           clockwork.Clock
}

// MCPExecutionRequest contains the transport inputs that select the durable
// Factory Session execution collaborator used by MCP serve.
type MCPExecutionRequest struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
}

// MCPExecutionBuilder constructs an MCP durable execution collaborator before
// the initializer starts the stdio transport.
type MCPExecutionBuilder func(context.Context, MCPExecutionRequest) (factorysessionexecution.Service, error)

// BuildMCPExecutionService constructs the durable execution collaborator
// selected by MCP command inputs. Protocol parsing remains in the transport;
// persistence and runtime construction remain in wire.
func BuildMCPExecutionService(
	ctx context.Context,
	request MCPExecutionRequest,
) (factorysessionexecution.Service, error) {
	return buildMCPExecutionService(ctx, request, InjectRuntimeCore)
}

func buildMCPExecutionService(
	ctx context.Context,
	request MCPExecutionRequest,
	buildCore runtimeSessionExecutionCoreBuilder,
) (factorysessionexecution.Service, error) {
	if request.RuntimeBacked {
		projectRoot, err := resolveSessionExecutionProjectRoot(request.ProjectRoot)
		if err != nil {
			return nil, err
		}
		return buildRuntimeBackedSessionExecutionService(ctx, projectRoot, buildCore)
	}
	catalogPath, err := resolveSessionExecutionFixtureCatalog(request.FixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return service, nil
}

// BuildProcessGraph constructs the concrete application graph selected by the
// process root without starting transports, sidecars, or runtime loops.
func BuildProcessGraph(ctx context.Context, request startupcli.Request, policy initializer.ProcessPolicy) (*initializer.ProcessGraph, error) {
	return buildProcessGraphWithDependencies(
		ctx, request, policy, productionProcessGraphDependencies(), buildProcessApplicationRunner,
	)
}

// BuildProcessGraphWithFunctionalEdges constructs the same application graph
// as BuildProcessGraph while replacing only explicitly supplied process edges.
// Functional edge selection is applied to a copy of invocation-local config.
func BuildProcessGraphWithFunctionalEdges(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	edges FunctionalEdges,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraphWithDependencies(
		ctx, request, policy, ProcessGraphDependencies{
			BuildMCPExecution: BuildMCPExecutionService, BuildWorkerApplication: InjectWorkerApplication,
			BuildSessionExecution: BuildSessionExecutionWithApplication, FunctionalEdges: edges, RuntimeMCPGraph: true,
		}, buildProcessApplicationRunner,
	)
}

// BuildProcessGraphWithMCPBuilder constructs a process graph with an explicit
// MCP execution collaborator builder. The root uses this seam to make all
// production transport dependencies visible before lifecycle startup.
func BuildProcessGraphWithMCPBuilder(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	buildMCP MCPExecutionBuilder,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraphWithDependencies(
		ctx, request, policy, ProcessGraphDependencies{
			BuildMCPExecution: buildMCP, BuildWorkerApplication: InjectWorkerApplication,
			BuildSessionExecution: BuildSessionExecutionWithApplication,
		}, buildProcessApplicationRunner,
	)
}

func productionProcessGraphDependencies() ProcessGraphDependencies {
	return ProcessGraphDependencies{
		BuildMCPExecution: BuildMCPExecutionService, BuildWorkerApplication: InjectWorkerApplication,
		BuildSessionExecution: BuildSessionExecutionWithApplication, RuntimeMCPGraph: true,
	}
}

type processRunnerBuilder func(
	context.Context,
	*service.FactoryServiceConfig,
	initializer.Mode,
) (runcli.RuntimeRunner, error)

func buildProcessApplicationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	mode initializer.Mode,
) (runcli.RuntimeRunner, error) {
	return buildApplicationRunner(ctx, cfg, mode)
}

func buildProcessGraphWithDependencies(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	dependencies ProcessGraphDependencies,
	buildRunner processRunnerBuilder,
	invocationBuilders ...runcli.InvocationBootstrapBuilder,
) (*initializer.ProcessGraph, error) {
	switch request.Kind {
	case startupcli.KindRun:
		if request.RunConfig == nil {
			return nil, fmt.Errorf("construct run graph: run config is required")
		}
		runConfig, err := applyRunProcessPolicy(*request.RunConfig, policy)
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		if javascriptRunConfig(runConfig) {
			return buildJavaScriptRunGraph(ctx, runConfig, policy, dependencies)
		}
		applicationMode, err := applicationModeForProcess(policy.Mode)
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		invocationBuilder := runcli.InvocationBootstrapBuilder(buildInvocationRunner)
		if len(invocationBuilders) > 0 {
			invocationBuilder = invocationBuilders[0]
		}
		var sharedWorkerApplication workerapplication.Components
		application, err := runcli.BuildApplication(ctx, runConfig, func(
			buildCtx context.Context,
			cfg *service.FactoryServiceConfig,
		) (runcli.RuntimeRunner, error) {
			configured, err := configWithProcessDependencies(cfg, dependencies, &sharedWorkerApplication)
			if err != nil {
				return nil, err
			}
			return buildRunner(buildCtx, configured, applicationMode)
		}, func(buildCtx context.Context, cfg *service.FactoryServiceConfig) (runcli.InvocationRunner, error) {
			configured, err := configWithProcessDependencies(cfg, dependencies, &sharedWorkerApplication)
			if err != nil {
				return nil, err
			}
			return invocationBuilder(buildCtx, configured)
		})
		if err != nil {
			return nil, fmt.Errorf("construct run graph: %w", err)
		}
		return &initializer.ProcessGraph{Policy: policy, Run: application}, nil
	case startupcli.KindMCPServe:
		return buildMCPProcessGraph(ctx, request.MCP, policy, dependencies.BuildMCPExecution, dependencies.RuntimeMCPGraph)
	default:
		return nil, fmt.Errorf("construct process graph: unsupported startup kind %q", request.Kind)
	}
}

// buildProcessGraph retains the narrow package-test seam while production root
// construction supplies the complete dependency bundle above.
func buildProcessGraph(
	ctx context.Context,
	request startupcli.Request,
	policy initializer.ProcessPolicy,
	edges FunctionalEdges,
	buildRunner processRunnerBuilder,
	buildMCP MCPExecutionBuilder,
	runtimeMCPGraph bool,
	invocationBuilders ...runcli.InvocationBootstrapBuilder,
) (*initializer.ProcessGraph, error) {
	return buildProcessGraphWithDependencies(ctx, request, policy, ProcessGraphDependencies{
		BuildMCPExecution: buildMCP, BuildWorkerApplication: InjectWorkerApplication,
		BuildSessionExecution: BuildSessionExecutionWithApplication, FunctionalEdges: edges,
		RuntimeMCPGraph: runtimeMCPGraph,
	}, buildRunner, invocationBuilders...)
}

func configWithProcessDependencies(
	cfg *service.FactoryServiceConfig,
	dependencies ProcessGraphDependencies,
	shared ...*workerapplication.Components,
) (*service.FactoryServiceConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("construct worker application: service config is required")
	}
	copied := *cfg
	if dependencies.FunctionalEdges.APIServerListener != nil {
		copied.APIServerStarter = runcli.APIServerStarterWithListener(
			dependencies.FunctionalEdges.APIServerListener,
		)
	}
	if len(shared) > 0 && shared[0] != nil && shared[0].Valid() {
		copied.WorkerApplication = *shared[0]
		return &copied, nil
	}
	edges := dependencies.FunctionalEdges
	hostedClock := edges.HostedClock
	if hostedClock == nil {
		hostedClock, _ = cfg.Clock.(clockwork.Clock)
	}
	if dependencies.BuildWorkerApplication == nil {
		return nil, fmt.Errorf("construct worker application: builder is required")
	}
	edges.HostedClock = hostedClock
	components, err := dependencies.BuildWorkerApplication(cfg.Logger, edges)
	if err != nil {
		return nil, err
	}
	copied.WorkerApplication = components
	if len(shared) > 0 && shared[0] != nil {
		*shared[0] = components
	}
	return &copied, nil
}

func configWithFunctionalEdges(
	cfg *service.FactoryServiceConfig,
	edges FunctionalEdges,
	shared ...*workerapplication.Components,
) (*service.FactoryServiceConfig, error) {
	return configWithProcessDependencies(cfg, ProcessGraphDependencies{
		BuildWorkerApplication: InjectWorkerApplication, FunctionalEdges: edges,
	}, shared...)
}

type sessionRunApplication struct{ config sessionexecutioncli.RunConfig }

func (application sessionRunApplication) Run(ctx context.Context) error {
	return sessionexecutioncli.RunSync(ctx, application.config)
}

func javascriptRunConfig(cfg runcli.RunConfig) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(cfg.FactoryConfigPath))) {
	case ".js", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func buildJavaScriptRunGraph(
	ctx context.Context,
	cfg runcli.RunConfig,
	policy initializer.ProcessPolicy,
	dependencies ProcessGraphDependencies,
) (*initializer.ProcessGraph, error) {
	if dependencies.BuildWorkerApplication == nil || dependencies.BuildSessionExecution == nil {
		return nil, fmt.Errorf("construct JavaScript run graph: worker application and session execution builders are required")
	}
	sourcePath, err := filepath.Abs(strings.TrimSpace(cfg.FactoryConfigPath))
	if err != nil {
		return nil, fmt.Errorf("construct JavaScript run graph: resolve workflow source: %w", err)
	}
	components, err := dependencies.BuildWorkerApplication(cfg.Logger, dependencies.FunctionalEdges)
	if err != nil {
		return nil, fmt.Errorf("construct JavaScript run graph: %w", err)
	}
	childExecutorMode := factorysessionexecution.ChildExecutorModeLive
	if cfg.MockWorkersEnabled {
		childExecutorMode = factorysessionexecution.ChildExecutorModeFake
	}
	serviceRequest := sessionexecutioncli.ServiceRequest{
		ExecutionBackendConfig: sessionexecutioncli.ExecutionBackendConfig{
			Provider: string(factorysessionexecution.ExecutionProviderJavaScriptRuntime), ProjectRoot: filepath.Dir(sourcePath),
		},
		ChildExecutorMode: childExecutorMode,
	}
	execution, err := dependencies.BuildSessionExecution(ctx, serviceRequest, components)
	if err != nil {
		return nil, fmt.Errorf("construct JavaScript run graph: %w", err)
	}
	application := sessionRunApplication{config: sessionexecutioncli.RunConfig{
		StartConfig: sessionexecutioncli.StartConfig{
			Mode: sessionexecutioncli.ExecutionModeSync, RequestID: "run-" + uuid.NewString(),
			WorkflowFile: sourcePath, ChildExecutorMode: childExecutorMode,
		},
		ExecutionBackendConfig: serviceRequest.ExecutionBackendConfig,
		JSON:                   cfg.JSONOutput, Output: cfg.Output, Service: execution,
	}}
	return &initializer.ProcessGraph{Policy: policy, Run: application}, nil
}

func buildMCPProcessGraph(
	ctx context.Context,
	intent startupcli.MCPIntent,
	policy initializer.ProcessPolicy,
	buildMCP MCPExecutionBuilder,
	runtimeMCPGraph bool,
) (*initializer.ProcessGraph, error) {
	if policy.Mode != initializer.ProcessModeMCPServe || policy.Sidecars != (initializer.SidecarPolicy{}) {
		return nil, fmt.Errorf("construct MCP graph: incompatible process policy %+v", policy)
	}
	if buildMCP == nil {
		return nil, fmt.Errorf("construct MCP graph: execution service builder is required")
	}
	var graph *Graph
	var err error
	if intent.RuntimeBacked && runtimeMCPGraph {
		projectRoot, rootErr := resolveSessionExecutionProjectRoot(intent.ProjectRoot)
		if rootErr != nil {
			return nil, fmt.Errorf("construct MCP graph: %w", rootErr)
		}
		graph, err = buildRuntimeBackedSessionExecutionGraph(ctx, projectRoot, intent.Stdin, intent.Stdout, InjectRuntimeCore)
	} else {
		execution, buildErr := buildMCP(ctx, MCPExecutionRequest{
			FixtureCatalogPath: intent.FixtureCatalogPath,
			RuntimeBacked:      intent.RuntimeBacked, ProjectRoot: intent.ProjectRoot,
		})
		if buildErr != nil {
			return nil, fmt.Errorf("construct MCP graph: %w", buildErr)
		}
		graph, err = Build(ctx, Inputs{MCPExecution: execution, MCPInput: intent.Stdin, MCPOutput: intent.Stdout})
	}
	if err != nil {
		return nil, fmt.Errorf("construct MCP graph: %w", err)
	}
	application, err := initializer.NewApplication(initializer.ModeMCP, graph)
	if err != nil {
		if cleanupErr := graph.Close(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close rejected MCP application graph: %w", cleanupErr))
		}
		return nil, fmt.Errorf("construct MCP graph: %w", err)
	}
	return &initializer.ProcessGraph{Policy: policy, MCP: application}, nil
}

func buildInvocationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
) (runcli.InvocationRunner, error) {
	normalized := service.NormalizeInvocationBootstrapConfig(cfg)
	if normalized == nil {
		return nil, fmt.Errorf("build invocation bootstrap: config is required")
	}
	svc, err := InjectFactoryService(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return service.NewInvocationBootstrap(svc)
}

func applicationModeForProcess(mode initializer.ProcessMode) (initializer.Mode, error) {
	switch mode {
	case initializer.ProcessModeDefaultRun, initializer.ProcessModeAPIService:
		return initializer.ModeAPI, nil
	case initializer.ProcessModeLocalRun:
		return initializer.ModeCLI, nil
	default:
		return "", fmt.Errorf("run graph requires a run process mode, got %q", mode)
	}
}

func applyRunProcessPolicy(cfg runcli.RunConfig, policy initializer.ProcessPolicy) (runcli.RunConfig, error) {
	if policy.Sidecars.Dashboard && !policy.Sidecars.API {
		return runcli.RunConfig{}, fmt.Errorf("dashboard sidecar requires API transport")
	}
	switch policy.Mode {
	case initializer.ProcessModeDefaultRun, initializer.ProcessModeAPIService:
		if !policy.Sidecars.WorkerScheduler || !policy.Sidecars.Watchers {
			return runcli.RunConfig{}, fmt.Errorf("%s policy requires worker scheduler and watchers", policy.Mode)
		}
		cfg.Continuously = true
	case initializer.ProcessModeLocalRun:
		if !policy.Sidecars.WorkerScheduler || policy.Sidecars.Watchers {
			return runcli.RunConfig{}, fmt.Errorf("local-run policy requires worker scheduler with watchers disabled")
		}
		cfg.Continuously = false
	default:
		return runcli.RunConfig{}, fmt.Errorf("run graph requires a run process mode, got %q", policy.Mode)
	}
	if !policy.Sidecars.API {
		cfg.Port = 0
	}
	cfg.SuppressDashboardRendering = !policy.Sidecars.Dashboard
	return cfg, nil
}
