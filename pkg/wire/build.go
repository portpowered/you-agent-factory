package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

const (
	persistencePhase    = "persistence"
	modelWorkersPhase   = "model and worker/provider services"
	factorySessionPhase = "Factory Session and durable execution services"
	transportPhase      = "transport dependencies"
	sidecarPhase        = "sidecar lifecycles"
)

type namedResource struct {
	phase  string
	closer io.Closer
}

// BuildCLIRunner constructs the local runtime selected by the process root.
// Construction remains inert; initializer activates and shuts down the graph
// when the returned application is run.
func BuildCLIRunner(ctx context.Context, cfg *service.FactoryServiceConfig) (initializer.LocalRuntimeRunner, error) {
	return buildApplicationRunner(ctx, cfg, initializer.ModeCLI)
}

func buildApplicationRunner(
	ctx context.Context,
	cfg *service.FactoryServiceConfig,
	mode initializer.Mode,
) (initializer.LocalRuntimeRunner, error) {
	runtimeCfg := serviceConfigAsRuntimeConfig(cfg)
	if runtimeCfg != nil {
		copied := *runtimeCfg
		runtimeCfg = &copied
		if runtimeCfg.Logger == nil {
			runtimeCfg.Logger = zap.NewNop()
		}
		runtimeCfg.Clock = factory.EnsureClock(runtimeCfg.Clock)
	}
	graph, err := Build(ctx, Inputs{
		Config: runtimeCfg, MCPInput: strings.NewReader(""), MCPOutput: io.Discard,
	})
	if err != nil {
		return nil, err
	}
	application, err := initializer.NewApplication(mode, graph)
	if err != nil {
		if cleanupErr := graph.Close(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close rejected %s application graph: %w", mode, cleanupErr))
		}
		return nil, err
	}
	return application, nil
}

type resourceSet struct {
	mu        sync.Mutex
	resources []namedResource
	closeOnce sync.Once
	closeErr  error
}

// buildPhasedGraph retains the dependency-ordered failure harness used to prove
// construction cleanup. Production callers use Build, whose phases are owned
// entirely by this package.
func buildPhasedGraph(ctx context.Context, inputs phasedInputs) (*phasedGraph, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build application graph: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}
	if err := validatePhasedInputs(inputs); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}

	resources := &resourceSet{}
	runtime, err := buildRuntime(ctx, inputs.Runtime, inputs.Build, resources)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	modelWorkers, err := construct(ctx, modelWorkersPhase, resources, func(ctx context.Context) (constructed[phasedModelWorkerServices], error) {
		return inputs.Build.ModelWorkers(ctx, runtime)
	}, validateModelWorkers)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	sessions, err := construct(ctx, factorySessionPhase, resources, func(ctx context.Context) (constructed[phasedFactorySessionServices], error) {
		return inputs.Build.FactorySessions(ctx, runtime, modelWorkers)
	}, validateFactorySessions)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	return buildEdges(ctx, inputs, runtime, modelWorkers, sessions, resources)
}

func buildRuntime(ctx context.Context, inputs RuntimeInputs, builders phasedBuilders, resources *resourceSet) (phasedRuntimeDependencies, error) {
	persistence, err := construct(ctx, persistencePhase, resources, func(ctx context.Context) (constructed[runtimepersist.Store], error) {
		return builders.Persistence(ctx, inputs)
	}, func(store runtimepersist.Store) error {
		if isNil(store) {
			return errors.New("persistence is required")
		}
		return nil
	})
	if err != nil {
		return phasedRuntimeDependencies{}, err
	}
	return phasedRuntimeDependencies{RuntimeInputs: inputs, Persistence: persistence}, nil
}

func buildEdges(
	ctx context.Context,
	inputs phasedInputs,
	runtime phasedRuntimeDependencies,
	modelWorkers phasedModelWorkerServices,
	sessions phasedFactorySessionServices,
	resources *resourceSet,
) (*phasedGraph, error) {
	transportDeps := newTransportDependencies(modelWorkers, sessions)
	transports, err := construct(ctx, transportPhase, resources, func(ctx context.Context) (constructed[TransportLifecycles], error) {
		return inputs.Build.Transports(ctx, transportDeps)
	}, validateTransports)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	sidecars, err := construct(ctx, sidecarPhase, resources, func(ctx context.Context) (constructed[SidecarLifecycles], error) {
		return inputs.Build.Sidecars(ctx, newSidecarDependencies(inputs.Config, runtime, modelWorkers, sessions))
	}, validateSidecars)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	return newPhasedGraph(inputs.Config, runtime, modelWorkers, sessions, transportDeps, transports, sidecars, resources), nil
}

func construct[T any](
	ctx context.Context,
	phase string,
	resources *resourceSet,
	builder func(context.Context) (constructed[T], error),
	validate func(T) error,
) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("construct %s: %w", phase, err)
	}
	result, err := builder(ctx)
	resources.add(phase, result.Resource)
	if err != nil {
		return zero, fmt.Errorf("construct %s: %w", phase, err)
	}
	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("construct %s: %w", phase, err)
	}
	if err := validate(result.Value); err != nil {
		return zero, fmt.Errorf("construct %s: %w", phase, err)
	}
	return result.Value, nil
}

func failBuild(resources *resourceSet, constructionErr error) error {
	buildErr := fmt.Errorf("build application graph: %w", constructionErr)
	if cleanupErr := resources.Close(); cleanupErr != nil {
		return errors.Join(buildErr, fmt.Errorf("build application graph: cleanup after construction failure: %w", cleanupErr))
	}
	return buildErr
}

func (r *resourceSet) add(phase string, closer io.Closer) {
	if isNil(closer) {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources = append(r.resources, namedResource{phase: phase, closer: closer})
}

func (r *resourceSet) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for index := len(r.resources) - 1; index >= 0; index-- {
			resource := r.resources[index]
			if err := resource.closer.Close(); err != nil {
				r.closeErr = errors.Join(r.closeErr, fmt.Errorf("close %s resource: %w", resource.phase, err))
			}
		}
	})
	return r.closeErr
}
