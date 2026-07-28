package invocationreturnpolicy

import (
	"context"
	"errors"
	"fmt"
)

// PolicyService serves the invocation/return-policy slice without live session
// runtime wiring.
type PolicyService struct {
	preparation InvocationInputPreparation
}

// NewPolicyService constructs the private invocation/return-policy service.
func NewPolicyService() *PolicyService {
	return &PolicyService{preparation: NewInvocationInputPreparation()}
}

func (s *PolicyService) PrepareInvocationInput(
	ctx context.Context,
	request InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	prepared, err := s.preparation.PrepareInvocationInput(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return PreparedInvocationInput{}, err
		}
		return PreparedInvocationInput{}, fmt.Errorf("%w: %w", ErrInvalidInvocationInput, err)
	}
	return prepared, nil
}

func (s *PolicyService) ResolvePrimaryResult(
	ctx context.Context,
	input PrimaryResultSelectionInput,
) (PrimaryResultSelection, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return PrimaryResultSelection{}, err
		}
	}
	return ResolvePrimaryResult(input)
}
