package service

import (
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// assembleRoot constructs an inert Factory Visualization root from parent-private
// owner services already wired by the owning wire package.
func assembleRoot(
	activation activationlifecycle.Service,
	projection liveviewprojection.Service,
	presentation responseeventpresentation.Service,
	source Source,
	recordingsPeer recordings.Service,
	clock Clock,
	sink Sink,
	reportError ErrorReporter,
) (*Service, error) {
	switch {
	case activation == nil:
		return nil, errors.New("assemble Factory visualization root: activation lifecycle owner is required")
	case projection == nil:
		return nil, errors.New("assemble Factory visualization root: live view projection owner is required")
	case presentation == nil:
		return nil, errors.New("assemble Factory visualization root: response event presentation service is required")
	case source == nil:
		return nil, errors.New("assemble Factory visualization root: event source is required")
	case recordingsPeer == nil:
		return nil, errors.New("assemble Factory visualization root: recordings service is required")
	case clock == nil:
		return nil, errors.New("assemble Factory visualization root: clock is required")
	case sink == nil:
		return nil, errors.New("assemble Factory visualization root: presentation sink is required")
	}
	liveviewprojectionwire.BindRetainedEventsSupplier(projection, func() []factorydefinitions.FactoryEvent {
		return activation.RetainedEvents()
	})
	return &Service{
		activation:        activation,
		projection:        projection,
		presentationOwner: presentation,
		source:            source,
		recordings:        recordingsPeer,
		clock:             clock,
		sink:              sink,
		reportError:       reportError,
	}, nil
}

// ScopeOwner retains prebuilt owner operations and the canonical fixed peers.
// Each Open creates independent lifecycle and presentation state.
type ScopeOwner struct {
	activationOpening func(activationlifecycle.EventSource, activationlifecycle.Clock, activationlifecycle.ViewSink, activationlifecycle.ErrorReporter) activationlifecycle.Service
	projectionOpening func(liveviewprojection.Source, liveviewprojection.Clock, liveviewprojection.Sink, liveviewprojection.ErrorReporter) liveviewprojection.Service
	presentation      responseeventpresentation.Service
	recordings        recordings.Service
}

// NewScopeOwner constructs only fixed behavior; it does not select a runtime.
func NewScopeOwner(
	activationOpening func(activationlifecycle.EventSource, activationlifecycle.Clock, activationlifecycle.ViewSink, activationlifecycle.ErrorReporter) activationlifecycle.Service,
	projectionOpening func(liveviewprojection.Source, liveviewprojection.Clock, liveviewprojection.Sink, liveviewprojection.ErrorReporter) liveviewprojection.Service,
	presentation responseeventpresentation.Service,
	peer recordings.Service,
) *ScopeOwner {
	return &ScopeOwner{activationOpening: activationOpening, projectionOpening: projectionOpening, presentation: presentation, recordings: peer}
}

// Open binds scoped resources to the prebuilt owners and allocates local state.
func (owner *ScopeOwner) Open(source Source, clock Clock, sink Sink, reportError ErrorReporter) (*Service, error) {
	switch {
	case source == nil:
		return nil, errors.New("construct Factory Visualization: event source is required")
	case clock == nil:
		return nil, errors.New("construct Factory Visualization: clock is required")
	case sink == nil:
		return nil, errors.New("construct Factory Visualization: presentation sink is required")
	}
	activation := owner.activationOpening(ActivationEventSourceAdapter{Source: source}, clock, ActivationViewSinkAdapter{Sink: sink}, activationlifecycle.ErrorReporter(reportError))
	projection := owner.projectionOpening(source, clock, sink, reportError)
	return assembleRoot(activation, projection, owner.presentation, source, owner.recordings, clock, sink, reportError)
}
