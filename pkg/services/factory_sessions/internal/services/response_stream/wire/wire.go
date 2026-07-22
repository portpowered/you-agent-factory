// Package wire constructs the owner-private Factory Session response-stream service.
package wire

import (
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
)

func NewService(eventIDs responseeventstore.ResponseEventIDGenerator) (responsestreamservice.Service, error) {
	service, err := internalservice.New(eventIDs)
	if err != nil {
		return nil, err
	}
	return service, nil
}
