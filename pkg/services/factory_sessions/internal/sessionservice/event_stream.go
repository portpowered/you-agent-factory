package service

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// QueryEventStream materializes a finite durable event read at the Sessions
// owner, so transports receive a ready stream without managing channels.
func (s *Service) QueryEventStream(ctx context.Context, request factorysessions.SessionEventQueryRequest) (*factorydefinitions.FactoryEventStream, error) {
	result, err := s.QueryEvents(ctx, request)
	if err != nil {
		return nil, err
	}
	return factorysessions.MaterializeEventReadStream(result), nil
}
