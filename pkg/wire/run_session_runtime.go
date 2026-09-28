package wire

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"go.uber.org/zap"
)

func provideRunRuntimeRunnerBuilder(
	root *factorysessionwire.Root,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (runcli.RuntimeRunnerBuilder, error) {
	if root == nil || visualizationFactory == nil || visualizationSinks == nil || httpBinding == nil || newRunner == nil || planLifecycle == nil {
		return nil, errors.New("run Factory Session: process root and presentation operations are required")
	}
	return func(ctx context.Context, request *factorysessions.SessionStartRequest, cancellation initializer.InvocationCancellation, sinkID factorysessions.VisualizationSinkID) (initializer.LocalRuntimeRunner, error) {
		if ctx == nil {
			return nil, errors.New("build run application: context is required")
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build run application: %w", err)
		}
		if request == nil {
			return nil, errors.New("build run application: session request is required")
		}
		inspection, historical, err := root.InspectHistoricalApplication(ctx, *request)
		if err != nil {
			return nil, fmt.Errorf("inspect historical Factory Session: %w", err)
		}
		if historical {
			return buildHistoricalRunSession(inspection, planLifecycle)
		}
		runner, err := buildCanonicalRunSession(*request, cancellation, sinkID, root, edges, visualizationFactory, visualizationSinks, httpBinding, newRunner, planLifecycle)
		if err != nil {
			return nil, err
		}
		wrapped := runcli.WithHostedInvocation(runner, root)
		wrapped = runcli.WithCleanInvocationSnapshot(wrapped, runSessionCleanSnapshot{root: root, process: runner.process})
		return wrapped, nil
	}, nil
}

// runSessionProcess is invocation-local lifecycle coordination. Its only
// product identity is the canonical Factory Session ID returned by Root.Start.
type runSessionProcess struct {
	root      *factorysessionwire.Root
	request   factorysessions.SessionStartRequest
	mu        sync.RWMutex
	sessionID string
	ready     chan initializer.RuntimeHostBinding
}

func newRunSessionProcess(root *factorysessionwire.Root, request factorysessions.SessionStartRequest) *runSessionProcess {
	process := &runSessionProcess{root: root, request: request}
	if request.RuntimeSelection != nil && request.RuntimeSelection.Host.Port > 0 {
		process.ready = make(chan initializer.RuntimeHostBinding, 1)
	}
	return process
}

func (process *runSessionProcess) ID() string {
	process.mu.RLock()
	defer process.mu.RUnlock()
	return process.sessionID
}

func (process *runSessionProcess) Start(ctx, runCtx context.Context) error {
	result, err := process.root.Start(ctx, process.request)
	if err != nil {
		return err
	}
	process.mu.Lock()
	process.sessionID = result.SessionID
	process.mu.Unlock()
	if process.ready != nil {
		ready, readyErr := process.root.ApplicationReady(result.SessionID)
		if readyErr != nil {
			return readyErr
		}
		go func() {
			defer close(process.ready)
			if ready == nil {
				return
			}
			select {
			case binding, ok := <-ready:
				if ok {
					process.ready <- initializer.RuntimeHostBinding{Host: binding.Host, Port: binding.Port}
				}
			case <-runCtx.Done():
			}
		}()
	}
	return nil
}

func (*runSessionProcess) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	// Root.Start has completed the worker lifecycle before returning.
	return func(context.Context) error { return nil }, nil
}

func (process *runSessionProcess) RunTransport(ctx context.Context, handler http.Handler) error {
	return process.root.RunApplicationTransport(ctx, process.ID(), handler)
}

func (process *runSessionProcess) Stop(ctx context.Context) error {
	if process.ID() == "" {
		return nil
	}
	return process.root.StopApplicationRuntime(ctx, process.ID())
}

func (process *runSessionProcess) close() error {
	if process.ID() == "" {
		return nil
	}
	return process.root.CloseApplicationSession(context.Background(), process.ID())
}

func (process *runSessionProcess) RuntimeHostReady() <-chan initializer.RuntimeHostBinding {
	return process.ready
}

type runSessionRunner struct {
	*runtimeapplication.ManagedRunner
	root    *factorysessionwire.Root
	process *runSessionProcess
}

func (runner runSessionRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	if runner.process.ID() != "" {
		if diagnostics, err := runner.root.ApplicationDiagnostics(runner.process.ID()); err == nil {
			return runtimeartifact.Diagnostics(diagnostics)
		}
	}
	return runner.ManagedRunner.RuntimeLogDiagnostics()
}

func (runner runSessionRunner) ResumeRecoveryMetadata() *recordings.ResumeRecoveryMetadata {
	if runner.process.ID() == "" {
		return nil
	}
	metadata, err := runner.root.ApplicationResumeRecoveryMetadata(runner.process.ID())
	if err != nil || metadata == nil {
		return nil
	}
	return metadata
}

func (runner runSessionRunner) ReplayMetadataWarnings() []recordings.MetadataMismatchWarning {
	if runner.process.ID() == "" {
		return nil
	}
	warnings, err := runner.root.ApplicationReplayMetadataWarnings(runner.process.ID())
	if err != nil {
		return nil
	}
	return warnings
}

