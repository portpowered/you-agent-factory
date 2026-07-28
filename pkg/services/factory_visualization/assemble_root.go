package factory_visualization

import (
	"errors"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// AssembleRoot constructs an inert Factory Visualization root from parent-private
// owner services already wired by the service-local Wire packet.
func AssembleRoot(
	activation activationlifecycle.Service,
	projection liveviewprojection.Service,
	presentation responsePresentationOwner,
	source Source,
	projections recordings.ProjectionService,
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
		return nil, errors.New("assemble Factory visualization root: response event presentation owner is required")
	case source == nil:
		return nil, errors.New("assemble Factory visualization root: event source is required")
	case projections == nil:
		return nil, errors.New("assemble Factory visualization root: projection service is required")
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
		projections:       projections,
		clock:             clock,
		sink:              sink,
		reportError:       reportError,
	}, nil
}

// ActivationEventSource adapts the root construction Source to the activation
// lifecycle owner EventSource port.
func ActivationEventSource(source Source) activationlifecycle.EventSource {
	return activationSourceAdapter{source: source}
}

// ActivationViewSink adapts the root construction Sink to the activation
// lifecycle owner ViewSink port.
func ActivationViewSink(sink Sink) activationlifecycle.ViewSink {
	return activationSinkAdapter{sink: sink}
}

// ProjectionSink adapts the root construction Sink to the live view projection
// owner Sink port.
func ProjectionSink(sink Sink) liveviewprojection.Sink {
	return adaptSink(sink)
}

func (s *Service) responsePresentationOwner() responsePresentationOwner {
	if s == nil || s.presentationOwner == nil {
		return defaultResponsePresentationOwner()
	}
	return s.presentationOwner
}

func (s *Service) openBestEffortOutput(writer io.Writer) Output {
	return s.responsePresentationOwner().OpenBestEffortOutput(writer)
}

func (s *Service) openLosslessOutput(writer io.Writer) Output {
	return s.responsePresentationOwner().OpenLosslessOutput(writer)
}
