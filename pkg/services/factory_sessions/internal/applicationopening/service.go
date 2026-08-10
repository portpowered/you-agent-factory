// Package applicationopening owns the exact Factory Session operation that
// opens and binds one process application from already-injected roles.
package applicationopening

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
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
	presentations factorysessions.OpeningPresentationOwner
}

func New(
	resolveInputs RuntimeInputResolver,
	openRuntime runtimeopening.ApplicationRuntimeOpening,
	adaptRuntime RuntimeAdapter,
	planLifecycle roles.LifecyclePlanOperation,
	presentations ...factorysessions.OpeningPresentationOwner,
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
		var presentationOwner factorysessions.OpeningPresentationOwner
		if len(presentations) > 0 {
			presentationOwner = presentations[0]
		}
		return &Service{
			resolveInputs: resolveInputs,
			openRuntime:   openRuntime,
			adaptRuntime:  adaptRuntime,
			planLifecycle: planLifecycle,
			presentations: presentationOwner,
		}, nil
	}
}

func (service *Service) OpenApplication(
	ctx context.Context,
	request roles.ApplicationOpeningRequest,
) (roles.OpenedProcessApplication, error) {
	if service == nil || service.resolveInputs == nil || service.openRuntime == nil || service.adaptRuntime == nil || service.planLifecycle == nil {
		return roles.OpenedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	presentation, err := service.presentationScope(request.ScopeID)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application: %w", err)
	}
	opened, err := service.openRuntimeForRequest(ctx, request)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	if opened.HistoricalReplay != nil {
		return service.openHistoricalReplayApplication(opened, presentation)
	}
	return service.bindLiveApplication(opened, presentation)
}

func (service *Service) openRuntimeForRequest(
	ctx context.Context,
	request roles.ApplicationOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	inputs, err := service.resolveInputs(ctx, request.Runtime)
	if err != nil {
		return roles.OpenedApplicationRuntime{}, fmt.Errorf("resolve runtime inputs: %w", err)
	}
	if inputs.Request != nil {
		inputs.Request.ScopeID = request.ScopeID
	}
	opened, err := service.openRuntime.OpenApplicationRuntime(ctx, inputs.Request)
	if err != nil {
		return roles.OpenedApplicationRuntime{}, err
	}
	return opened, nil
}

func (service *Service) bindLiveApplication(
	opened roles.OpenedApplicationRuntime,
	presentation factorysessions.ApplicationOpeningScope,
) (roles.OpenedProcessApplication, error) {
	if presentation.RuntimeHTTPServicesBound != nil {
		presentation.RuntimeHTTPServicesBound(opened.HTTP)
	}
	visualizationSink, ok := presentation.VisualizationSink.(factoryvisualization.Sink)
	if presentation.VisualizationSink != nil && !ok {
		err := closeOpenedRuntime(opened, errors.New("visualization sink has an invalid type"))
		return roles.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
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
		Completion: presentation.Completion,
	})
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	return roles.OpenedProcessApplication{
		Plan:        plan,
		Diagnostics: opened.Resources.Diagnostics,
	}, nil
}

func (service *Service) openHistoricalReplayApplication(
	opened roles.OpenedApplicationRuntime,
	presentation factorysessions.ApplicationOpeningScope,
) (roles.OpenedProcessApplication, error) {
	if presentation.HistoricalReplayBound != nil && opened.HistoricalReplay != nil {
		presentation.HistoricalReplayBound(*opened.HistoricalReplay)
	}
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
		Plan:        plan,
		Diagnostics: opened.Resources.Diagnostics,
	}, nil
}

func (service *Service) presentationScope(id factorysessions.OpeningScopeID) (factorysessions.ApplicationOpeningScope, error) {
	if id == "" {
		return factorysessions.ApplicationOpeningScope{}, nil
	}
	if service.presentations == nil {
		return factorysessions.ApplicationOpeningScope{}, fmt.Errorf("application opening presentation scope %q is unavailable", id)
	}
	scope, ok := service.presentations.Application(id)
	if !ok {
		return factorysessions.ApplicationOpeningScope{}, fmt.Errorf("application opening presentation scope %q is unavailable", id)
	}
	return scope, nil
}

func closeOpenedRuntime(opened roles.OpenedApplicationRuntime, cause error) error {
	if opened.Resources.Close == nil {
		return cause
	}
	return errors.Join(cause, opened.Resources.Close())
}
