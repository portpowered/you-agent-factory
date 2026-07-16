package wire

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/providerexecution"
	"go.uber.org/zap"
)

type runtimeSessionExecutionCoreBuilder func(context.Context, *runtimehost.Config) (*runtimehost.Core, error)

// BuildSessionExecutionService constructs the durable execution collaborator
// selected by CLI inputs. Transport packages receive the narrow service with
// its cleanup owner and retain command-lifecycle, parsing, and rendering
// ownership.
func BuildSessionExecutionService(
	ctx context.Context,
	request sessionexecutioncli.ServiceRequest,
) (sessionexecutioncli.ServiceOwner, error) {
	return buildSessionExecutionService(ctx, request, InjectRuntimeCore)
}

// BuildSessionExecutionWithApplication constructs an owned runtime-backed
// execution service using the worker application selected for the process.
func BuildSessionExecutionWithApplication(
	ctx context.Context,
	request sessionexecutioncli.ServiceRequest,
	application workerapplication.Components,
) (sessionexecutioncli.ServiceOwner, error) {
	provider, err := normalizeSessionExecutionProvider(request.Provider)
	if err != nil {
		return nil, err
	}
	if provider != factorysessionexecution.ExecutionProviderJavaScriptRuntime {
		return buildSessionExecutionService(ctx, request, InjectRuntimeCore)
	}
	projectRoot, err := resolveSessionExecutionProjectRoot(request.ProjectRoot)
	if err != nil {
		return nil, err
	}
	persistence, err := factorysessionexecution.ProjectPersistence(projectRoot)
	if err != nil {
		return nil, err
	}
	var workerProvider workers.Provider
	if request.ChildExecutorMode == factorysessionexecution.ChildExecutorModeLive {
		if !application.Valid() {
			return nil, fmt.Errorf("construct session worker provider: worker application is required")
		}
		workerProvider, err = application.Provider.New()
		if err != nil {
			return nil, fmt.Errorf("construct session worker provider: %w", err)
		}
	}
	execution, err := factorysessionexecution.NewExecutionService(provider, factorysessionexecution.ServiceConfig{
		ProjectRoot: projectRoot, ChildExecutorMode: request.ChildExecutorMode,
		Provider: workerProvider, ProviderExecutor: providerexecution.NewExecutor(workerProvider),
		Persistence: persistence, Clock: factory.EnsureClock(nil),
	})
	if err != nil {
		return nil, err
	}
	return ownedExecutionService{Service: execution}, nil
}

// SessionExecutionBuilderWithEdges binds process-selected worker edges to the
// transport-facing durable execution builder.
func SessionExecutionBuilderWithEdges(
	buildApplication WorkerApplicationBuilder,
	edges FunctionalEdges,
) sessionexecutioncli.ServiceBuilder {
	return func(ctx context.Context, request sessionexecutioncli.ServiceRequest) (sessionexecutioncli.ServiceOwner, error) {
		if buildApplication == nil {
			return nil, fmt.Errorf("construct session execution: worker application builder is required")
		}
		application, err := buildApplication(zap.NewNop(), edges)
		if err != nil {
			return nil, fmt.Errorf("construct session execution: %w", err)
		}
		return BuildSessionExecutionWithApplication(ctx, request, application)
	}
}

type providerCommandEdge struct{ workers.CommandRunner }
type scriptCommandEdge struct{ workers.CommandRunner }
type providerPTYEdge struct{ agypty.PTYAllocator }

func provideProviderCommandEdge(edges FunctionalEdges) providerCommandEdge {
	runner := edges.ProviderCommandRunner
	if runner == nil {
		runner = workerprocess.ExecCommandRunner{}
	}
	return providerCommandEdge{CommandRunner: runner}
}

func provideScriptCommandEdge(edges FunctionalEdges) scriptCommandEdge {
	runner := edges.ScriptCommandRunner
	if runner == nil {
		runner = workerprocess.ExecCommandRunner{}
	}
	return scriptCommandEdge{CommandRunner: runner}
}

func provideProviderPTYEdge(edges FunctionalEdges) (providerPTYEdge, error) {
	allocator := edges.AgyPTYAllocator
	if allocator == nil {
		var err error
		allocator, err = agypty.NewDefaultPlatformAllocatorFactory().NewAllocator()
		if err != nil {
			return providerPTYEdge{}, fmt.Errorf("construct worker application: create Agy PTY allocator: %w", err)
		}
	}
	return providerPTYEdge{PTYAllocator: allocator}, nil
}

func provideWorkerProviderFactory(runner providerCommandEdge, allocator providerPTYEdge) (*workerprovider.Factory, error) {
	return workerprovider.NewFactory(workerprovider.ConstructionInput{
		CommandRunner: runner.CommandRunner, AgyPTYAllocator: allocator.PTYAllocator,
	})
}

func provideWorkerScriptFactory(runner scriptCommandEdge) (*workerexecutor.ScriptFactory, error) {
	return workerexecutor.NewScriptFactory(runner.CommandRunner)
}

