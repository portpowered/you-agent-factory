package work

import (
	"context"
	"errors"
	"fmt"
)

type invocationPolicyService struct {
	preparation InvocationInputPreparation
}

// NewInvocationPolicyService returns an inert Work root that serves only the
// published invocation/return-policy slice. Peers that need input preparation
// or primary-result selection without live session runtime wiring should depend
// on this constructor rather than loose operational helpers.
func NewInvocationPolicyService() Service {
	return invocationPolicyService{preparation: NewInvocationInputPreparation()}
}

func (invocationPolicyService) SubmitWorkRequestForSession(
	context.Context,
	string,
	WorkRequest,
) (WorkRequestSubmitResult, error) {
	return WorkRequestSubmitResult{}, fmt.Errorf("Work invocation policy service does not support admission")
}

func (invocationPolicyService) MoveWorkForSession(
	context.Context,
	string,
	string,
	string,
	string,
) (OperatorMoveResult, error) {
	return OperatorMoveResult{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyService) ListWork(context.Context, string, ListOptions) (ListResult, error) {
	return ListResult{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyService) GetWork(context.Context, string, string) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyService) MoveWorkAndRead(
	context.Context,
	string,
	string,
	string,
	string,
) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyService) StageContent(context.Context, StageContentRequest) (StageContentResult, error) {
	return StageContentResult{}, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyService) PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error) {
	return nil, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyService) ResolveContent(context.Context, string) (ResolvedStagedContent, error) {
	return ResolvedStagedContent{}, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyService) CleanupContent(context.Context, string) error {
	return fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyService) MaterializeContentURL(context.Context, string) (string, ContentCleanup, error) {
	return "", nil, fmt.Errorf("Work invocation policy service does not support content materialization")
}

func (s invocationPolicyService) PrepareInvocationInput(
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

func (invocationPolicyService) ResolvePrimaryResult(
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
