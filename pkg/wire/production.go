package wire

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/composebridge"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	mcpserver "github.com/portpowered/infinite-you/pkg/mcp/server"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
)

// Inputs contains normalized process configuration and the stdio edges
// required to construct every currently supported transport. Build
// copies Config before normalization so graph construction does not mutate the
// caller's configuration object.
type Inputs struct {
	Config    *runtimehost.Config
	MCPInput  io.Reader
	MCPOutput io.Writer
}

// Build eagerly constructs the real runtime core, domain services,
// and transport lifecycle handles without starting listeners or goroutines.
func Build(ctx context.Context, inputs Inputs) (*Graph, error) {
	if err := validateProductionInputs(ctx, inputs); err != nil {
		return nil, fmt.Errorf("build production application graph: %w", err)
	}

	cfg := *inputs.Config
	core, err := composebridge.BuildCore(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("build production application graph: construct runtime core: %w", err)
	}
	resources := &resourceSet{}
	if bundle := core.StartupBundle(); bundle != nil {
		resources.add("runtime core", closeFunc(func() error {
			return composebridge.CloseRuntimeBundleSinks(bundle.LogSink, bundle.MetricsSink)
		}))
	}

	graph, err := assembleProductionGraph(core, &cfg, inputs, resources)
	if err != nil {
		return nil, failProductionBuild(resources, err)
	}
	return graph, nil
}

func validateProductionInputs(ctx context.Context, inputs Inputs) error {
	switch {
	case ctx == nil:
		return errors.New("context is required")
	case ctx.Err() != nil:
		return ctx.Err()
	case inputs.Config == nil:
		return errors.New("config is required")
	case inputs.Config.Logger == nil:
		return errors.New("config.logger is required")
	case isNil(inputs.Config.Clock):
		return errors.New("config.clock is required")
	case inputs.MCPInput == nil:
		return errors.New("mcpInput is required")
	case inputs.MCPOutput == nil:
		return errors.New("mcpOutput is required")
	default:
		return nil
	}
}

func assembleProductionGraph(
	core *runtimehost.Core,
	cfg *runtimehost.Config,
	inputs Inputs,
	resources *resourceSet,
) (*Graph, error) {
	if core == nil || core.StartupBundle() == nil {
		return nil, errors.New("construct runtime core: startup runtime bundle is required")
	}
	host := runtimehost.NewHostFromCore(core)
	shell := runtimehost.HostShell{Host: host}
	host = runtimehost.AttachFactorySaveCollaborator(shell, runtimehost.ProvideFactorySaveCollaborator(shell, cfg))

	models := host.ModelService()
	sessions := host.SessionAPI()
	definition := host.FactoryDefinitionAPI()
	apiSurface, err := apisurface.NewSessionAPISurface(
		sessions,
		models,
		definition,
		host.InvocationAPI(),
		host.DurableExecutionAPI(),
	)
	if err != nil {
		return nil, fmt.Errorf("construct API transport dependencies: %w", err)
	}
	mcp, err := mcpserver.New(mcpserver.Options{
		Client: mcpfactorysession.NewClientWithService(core.DurableExecution()),
	})
	if err != nil {
		return nil, fmt.Errorf("construct MCP transport dependencies: %w", err)
	}

	bundle := core.StartupBundle()
	runtimeInputs := RuntimeInputs{
		FactoryRootDir:   core.FactoryRootDir(),
		ExecutionBaseDir: cfg.ExecutionBaseDir,
		Logger:           core.BaseLogger(),
		Clock:            core.Clock(),
	}
	if runtimeInputs.ExecutionBaseDir == "" {
		runtimeInputs.ExecutionBaseDir = core.FactoryRootDir()
	}
	transport := TransportDependencies{
		API:               apiSurface,
		Models:            models,
		FactoryDefinition: definition,
		FactorySessions:   sessions,
		DurableExecution:  core.DurableExecution(),
	}
	return &Graph{
		Config:            bundle.RuntimeCfg,
		Runtime:           runtimeInputs,
		RuntimeLog:        host.RuntimeLogDiagnostics(),
		Models:            models,
		Workers:           core.WorkersScheduler(),
		WorkerProvider:    core.RuntimeBuild(),
		SessionRegistry:   core.Sessions(),
		FactorySessions:   sessions,
		FactoryDefinition: definition,
		DurableExecution:  core.DurableExecution(),
		Transport:         transport,
		Transports: TransportLifecycles{
			API: newRunnerLifecycle(func(runCtx context.Context) error {
				return host.RunWithAPISurface(runCtx, apiSurface)
			}),
			CLI: newRunnerLifecycle(func(runCtx context.Context) error {
				return host.RunWithAPISurface(runCtx, apiSurface)
			}),
			MCP: newRunnerLifecycle(func(runCtx context.Context) error {
				return mcp.ServeStdio(runCtx, inputs.MCPInput, inputs.MCPOutput)
			}),
		},
		resources: resources,
	}, nil
}

func failProductionBuild(resources *resourceSet, constructionErr error) error {
	result := fmt.Errorf("build production application graph: %w", constructionErr)
	if cleanupErr := resources.Close(); cleanupErr != nil {
		return errors.Join(result, fmt.Errorf("cleanup after production graph construction failure: %w", cleanupErr))
	}
	return result
}

type closeFunc func() error

func (fn closeFunc) Close() error { return fn() }

type runnerLifecycle struct {
	run      func(context.Context) error
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	runErr   error
	stopOnce sync.Once
	stopErr  error
}

func newRunnerLifecycle(run func(context.Context) error) *runnerLifecycle {
	return &runnerLifecycle{run: run}
}

func (l *runnerLifecycle) Start(ctx context.Context) error {
	if l == nil || l.run == nil {
		return errors.New("start transport lifecycle: runner is required")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done != nil {
		return errors.New("start transport lifecycle: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.done = make(chan struct{})
	go func() {
		err := l.run(runCtx)
		l.mu.Lock()
		l.runErr = err
		close(l.done)
		l.mu.Unlock()
	}()
	return nil
}

// Wait blocks until the lifecycle runner exits or ctx is canceled. It does not
// stop the runner; initializer owns cancellation and shutdown ordering.
func (l *runnerLifecycle) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	done := l.done
	l.mu.Unlock()
	if done == nil {
		return errors.New("wait for transport lifecycle: not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *runnerLifecycle) Stop(context.Context) error {
	if l == nil {
		return nil
	}
	l.stopOnce.Do(func() {
		l.mu.Lock()
		cancel, done := l.cancel, l.done
		l.mu.Unlock()
		if cancel == nil || done == nil {
			return
		}
		cancel()
		<-done
		l.mu.Lock()
		err := l.runErr
		l.mu.Unlock()
		if err != nil && !errors.Is(err, context.Canceled) {
			l.stopErr = err
		}
	})
	return l.stopErr
}
