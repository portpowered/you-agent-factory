package agypty

import (
	"context"
	"errors"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
)

// Allocator combines the injected native host effect with Workers-owned
// validation, session limits, capture, timeout, and output-cleaning policy.
type Allocator struct {
	host  platformpty.Host
	clock platformclock.Source
}

// NewAllocator fails closed unless both external effects are injected.
func NewAllocator(host platformpty.Host, clock platformclock.Source) (*Allocator, error) {
	if host == nil {
		return nil, ErrHostRequired
	}
	if clock == nil {
		return nil, ErrClockRequired
	}
	return &Allocator{host: host, clock: clock}, nil
}

// Allocate validates owner input, obtains an opaque native PTY, and returns an
// inert session whose policy remains owned by Workers.
func (a *Allocator) Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
	if a == nil || a.host == nil {
		return nil, ErrHostRequired
	}
	if a.clock == nil {
		return nil, ErrClockRequired
	}
	if err := checkAllocateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateProcessLaunch(launch); err != nil {
		return nil, err
	}
	native, err := a.host.Allocate(ctx)
	if err != nil {
		if errors.Is(err, platformpty.ErrUnsupportedPlatform) {
			return nil, ErrUnsupportedPlatform
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, wrapPTYAllocationFailure(err)
	}
	if native == nil {
		return nil, wrapPTYAllocationFailure(ErrPTYAllocationFailed)
	}
	session, err := newPlatformSession(launch, normalizeSessionConfig(cfg), platformPTYKind(native.Kind()), native, a.host, a.clock)
	if err != nil {
		_ = native.Close()
		return nil, err
	}
	return session, nil
}
