// Package factory_visualization owns the event-driven projection lifecycle for
// presenting one live Factory.
package factory_visualization

import (
	"context"
	"errors"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
)

// View is the transport-independent presentation input emitted after the
// canonical Factory event projection changes.
type View struct {
	EngineState factoryruntime.StateSnapshot
	RenderData  recordings.SimpleDashboardRenderData
	ObservedAt  time.Time
}

// Sink presents one projected Factory view at an external boundary.
type Sink interface {
	PresentFactoryView(View)
}

// SinkFunc adapts a presentation function to Sink.
type SinkFunc func(View)

func (f SinkFunc) PresentFactoryView(view View) { f(view) }

// Clock supplies observation timestamps without hiding process-global time.
type Clock interface {
	Now() time.Time
}

// Source supplies retained-then-live canonical events and the corresponding
// runtime snapshot. Implementations may adapt a currently selected Factory
// Session, but the visualization service never reaches into its registry.
type Source interface {
	SubscribeFactoryEvents(
		context.Context,
		*factorydefinitions.FactoryEventReconnectCursor,
		factorydefinitions.FactoryEventReconnectScope,
	) (*factorydefinitions.FactoryEventStream, error)
	GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error)
}

// ErrorReporter receives non-fatal projection or presentation-read failures.
type ErrorReporter func(error)

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
	activation activationlifecycle.Service

	source      Source
	projections recordings.ProjectionService
	clock       Clock
	sink        Sink
	reportError ErrorReporter

	mu              sync.Mutex
	presentationSeq int
	presentations   map[PresentationSessionID]*rootPresentationSession
}

// New constructs an inert Factory visualization service.
func New(
	source Source,
	projections recordings.ProjectionService,
	clock Clock,
	sink Sink,
	reportError ErrorReporter,
) (*Service, error) {
	switch {
	case source == nil:
		return nil, errors.New("initialize Factory visualization: event source is required")
	case projections == nil:
		return nil, errors.New("initialize Factory visualization: projection service is required")
	case clock == nil:
		return nil, errors.New("initialize Factory visualization: clock is required")
	case sink == nil:
		return nil, errors.New("initialize Factory visualization: presentation sink is required")
	}
	activation, err := activationlifecyclewire.NewService(
		source,
		projections,
		clock,
		activationSinkAdapter{sink: sink},
		activationlifecycle.ErrorReporter(reportError),
	)
	if err != nil {
		return nil, err
	}
	return &Service{
		activation:  activation,
		source:      source,
		projections: projections,
		clock:       clock,
		sink:        sink,
		reportError: reportError,
	}, nil
}

// Start subscribes once to retained-then-live canonical Factory events.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return errors.New("start Factory visualization: service is required")
	}
	return s.activation.Start(ctx)
}

// Stop cancels and joins the event subscription, then emits one final view
// while the Factory Runtime is still active.
func (s *Service) Stop(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return nil
	}
	return s.activation.Stop(ctx)
}

// Wait blocks until the live Factory event subscription exits.
func (s *Service) Wait(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return nil
	}
	return s.activation.Wait(ctx)
}

type activationSinkAdapter struct {
	sink Sink
}

func (a activationSinkAdapter) PresentFactoryView(view activationlifecycle.View) {
	if a.sink == nil {
		return
	}
	a.sink.PresentFactoryView(View{
		EngineState: view.EngineState,
		RenderData:  view.RenderData,
		ObservedAt:  view.ObservedAt,
	})
}
