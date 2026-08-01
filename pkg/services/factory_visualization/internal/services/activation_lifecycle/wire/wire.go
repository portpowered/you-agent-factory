// Package wire constructs the Factory Visualization activation-lifecycle subservice.
package wire

import (
	visualizationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService constructs the private activation lifecycle owner from the exact
// collaborators selected by the Visualization root composition.
func NewService(
	source visualizationcontracts.ActivationEventSource,
	recordingsPeer recordings.Service,
	clock visualizationcontracts.ActivationClock,
	sink visualizationcontracts.ActivationViewSink,
	reportError activationlifecycle.ErrorReporter,
) (activationlifecycle.Service, error) {
	return lifecycleservice.New(source, recordingsPeer, clock, sink, reportError)
}
