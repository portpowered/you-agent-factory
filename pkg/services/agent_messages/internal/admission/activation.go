package admission

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type ProfileStartup interface {
	StartAt(context.Context, string, bool, time.Duration) error
}

// Configured gates the already constructed Engine until the host selects its
// profile. Policy is published once and never changes beneath an admitted call.
// Journal faults remain Messaging faults; activation does not fail the host.
type Configured struct {
	mu       sync.Mutex
	engine   *Engine
	startup  ProfileStartup
	load     operatorsettings.ConfigLoader
	path     string
	selected bool
	fault    error
}

func NewConfigured(engine *Engine, startup ProfileStartup, load operatorsettings.ConfigLoader) (*Configured, error) {
	if engine == nil || startup == nil || load == nil {
		return nil, agentmessages.ErrBadRequest
	}
	if _, ok := engine.validator.(Validator); !ok {
		return nil, agentmessages.ErrBadRequest
	}
	return &Configured{engine: engine, startup: startup, load: load}, nil
}

func (s *Configured) Activate(ctx context.Context, configPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if configPath == "" || !filepath.IsAbs(configPath) {
		return agentmessages.ErrBadRequest
	}
	configPath = filepath.Clean(configPath)
	if s.selected {
		if s.path != configPath {
			return agentmessages.ErrBadRequest
		}
		return nil
	}
	config, err := s.load(configPath)
	if err != nil {
		return s.quarantine(configPath)
	}
	settings, err := config.Messaging.Effective()
	if err != nil {
		return s.quarantine(configPath)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(configPath), "messages", "messages.jsonl")
	if err := s.startup.StartAt(ctx, path, settings.Enabled, time.Duration(settings.RetentionDays)*24*time.Hour); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return s.quarantine(configPath)
	}
	validator := s.engine.validator.(Validator)
	validator.BodyBytes, validator.DefaultExpirySeconds = settings.MaxBodyBytes, settings.DefaultExpirySeconds
	s.engine.validator = validator
	s.engine.quota = Limits{SendsPerHour: settings.SendsPerChainPerHour, ThreadMessages: settings.MessagesPerThread, HopLimit: &settings.MaxHop}
	s.engine.enabled = settings.Enabled
	s.path, s.selected = configPath, true
	return nil
}

func (s *Configured) quarantine(path string) error {
	s.path, s.selected, s.fault = path, true, agentmessages.ErrStoreUnavailable
	s.engine.logger.Warn("Agent Message configuration quarantined", "code", "MESSAGE_STORE_UNAVAILABLE")
	return nil
}

func (s *Configured) ready() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.selected {
		return agentmessages.ErrDisabled
	}
	return s.fault
}

func (s *Configured) Send(ctx context.Context, request agentmessages.SendRequest, caller *workersessions.CallerIdentity, factory string) (agentmessages.Message, error) {
	if err := s.ready(); err != nil {
		return agentmessages.Message{}, err
	}
	return s.engine.Send(ctx, request, caller, factory)
}

func (s *Configured) Get(ctx context.Context, request agentmessages.GetRequest) (agentmessages.Message, error) {
	if err := s.ready(); err != nil {
		return agentmessages.Message{}, err
	}
	return s.engine.Get(ctx, request)
}

func (s *Configured) List(ctx context.Context, request agentmessages.ListRequest) (agentmessages.Page, error) {
	if err := s.ready(); err != nil {
		return agentmessages.Page{}, err
	}
	return s.engine.List(ctx, request)
}

func (s *Configured) Follow(ctx context.Context, request agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.engine.Follow(ctx, request, observe)
}
