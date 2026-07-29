package agypty

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed testdata/argv_fixtures.json
var argvFixturesJSON []byte

//go:embed testdata/workspace_fixtures.json
var workspaceFixturesJSON []byte

type argvFixtureFile struct {
	Fixtures []argvFixture `json:"fixtures"`
}

type argvFixture struct {
	Name      string    `json:"name"`
	Spec      *ArgvSpec `json:"spec,omitempty"`
	Argv      []string  `json:"argv,omitempty"`
	WantArgv  []string  `json:"want_argv,omitempty"`
	WantError string    `json:"want_error,omitempty"`
}

type workspaceFixtureFile struct {
	Fixtures []workspaceFixture `json:"fixtures"`
}

type workspaceFixture struct {
	Name        string   `json:"name"`
	FactoryRoot string   `json:"factory_root"`
	RawPath     string   `json:"raw_path"`
	WantSuffix  []string `json:"want_suffix,omitempty"`
	WantError   string   `json:"want_error,omitempty"`
}

// LoadArgvFixtures returns hermetic argv corpus entries for unit tests.
func LoadArgvFixtures() ([]argvFixture, error) {
	var file argvFixtureFile
	if err := json.Unmarshal(argvFixturesJSON, &file); err != nil {
		return nil, fmt.Errorf("agypty: decode argv fixtures: %w", err)
	}
	return file.Fixtures, nil
}

// LoadWorkspaceFixtures returns hermetic workspace path corpus entries for unit tests.
func LoadWorkspaceFixtures() ([]workspaceFixture, error) {
	var file workspaceFixtureFile
	if err := json.Unmarshal(workspaceFixturesJSON, &file); err != nil {
		return nil, fmt.Errorf("agypty: decode workspace fixtures: %w", err)
	}
	return file.Fixtures, nil
}

// MockAllocator records Allocate calls and returns MockSession values for
// hermetic unit tests without ConPTY, POSIX PTY, or an installed Agy binary.
type MockAllocator struct {
	Sessions []*MockSession
	Result   SessionResult
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
		Result: m.Result,
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
