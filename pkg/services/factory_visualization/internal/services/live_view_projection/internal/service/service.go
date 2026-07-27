package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
)

// Service owns retained event projection, reconnect cursor, and live
// subscription lifecycle for one Factory visualization.
type Service struct {
	source      liveviewprojection.Source
	projections recordings.ProjectionService
	clock       liveviewprojection.Clock
	sink        liveviewprojection.Sink
	reportError liveviewprojection.ErrorReporter

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	runErr   error
	started  bool
	stopOnce sync.Once

	events []factorydefinitions.FactoryEvent
	cursor *factorydefinitions.FactoryEventReconnectCursor
}

var _ liveviewprojection.Service = (*Service)(nil)

var (
	errAlreadyStarted = liveviewprojection.ErrLiveViewProjectionAlreadyStarted
	errNotStarted     = liveviewprojection.ErrLiveViewProjectionNotStarted
)

// New constructs the private live_view_projection implementation.
func New(
	source liveviewprojection.Source,
	projections recordings.ProjectionService,
	clock liveviewprojection.Clock,
	sink liveviewprojection.Sink,
	reportError liveviewprojection.ErrorReporter,
) (*Service, error) {
	switch {
	case source == nil:
		return nil, errors.New("initialize Factory visualization live view projection: event source is required")
	case projections == nil:
		return nil, errors.New("initialize Factory visualization live view projection: projection service is required")
	case clock == nil:
		return nil, errors.New("initialize Factory visualization live view projection: clock is required")
	case sink == nil:
		return nil, errors.New("initialize Factory visualization live view projection: presentation sink is required")
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
		return errors.New("start Factory visualization live view projection: service is required")
	}
	if ctx == nil {
		return errors.New("start Factory visualization live view projection: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errAlreadyStarted
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
		return &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "start Factory visualization live view projection: subscribe to Factory events failed",
			Cause:   err,
		}
	}
	if stream == nil || stream.Events == nil {
		cancel()
		s.mu.Unlock()
		return &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "start Factory visualization live view projection: event source returned an invalid stream",
		}
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
		return errNotStarted
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

// Observe returns one detached retained-then-live Factory view projection.
func (s *Service) Observe(
	ctx context.Context,
	req liveviewprojection.ObserveRequest,
) (liveviewprojection.ObserveResult, error) {
	if s == nil {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization live view projection: service is required",
		}
	}
	if ctx == nil {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization live view projection: context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return liveviewprojection.ObserveResult{}, err
	}
	if req.Mode == "" {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization live view projection: required request parameters are missing",
		}
	}
	if req.Mode != liveviewprojection.ObserveModeRetainedThenLive {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: fmt.Sprintf("observe Factory visualization live view projection: observe mode %q is not supported", req.Mode),
		}
	}
	if err := validateObserveReconnect(req.Reconnect); err != nil {
		return liveviewprojection.ObserveResult{}, err
	}

	snapshot, err := s.source.GetEngineStateSnapshot(ctx)
	if err != nil {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorSnapshotUnavailable,
			Message: "observe Factory visualization live view projection: snapshot is unavailable",
			Cause:   err,
		}
	}
	if snapshot == nil {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorSnapshotUnavailable,
			Message: "observe Factory visualization live view projection: snapshot is unavailable",
		}
	}

	s.mu.Lock()
	events := append([]factorydefinitions.FactoryEvent(nil), s.events...)
	s.mu.Unlock()

	if req.Reconnect != nil {
		cursor := factorydefinitions.FactoryEventReconnectCursor{
			AfterEventID:  req.Reconnect.AfterEventID,
			AfterSequence: req.Reconnect.AfterSequence,
		}
		if err := s.projections.ValidateReconnectReplay(
			events,
			cursor,
			factorydefinitions.FactoryEventReconnectScope{},
		); err != nil {
			return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
				Kind:    liveviewprojection.ProjectionErrorInvalidInput,
				Message: "observe Factory visualization live view projection: reconnect observe input is invalid",
				Cause:   err,
			}
		}
	}

	if _, err := s.projections.ReconstructFactoryWorldState(events, snapshot.TickCount); err != nil {
		return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorReconstructionFailed,
			Message: "observe Factory visualization live view projection: projection reconstruction failed",
			Cause:   err,
		}
	}

	return liveviewprojection.ObserveResult{
		View: liveviewprojection.ProjectedView{
			TickCount:          snapshot.TickCount,
			RetainedEventCount: len(events),
			ObservedAt:         s.clock.Now(),
		},
	}, nil
}

// ReconnectCursor returns the Visualization-owned reconnect cursor after the
// latest retained or live event was applied.
func (s *Service) ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor == nil {
		return nil
	}
	cursor := *s.cursor
	return &cursor
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
	s.sink.PresentFactoryView(liveviewprojection.View{
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

func validateObserveReconnect(reconnect *liveviewprojection.ObserveReconnectCursor) error {
	if reconnect == nil {
		return nil
	}
	if reconnect.AfterEventID == "" && reconnect.AfterSequence == nil {
		return &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization live view projection: reconnect cursor is empty",
		}
	}
	if reconnect.AfterSequence != nil && *reconnect.AfterSequence < 0 {
		return &liveviewprojection.ProjectionError{
			Kind:    liveviewprojection.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization live view projection: reconnect after_sequence is invalid",
		}
	}
	return nil
}
