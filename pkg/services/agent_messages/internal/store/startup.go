package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// Activator is the journal's reconstruction/retention capability, not a Factory
// lifecycle or execution dependency. Journal retains any activation fault.
type Activator interface {
	Activate(time.Time, time.Duration) error
}

// Startup activates the already constructed store once. It owns no background
// process, ticker or open descriptor. Store faults leave messaging quarantined
// and never fail the application's other startup roles.
type Startup struct {
	mu        sync.Mutex
	started   bool
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.started {
		return nil
	}
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