type runSessionCleanSnapshot struct {
	root    *factorysessionwire.Root
	process *runSessionProcess
}

func (snapshot runSessionCleanSnapshot) CleanInvocationSnapshot(ctx context.Context) (factoryruntime.CleanInvocationSnapshot, error) {
	if snapshot.process.ID() == "" {
		return factoryruntime.CleanInvocationSnapshot{}, factoryruntime.ErrNotRunning
	}
	return snapshot.root.ApplicationCleanSnapshot(ctx, snapshot.process.ID())
}

func (snapshot runSessionCleanSnapshot) ControlWaitToComplete(request factoryruntime.WaitToCompleteRequest) factoryruntime.WaitToCompleteResult {
	if snapshot.process.ID() == "" {
		return factoryruntime.WaitToCompleteResult{}
	}
	return snapshot.root.ApplicationControlWaitToComplete(snapshot.process.ID(), request)
}

func buildCanonicalRunSession(
	request factorysessions.SessionStartRequest,
	cancellation initializer.InvocationCancellation,
	sinkID factorysessions.VisualizationSinkID,
	root *factorysessionwire.Root,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (runSessionRunner, error) {
	if root == nil || visualizationFactory == nil || visualizationSinks == nil || httpBinding == nil || newRunner == nil || planLifecycle == nil {
		return runSessionRunner{}, errors.New("run Factory Session: process root and presentation operations are required")
	}
	process := newRunSessionProcess(root, request)
	var visualization factoryvisualization.Service
	visualizationComponent := lifecycle.Functions{
		StartFunc: func(context.Context) error {
			sink, err := selectVisualizationSink(visualizationSinks, sinkID)
			if err != nil {
				return err
			}
			if edges.FactoryVisualizationSink != nil {
				sink = edges.FactoryVisualizationSink
			}
			if sink == nil {
				return nil
			}
			view, err := root.SessionPresentation(process.ID())
			if err != nil {
				return err
			}
			logger := view.Logger
			if logger == nil {
				logger = zap.NewNop()
			}
			visualization, err = visualizationFactory(view.Reader, view.Projections, view.Clock, sink, func(err error) { logger.Error("Factory visualization failed", zap.Error(err)) })
			if err != nil {
				return err
			}
			if edges.FactoryVisualizationRootObserver != nil {
				edges.FactoryVisualizationRootObserver(visualization)
			}
			return nil
		},
		StopFunc: func(ctx context.Context) error {
			if visualization == nil {
				return nil
			}
			_, err := visualization.StopDrain(ctx, factoryvisualization.StopDrainRequest{})
			return err
		},
	}
	transport := newRunner(func(ctx context.Context) error {
		handler, err := httpBinding(root, process.ID(), cancellation)
		if err != nil {
			return err
		}
		return process.RunTransport(ctx, handler)
	})
	plan, err := planLifecycle(factorysessionwire.LifecyclePlanRequest{
		Runtime:     process,
		Components:  factorysessions.BoundProcessComponents{Transport: transport, Visualization: visualizationComponent},
		Close:       process.close,
		OrderlyStop: func(ctx context.Context) error { return root.StopApplicationOrderly(ctx, process.ID()) },
	})
	if err != nil {
		return runSessionRunner{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	managed, err := runtimeapplication.NewManagedRunner(plan, runtimeartifact.Diagnostics{})
	if err != nil {
		return runSessionRunner{}, err
	}
	managed.SetRuntimeHostReady(process.RuntimeHostReady())
	runner := runSessionRunner{ManagedRunner: managed, root: root, process: process}
	return runner, nil
}

type historicalRunProcess struct{}

func (historicalRunProcess) Start(context.Context, context.Context) error { return nil }
func (historicalRunProcess) StartWorkers(context.Context) (factorysessions.RuntimeStop, error) {
	return func(context.Context) error { return nil }, nil
}
func (historicalRunProcess) RunTransport(context.Context, http.Handler) error { return nil }
func (historicalRunProcess) Stop(context.Context) error                       { return nil }

func buildHistoricalRunSession(
	inspection factorysessionwire.HistoricalApplicationInspection,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (initializer.LocalRuntimeRunner, error) {
	plan, err := planLifecycle(factorysessionwire.LifecyclePlanRequest{
		Runtime:    historicalRunProcess{},
		Components: factorysessions.BoundProcessComponents{Transport: lifecycle.NewRunner(func(context.Context) error { return nil })},
		Close:      inspection.Close,
	})
	if err != nil {
		return nil, err
	}
	managed, err := runtimeapplication.NewManagedRunner(plan, runtimeartifact.Diagnostics(inspection.Diagnostics))
	if err != nil {
		return nil, lifecycle.CloseResources(plan.Resources, err)
	}
	runner := initializer.LocalRuntimeRunner(managed)
	runner = runcli.WithHostedInvocation(runner, nil, inspection.ResumeRecoveryMetadata)
	runner = runcli.WithReplayMetadataWarnings(runner, inspection.ReplayMetadataWarnings)
	runner = runcli.WithHistoricalReplay(runner, inspection.Replay)
	return runner, nil
}
