//go:build !windows && !linux && !darwin

package pty

import (
	"context"
	"io"
)

type unsupportedHost struct{}

// NewHost fails closed on platforms without an approved native PTY.
func NewHost() Host { return unsupportedHost{} }

func (unsupportedHost) Allocate(context.Context) (Allocation, error) {
	return nil, ErrUnsupportedPlatform
}

func (unsupportedHost) Start(ProcessLaunch, Allocation) (Process, io.ReadCloser, error) {
	return nil, nil, ErrUnsupportedPlatform
}
