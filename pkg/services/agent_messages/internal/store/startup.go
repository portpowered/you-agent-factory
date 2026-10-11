package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

// Activator is the journal's reconstruction/retention capability, not a Factory
// lifecycle or execution dependency. Journal retains any activation fault.
type Activator interface {
	Bind(string) error
	Activate(time.Time, time.Duration) error
}

// Startup activates the already constructed store once. It owns no background
// process, ticker or open descriptor. Store faults leave messaging quarantined
// and never fail the application's other startup roles.
type Startup struct {
	mu        sync.Mutex
	started   bool
	path      string
	enabled   bool
	journal   Activator
	now       func() time.Time
	retention time.Duration
	logger    logging.Logger
}

func NewStartup(enabled bool, journal Activator, now func() time.Time, retention time.Duration, logger logging.Logger) *Startup {
	return &Startup{enabled: enabled, journal: journal, now: now, retention: retention, logger: logger}
}

func (s *Startup) Start(ctx context.Context) error {
	return s.start(ctx, "", nil)
}

// StartAt binds the already constructed journal to the selected profile's
// absolute store path. A mismatched profile is rejected before exposure; store
// activation faults still quarantine Messaging without failing Factory startup.
// Explicit settings arrive after profile selection, rather than at construction.
// Once selected, they cannot be changed, even for a disabled/quarantined store.
func (s *Startup) StartAt(ctx context.Context, path string, enabled bool, retention time.Duration) error {
	if path == "" || !filepath.IsAbs(path) || retention < 24*time.Hour {
		return agentmessages.ErrBadRequest
	}
	return s.start(ctx, filepath.Clean(path), &startupSettings{enabled, retention})
}

type startupSettings struct {
	enabled   bool
	retention time.Duration
}

func (s *Startup) start(ctx context.Context, path string, settings *startupSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.started {
		if path != "" && (s.path != path || s.enabled != settings.enabled || s.retention != settings.retention) {
			return agentmessages.ErrBadRequest
		}
		return nil
	}
	enabled, retention := s.enabled, s.retention
	if settings != nil {
		enabled, retention = settings.enabled, settings.retention
	}
	if path != "" && enabled {
		if err := s.journal.Bind(path); err != nil {
			return err
		}
	}
	s.enabled, s.retention = enabled, retention
	s.path = path
	s.started = true
	if !s.enabled {
		s.logger.Info("Agent Message store disabled")
		return nil
	}
	s.logger.Info("Agent Message store activation started")
	if err := s.journal.Activate(s.now().UTC(), s.retention); err != nil {
		code := ErrUnavailable.Error()
		if errors.Is(err, ErrCorrupt) {
			code = ErrCorrupt.Error()
		}
		// Never attach collaborator diagnostics, paths or bytes. The journal
		// preserves its own safe typed fault for subsequent message operations.
		s.logger.Warn("Agent Message store quarantined", "code", code)
		return nil
	}
	s.logger.Info("Agent Message store activated")
	return nil
}
