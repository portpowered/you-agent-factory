package service

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

// Service keeps the durable execution implementation private while preserving
// the established execution contract during the package migration.
type Service struct {
	factorysessions.ExecutionService
}

// SubscribeResponseEvents forwards durable-session response-event subscriptions
// to the underlying JavaScript runtime when that capability is present.
func (s *Service) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	if s == nil || s.ExecutionService == nil {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	subscriber, ok := s.ExecutionService.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessions.ErrRuntimeNotAvailable
	}
	return subscriber.SubscribeResponseEvents(ctx, sessionID, request)
}

// New constructs an inert durable execution capability around an explicitly
// injected implementation.
func New(execution factorysessions.ExecutionService) (*Service, error) {
	if execution == nil {
		return nil, fmt.Errorf("construct durable Factory Sessions execution: execution service is required")
	}
	return &Service{ExecutionService: execution}, nil
}

// IsNonLiveReplay preserves the optional replay routing capability without
// exposing the underlying recording-replay implementation.
func (s *Service) IsNonLiveReplay() bool {
	if s == nil || s.ExecutionService == nil {
		return false
	}
	replay, ok := s.ExecutionService.(interface{ IsNonLiveReplay() bool })
	return ok && replay.IsNonLiveReplay()
}

// RecordPetriTokenMutations preserves the runtime event bridge used by the
// Petri orchestrator while the execution engine remains behind this service.
func (s *Service) RecordPetriTokenMutations(
	sessionID string,
	records []factorydefinitions.TokenMutationRecord,
) error {
	if s == nil || s.ExecutionService == nil {
		return factorysessions.ErrExecutionServiceNotConfigured
	}
	recorder, ok := s.ExecutionService.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return fmt.Errorf("durable execution does not record Petri mutations")
	}
	return recorder.RecordPetriTokenMutations(sessionID, records)
}

var _ durableexecution.Service = (*Service)(nil)
