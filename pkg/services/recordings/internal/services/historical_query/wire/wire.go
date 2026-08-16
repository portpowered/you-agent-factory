// Package wire constructs the Recordings-owned historical-query capability.
package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	historicalquery "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query"
	service "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/historical_query/internal/service"
)

// NewService constructs the inert read-only historical query from the exact
// artifact reader and projection selected by Recordings Wire.
func NewService(
	readArtifact recordings.RecordingReadFile,
	projection recordings.ProjectionService,
) historicalquery.Service {
	return service.New(readArtifact, projection)
}
