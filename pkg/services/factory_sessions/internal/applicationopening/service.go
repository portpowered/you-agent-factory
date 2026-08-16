// Package applicationopening owns the exact Factory Session operation that
// opens and binds one process application from already-injected roles.
package applicationopening

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// RuntimeInputs are the resolved invocation values selected by the canonical
// injector.
type RuntimeInputs struct {
	Request *factorysessions.RuntimeOpeningRequest
}

type RuntimeInputResolver func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
) (RuntimeInputs, error)

// RuntimeAdapter binds the exact HTTP and optional visualization components
// selected by Wire to one opened Factory Session. It contains no product
// lifecycle selection or ordering policy.
type RuntimeAdapter func(
	roles.OpenedApplicationRuntime,
	factoryvisualization.Sink,
) (factorysessions.BoundProcessComponents, error)

// Service is constructed once by Wire. OpenApplication supplies only
// invocation values to the already-selected runtime-opening and
// application-binding operations.
type Service struct {
	resolveInputs RuntimeInputResolver
	openRuntime   runtimeopening.ApplicationRuntimeOpening
	adaptRuntime  RuntimeAdapter
	planLifecycle roles.LifecyclePlanOperation
	visualization factoryvisualization.RuntimeSinkOwner
}

func New(
	resolveInputs RuntimeInputResolver,
	openRuntime runtimeopening.ApplicationRuntimeOpening,
	adaptRuntime RuntimeAdapter,
	planLifecycle roles.LifecyclePlanOperation,
	visualization factoryvisualization.RuntimeSinkOwner,
) (*Service, error) {
	switch {
	case resolveInputs == nil:
		return nil, errors.New("construct application opener: runtime input resolver is required")
	case openRuntime == nil:
		return nil, errors.New("construct application opener: runtime opening service is required")
	case adaptRuntime == nil:
		return nil, errors.New("construct application opener: application adapter is required")
	case planLifecycle == nil:
		return nil, errors.New("construct application opener: lifecycle plan operation is required")
	case visualization == nil:
		return nil, errors.New("construct application opener: Factory Visualization sink owner is required")
	default:
		return &Service{
			resolveInputs: resolveInputs,
			openRuntime:   openRuntime,
			adaptRuntime:  adaptRuntime,
			planLifecycle: planLifecycle,
			visualization: visualization,
		}, nil
	}
}

func (service *Service) OpenApplication(
	ctx context.Context,
	request roles.ApplicationOpeningRequest,
) (roles.OpenedProcessApplication, error) {
	if service == nil || service.resolveInputs == nil || service.openRuntime == nil || service.adaptRuntime == nil || service.planLifecycle == nil || service.visualization == nil {
		return roles.OpenedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	opened, err := service.openRuntimeForRequest(ctx, request)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	if opened.HistoricalReplay != nil {
		return service.openHistoricalReplayApplication(opened)
	}
	return service.bindLiveApplication(opened, request.VisualizationSinkID)
}

func (service *Service) openRuntimeForRequest(
	ctx context.Context,
	request roles.ApplicationOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	inputs, err := service.resolveInputs(ctx, request.Runtime)
	if err != nil {
		return roles.OpenedApplicationRuntime{}, fmt.Errorf("resolve runtime inputs: %w", err)
	}
	opened, err := service.openRuntime.OpenApplicationRuntime(ctx, inputs.Request)
	if err != nil {
		return roles.OpenedApplicationRuntime{}, err
	}
	return opened, nil
}

func (service *Service) bindLiveApplication(
	opened roles.OpenedApplicationRuntime,
	sinkID factorysessions.VisualizationSinkID,
) (roles.OpenedProcessApplication, error) {
	var visualizationSink factoryvisualization.Sink
	if sinkID != "" {
		var ok bool
		visualizationSink, ok = service.visualization.RuntimeSink(factoryvisualization.RuntimeSinkID(sinkID))
		if !ok {
			err := closeOpenedRuntime(opened, fmt.Errorf("Factory Visualization sink %q is unavailable", sinkID))
			return roles.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
		}
	}
	components, err := service.adaptRuntime(opened, visualizationSink)
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
	}
	plan, err := service.planLifecycle(roles.LifecyclePlanRequest{
		Runtime:    opened.Process,
		Components: components,
		Close:      opened.Resources.Close,
	})
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	return roles.OpenedProcessApplication{
		Plan:             plan,
		Diagnostics:      opened.Resources.Diagnostics,
		Ready:            runtimeReady(opened.Process),
		CleanInvocation:  cleanInvocationProvider(opened.HTTP.FactoryRuntime),
		HostedInvocation: hostedInvocation(opened.HTTP.FactorySessions),
	}, nil
}

func cleanInvocationProvider(service factoryruntime.Service) factoryruntime.CleanInvocationSnapshotProvider {
	provider, _ := service.(factoryruntime.CleanInvocationSnapshotProvider)
	return provider
}

func (service *Service) openHistoricalReplayApplication(
	opened roles.OpenedApplicationRuntime,
) (roles.OpenedProcessApplication, error) {
	plan, err := service.planLifecycle(roles.LifecyclePlanRequest{
		Runtime: opened.Process,
		Components: factorysessions.BoundProcessComponents{
			Transport: lifecycle.NewRunner(func(context.Context) error { return nil }),
		},
		Close: opened.Resources.Close,
	})
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("plan Factory Session historical replay lifecycle: %w", err)
	}
	return roles.OpenedProcessApplication{
		Plan:             plan,
		Diagnostics:      opened.Resources.Diagnostics,
		HistoricalReplay: opened.HistoricalReplay,
	}, nil
}

func hostedInvocation(service factorysessions.Service) roles.HostedInvocationOperation {
	if service == nil {
		return nil
	}
	operation, _ := service.(roles.HostedInvocationOperation)
	return operation
}

type runtimeReadySource interface {
	RuntimeHostReady() <-chan factorysessions.RuntimeHostBinding
}

func runtimeReady(process roles.ProcessRuntime) <-chan factorysessions.RuntimeHostBinding {
	if source, ok := process.(runtimeReadySource); ok {
		return source.RuntimeHostReady()
	}
	return nil
}

func closeOpenedRuntime(opened roles.OpenedApplicationRuntime, cause error) error {
	if opened.Resources.Close == nil {
		return cause
	}
	return errors.Join(cause, opened.Resources.Close())
}
