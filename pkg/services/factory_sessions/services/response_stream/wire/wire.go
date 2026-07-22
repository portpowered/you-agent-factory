// Package wire constructs the owner-private Factory Session response-stream service.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseeventstore"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/response_stream"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/response_stream/internal/service"
)

func NewService(eventIDs responseeventstore.ResponseEventIDGenerator) (responsestreamservice.Service, error) {
	service, err := internalservice.New(eventIDs)
	if err != nil {
		return nil, err
	}
	return service, nil
}