func provideWorkerHostedConfig(logger *zap.Logger, edges FunctionalEdges) hostedworkers.Config {
	return hostedworkers.NewConfig(hostedworkers.Config{
		Logger: logger, Clock: edges.HostedClock, HTTPClient: edges.HostedHTTPClient,
		SecretResolver: edges.HostedSecretResolver, LinearEndpoint: edges.HostedLinearEndpoint,
	})
}

func provideWorkerApplication(
	provider *workerprovider.Factory,
	script *workerexecutor.ScriptFactory,
	hosted hostedworkers.Config,
	providerRunner providerCommandEdge,
	scriptRunner scriptCommandEdge,
	edges FunctionalEdges,
) workerapplication.Components {
	return workerapplication.Components{
		Provider: provider, Script: script, Hosted: hosted,
		ProviderCommandRunner: providerRunner.CommandRunner, ScriptCommandRunner: scriptRunner.CommandRunner,
		ProviderCommandInjected: edges.ProviderCommandRunner != nil,
		ScriptCommandInjected:   edges.ScriptCommandRunner != nil,
	}
}

func buildSessionExecutionService(
	ctx context.Context,
	request sessionexecutioncli.ServiceRequest,
	buildCore runtimeSessionExecutionCoreBuilder,
) (sessionexecutioncli.ServiceOwner, error) {
	provider, err := normalizeSessionExecutionProvider(request.Provider)
	if err != nil {
		return nil, err
	}
	if provider == factorysessionexecution.ExecutionProviderJavaScriptRuntime {
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
	return ownedExecutionService{Service: service}, nil
}

// buildRuntimeBackedSessionExecutionService retains the complete application
// graph behind the narrow durable-execution contract used by CLI commands.
// Callers that own a command lifecycle may close the returned service to
// release the graph's construction resources.
func buildRuntimeBackedSessionExecutionService(
	ctx context.Context,
	projectRoot string,
	buildCore runtimeSessionExecutionCoreBuilder,
) (sessionexecutioncli.ServiceOwner, error) {
	graph, err := buildRuntimeBackedSessionExecutionGraph(ctx, projectRoot, strings.NewReader(""), io.Discard, buildCore)
	if err != nil {
		return nil, err
	}
	return ownedExecutionService{Service: graph.DurableExecution, graph: graph}, nil

}

// buildRuntimeBackedSessionExecutionGraph constructs the single shared
// application graph used by runtime-backed MCP and CLI execution paths.
func buildRuntimeBackedSessionExecutionGraph(
	ctx context.Context,
	projectRoot string,
	input io.Reader,
	output io.Writer,
	buildCore runtimeSessionExecutionCoreBuilder,
) (*Graph, error) {
	if buildCore == nil {
		return nil, fmt.Errorf("construct runtime-backed execution graph: runtime core builder is required")
	}
	cfg := &runtimehost.Config{
		Dir: projectRoot, ExecutionBaseDir: projectRoot,
		Logger: zap.NewNop(), Clock: factory.EnsureClock(nil),
	}
	core, err := buildCore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("construct runtime-backed execution graph: %w", err)
	}
	if core == nil || core.DurableExecution() == nil {
		return nil, fmt.Errorf("construct runtime-backed execution graph: shared runtime core durable execution is required")
	}
	resources := &resourceSet{}
	if bundle := core.StartupBundle(); bundle != nil {
		resources.add("runtime core", closeFunc(func() error {
			return factoryservice.CloseBundleSinks(bundle.LogSink, bundle.MetricsSink)
		}))
	}
	graph, err := assembleProductionGraph(core, cfg, Inputs{MCPInput: input, MCPOutput: output}, resources)
	if err != nil {
		return nil, failProductionBuild(resources, err)
	}
	return graph, nil
}

type ownedExecutionService struct {
	factorysessionexecution.Service
	graph *Graph
}

func (service ownedExecutionService) Close() error {
	if service.graph == nil {
		return nil
	}
	return service.graph.Close()
}

func normalizeSessionExecutionProvider(value string) (factorysessionexecution.ExecutionProvider, error) {
	switch strings.TrimSpace(value) {
	case "", string(factorysessionexecution.ExecutionProviderFake):
		return factorysessionexecution.ExecutionProviderFake, nil
	case string(factorysessionexecution.ExecutionProviderJavaScriptRuntime):
		return factorysessionexecution.ExecutionProviderJavaScriptRuntime, nil
	default:
		return "", fmt.Errorf("execution provider %q is unsupported: use fake or javascript-runtime", value)
	}
}

func resolveSessionExecutionProjectRoot(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	return cwd, nil
}

func resolveSessionExecutionFixtureCatalog(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	relative := filepath.FromSlash(fixtures.ContractFixtureCatalogRelativePath)
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, relative)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return "", fmt.Errorf(
		"fixture catalog not found; run from the repository root or set --fixture-catalog to %s",
		fixtures.ContractFixtureCatalogRelativePath,
	)
}
