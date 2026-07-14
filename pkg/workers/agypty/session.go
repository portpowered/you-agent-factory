package agypty

import (
	"context"
	"errors"
	"sync"
)

// platformSession holds an allocated platform PTY for one supervised Agy child.
// Story 002 implements Run with capture, timeout, and cleanup.
type platformSession struct {
	launch ProcessLaunch
	cfg    SessionConfig
	kind   PTYKind
	pty    ptyAllocation
	mu     sync.Mutex
	closed bool
}

func (s *platformSession) Run(context.Context) (SessionResult, error) {
	return SessionResult{}, errSessionRunPending
}

func (s *platformSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.pty == nil {
		return nil
	}
	return s.pty.Close()
}

func (s *platformSession) PTYKind() PTYKind {
	if s == nil {
		return PTYKindUnknown
	}
	return s.kind
}

func newPlatformSession(launch ProcessLaunch, cfg SessionConfig, kind PTYKind, pty ptyAllocation) (*platformSession, error) {
	if pty == nil {
		return nil, errors.New("agypty: PTY allocation is required")
	}
	return &platformSession{
		launch: launch,
		cfg:    cfg,
		kind:   kind,
		pty:    pty,
	}, nil
}
