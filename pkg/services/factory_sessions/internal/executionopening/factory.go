// Package executionopening owns selection and construction of durable Factory Session
// execution services.
package executionopening

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// StandaloneSessionExecutionFactory and WorkerInvocationFactory are consumed
// only by this Factory Session execution-opening operation. Wire supplies their
// implementations; this package owns the exact signatures it invokes.
type StandaloneSessionExecutionFactory = func(
	factorysessions.ExecutionProvider,
	string,
	string,
	string,
	workers.InvocationExecutor,
	factory.Clock,
) (durableexecution.Service, error)

type WorkerInvocationFactory = func(
	workers.CommandRunner,
	workers.PTYAllocator,
) (workers.InvocationExecutor, error)

type WorkerInvocationWithProgressFactory = func(
	workers.CommandRunner,
	workers.PTYAllocator,
	workers.ProgressPublisher,
) (workers.InvocationExecutor, error)

// Factory owns provider selection and lazy runtime-backed execution scopes.
type Factory struct {
	runtimes       *runtimeopening.Factory
	runtimeEffects runtimeopening.ExternalEffects
	commandRunner  workers.CommandRunner
	allocator      workers.PTYAllocator
	standalone     StandaloneSessionExecutionFactory
	invocation     WorkerInvocationFactory
	resolveClock   factory.ClockResolver
	artifactRoots  factory.RuntimeArtifactRootResolver
	adaptRunner    runtimeopening.WorkerCommandRunnerAdapter
	paths          roles.ExecutionOpeningFileSystem
	logger         *zap.Logger
}

var _ roles.StdioExecutionOpening = (*Factory)(nil)

