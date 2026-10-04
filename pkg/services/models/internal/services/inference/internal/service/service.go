package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferenceartifacts "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/internal/artifacts"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// InvocationRuntime is the private execution port injected by Inference wire.
// It is intentionally owned by the implementation package rather than the
// parent service contract.
type InvocationRuntime interface {
	Invoke(context.Context, inference.InvocationRuntimeRequest) (inference.InvocationRuntimeResult, error)
}

type service struct {
	scopes            runtimescopes.Service
	assets            scopedassets.Service
	catalog           modelcatalog.Service
	runtimeHost       runtimehost.Service
	runtime           InvocationRuntime
	artifacts         *inferenceartifacts.Registrar
	clock             func() time.Time
	executionDeadline func() time.Duration

	mu             sync.Mutex
	nextInvocation int
	invocations    map[models.ModelInvocationRef]models.InvokeModelResult
	running        map[models.ModelInvocationRef]context.CancelFunc
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
	runtime InvocationRuntime,
	artifacts *inferenceartifacts.Registrar,
	clock func() time.Time,
	executionDeadline func() time.Duration,
) inference.Service {
	return &service{
		scopes:            scopes,
		assets:            assets,
		catalog:           catalog,
		runtimeHost:       runtimeHost,
		runtime:           runtime,
		artifacts:         artifacts,
		clock:             clock,
		executionDeadline: executionDeadline,
		invocations:       make(map[models.ModelInvocationRef]models.InvokeModelResult),
		running:           make(map[models.ModelInvocationRef]context.CancelFunc),
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

	catalogResult, err := s.catalog.GetCatalogModel(validationCtx, models.GetModelRequest{
		Scope:     request.Scope,
		Name:      request.ModelName,
		Operation: request.Operation,
	})
	if err != nil {
		return models.InvokeModelResult{}, catalogInvokeError(err)
	}

	if err := s.ensureModelAssetsAvailable(validationCtx, request); err != nil {
		return models.InvokeModelResult{}, err
	}

	hostSlot, err := s.acquireHostSlot(validationCtx, request)
	if err != nil {
		return models.InvokeModelResult{}, err
	}
	if _, err := s.runtimeHost.ClaimInvocationLease(validationCtx, request); err != nil {
		return models.InvokeModelResult{}, err
	}

	invocation, err := s.nextInvocationRef()
	if err != nil {
		disposition, releaseErr := s.releaseInvocationLease(ctx, request)
		return failedLeaseCleanupResult(request, disposition), joinInvocationCleanupError(err, releaseErr)
	}

	invokeCtx, cancelDeadline := s.invokeWithDeadline(ctx)
	defer cancelDeadline()
	accepted := acceptedInvocationResult(request, invocation)
	s.mu.Lock()
	s.invocations[invocation] = accepted.Clone()
	s.running[invocation] = cancelDeadline
	s.mu.Unlock()
	if err := invokeContextError(ctx); err != nil {
		return s.finishCancelledInvocation(ctx, request, invocation, err)
	}

	operation := catalogOperation(catalogResult.Model, request.Operation)
	runtimeResult, err := s.runtime.Invoke(invokeCtx, inference.InvocationRuntimeRequest{
		Request:   request,
		Operation: operation,
		HostSlot:  hostSlot,
	})
	return s.finishRuntimeInvocation(
		ctx, invokeCtx, request, invocation, accepted, runtimeResult, err, operation,
	)
}

func (s *service) finishRuntimeInvocation(
	ctx context.Context,
	invokeCtx context.Context,
	request models.InvokeModelRequest,
	invocation models.ModelInvocationRef,
	accepted models.InvokeModelResult,
	runtimeResult inference.InvocationRuntimeResult,
	err error,
	operation models.Operation,
) (models.InvokeModelResult, error) {
	if isInvocationInFlight(err) {
		s.mu.Lock()
		delete(s.running, invocation)
		cancelled := s.invocations[invocation].Status == models.ModelInvocationStatusCancelled
		s.mu.Unlock()
		if cancelled {
			return s.finishCancelledInvocation(ctx, request, invocation, models.ErrInferenceCancelled)
		}
		return accepted.Clone(), nil
	}
	if invokeCtx.Err() != nil {
		return s.finishFailedInvocation(invokeCtx, request, invocation, invokeCtx.Err())
	}
	if err != nil {
		return s.finishFailedInvocation(invokeCtx, request, invocation, err)
	}

	return s.finishCompletedInvocation(
		ctx,
		request,
		invocation,
		runtimeResult,
		operation,
	)
}

func (s *service) ensureModelAssetsAvailable(
	ctx context.Context,
	request models.InvokeModelRequest,
) error {
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
) (models.InvocationLeaseDisposition, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	releaseContext := context.WithoutCancel(ctx)
	modelseffects.MarkRuntimeLeaseReleaseAttempted(releaseContext)
	released, err := s.runtimeHost.ReleaseInvocationLease(releaseContext, models.ReleaseModelLeaseRequest{
		Scope: request.Scope,
		Lease: request.Lease,
	})
	return invocationLeaseDisposition(released), err
}

func invocationLeaseDisposition(
	result models.ReleaseModelLeaseResult,
) models.InvocationLeaseDisposition {
	switch result.Lease.Status {
	case models.ModelLeaseStatusReleased:
		return models.InvocationLeaseReleased
	case models.ModelLeaseStatusExpired:
		return models.InvocationLeaseExpired
	}
	switch result.Outcome {
	case models.ModelLeaseReleased, models.ModelLeaseAlreadyReleased:
		return models.InvocationLeaseReleased
	default:
		return models.InvocationLeaseRetained
	}
}
