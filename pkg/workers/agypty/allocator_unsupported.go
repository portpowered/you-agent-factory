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
