package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	scopes      runtimescopes.Service
	assets      scopedassets.Service
	catalog     modelcatalog.Service
	runtimeHost runtimehost.Service
	runtime     inference.InvocationRuntime
	clock       func() time.Time
	executionDeadline func() time.Duration

	mu             sync.Mutex
	nextInvocation int
	invocations    map[models.ModelInvocationRef]models.InvokeModelResult
}

var _ inference.Service = (*service)(nil)

// New constructs an inert Inference owner that validates and retains injected
// effects without launching subprocesses, opening listeners, or starting
// application lifecycle.
func New(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	catalog modelcatalog.Service,
	runtimeHost runtimehost.Service,
	runtime inference.InvocationRuntime,
	clock func() time.Time,
	executionDeadline func() time.Duration,
) inference.Service {
	if executionDeadline == nil {
		executionDeadline = defaultExecutionDeadline
	}
	return &service{
		scopes:            scopes,
		assets:            assets,
		catalog:           catalog,
		runtimeHost:       runtimeHost,
		runtime:           runtime,
		clock:             clock,
		executionDeadline: executionDeadline,
		invocations:       make(map[models.ModelInvocationRef]models.InvokeModelResult),
	}
}

func (s *service) InvokeModelWithLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	if s == nil {
		return models.InvokeModelResult{}, models.ErrUnsupportedOperation
	}
	if err := request.Validate(); err != nil {
		return models.InvokeModelResult{}, err
	}
	if err := validateInvocationResponseMode(request); err != nil {
		return models.InvokeModelResult{}, err
	}

	validationCtx := context.WithoutCancel(ctx)

	leaseResult, err := s.runtimeHost.GetModelLease(validationCtx, models.GetModelLeaseRequest{
		Scope: request.Scope,
		Lease: request.Lease,
	})
	if err != nil {
		return models.InvokeModelResult{}, err
	}
	if err := validateInvocationLease(request, leaseResult.Lease); err != nil {
		return models.InvokeModelResult{}, err
	}

	if _, err := s.catalog.GetCatalogModel(validationCtx, models.GetModelRequest{
		Scope:     request.Scope,
		Name:      request.ModelName,
		Operation: request.Operation,
	}); err != nil {
		return models.InvokeModelResult{}, catalogInvokeError(err)
	}

	if err := s.ensureModelAssetsAvailable(validationCtx, request); err != nil {
		return models.InvokeModelResult{}, err
	}

	if s.runtime == nil {
		return models.InvokeModelResult{}, models.ErrUnavailable
	}

	invocation, err := s.nextInvocationRef()
	if err != nil {
		return models.InvokeModelResult{}, err
	}

	accepted := acceptedInvocationResult(request, invocation)
	s.putInvocation(invocation, accepted)

	if err := invokeContextError(ctx); err != nil {
		return s.finishCancelledInvocation(ctx, request, invocation, err)
	}

	invokeCtx, cancelDeadline := s.invokeWithDeadline(ctx)
	defer cancelDeadline()

	content, err := s.runtime.Invoke(invokeCtx, request)
	if isInvocationInFlight(err) {
		return accepted.Clone(), nil
	}
	if err != nil {
		return s.finishFailedInvocation(invokeCtx, request, invocation, err)
	}

	return s.finishCompletedInvocation(ctx, request, invocation, content)
}

func (s *service) ensureModelAssetsAvailable(
	ctx context.Context,
	request models.InvokeModelRequest,
) error {
	if s == nil || s.assets == nil {
		return models.ErrUnavailable
	}
	inspection, err := s.assets.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: request.Scope,
		Name:  request.ModelName,
	})
	if err != nil {
		return err
	}
	if inspection.Supported && !inspection.Installed {
		return fmt.Errorf("%w: %s", models.ErrAssetUnavailable, request.ModelName)
	}
	return nil
}

func (s *service) releaseInvocationLease(
	ctx context.Context,
	request models.InvokeModelRequest,
) {
	_, _ = s.runtimeHost.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{
		Scope: request.Scope,
		Lease: request.Lease,
	})
}
