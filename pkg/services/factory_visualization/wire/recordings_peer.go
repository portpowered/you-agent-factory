package wire

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// recordingsPeerFromProjectionService adapts the legacy projection peer to
// the Recordings root contract required by the private Visualization owners.
func recordingsPeerFromProjectionService(
	peer recordings.ProjectionService,
) (recordings.Service, error) {
	if peer == nil {
		return nil, errors.New("construct Factory Visualization: projection service is required")
	}
	if service, ok := peer.(recordings.Service); ok {
		return service, nil
	}
	return &projectionServiceRoot{projection: peer}, nil
}
