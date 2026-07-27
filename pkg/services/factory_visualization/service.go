// Package factory_visualization owns the event-driven projection lifecycle for
// presenting one live Factory.
package factory_visualization

import (
	"context"
	"errors"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// View is the transport-independent presentation input emitted after the
// canonical Factory event projection changes.
type View = liveviewprojection.View

// RuntimeObservation is the sanitized Runtime fact shape carried on emitted Views.
type RuntimeObservation = liveviewprojection.RuntimeObservation

// Sink presents one projected Factory view at an external boundary.
type Sink interface {
	PresentFactoryView(View)
}

// SinkFunc adapts a presentation function to Sink.
type SinkFunc func(View)

func (f SinkFunc) PresentFactoryView(view View) { f(view) }

// Clock supplies observation timestamps without hiding process-global time.
type Clock = liveviewprojection.Clock

// Source supplies retained-then-live canonical events and the corresponding
// runtime snapshot. Implementations may adapt a currently selected Factory
// Session, but the visualization service never reaches into its registry.
type Source = liveviewprojection.Source

// ErrorReporter receives non-fatal projection or presentation-read failures.
type ErrorReporter = liveviewprojection.ErrorReporter

// RuntimeFactory constructs one inert visualization lifecycle for a selected
// Factory Session runtime. Wire injects this operation into runtime assembly.
type RuntimeFactory func(
	RuntimeReader,
	recordings.ProjectionService,
	Clock,
	Sink,
	ErrorReporter,
) (*Service, error)

// Service owns the retained event projection, reconnect cursor, and live
// subscription lifecycle for one Factory visualization.
type Service struct {
	projection liveviewprojection.Service

	mu              sync.Mutex
	presentationSeq int
	presentations   map[PresentationSessionID]*rootPresentationSession
}

var (
	errAlreadyStarted = errors.New("start Factory visualization: already started")
	errNotStarted     = errors.New("wait for Factory visualization: not started")
)

// New constructs an inert Factory visualization service.
func New(
	source Source,
	projections recordings.ProjectionService,
	clock Clock,
	sink Sink,
	reportError ErrorReporter,
) (*Service, error) {
	projection, err := liveviewprojectionwire.NewService(
		source,
		projections,
		clock,
		adaptSink(sink),
		reportError,
	)
	if err != nil {
		return nil, err
	}
	return &Service{projection: projection}, nil
}

// Start subscribes once to retained-then-live canonical Factory events. It
// renders the retained projection before returning and then observes deltas.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.projection == nil {
		return errors.New("start Factory visualization: service is required")
	}
	err := s.projection.Start(ctx)
	if errors.Is(err, liveviewprojection.ErrLiveViewProjectionAlreadyStarted) {
		return errAlreadyStarted
	}
	return err
}

// Stop cancels and joins the event subscription, then emits one final view
// while the Factory Runtime is still active.
func (s *Service) Stop(ctx context.Context) error {
	if s == nil || s.projection == nil {
		return nil
	}
	return s.projection.Stop(ctx)
}

// Wait blocks until the live Factory event subscription exits.
func (s *Service) Wait(ctx context.Context) error {
	if s == nil || s.projection == nil {
		return nil
	}
	err := s.projection.Wait(ctx)
	if errors.Is(err, liveviewprojection.ErrLiveViewProjectionNotStarted) {
		return errNotStarted
	}
	return err
}

func adaptSink(sink Sink) liveviewprojection.Sink {
	if sink == nil {
		return nil
	}
	return SinkFunc(sink.PresentFactoryView)
}

// reconnectCursor exposes the Visualization-owned reconnect cursor for focused
// characterization tests in the root package.
func (s *Service) reconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	if s == nil || s.projection == nil {
		return nil
	}
	return s.projection.ReconnectCursor()
}
