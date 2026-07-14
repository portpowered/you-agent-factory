//go:build !windows && !linux && !darwin

package agypty

import "context"

type unsupportedPTYAllocator struct{}

// NewUnsupportedPTYAllocator returns an allocator that always fails closed.
func NewUnsupportedPTYAllocator() PTYAllocator {
	return unsupportedPTYAllocator{}
}

// Allocate implements PTYAllocator.
func (unsupportedPTYAllocator) Allocate(context.Context, ProcessLaunch, SessionConfig) (PTYSession, error) {
	return nil, ErrUnsupportedPlatform
}

func newPlatformPTYAllocator() PTYAllocator {
	return NewUnsupportedPTYAllocator()
}

func runPlatformSession(context.Context, *platformSession) (SessionResult, error) {
	return SessionResult{}, ErrUnsupportedPlatform
}

func sessionProcessRunning(int) bool {
	return false
}

func terminateSessionTestProcess(int) {}

func (p *sessionProcess) Wait() error {
	return ErrUnsupportedPlatform
}
