// Package wire constructs the Factory Visualization activation-lifecycle subservice.
package wire

import (
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService constructs the private activation lifecycle owner from the exact
// collaborators selected by the Visualization root composition.
func NewService(
	source activationlifecycle.EventSource,
	recordingsPeer recordings.Service,
	clock activationlifecycle.Clock,
	sink activationlifecycle.ViewSink,
	reportError activationlifecycle.ErrorReporter,
) (activationlifecycle.Service, error) {
	return lifecycleservice.New(source, recordingsPeer, clock, sink, reportError)
}
