package service

import (
	"context"
	"errors"
	"fmt"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	visualizationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/recordingsqueries"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

var (
	errAlreadyStarted = errors.New("start Factory visualization: already started")
	errNotStarted     = errors.New("wait for Factory visualization: not started")
)

// Service owns retained event projection, reconnect cursor, and live
// subscription lifecycle for Factory visualization activation.
type Service struct {
	source      visualizationcontracts.ActivationEventSource
	recordings  recordings.Service
	clock       visualizationcontracts.ActivationClock
	sink        visualizationcontracts.ActivationViewSink
	reportError activationlifecycle.ErrorReporter

	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	runErr   error
	started  bool
	stopOnce sync.Once

	events []factorydefinitions.FactoryEvent
	cursor *factorydefinitions.FactoryEventReconnectCursor
}

var _ activationlifecycle.Service = (*Service)(nil)

// New constructs an inert activation lifecycle owner.
func New(
	source visualizationcontracts.ActivationEventSource,
	recordingsPeer recordings.Service,
	clock visualizationcontracts.ActivationClock,
	sink visualizationcontracts.ActivationViewSink,
	reportError activationlifecycle.ErrorReporter,
) (*Service, error) {
	switch {
	case source == nil:
		return nil, errors.New("initialize Factory visualization activation: event source is required")
	case recordingsPeer == nil:
		return nil, errors.New("initialize Factory visualization activation: recordings service is required")
	case clock == nil:
		return nil, errors.New("initialize Factory visualization activation: clock is required")
	case sink == nil:
		return nil, errors.New("initialize Factory visualization activation: presentation sink is required")
	default:
		return &Service{
			source: source, recordings: recordingsPeer, clock: clock,
			sink: sink, reportError: reportError,
		}, nil
	}
}

// Start subscribes once to retained-then-live canonical Factory events.
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

// Stop cancels and joins the event subscription, then emits one final view.
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

// Activate validates explicit request parameters then delegates to Start.
func (s *Service) Activate(
	ctx context.Context,
	req activationlifecycle.ActivateRequest,
) (activationlifecycle.ActivateResult, error) {
	if req.Mode == "" {
		return activationlifecycle.ActivateResult{}, &activationlifecycle.LifecycleError{
			Kind:    activationlifecycle.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	if req.Mode != activationlifecycle.ActivateModeRetainedThenLive {
		return activationlifecycle.ActivateResult{}, &activationlifecycle.LifecycleError{
			Kind:    activationlifecycle.LifecycleErrorMissingParameters,
			Message: fmt.Sprintf("activate Factory visualization: activate mode %q is not supported", req.Mode),
		}
	}
	err := s.Start(ctx)
	if err == nil {
		return activationlifecycle.ActivateResult{State: activationlifecycle.LifecycleStateStarted}, nil
	}
	if errors.Is(err, errAlreadyStarted) {
		return activationlifecycle.ActivateResult{}, &activationlifecycle.LifecycleError{
			Kind:    activationlifecycle.LifecycleErrorAlreadyActivated,
			Message: "activate Factory visualization: already activated",
			Cause:   err,
		}
	}
	return activationlifecycle.ActivateResult{}, err
}

// Join waits for the live subscription to exit.
func (s *Service) Join(
	ctx context.Context,
	_ activationlifecycle.JoinRequest,
) (activationlifecycle.JoinResult, error) {
	err := s.Wait(ctx)
	if err == nil {
		return activationlifecycle.JoinResult{State: activationlifecycle.LifecycleStateStarted}, nil
	}
	if errors.Is(err, errNotStarted) {
		return activationlifecycle.JoinResult{}, &activationlifecycle.LifecycleError{
			Kind:    activationlifecycle.LifecycleErrorNotActivated,
			Message: "join Factory visualization: not activated",
			Cause:   err,
		}
	}
	return activationlifecycle.JoinResult{}, err
}

// StopDrain cancels the live subscription and joins it.
func (s *Service) StopDrain(
	ctx context.Context,
	_ activationlifecycle.StopDrainRequest,
) (activationlifecycle.StopDrainResult, error) {
	if err := s.Stop(ctx); err != nil {
		return activationlifecycle.StopDrainResult{}, err
	}
	return activationlifecycle.StopDrainResult{State: activationlifecycle.LifecycleStateStopped}, nil
}

// RetainedEvents returns a copy of retained canonical events for sibling root use.
func (s *Service) RetainedEvents() []factorydefinitions.FactoryEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]factorydefinitions.FactoryEvent(nil), s.events...)
}

// ReconnectCursor returns the current reconnect cursor for sibling root use.
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
	observation, err := s.source.GetEngineObservation(ctx)
	if err != nil {
		s.report(fmt.Errorf("read Factory visualization engine observation: %w", err))
		return
	}
	if observation == nil {
		s.report(errors.New("read Factory visualization engine observation: observation is unavailable"))
		return
	}
	s.mu.Lock()
	events := append([]factorydefinitions.FactoryEvent(nil), s.events...)
	s.mu.Unlock()
	worldView, err := recordingsqueries.ReconstructWorldState(s.recordings, events, observation.TickCount)
	if err != nil {
		s.report(fmt.Errorf("project Factory visualization: %w", err))
		return
	}
	renderData, err := recordingsqueries.QuerySimpleDashboard(s.recordings, worldView)
	if err != nil {
		s.report(fmt.Errorf("project Factory visualization dashboard: %w", err))
		return
	}
	worldState, err := recordingsqueries.DecodeWorldStatePayload(worldView)
	if err != nil {
		s.report(fmt.Errorf("decode Factory visualization world state: %w", err))
		return
	}
	renderData.ActiveThrottlePauses = recordingsqueries.ProjectActiveThrottlePauses(
		worldState.Topology,
		observation.ActiveThrottlePauses,
	)
	s.sink.PresentFactoryView(activationlifecycle.View{
		EngineObservation: *observation,
		RenderData:        renderData,
		ObservedAt:        s.clock.Now(),
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
