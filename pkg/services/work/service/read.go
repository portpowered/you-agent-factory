package service

import (
	"context"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (s *applicationService) ListWork(
	ctx context.Context,
	sessionID string,
	options work.ListOptions,
) (work.ListResult, error) {
	if s == nil || s.stateAccess == nil {
		return work.ListResult{}, errStateAccessRequired()
	}
	return s.stateAccess.ListWork(ctx, sessionID, options)
}

func (s *applicationService) GetWork(
	ctx context.Context,
	sessionID string,
	id string,
) (work.ReadModel, error) {
	if s == nil || s.stateAccess == nil {
		return work.ReadModel{}, errStateAccessRequired()
	}
	return s.stateAccess.GetWork(ctx, sessionID, id)
}

func (s *applicationService) MoveWorkAndRead(
	ctx context.Context,
	sessionID string,
	id string,
	stateName string,
	requestID string,
) (work.ReadModel, error) {
	if s == nil || s.stateAccess == nil {
		return work.ReadModel{}, errStateAccessRequired()
	}
	return s.stateAccess.MoveWorkAndRead(ctx, sessionID, id, stateName, requestID)
}

func errStateAccessRequired() error {
	return fmt.Errorf("Work state access is required")
}
