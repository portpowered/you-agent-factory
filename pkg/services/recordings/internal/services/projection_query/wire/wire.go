// Package wire constructs the Recordings projection-query subservice.
package wire

import (
	projectionquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/internal/service"
)

// NewService constructs the stateless private projection-query capability.
func NewService() projectionquery.Service {
	return projectionservice.New()
}
