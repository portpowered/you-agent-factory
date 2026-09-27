package wire

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/initializer/runtimeapplication"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

// runRuntimeAdapter binds the exact HTTP and optional visualization components
// selected by Wire to one opened Factory Session.
type runRuntimeAdapter func(
	factorysessionwire.OpenedApplicationRuntime,
	factorysessions.VisualizationSinkID,
) (factorysessions.BoundProcessComponents, error)

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
	adaptRuntime runRuntimeAdapter,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (runcli.RuntimeRunnerBuilder, error) {
	if build == nil || openRuntime == nil || adaptRuntime == nil || planLifecycle == nil {
		return nil, errors.New("run application lifecycle builder and Factory Session opener are required")
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

		opened, err := openWireApplicationWithCancellation(ctx, request, cancellation, sinkID, openRuntime, adaptRuntime, planLifecycle)
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
	adaptRuntime runRuntimeAdapter,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (openedProcessApplication, error) {
	if openRuntime == nil || adaptRuntime == nil || planLifecycle == nil {
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
	return bindWireLiveApplication(opened, sinkID, adaptRuntime, planLifecycle)
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

func bindWireLiveApplication(
	opened factorysessionwire.OpenedApplicationRuntime,
	sinkID factorysessions.VisualizationSinkID,
	adaptRuntime runRuntimeAdapter,
	planLifecycle factorysessionwire.LifecyclePlanOperation,
) (openedProcessApplication, error) {
	components, err := adaptRuntime(opened, sinkID)
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
