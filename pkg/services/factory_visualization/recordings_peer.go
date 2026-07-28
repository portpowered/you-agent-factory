package factory_visualization

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RecordingsPeerFromProjectionService resolves the Recordings root contract from
// a legacy projection peer. Runtime wiring still supplies ProjectionService
// today, but Visualization presentation paths require recordings.Service.
func RecordingsPeerFromProjectionService(
	peer recordings.ProjectionService,
) (recordings.Service, error) {
	if peer == nil {
		return nil, errors.New("initialize Factory visualization: projection service is required")
	}
	service, ok := peer.(recordings.Service)
	if !ok {
		return nil, errors.New(
			"initialize Factory visualization: recordings peer must implement recordings.Service root contract",
		)
	}
	return service, nil
}