func NewFactory(
	runtimes *runtimeopening.Factory,
	runtimeEffects runtimeopening.ExternalEffects,
	commandRunner workers.CommandRunner,
	allocator workers.PTYAllocator,
	build StandaloneSessionExecutionFactory,
	invocation WorkerInvocationFactory,
	resolveClock factory.ClockResolver,
	artifactRoots factory.RuntimeArtifactRootResolver,
	adaptRunner runtimeopening.WorkerCommandRunnerAdapter,
	paths roles.ExecutionOpeningFileSystem,
	logger *zap.Logger,
) (*Factory, error) {
	if runtimes == nil {
		return nil, fmt.Errorf("session execution runtime factory is required")
	}
	if build == nil {
		return nil, fmt.Errorf("standalone session execution factory is required")
	}
	if allocator == nil {
		return nil, fmt.Errorf("Agy PTY allocator is required")
	}
	if commandRunner == nil {
		return nil, fmt.Errorf("Worker command runner is required")
	}
	if invocation == nil {
		return nil, fmt.Errorf("Worker invocation factory is required")
	}
	if resolveClock == nil {
		return nil, fmt.Errorf("Factory Runtime clock resolver is required")
	}
	if artifactRoots == nil {
		return nil, fmt.Errorf("Factory Runtime artifact root resolver is required")
	}
	if adaptRunner == nil {
		return nil, fmt.Errorf("Worker command runner adapter is required")
	}
	if paths == nil {
		return nil, fmt.Errorf("Factory Session execution-opening filesystem is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("runtime logger is required")
	}
	return &Factory{
		runtimes: runtimes, runtimeEffects: runtimeEffects, commandRunner: commandRunner, allocator: allocator, standalone: build,
		invocation: invocation, resolveClock: resolveClock, artifactRoots: artifactRoots, adaptRunner: adaptRunner, paths: paths,
		logger: logger,
	}, nil
}

// BuildSessionExecutionService constructs the durable execution collaborator
// selected by CLI inputs. Transport packages receive the narrow service with
// its cleanup owner and retain command-lifecycle, parsing, and rendering
// ownership.
func (f *Factory) Build(
	ctx context.Context,
	provider string,
	projectRoot string,
	fixtureCatalogPath string,
	childExecutorMode string,
) (roles.OwnedExecutionService, error) {
	return f.build(ctx, provider, projectRoot, fixtureCatalogPath)
}

// buildWithWorkerEffects constructs an owned runtime-backed execution service
// using the injected external process effect and canonical Workers allocator.
func (f *Factory) buildWithWorkerEffects(
	ctx context.Context,
	providerName string,
	projectRootValue string,
	fixtureCatalogPath string,
	childExecutorMode string,
) (roles.OwnedExecutionService, error) {
	provider, err := normalizeSessionExecutionProvider(providerName)
	if err != nil {
		return nil, err
	}
	if provider != factorysessions.ExecutionProviderJavaScriptRuntime {
		return f.build(ctx, providerName, projectRootValue, fixtureCatalogPath)
	}
	projectRoot, err := f.ResolveProjectRoot(projectRootValue)
	if err != nil {
		return nil, err
	}
	var workerInvocation workers.InvocationExecutor
	if childExecutorMode == factorysessions.ChildExecutorModeLive {
		invocation, invocationErr := f.invocation(f.commandRunner, f.allocator)
		if invocationErr != nil {
			return nil, invocationErr
		}
		workerInvocation = invocation
	}
	execution, err := f.standalone(
		factorysessions.ExecutionProviderJavaScriptRuntime,
		projectRoot,
		"",
		childExecutorMode,
		workerInvocation,
		f.resolveClock(nil),
	)
	if err != nil {
		return nil, err
	}
	return ownedExecutionService{Service: execution}, nil
}

// SessionExecutionBuilderWithEdges binds process-selected worker edges to the
// transport-facing durable execution builder.
func (f *Factory) Builder() roles.ExecutionServiceBuilder {
	return func(ctx context.Context, provider, projectRoot, fixtureCatalogPath, childExecutorMode string) (roles.OwnedExecutionService, error) {
		return f.buildWithWorkerEffects(ctx, provider, projectRoot, fixtureCatalogPath, childExecutorMode)
	}
}

// NewServiceBuilder exposes the lazy durable-session constructor owned by the
// session execution initializer.
func NewServiceBuilder(factory *Factory) roles.ExecutionServiceBuilder {
	return factory.Builder()
}

func (f *Factory) build(
	ctx context.Context,
	providerName string,
	projectRootValue string,
	fixtureCatalogPath string,
) (roles.OwnedExecutionService, error) {
	provider, err := normalizeSessionExecutionProvider(providerName)
	if err != nil {
		return nil, err
	}
	if provider == factorysessions.ExecutionProviderJavaScriptRuntime {
		projectRoot, err := f.ResolveProjectRoot(projectRootValue)
		if err != nil {
			return nil, err
		}
		return f.BuildRuntimeBacked(ctx, projectRoot)
	}
	catalogPath, err := f.resolveFixtureCatalog(fixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := f.standalone(
		factorysessions.ExecutionProviderFake,
		"",
		catalogPath,
		"",
		nil,
		f.resolveClock(nil),
	)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return ownedExecutionService{Service: service}, nil
}

// BuildRuntimeBacked opens one service-owned runtime entity and exposes its
// durable-execution role to CLI commands.
func (f *Factory) BuildRuntimeBacked(
	ctx context.Context,
	projectRoot string,
) (roles.OwnedExecutionService, error) {
	opened, err := f.OpenExecutionRuntime(ctx, factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot: projectRoot,
	})
	if err != nil {
		return nil, err
	}
	return ownedExecutionService{
		Service: opened.Execution,
		close:   opened.Resources.Close,
	}, nil
}

// OpenExecutionRuntime creates per-session domain state through the already-injected
// runtime-opening service. It does not construct an application lifecycle or
// transport.
func (f *Factory) OpenExecutionRuntime(
	ctx context.Context,
	opening factorysessions.ExecutionRuntimeOpeningRequest,
) (roles.OpenedExecutionRuntime, error) {
	if f == nil || f.runtimes == nil {
		return roles.OpenedExecutionRuntime{}, fmt.Errorf("construct runtime-backed execution: runtime opening service is required")
	}
	artifactRoots := f.artifactRoots(opening.SystemConfigHome)
	if strings.TrimSpace(artifactRoots.Logs) == "" {
		return roles.OpenedExecutionRuntime{}, fmt.Errorf("construct runtime-backed execution graph: runtime log root is required")
	}
	if strings.TrimSpace(artifactRoots.Metrics) == "" {
		return roles.OpenedExecutionRuntime{}, fmt.Errorf("construct runtime-backed execution graph: runtime metrics root is required")
	}
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
			Directory: opening.ProjectRoot, ExecutionBaseDir: opening.ProjectRoot,
		},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			SystemConfigHome: opening.SystemConfigHome,
		},
		FactoryRuntime: factory.RuntimeOpeningRequest{
			LogDirectory:     artifactRoots.Logs,
			MetricsDirectory: artifactRoots.Metrics,
		},
	}
	opened, err := f.runtimes.OpenExecutionRuntime(ctx, request, f.runtimeEffects, f.logger)
	if err != nil {
		return roles.OpenedExecutionRuntime{}, fmt.Errorf("construct runtime-backed execution graph: %w", err)
	}
	if opened.Execution == nil {
		if opened.Resources.Close != nil {
			_ = opened.Resources.Close()
		}
		return roles.OpenedExecutionRuntime{}, fmt.Errorf("construct runtime-backed execution graph: durable execution service is required")
	}
	return opened, nil
}

type ownedExecutionService struct {
	durableexecution.Service
	close func() error
}

func (service ownedExecutionService) Close() error {
	if service.close == nil {
		return nil
	}
	return service.close()
}

func normalizeSessionExecutionProvider(value string) (factorysessions.ExecutionProvider, error) {
	switch strings.TrimSpace(value) {
	case "", string(factorysessions.ExecutionProviderFake):
		return factorysessions.ExecutionProviderFake, nil
	case string(factorysessions.ExecutionProviderJavaScriptRuntime):
		return factorysessions.ExecutionProviderJavaScriptRuntime, nil
	default:
		return "", fmt.Errorf("execution provider %q is unsupported: use fake or javascript-runtime", value)
	}
}

func (f *Factory) ResolveProjectRoot(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	if f == nil || f.paths == nil {
		return "", fmt.Errorf("resolve current working directory: Factory Session execution-opening filesystem is required")
	}
	cwd, err := f.paths.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	return cwd, nil
}

func (f *Factory) resolveFixtureCatalog(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	if f == nil || f.paths == nil {
		return "", fmt.Errorf("resolve fixture catalog: Factory Session execution-opening filesystem is required")
	}
	cwd, err := f.paths.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	relative := filepath.FromSlash(factorysessions.ContractFixtureCatalogRelativePath)
	for dir := cwd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, relative)
		if _, statErr := f.paths.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return "", fmt.Errorf(
		"fixture catalog not found; run from the repository root or set --fixture-catalog to %s",
		factorysessions.ContractFixtureCatalogRelativePath,
	)
}
