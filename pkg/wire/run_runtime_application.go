package wire

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"go.uber.org/zap"
)

type openedProcessApplication struct {
	Plan                   lifecycle.Plan
	Diagnostics            factoryruntime.RuntimeLogDiagnostics
	Ready                  <-chan initializer.RuntimeHostBinding
	CleanInvocation        factoryruntime.Service
	ReplayMetadataWarnings []recordings.MetadataMismatchWarning
	ResumeRecoveryMetadata *recordings.ResumeRecoveryMetadata
	// HostedInvocation is a narrow operation result for the hosted CLI path;
	// it is not the opened runtime's HTTP service table.
	HostedInvocation runcli.HostedInvocationOperation
	HistoricalReplay *factorysessions.HistoricalReplayInspection
}

func provideRunRuntimeRunnerBuilder(
	build initializer.RuntimeRunnerBuilder,
	openRuntime factorysessionwire.ApplicationRuntimeOpening,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (runcli.RuntimeRunnerBuilder, error) {
	if build == nil || openRuntime == nil || planLifecycle == nil {
		return nil, errors.New("run application lifecycle builder and Factory Session opener are required")
	}
	if visualizationFactory == nil || visualizationSinks == nil || httpBinding == nil || newRunner == nil {
		return nil, errors.New("Factory visualization, HTTP binding, and lifecycle component operations are required")
	}
	return func(
		ctx context.Context,
		request *factorysessions.RuntimeOpeningRequest,
		cancellation initializer.InvocationCancellation,
		sinkID factorysessions.VisualizationSinkID,
	) (initializer.LocalRuntimeRunner, error) {
		if ctx == nil {
			return nil, errors.New("build run application: context is required")
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build run application: %w", err)
		}
		if request == nil {
			return nil, errors.New("build run application: opening request is required")
		}

		opened, err := openWireApplicationWithCancellation(ctx, request, cancellation, sinkID, openRuntime, edges, visualizationFactory, visualizationSinks, httpBinding, newRunner, planLifecycle)
		if err != nil {
			return nil, err
		}
		replay := opened.HistoricalReplay
		replayMetadataWarnings := append(
			[]recordings.MetadataMismatchWarning(nil),
			opened.ReplayMetadataWarnings...,
		)
		hostedInvocation := opened.HostedInvocation
		cleanInvocation := opened.CleanInvocation

		var runner initializer.LocalRuntimeRunner
		if replay != nil {
			runner, err = runtimeapplication.NewManagedRunner(
				opened.Plan,
				platformruntimeartifact.Diagnostics(opened.Diagnostics),
			)
			if err != nil {
				return nil, lifecycle.CloseResources(opened.Plan.Resources, err)
			}
			if managed, ok := runner.(interface {
				SetRuntimeHostReady(<-chan initializer.RuntimeHostBinding)
			}); ok {
				managed.SetRuntimeHostReady(opened.Ready)
			}
		} else {
			if build == nil {
				return nil, errors.New("build run application: lifecycle builder is required")
			}
			runner, err = build(ctx, func(context.Context) (initializer.OpenedApplication, error) {
				return initializer.OpenedApplication{
					Plan:        opened.Plan,
					Diagnostics: platformruntimeartifact.Diagnostics(opened.Diagnostics),
					Ready:       opened.Ready,
				}, nil
			})
			if err != nil {
				return nil, err
			}
		}
		runner = runcli.WithHostedInvocation(runner, hostedInvocation, opened.ResumeRecoveryMetadata)
		runner = runcli.WithCleanInvocationSnapshot(runner, cleanInvocation)
		runner = runcli.WithReplayMetadataWarnings(runner, replayMetadataWarnings)
		return runcli.WithHistoricalReplay(runner, replay), nil
	}, nil
}

func openWireApplicationWithCancellation(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	cancellation initializer.InvocationCancellation,
	sinkID factorysessions.VisualizationSinkID,
	openRuntime factorysessionwire.ApplicationRuntimeOpening,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (openedProcessApplication, error) {
	if openRuntime == nil || planLifecycle == nil {
		return openedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	opened, err := openWireRuntimeForRequest(ctx, request, openRuntime)
	if err != nil {
		return openedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	opened.Cancellation = cancellation
	if opened.HistoricalReplay != nil {
		return openWireHistoricalReplayApplication(opened, planLifecycle)
	}
	return bindWireLiveApplication(opened, sinkID, edges, visualizationFactory, visualizationSinks, httpBinding, newRunner, planLifecycle)
}

func openWireRuntimeForRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	openRuntime factorysessionwire.ApplicationRuntimeOpening,
) (factorysessionwire.OpenedApplicationRuntime, error) {
	configured, err := copyWireRuntimeOpeningRequest(ctx, request)
	if err != nil {
		return factorysessionwire.OpenedApplicationRuntime{}, fmt.Errorf("resolve runtime inputs: %w", err)
	}
	opened, err := openRuntime.OpenApplicationRuntime(ctx, configured)
	if err != nil {
		return factorysessionwire.OpenedApplicationRuntime{}, err
	}
	return opened, nil
}

func copyWireRuntimeOpeningRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (*factorysessions.RuntimeOpeningRequest, error) {
	switch {
	case ctx == nil:
		return nil, errors.New("context is required")
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case request == nil:
		return nil, errors.New("runtime opening request is required")
	default:
		configured := *request
		return &configured, nil
	}
}

func bindWireProcessComponents(
	opened factorysessionwire.OpenedApplicationRuntime,
	sinkID factorysessions.VisualizationSinkID,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
) (factorysessions.BoundProcessComponents, error) {
	sink, err := selectVisualizationSink(visualizationSinks, sinkID)
	if err != nil {
		return factorysessions.BoundProcessComponents{}, err
	}
	if fixedSink := edges.FactoryVisualizationSink; fixedSink != nil {
		sink = fixedSink
	}
	var visualization lifecycle.Component
	if sink != nil {
		logger := opened.Resources.Logger
		if logger == nil {
			logger = zap.NewNop()
		}
		visualized, err := visualizationFactory(
			opened.Visualization.Reader, opened.Visualization.Projections, opened.Resources.Clock, sink,
			func(err error) {
				logger.Error("Factory visualization failed", zap.Error(err))
			},
		)
		if err != nil {
			return factorysessions.BoundProcessComponents{}, err
		}
		if fixedRootObserver := edges.FactoryVisualizationRootObserver; fixedRootObserver != nil {
			fixedRootObserver(visualized)
		}
		// Factory Session lifecycle must not auto-activate Visualization.
		// Peers leave the composed root inert until explicit Activate.
		visualization = lifecycle.Functions{
			StartFunc: func(context.Context) error { return nil },
			StopFunc: func(ctx context.Context) error {
				_, err := visualized.StopDrain(ctx, factoryvisualization.StopDrainRequest{})
				return err
			},
		}
	}
	handler, err := httpBinding(opened)
	if err != nil {
		return factorysessions.BoundProcessComponents{}, err
	}
	transport := newRunner(func(ctx context.Context) error {
		return opened.Process.RunTransport(ctx, handler)
	})
	return factorysessions.BoundProcessComponents{
		Transport:     transport,
		Visualization: visualization,
	}, nil
}

func bindWireLiveApplication(
	opened factorysessionwire.OpenedApplicationRuntime,
	sinkID factorysessions.VisualizationSinkID,
	edges serviceedges.Edges,
	visualizationFactory factoryvisualization.RuntimeFactory,
	visualizationSinks factoryvisualization.RuntimeSinkOwner,
	httpBinding httpRuntimeBinding,
	newRunner lifecycle.RunnerFactory,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (openedProcessApplication, error) {
	components, err := bindWireProcessComponents(opened, sinkID, edges, visualizationFactory, visualizationSinks, httpBinding, newRunner)
	if err != nil {
		err = closeWireOpenedRuntime(opened, err)
		return openedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
	}
	plan, err := planLifecycle(factorysessionwire.LifecyclePlanRequest{
		Runtime:     opened.Process,
		Components:  components,
		Close:       opened.Resources.Close,
		OrderlyStop: opened.OrderlyStop,
	})
	if err != nil {
		err = closeWireOpenedRuntime(opened, err)
		return openedProcessApplication{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	return openedProcessApplication{
		Plan:            plan,
		Diagnostics:     opened.Resources.Diagnostics,
		Ready:           wireRuntimeReady(opened.Process),
		CleanInvocation: opened.FactoryRuntime,
		ReplayMetadataWarnings: append(
			[]recordings.MetadataMismatchWarning(nil),
			opened.ReplayMetadataWarnings...,
		),
		HostedInvocation:       wireHostedInvocation(opened.FactorySessions),
		ResumeRecoveryMetadata: cloneWireResumeRecoveryMetadata(opened.ResumeRecoveryMetadata),
	}, nil
}

func openWireHistoricalReplayApplication(
	opened factorysessionwire.OpenedApplicationRuntime,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (openedProcessApplication, error) {
	plan, err := planLifecycle(factorysessionwire.LifecyclePlanRequest{
		Runtime: opened.Process,
		Components: factorysessions.BoundProcessComponents{
			Transport: lifecycle.NewRunner(func(context.Context) error { return nil }),
		},
		Close: opened.Resources.Close,
	})
	if err != nil {
		err = closeWireOpenedRuntime(opened, err)
		return openedProcessApplication{}, fmt.Errorf("plan Factory Session historical replay lifecycle: %w", err)
	}
	return openedProcessApplication{
		Plan:                   plan,
		Diagnostics:            opened.Resources.Diagnostics,
		HistoricalReplay:       opened.HistoricalReplay,
		ResumeRecoveryMetadata: cloneWireResumeRecoveryMetadata(opened.ResumeRecoveryMetadata),
		ReplayMetadataWarnings: append(
			[]recordings.MetadataMismatchWarning(nil),
			opened.ReplayMetadataWarnings...,
		),
	}, nil
}

func wireHostedInvocation(service factorysessions.Service) runcli.HostedInvocationOperation {
	if service == nil {
		return nil
	}
	operation, _ := service.(runcli.HostedInvocationOperation)
	return operation
}

type wireRuntimeReadySource interface {
	RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding
}

func wireRuntimeReady(process factorysessionwire.ProcessRuntime) <-chan factorysessions.RuntimeHostBinding {
	if source, ok := process.(wireRuntimeReadySource); ok {
		return source.RuntimeHostReady()
	}
	return nil
}

func closeWireOpenedRuntime(opened factorysessionwire.OpenedApplicationRuntime, cause error) error {
	if opened.Resources.Close == nil {
		return cause
	}
	return errors.Join(cause, opened.Resources.Close())
}

func cloneWireResumeRecoveryMetadata(
	metadata *recordings.ResumeRecoveryMetadata,
) *recordings.ResumeRecoveryMetadata {
	if metadata == nil {
		return nil
	}
	clone := *metadata
	return &clone
}
