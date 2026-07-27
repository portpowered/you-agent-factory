package service

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

// Service owns session-scoped submit and operator move through the private
// Session adapter port.
type Service struct {
	sessions stateaccess.SessionResolver
}

var _ stateaccess.Service = (*Service)(nil)

// New constructs the private state_access implementation.
func New(sessions stateaccess.SessionResolver) *Service {
	return &Service{sessions: sessions}
}

func (s *Service) SubmitWorkRequestForSession(
	ctx context.Context,
	sessionID string,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	adapter, err := s.resolveSession(sessionID)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	result, err := adapter.SubmitWorkRequest(ctx, request)
	if err != nil {
		return work.WorkRequestSubmitResult{}, err
	}
	return detachSubmitResult(result), nil
}

func (s *Service) MoveWorkForSession(
	ctx context.Context,
	sessionID string,
	workID string,
	stateName string,
	requestID string,
) (work.OperatorMoveResult, error) {
	if err := requireContext(ctx); err != nil {
		return work.OperatorMoveResult{}, err
	}
	adapter, err := s.resolveSession(sessionID)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	result, err := adapter.MoveWork(
		ctx,
		workID,
		stateName,
		work.WorkStateChangeSourceAPI,
		requestID,
	)
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return detachMoveResult(result), nil
}

func (s *Service) resolveSession(sessionID string) (stateaccess.SessionAdapter, error) {
	if s == nil || s.sessions == nil {
		return nil, errors.New("Work state access session resolver is required")
	}
	adapter, err := s.sessions.ResolveSessionAdapter(sessionID)
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, errors.New("Work state access session adapter is unavailable")
	}
	return adapter, nil
}

func requireContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Work state access context is required")
	}
	return ctx.Err()
}

func detachSubmitResult(result work.WorkRequestSubmitResult) work.WorkRequestSubmitResult {
	works := make([]work.WorkRequestSubmittedWork, len(result.Works))
	for i, workItem := range result.Works {
		works[i] = work.WorkRequestSubmittedWork{
			Name:         workItem.Name,
			WorkTypeName: workItem.WorkTypeName,
			WorkID:       workItem.WorkID,
		}
	}
	return work.WorkRequestSubmitResult{
		RequestID:    result.RequestID,
		TraceID:      result.TraceID,
		WorkID:       result.WorkID,
		Name:         result.Name,
		WorkTypeName: result.WorkTypeName,
		Accepted:     result.Accepted,
		Works:        works,
	}
}

func detachMoveResult(result work.OperatorMoveResult) work.OperatorMoveResult {
	return work.OperatorMoveResult{
		WorkID:     result.WorkID,
		WorkTypeID: result.WorkTypeID,
		FromState:  result.FromState,
		ToState:    result.ToState,
	}
}
