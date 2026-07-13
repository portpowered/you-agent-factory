package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
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

type resourceSet struct {
	mu        sync.Mutex
	resources []namedResource
	closeOnce sync.Once
	closeErr  error
}

// Build validates and constructs one application graph without activating any
// transport, runtime, scheduler, poller, or dashboard lifecycle.
func Build(ctx context.Context, inputs Inputs) (*Graph, error) {
	if ctx == nil {
		return nil, fmt.Errorf("build application graph: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}
	if err := validateInputs(inputs); err != nil {
		return nil, fmt.Errorf("build application graph: %w", err)
	}

	resources := &resourceSet{}
	runtime, err := buildRuntime(ctx, inputs.Runtime, inputs.Build, resources)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	modelWorkers, err := construct(ctx, modelWorkersPhase, resources, func(ctx context.Context) (Constructed[ModelWorkerServices], error) {
		return inputs.Build.ModelWorkers(ctx, runtime)
	}, validateModelWorkers)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	sessions, err := construct(ctx, factorySessionPhase, resources, func(ctx context.Context) (Constructed[FactorySessionServices], error) {
		return inputs.Build.FactorySessions(ctx, runtime, modelWorkers)
	}, validateFactorySessions)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	return buildEdges(ctx, inputs, runtime, modelWorkers, sessions, resources)
}

func buildRuntime(ctx context.Context, inputs RuntimeInputs, builders Builders, resources *resourceSet) (RuntimeDependencies, error) {
	persistence, err := construct(ctx, persistencePhase, resources, func(ctx context.Context) (Constructed[runtimepersist.Store], error) {
		return builders.Persistence(ctx, inputs)
	}, func(store runtimepersist.Store) error {
		if isNil(store) {
			return errors.New("persistence is required")
		}
		return nil
	})
	if err != nil {
		return RuntimeDependencies{}, err
	}
	return RuntimeDependencies{RuntimeInputs: inputs, Persistence: persistence}, nil
}

func buildEdges(
	ctx context.Context,
	inputs Inputs,
	runtime RuntimeDependencies,
	modelWorkers ModelWorkerServices,
	sessions FactorySessionServices,
	resources *resourceSet,
) (*Graph, error) {
	transportDeps := newTransportDependencies(modelWorkers, sessions)
	transports, err := construct(ctx, transportPhase, resources, func(ctx context.Context) (Constructed[TransportLifecycles], error) {
		return inputs.Build.Transports(ctx, transportDeps)
	}, validateTransports)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	sidecars, err := construct(ctx, sidecarPhase, resources, func(ctx context.Context) (Constructed[SidecarLifecycles], error) {
		return inputs.Build.Sidecars(ctx, newSidecarDependencies(inputs.Config, runtime, modelWorkers, sessions))
	}, validateSidecars)
	if err != nil {
		return nil, failBuild(resources, err)
	}
	return newGraph(inputs.Config, runtime, modelWorkers, sessions, transportDeps, transports, sidecars, resources), nil
}

func construct[T any](
	ctx context.Context,
	phase string,
	resources *resourceSet,
	builder func(context.Context) (Constructed[T], error),
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
