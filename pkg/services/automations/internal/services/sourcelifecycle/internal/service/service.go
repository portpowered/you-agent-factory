package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jonboulle/clockwork"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	filesystemwatchers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/filesystem_watchers"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	sourcelifecycle "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/sourcelifecycle"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

const schedulerSourceKind = "runtime-scheduler-sidecars"

type sourceKey struct {
	runtimeID    string
	automationID string
}

type service struct {
	logger             *zap.Logger
	clock              clockwork.Clock
	scriptPollers      scriptpollers.Service
	cron               cron.Service
	filesystemWatchers filesystemwatchers.Service
	hostedPollers      automations.HostedPollers
	cursors            cursorscopes.CursorScopes
	executionPolicy    factorydefinitions.WorkstationExecutionPolicyService
	mu                 sync.Mutex
	sources            map[sourceKey]*source
}

var _ sourcelifecycle.SourceLifecycle = (*service)(nil)

// New stores completed collaborators and starts no source effects.
func New(logger *zap.Logger, clock clockwork.Clock, scriptPollers scriptpollers.Service,
	cronService cron.Service, filesystemWatchers filesystemwatchers.Service,
	hostedPollers automations.HostedPollers, cursors cursorscopes.CursorScopes,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) sourcelifecycle.SourceLifecycle {
	return &service{logger: logger, clock: clock, scriptPollers: scriptPollers,
		cron: cronService, filesystemWatchers: filesystemWatchers, hostedPollers: hostedPollers,
		cursors: cursors, executionPolicy: executionPolicy, sources: make(map[sourceKey]*source)}
}

func (s *service) ConfigureRuntimeSource(ctx context.Context, configuration sourcelifecycle.RuntimeSourceConfiguration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	config, err := newRuntimeSnapshotConfig(configuration.RuntimeID, configuration.Snapshot)
	if err != nil {
		return err
	}
	automationID := strings.TrimSpace(configuration.Snapshot.Invocation.WorkflowID)
	if automationID == "" {
		automationID = configuration.Snapshot.FactoryDir
	}
	key := sourceKey{runtimeID: configuration.RuntimeID, automationID: automationID}
	if configuration.Inputs.StartSchedulers {
		if err := s.validatePollers(config, configuration.Inputs.Submitter); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing := s.sources[key]; existing != nil {
		existing.mu.Lock()
		configured := existing.configured
		existing.mu.Unlock()
		if configured {
			return nil
		}
	}
	registered := &source{}
	registered.configureLocked(configuration, config)
	s.sources[key] = registered
	s.logger.Debug("configured automation source", zap.String("runtime_id", configuration.RuntimeID), zap.String("automation_id", automationID))
	return nil
}

func (s *service) find(runtimeID string, identity automations.SourceIdentity) (*source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registered := s.sources[sourceKey{runtimeID: runtimeID, automationID: identity.AutomationID}]
	if registered == nil {
		return nil, fmt.Errorf("scheduler source %q is not configured", identity.SourceID)
	}
	return registered, nil
}

func (s *service) Start(ctx context.Context, effect sourcelifecycle.StartEffect) error {
	if effect.Kind != schedulerSourceKind {
		return fmt.Errorf("unsupported scheduler source kind %q", effect.Kind)
	}
	registered, err := s.find(effect.RuntimeID, effect.Observation.Identity)
	if err != nil {
		return err
	}
	err = registered.start(ctx, s, effect.Observation.Identity.AutomationID)
	s.logger.Debug("automation source start completed", zap.String("runtime_id", effect.RuntimeID), zap.Bool("succeeded", err == nil))
	return err
}

func (s *service) Stop(ctx context.Context, effect sourcelifecycle.StopEffect) error {
	registered, err := s.find(effect.RuntimeID, effect.Observation.Identity)
	if err != nil {
		return err
	}
	err = registered.stop(ctx)
	s.logger.Debug("automation source stop completed", zap.String("runtime_id", effect.RuntimeID), zap.Bool("joined", err == nil))
	return err
}

func (s *service) Wait(ctx context.Context, effect sourcelifecycle.WaitEffect) (automations.SourceObservation, error) {
	registered, err := s.find(effect.RuntimeID, effect.Observation.Identity)
	if err != nil {
		return automations.SourceObservation{}, err
	}
	return registered.observe(ctx, effect)
}

func (s *service) ReleaseRuntimeSource(ctx context.Context, runtimeID string) error {
	s.mu.Lock()
	owned := make(map[sourceKey]*source)
	for key, registered := range s.sources {
		if key.runtimeID == runtimeID {
			owned[key] = registered
		}
	}
	s.mu.Unlock()
	for key, registered := range owned {
		if err := registered.stop(ctx); err != nil {
			return err
		}
		s.mu.Lock()
		if s.sources[key] == registered {
			s.cursors.ReleaseScope(cursorscopes.CursorScope{RuntimeID: runtimeID, BaseDir: registered.configuration.CursorBaseDir})
			delete(s.sources, key)
			s.logger.Debug("released automation source", zap.String("runtime_id", runtimeID), zap.String("automation_id", key.automationID))
		}
		s.mu.Unlock()
	}
	return nil
}
