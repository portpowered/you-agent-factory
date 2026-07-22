// Package applicationopening owns the exact Factory Session operation that
// opens and binds one process application from already-injected roles.
package applicationopening

import (
	"context"
	"errors"
	"fmt"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

// RuntimeInputs are the resolved operation inputs selected by the canonical
// injector. They contain values and external edges only.
type RuntimeInputs struct {
	Request *factorysessions.RuntimeOpeningRequest
	Edges   serviceedges.Edges
	Logger  *zap.Logger
}

type RuntimeInputResolver func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
	factorysessions.ApplicationOpeningPorts,
	*zap.Logger,
) (RuntimeInputs, error)

type RuntimeOpener interface {
	OpenApplicationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
		serviceedges.Edges,
		*zap.Logger,
	) (factorysessions.OpenedApplicationRuntime, error)
}

// RuntimeAdapter binds the exact HTTP and optional visualization components
// selected by Wire to one opened Factory Session. It contains no product
// lifecycle selection or ordering policy.
type RuntimeAdapter func(
	factorysessions.OpenedApplicationRuntime,
	serviceedges.Edges,
	factoryvisualization.Sink,
) (factorysessions.BoundProcessComponents, error)

// Service is constructed once by Wire. OpenApplication supplies only
// invocation values and external-edge replacements to the already-selected
// runtime-opening and application-binding operations.
type Service struct {
	resolveInputs RuntimeInputResolver
	openRuntime   RuntimeOpener
	adaptRuntime  RuntimeAdapter
	planLifecycle factorysessions.LifecyclePlanOperation
}

func New(
	resolveInputs RuntimeInputResolver,
	openRuntime RuntimeOpener,
	adaptRuntime RuntimeAdapter,
	planLifecycle factorysessions.LifecyclePlanOperation,
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

func (service *Service) OpenApplication(
	ctx context.Context,
	request factorysessions.ApplicationOpeningRequest,
	logger *zap.Logger,
	visualizationSink factoryvisualization.Sink,
) (factorysessions.OpenedProcessApplication, error) {
	if service == nil || service.resolveInputs == nil || service.openRuntime == nil || service.adaptRuntime == nil || service.planLifecycle == nil {
		return factorysessions.OpenedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	inputs, err := service.resolveInputs(ctx, request.Runtime, request.Ports, logger)
	if err != nil {
		return factorysessions.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application: %w", err)
	}
	opened, err := service.openRuntime.OpenApplicationRuntime(
		ctx, inputs.Request, inputs.Edges, inputs.Logger,
	)
	if err != nil {
		return factorysessions.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	components, err := service.adaptRuntime(opened, inputs.Edges, visualizationSink)
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return factorysessions.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
	}
	plan, err := service.planLifecycle(factorysessions.LifecyclePlanRequest{
		Runtime:    opened.Process,
		Components: components,
		Close:      opened.Resources.Close,
	})
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return factorysessions.OpenedProcessApplication{}, fmt.Errorf("plan Factory Session application lifecycle: %w", err)
	}
	return factorysessions.OpenedProcessApplication{
		Plan:        plan,
		Diagnostics: opened.Resources.Diagnostics,
	}, nil
}

func closeOpenedRuntime(opened factorysessions.OpenedApplicationRuntime, cause error) error {
	if opened.Resources.Close == nil {
		return cause
	}
	return errors.Join(cause, opened.Resources.Close())
}
