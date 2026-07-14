package agypty

import "context"

// MockAllocator records Allocate calls and returns MockSession values for
// hermetic unit tests without ConPTY, POSIX PTY, or an installed Agy binary.
type MockAllocator struct {
	Sessions []*MockSession
	Err      error
}

// Allocate implements PTYAllocator.
func (m *MockAllocator) Allocate(_ context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	session := &MockSession{
		Launch: launch,
		Config: cfg,
	}
	m.Sessions = append(m.Sessions, session)
	return session, nil
}

// MockSession implements PTYSession with predetermined capture for tests.
type MockSession struct {
	Launch  ProcessLaunch
	Config  SessionConfig
	Result  SessionResult
	RunErr  error
	Closed  bool
	RunCall int
}

// Run implements PTYSession.
func (s *MockSession) Run(_ context.Context) (SessionResult, error) {
	s.RunCall++
	if s.RunErr != nil {
		return SessionResult{}, s.RunErr
	}
	result := s.Result
	if result.CleanedText == "" && len(result.RawBytes) > 0 {
		result.CleanedText = CleanTerminal(result.RawBytes)
	}
	return result, nil
}

// Close implements PTYSession.
func (s *MockSession) Close() error {
	s.Closed = true
	return nil
}
