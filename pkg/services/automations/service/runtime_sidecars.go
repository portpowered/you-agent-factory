package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
	reconciliationwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation/wire"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

const (
	runtimeSchedulerSourceID   = "scheduler-sidecars"
	runtimeSchedulerSourceKind = "runtime-scheduler-sidecars"
)

type schedulerSourceConfig struct {
	sidecars      *sync.WaitGroup
	factoryDir    string
	factoryConfig *interfaces.FactoryConfig
	runtimeConfig interfaces.RuntimeConfigLookup
	submitter     automations.WorkRequestSubmitter
}

type schedulerSource struct {
	mu sync.Mutex

	config     schedulerSourceConfig
	configured bool
	active     bool
	ctx        context.Context
	cancel     context.CancelFunc
	children   *sync.WaitGroup
	started    chan struct{}
	launchErr  error
}

// StartSchedulerSidecarsForRuntime supervises configured poller and cron workstations
// until ctx is canceled. Runtime hosts attach both schedulers through this narrow API.
func (s *Service) StartSchedulerSidecarsForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryConfig *interfaces.FactoryConfig,
	runtimeConfig interfaces.RuntimeConfigLookup,
	submitter automations.WorkRequestSubmitter,
) error {
	if factoryConfig == nil || runtimeConfig == nil || sidecars == nil || submitter == nil {
		return nil
	}
	if err := s.ValidatePollersForRuntime(factoryConfig, runtimeConfig, submitter); err != nil {
		return err
	}

	identity := s.schedulerSourceIdentity(factoryDir)
	source := s.schedulerSource(identity)
	source.configure(schedulerSourceConfig{
		sidecars:      sidecars,
		factoryDir:    factoryDir,
		factoryConfig: factoryConfig,
		runtimeConfig: runtimeConfig,
		submitter:     submitter,
	})

	_, err := s.reconciler.StartSource(ctx, automations.StartSourceRequest{
		Identity: identity,
		Kind:     runtimeSchedulerSourceKind,
	})
	if err != nil {
		return err
	}
	_, err = s.reconciler.WaitSource(ctx, automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleRunning,
	})
	return err
}

func (s *Service) newSchedulerReconciler() reconciliation.Service {
	return reconciliationwire.NewService(reconciliation.Effects{
		Start: s.startSchedulerSource,
		Stop:  s.stopSchedulerSource,
		Wait:  s.waitSchedulerSource,
	})
}

func (s *Service) schedulerSourceIdentity(factoryDir string) automations.SourceIdentity {
	return automations.SourceIdentity{
		AutomationID: strings.TrimSpace(s.workflowIdentity(factoryDir)),
		SourceID:     runtimeSchedulerSourceID,
	}
}

func (s *Service) schedulerSource(identity automations.SourceIdentity) *schedulerSource {
	s.schedulerMu.Lock()
	defer s.schedulerMu.Unlock()
	source := s.schedulerSources[identity]
	if source == nil {
		source = &schedulerSource{}
		s.schedulerSources[identity] = source
	}
	return source
}

func (s *Service) findSchedulerSource(identity automations.SourceIdentity) (*schedulerSource, error) {
	s.schedulerMu.Lock()
	defer s.schedulerMu.Unlock()
	source := s.schedulerSources[identity]
	if source == nil {
		return nil, fmt.Errorf("scheduler source %q is not configured", identity.SourceID)
	}
	return source, nil
}

func (s *Service) startSchedulerSource(
	ctx context.Context,
	effect reconciliation.StartEffect,
) error {
	if effect.Kind != runtimeSchedulerSourceKind {
		return fmt.Errorf("unsupported scheduler source kind %q", effect.Kind)
	}
	source, err := s.findSchedulerSource(effect.Observation.Identity)
	if err != nil {
		return err
	}
	return source.start(ctx, s, effect.Observation.Identity)
}

func (s *Service) stopSchedulerSource(
	ctx context.Context,
	effect reconciliation.StopEffect,
) error {
	source, err := s.findSchedulerSource(effect.Observation.Identity)
	if err != nil {
		return err
	}
	return source.stop(ctx)
}

func (s *Service) waitSchedulerSource(
	ctx context.Context,
	effect reconciliation.WaitEffect,
) (automations.SourceObservation, error) {
	source, err := s.findSchedulerSource(effect.Observation.Identity)
	if err != nil {
		return automations.SourceObservation{}, err
	}
	return source.observe(ctx, effect)
}

func (s *Service) monitorSchedulerSource(
	identity automations.SourceIdentity,
	sourceCtx context.Context,
	sidecars *sync.WaitGroup,
) {
	defer sidecars.Done()
	<-sourceCtx.Done()
	stopCtx := context.WithoutCancel(sourceCtx)
	if _, err := s.reconciler.StopSource(stopCtx, automations.StopSourceRequest{
		Identity: identity,
	}); err != nil {
		s.logger().Error("stop scheduler source reconciliation failed", zap.Error(err))
		return
	}
	if _, err := s.reconciler.WaitSource(stopCtx, automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleStopped,
	}); err != nil {
		s.logger().Error("wait scheduler source reconciliation failed", zap.Error(err))
	}
}

func (s *Service) launchSchedulerSource(
	ctx context.Context,
	children *sync.WaitGroup,
	config schedulerSourceConfig,
) error {
	s.StartCronWatchersForRuntime(
		ctx,
		children,
		config.factoryDir,
		config.factoryConfig,
		config.runtimeConfig,
		config.submitter,
	)
	if err := s.StartPollersForRuntime(
		ctx,
		children,
		config.factoryConfig,
		config.runtimeConfig,
		config.submitter,
	); err != nil {
		return err
	}
	return nil
}

func (s *schedulerSource) configure(config schedulerSourceConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.configured {
		s.config = config
		s.configured = true
	}
}

func (s *schedulerSource) start(
	parent context.Context,
	owner *Service,
	identity automations.SourceIdentity,
) error {
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
	s.ctx = sourceCtx
	s.cancel = cancel
	s.children = &sync.WaitGroup{}
	s.started = make(chan struct{})
	s.launchErr = nil
	s.active = true
	s.config.sidecars.Add(1)
	go owner.monitorSchedulerSource(identity, sourceCtx, s.config.sidecars)
	config := s.config
	children := s.children
	started := s.started
	s.mu.Unlock()

	err := owner.launchSchedulerSource(sourceCtx, children, config)
	s.mu.Lock()
	s.launchErr = err
	close(started)
	s.mu.Unlock()
	return err
}

func (s *schedulerSource) stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return nil
	}
	cancel := s.cancel
	started := s.started
	children := s.children
	s.mu.Unlock()

	cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-started:
	}
	children.Wait()

	s.mu.Lock()
	s.active = false
	s.configured = false
	s.mu.Unlock()
	return nil
}

func (s *schedulerSource) observe(
	ctx context.Context,
	effect reconciliation.WaitEffect,
) (automations.SourceObservation, error) {
	observation := effect.Observation
	s.mu.Lock()
	active := s.active
	started := s.started
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

	s.mu.Lock()
	active = s.active
	launchErr := s.launchErr
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
