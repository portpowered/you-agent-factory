// Package wire is the Factory Visualization service composition boundary.
//
// Wire performs construction only, returns the singular factoryvisualization.Root
// interface, and starts no lifecycle components. Parent-private activation_lifecycle,
// live_view_projection, and response_event_presentation owner wiring stays inside
// the owner service assembly path; peers depend on Root rather than owner internals
// or construction ports.
package wire

import (
	"fmt"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	responseeventpresentationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewRoot constructs an inert Factory Visualization root from construction and
// process-edge ports. It composes the accepted root through parent-private
// activation_lifecycle, live_view_projection, and response_event_presentation
// owner construction without publishing owner types on the returned peer surface.
// Missing required construction ports fail with a deterministic construction error
// and a nil root.
func NewRoot(
	source factoryvisualization.Source,
	peer recordings.ProjectionService,
	clock factoryvisualization.Clock,
	sink factoryvisualization.Sink,
	reportError factoryvisualization.ErrorReporter,
) (factoryvisualization.Root, error) {
	switch {
	case source == nil:
		return nil, fmt.Errorf("construct Factory Visualization: event source is required")
	case peer == nil:
		return nil, fmt.Errorf("construct Factory Visualization: projection service is required")
	case clock == nil:
		return nil, fmt.Errorf("construct Factory Visualization: clock is required")
	case sink == nil:
		return nil, fmt.Errorf("construct Factory Visualization: presentation sink is required")
	}
	recordingsPeer, err := factoryvisualization.RecordingsPeerFromProjectionService(peer)
	if err != nil {
		return nil, err
	}

	activation, err := activationlifecyclewire.NewService(
		factoryvisualization.ActivationEventSource(source),
		recordingsPeer,
		clock,
		factoryvisualization.ActivationViewSink(sink),
		activationlifecycle.ErrorReporter(reportError),
	)
	if err != nil {
		return nil, err
	}
	projection, err := liveviewprojectionwire.NewService(
		source,
		recordingsPeer,
		clock,
		factoryvisualization.ProjectionSink(sink),
		reportError,
	)
	if err != nil {
		return nil, err
	}
	presentation := responseeventpresentationwire.NewService()

	root, err := factoryvisualization.AssembleRoot(
		activation,
		projection,
		presentation,
		source,
		recordingsPeer,
		clock,
		sink,
		reportError,
	)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("construct Factory Visualization: implementation rejected its dependencies")
	}
	return root, nil
}
