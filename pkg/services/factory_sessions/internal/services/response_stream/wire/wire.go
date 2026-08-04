// Package wire constructs the owner-private Factory Session response-stream service.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/internal/service"
)

func NewService(
	eventIDs responseeventstore.ResponseEventIDGenerator,
	retentionLimits *factorysessions.ResponseEventRetentionLimits,
	eventsService events.Service,
	logger ...logging.Logger,
) (responsestreamservice.Service, error) {
	service, err := internalservice.New(eventIDs, retentionLimits, eventsService, logger...)
	if err != nil {
		return nil, err
	}
	return service, nil
}
