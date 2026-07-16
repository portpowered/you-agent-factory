package wire

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/composebridge"
	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
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
			return composebridge.CloseRuntimeBundleSinks(bundle.LogSink, bundle.MetricsSink)
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
