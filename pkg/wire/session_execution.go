package wire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	sessionexecutioncli "github.com/portpowered/infinite-you/pkg/transports/cli/sessionexecution"
	"go.uber.org/zap"
)

type runtimeSessionExecutionCoreBuilder func(context.Context, *runtimehost.Config) (*runtimehost.Core, error)

// BuildSessionExecutionService constructs the durable execution collaborator
// selected by CLI inputs. Transport packages receive only the resulting
// service and retain parsing and rendering ownership.
func BuildSessionExecutionService(
	ctx context.Context,
	request sessionexecutioncli.ServiceRequest,
) (factorysessionexecution.Service, error) {
	return buildSessionExecutionService(ctx, request, InjectRuntimeCore)
}

func buildSessionExecutionService(
	ctx context.Context,
	request sessionexecutioncli.ServiceRequest,
	buildCore runtimeSessionExecutionCoreBuilder,
) (factorysessionexecution.Service, error) {
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
	return service, nil
}

// buildRuntimeBackedSessionExecutionService adapts a completed Wire runtime
// core to the narrow durable-execution contract used by CLI and MCP. It never
// constructs persistence or execution independently of that shared graph.
func buildRuntimeBackedSessionExecutionService(
	ctx context.Context,
	projectRoot string,
	buildCore runtimeSessionExecutionCoreBuilder,
) (factorysessionexecution.Service, error) {
	if buildCore == nil {
		return nil, fmt.Errorf("construct runtime-backed execution service: runtime core builder is required")
	}
	core, err := buildCore(ctx, &runtimehost.Config{
		Dir: projectRoot, ExecutionBaseDir: projectRoot,
		Logger: zap.NewNop(), Clock: factory.EnsureClock(nil),
	})
	if err != nil {
		return nil, fmt.Errorf("construct runtime-backed execution service: %w", err)
	}
	if core == nil || core.DurableExecution() == nil {
		return nil, fmt.Errorf("construct runtime-backed execution service: shared runtime core durable execution is required")
	}
	return core.DurableExecution(), nil
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
