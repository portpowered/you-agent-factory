package service

import (
	"context"
	"errors"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func (s *service) CancelInvocation(
	ctx context.Context,
	request models.CancelInvocationRequest,
) (models.CancelInvocationResult, error) {
	if s == nil {
		return models.CancelInvocationResult{}, models.ErrUnsupportedOperation
	}
	if err := request.Validate(); err != nil {
		return models.CancelInvocationResult{}, err
	}
	if err := invokeContextError(ctx); err != nil {
		return models.CancelInvocationResult{}, err
	}
	if err := s.resolveScopeError(request.Scope); err != nil {
		return models.CancelInvocationResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	invocation, ok := s.invocations[request.Invocation]
	if !ok || invocation.Scope != request.Scope {
		return models.CancelInvocationResult{}, models.ErrInvocationNotFound
	}

	result := models.CancelInvocationResult{
		Invocation:       request.Invocation,
		Status:           invocation.Status,
		LeaseDisposition: invocation.LeaseDisposition,
	}
	switch invocation.Status {
	case models.ModelInvocationStatusAccepted:
		invocation.Status = models.ModelInvocationStatusCancelled
		invocation.LeaseDisposition = models.InvocationLeaseReleased
		invocation.CancellationOutcome = models.InvocationCancellationRequested
		s.invocations[request.Invocation] = invocation
		s.releaseInvocationLease(ctx, models.InvokeModelRequest{
			Scope: invocation.Scope,
			Lease: invocation.Lease,
		})
		result.Status = invocation.Status
		result.LeaseDisposition = invocation.LeaseDisposition
		result.Outcome = models.InvocationCancellationRequested
	case models.ModelInvocationStatusCancelled:
		result.Outcome = models.InvocationCancellationAlreadyCancelled
	default:
		result.Outcome = models.InvocationCancellationAlreadyCompleted
	}
	return result, nil
}

func (s *service) resolveScopeError(scope models.RuntimeScopeRef) error {
	if scope.IsZero() {
		return models.ErrRuntimeScopeInvalid
	}
	if s == nil || s.scopes == nil {
		return models.ErrUnavailable
	}
	_, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return inferenceScopeError(err)
	}
	return nil
}

func inferenceScopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeClosed):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeClosed, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}
