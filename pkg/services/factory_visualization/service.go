// Package factory_visualization owns the event-driven projection lifecycle for
// presenting one live Factory.
package factory_visualization

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	source      Source
	projections recordings.ProjectionService
	clock       Clock
	sink        Sink
	reportError ErrorReporter

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	runErr   error
	started  bool
	stopOnce sync.Once

	events []factorydefinitions.FactoryEvent
	cursor *factorydefinitions.FactoryEventReconnectCursor
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
	default:
		return &Service{
			source: source, projections: projections, clock: clock,
			sink: sink, reportError: reportError,
		}, nil
	}
}

// Start subscribes once to retained-then-live canonical Factory events. It
// renders the retained projection before returning and then observes deltas.
func (s *Service) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("start Factory visualization: service is required")
	}
	if ctx == nil {
		return errors.New("start Factory visualization: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("start Factory visualization: already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	stream, err := s.source.SubscribeFactoryEvents(
		runCtx,
		s.cursor,
		factorydefinitions.FactoryEventReconnectScope{},
	)
	if err != nil {
		cancel()
		s.mu.Unlock()
		return fmt.Errorf("start Factory visualization: subscribe to Factory events: %w", err)
	}
	if stream == nil || stream.Events == nil {
		cancel()
		s.mu.Unlock()
		return errors.New("start Factory visualization: event source returned an invalid stream")
	}
	s.events = append(s.events[:0], stream.History...)
	s.advanceCursorLocked(stream.History)
	s.cancel = cancel
	s.done = make(chan struct{})
	s.started = true
	s.mu.Unlock()

	s.projectAndPresent(runCtx)
	go s.run(runCtx, stream.Events)
	return nil
}

// Stop cancels and joins the event subscription, then emits one final view
// while the Factory Runtime is still active.
func (s *Service) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel, done := s.cancel, s.done
		s.mu.Unlock()
		if cancel == nil || done == nil {
			return
		}
		cancel()
		<-done
		s.projectAndPresent(ctx)
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runErr
}

// Wait blocks until the live Factory event subscription exits.
func (s *Service) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return errors.New("wait for Factory visualization: not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) run(
	ctx context.Context,
	events <-chan factorydefinitions.FactoryEvent,
) {
	defer func() {
		s.mu.Lock()
		close(s.done)
		s.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				s.mu.Lock()
				if ctx.Err() == nil {
					s.runErr = errors.New("Factory event stream closed unexpectedly")
				}
				s.mu.Unlock()
				return
			}
			s.mu.Lock()
			s.events = append(s.events, event)
			s.advanceCursorLocked([]factorydefinitions.FactoryEvent{event})
			s.mu.Unlock()
			s.projectAndPresent(ctx)
		}
	}
}

func (s *Service) projectAndPresent(ctx context.Context) {
	snapshot, err := s.source.GetEngineStateSnapshot(ctx)
	if err != nil {
		s.report(fmt.Errorf("read Factory visualization snapshot: %w", err))
		return
	}
	if snapshot == nil {
		s.report(errors.New("read Factory visualization snapshot: snapshot is unavailable"))
		return
	}
	s.mu.Lock()
	events := append([]factorydefinitions.FactoryEvent(nil), s.events...)
	s.mu.Unlock()
	worldState, err := s.projections.ReconstructFactoryWorldState(events, snapshot.TickCount)
	if err != nil {
		s.report(fmt.Errorf("project Factory visualization: %w", err))
		return
	}
	renderData := s.projections.SimpleDashboardRenderData(worldState)
	renderData.ActiveThrottlePauses = s.projections.ProjectActiveThrottlePauses(
		worldState.Topology,
		snapshot.ActiveThrottlePauses,
	)
	s.sink.PresentFactoryView(View{
		EngineState: *snapshot,
		RenderData:  renderData,
		ObservedAt:  s.clock.Now(),
	})
}

func (s *Service) advanceCursorLocked(events []factorydefinitions.FactoryEvent) {
	if len(events) == 0 {
		return
	}
	last := events[len(events)-1]
	sequence := last.Context.Sequence
	s.cursor = &factorydefinitions.FactoryEventReconnectCursor{
		AfterEventID:  last.Id,
		AfterSequence: &sequence,
	}
}

func (s *Service) report(err error) {
	if err != nil && s.reportError != nil {
		s.reportError(err)
	}
}
