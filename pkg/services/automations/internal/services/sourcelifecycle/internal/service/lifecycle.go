package service

import (
	"context"
	"fmt"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
)

type source struct {
	mu            sync.Mutex
	configuration sourcelifecycle.RuntimeSourceConfiguration
	config        *runtimeSnapshotConfig
	configured    bool
	active        bool
	cancel        context.CancelFunc
	children      *sync.WaitGroup
	started       chan struct{}
	stopped       chan struct{}
	launchErr     error
}

func (s *source) configureLocked(configuration sourcelifecycle.RuntimeSourceConfiguration, config *runtimeSnapshotConfig) {
	s.configuration = configuration
	s.config = config
	s.configured = true
	s.started = make(chan struct{})
	s.stopped = make(chan struct{})
	s.launchErr = nil
}

func (s *source) start(parent context.Context, owner *service, automationID string) error {
	s.mu.Lock()
	if !s.configured {
		s.mu.Unlock()
		return fmt.Errorf("scheduler source configuration is required")
	}
	if s.active {
		s.mu.Unlock()
		return fmt.Errorf("scheduler source is already active")
	}
	sourceCtx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.children = &sync.WaitGroup{}
	s.active = true
	configuration, config, children, started := s.configuration, s.config, s.children, s.started
	s.mu.Unlock()
	err := owner.launch(sourceCtx, children, automationID, configuration, config)
	stopped := s.stopped
	go func() {
		<-sourceCtx.Done()
		children.Wait()
		close(stopped)
	}()
	s.mu.Lock()
	s.launchErr = err
	close(started)
	s.mu.Unlock()
	if err != nil {
		_ = s.stop(context.Background())
	}
	return err
}

func (s *source) stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	cancel, started, children := s.cancel, s.started, s.children
	s.mu.Unlock()
	cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-started:
	}
	children.Wait()
	s.mu.Lock()
	// Another completed stop may already have permitted a replacement start.
	if s.children == children {
		s.active = false
		s.configured = false
	}
	s.mu.Unlock()
	return nil
}

func (s *source) observe(ctx context.Context, effect sourcelifecycle.WaitEffect) (automations.SourceObservation, error) {
	observation := effect.Observation
	s.mu.Lock()
	active, started, stopped := s.active, s.started, s.stopped
	s.mu.Unlock()
	if effect.Desired == automations.DesiredLifecycleStopped && !active {
		observation.State = automations.ObservedLifecycleStopped
		return observation, nil
	}
	if started == nil {
		return automations.SourceObservation{}, fmt.Errorf("scheduler source has not started")
	}
	select {
	case <-ctx.Done():
		return automations.SourceObservation{}, ctx.Err()
	case <-started:
	}
	if effect.Desired == automations.DesiredLifecycleStopped {
		select {
		case <-ctx.Done():
			return automations.SourceObservation{}, ctx.Err()
		case <-stopped:
		}
	}
	s.mu.Lock()
	active, launchErr := s.active, s.launchErr
	s.mu.Unlock()
	if launchErr != nil {
		return automations.SourceObservation{}, launchErr
	}
	if effect.Desired == automations.DesiredLifecycleStopped || !active {
		observation.State = automations.ObservedLifecycleStopped
	} else {
		observation.State = automations.ObservedLifecycleRunning
	}
	return observation, nil
}
