// Package applicationopening owns the exact Factory Session operation that
// opens and binds one process application from already-injected roles.
package applicationopening

import (
	"context"
	"errors"
	"fmt"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"go.uber.org/zap"
)

// RuntimeInputs are the resolved operation inputs selected by the canonical
// injector. They contain values and exact external effects only.
type RuntimeInputs struct {
	Request *factorysessions.RuntimeOpeningRequest
	Effects runtimeopening.ExternalEffects
	Logger  *zap.Logger
}

type RuntimeInputResolver func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
	roles.ApplicationOpeningPorts,
	*zap.Logger,
) (RuntimeInputs, error)

type RuntimeOpener interface {
	OpenApplicationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
		runtimeopening.ExternalEffects,
		*zap.Logger,
	) (roles.OpenedApplicationRuntime, error)
}

// RuntimeAdapter binds the exact HTTP and optional visualization components
// selected by Wire to one opened Factory Session. It contains no product
// lifecycle selection or ordering policy.
type RuntimeAdapter func(
	roles.OpenedApplicationRuntime,
	runtimeopening.ExternalEffects,
	factoryvisualization.Sink,
) (factorysessions.BoundProcessComponents, error)

// Service is constructed once by Wire. OpenApplication supplies only
// invocation values and exact external-effect replacements to the already-selected
// runtime-opening and application-binding operations.
type Service struct {
	resolveInputs RuntimeInputResolver
	openRuntime   RuntimeOpener
	adaptRuntime  RuntimeAdapter
	planLifecycle roles.LifecyclePlanOperation
}

func New(
	resolveInputs RuntimeInputResolver,
	openRuntime RuntimeOpener,
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

func (service *Service) OpenApplication(
	ctx context.Context,
	request roles.ApplicationOpeningRequest,
	logger *zap.Logger,
	visualizationSink factoryvisualization.Sink,
) (roles.OpenedProcessApplication, error) {
	if service == nil || service.resolveInputs == nil || service.openRuntime == nil || service.adaptRuntime == nil || service.planLifecycle == nil {
		return roles.OpenedProcessApplication{}, errors.New("open Factory Session application: service is required")
	}
	ports, completion := gateCompletionOnRuntimeHost(request.Ports, request.Completion)
	inputs, err := service.resolveInputs(ctx, request.Runtime, ports, logger)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application: %w", err)
	}
	opened, err := service.openRuntime.OpenApplicationRuntime(
		ctx, inputs.Request, inputs.Effects, inputs.Logger,
	)
	if err != nil {
		return roles.OpenedProcessApplication{}, fmt.Errorf("open Factory Session application runtime: %w", err)
	}
	if ports.RuntimeHTTPServicesBound != nil {
		ports.RuntimeHTTPServicesBound(opened.HTTP)
	}
	components, err := service.adaptRuntime(opened, inputs.Effects, visualizationSink)
	if err != nil {
		err = closeOpenedRuntime(opened, err)
		return roles.OpenedProcessApplication{}, fmt.Errorf("bind Factory Session application: %w", err)
	}
	plan, err := service.planLifecycle(roles.LifecyclePlanRequest{
		Runtime:    opened.Process,
		Components: components,
		Close:      opened.Resources.Close,
		Completion: completion,
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

func gateCompletionOnRuntimeHost(
	ports roles.ApplicationOpeningPorts,
	completion func(context.Context) error,
) (roles.ApplicationOpeningPorts, func(context.Context) error) {
	if completion == nil {
		return ports, nil
	}
	ready := make(chan struct{})
	observer := ports.RuntimeHostObserver
	var publish sync.Once
	ports.RuntimeHostObserver = func(binding factorysessions.RuntimeHostBinding) {
		publish.Do(func() {
			if observer != nil {
				observer(binding)
			}
			close(ready)
		})
	}
	return ports, func(ctx context.Context) error {
		select {
		case <-ready:
			return completion(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func closeOpenedRuntime(opened roles.OpenedApplicationRuntime, cause error) error {
	if opened.Resources.Close == nil {
		return cause
	}
	return errors.Join(cause, opened.Resources.Close())
}
