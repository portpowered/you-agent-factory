// Package agypty contains the small PTY compatibility seam used by the
// retained provider adapter tests. The active native PTY implementation stays
// owned by Providers; this package only adapts the Workers-owned test seam.
package agypty

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type SessionConfig = workers.PTYSessionConfig
type ProcessLaunch = workers.PTYProcessLaunch
type SessionResult = workers.PTYSessionResult
type PTYAllocator = workers.PTYAllocator
type PTYSession = workers.PTYSession

// MockAllocator records compatibility allocation calls without opening a
// native PTY.
type MockAllocator struct {
	Sessions []*MockSession
	Result   SessionResult
	Err      error
}

func (allocator *MockAllocator) Allocate(
	_ context.Context,
	launch ProcessLaunch,
	config SessionConfig,
) (PTYSession, error) {
	if allocator.Err != nil {
		return nil, allocator.Err
	}
	session := &MockSession{Launch: launch, Config: config, Result: allocator.Result}
	allocator.Sessions = append(allocator.Sessions, session)
	return session, nil
}

type MockSession struct {
	Launch  ProcessLaunch
	Config  SessionConfig
	Result  SessionResult
	RunErr  error
	Closed  bool
	RunCall int
}

func (session *MockSession) Run(_ context.Context) (SessionResult, error) {
	session.RunCall++
	if session.RunErr != nil {
		return SessionResult{}, session.RunErr
	}
	return session.Result, nil
}

func (session *MockSession) Close() error {
	session.Closed = true
	return nil
}
