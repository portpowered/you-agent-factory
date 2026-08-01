// Package wire is the Factory Visualization service composition boundary.
//
// Wire performs construction only, returns the singular factoryvisualization.Service
// interface, and starts no lifecycle components. Parent-private activation_lifecycle,
// live_view_projection, and response_event_presentation capabilities remain behind
// this boundary; peers depend on Root rather than owner internals or construction
// ports.
package wire

import (
	"fmt"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
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
) (factoryvisualization.Service, error) {
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
	recordingsPeer, err := recordingsPeerFromProjectionService(peer)
	if err != nil {
		return nil, err
	}
	activation, err := activationlifecyclewire.NewService(
		internalservice.ActivationEventSourceAdapter{Source: source},
		recordingsPeer,
		clock,
		internalservice.ActivationViewSinkAdapter{Sink: sink},
		activationlifecycle.ErrorReporter(reportError),
	)
	if err != nil {
		return nil, err
	}
	projection, err := liveviewprojectionwire.NewService(
		source,
		recordingsPeer,
		clock,
		sink,
		reportError,
	)
	if err != nil {
		return nil, err
	}
	presentation := responseeventpresentationwire.NewService()
	root, err := internalservice.New(
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

// NewService is the canonical Factory Visualization root constructor.
func NewService(
	source factoryvisualization.Source,
	peer recordings.ProjectionService,
	clock factoryvisualization.Clock,
	sink factoryvisualization.Sink,
	reportError factoryvisualization.ErrorReporter,
) (factoryvisualization.Service, error) {
	return NewRoot(source, peer, clock, sink, reportError)
}

// NewCurrentRuntimeSource adapts a selected Factory Session runtime reader to
// the Visualization source contract.
func NewCurrentRuntimeSource(reader factoryvisualization.RuntimeReader) factoryvisualization.Source {
	return internalservice.NewCurrentRuntimeSource(reader)
}

// NewResponsePresentation constructs the inert response/event presentation
// capability used by transport composition.
func NewResponsePresentation() factoryvisualization.ResponsePresentation {
	return responseeventpresentationwire.NewService()
}
