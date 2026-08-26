// Package applicationopening owns the exact Factory Session operation that
// opens and binds one process application from already-injected roles.
package applicationopening

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
// selected by Wire to one opened Factory Session, for the visualization sink
// the caller selected. It owns resolving that opaque selection; Factory
// Sessions never names the presentation service that retains it. This
// operation contains no product lifecycle selection or ordering policy.
type RuntimeAdapter func(
	roles.OpenedApplicationRuntime,
	factorysessions.VisualizationSinkID,
) (factorysessions.BoundProcessComponents, error)

// Service is constructed once by Wire. OpenApplication supplies only
// invocation values to the already-selected runtime-opening and
// application-binding operations.
type Service struct {
	resolveInputs RuntimeInputResolver
	openRuntime   runtimeopening.ApplicationRuntimeOpening
	adaptRuntime  RuntimeAdapter
	planLifecycle roles.LifecyclePlanOperation
}

func New(
	resolveInputs RuntimeInputResolver,
	openRuntime runtimeopening.ApplicationRuntimeOpening,
	adaptRuntime RuntimeAdapter,
	planLifecycle roles.LifecyclePlanOperation,
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
	default:
		return &Service{
			resolveInputs: resolveInputs,
			openRuntime:   openRuntime,
			adaptRuntime:  adaptRuntime,
			planLifecycle: planLifecycle,
		}, nil
	}
}

// OpenApplicationWithCancellation opens one application with the explicit
// invocation-local authority used by hosted administrative controls. The
// authority is an operation input rather than part of the immutable runtime
// opening request.
func (service *Service) OpenApplicationWithCancellation(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	cancellation initializer.InvocationCancellation,
	sinkID factorysessions.VisualizationSinkID,
) (roles.OpenedProcessApplication, error) {
	return service.openApplication(ctx, request, cancellation, sinkID)
}

func (service *Service) openApplication(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	cancellation initializer.InvocationCancellation,
	sinkID factorysessions.VisualizationSinkID,
) (roles.OpenedProcessApplication, error) {
	if service == nil || service.resolveInputs == nil || service.openRuntime == nil || service.adaptRuntime == nil || service.planLifecycle == nil {
		return roles.OpenedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	opened, err := service.openRuntimeForRequest(ctx, request)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	opened.Cancellation = cancellation
	if opened.HistoricalReplay != nil {
		return service.openHistoricalReplayApplication(opened)
	}
	return service.bindLiveApplication(opened, sinkID)
}

func (service *Service) openRuntimeForRequest(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	inputs, err := service.resolveInputs(ctx, request)
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
	components, err := service.adaptRuntime(opened, sinkID)
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
	}
	plan, err := service.planLifecycle(roles.LifecyclePlanRequest{
		Runtime:     opened.Process,
		Components:  components,
		Close:       opened.Resources.Close,
		OrderlyStop: opened.OrderlyStop,
	})
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	return roles.OpenedProcessApplication{
		Plan:            plan,
		Diagnostics:     opened.Resources.Diagnostics,
		Ready:           runtimeReady(opened.Process),
		CleanInvocation: opened.FactoryRuntime,
		ReplayMetadataWarnings: append(
			[]recordings.MetadataMismatchWarning(nil),
			opened.ReplayMetadataWarnings...,
		),
		HostedInvocation: hostedInvocation(opened.FactorySessions),
	}, nil
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
		ReplayMetadataWarnings: append(
			[]recordings.MetadataMismatchWarning(nil),
			opened.ReplayMetadataWarnings...,
		),
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
