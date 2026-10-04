package agypty

import (
	"context"
	"errors"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
)

// Allocator combines the injected native host effect with Providers-owned
// validation, session limits, capture, timeout, and output-cleaning policy.
type Allocator struct {
	host      platformpty.Host
	clock     platformclock.Source
	scheduler platformclock.TimerSource
}

// NewAllocator stores the completed host, duration clock, and scheduler.
// Canonical composition supplies every required effect before construction.
func NewAllocator(host platformpty.Host, clock platformclock.Source, scheduler platformclock.TimerSource) (*Allocator, error) {
	return &Allocator{host: host, clock: clock, scheduler: scheduler}, nil
}

// Allocate validates owner input, obtains an opaque native PTY, and returns an
// inert session whose policy remains owned by Providers.
func (a *Allocator) Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
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
	session, err := newPlatformSession(launch, normalizeSessionConfig(cfg), platformPTYKind(native.Kind()), native, a.host, a.clock, a.scheduler)
	if err != nil {
		_ = native.Close()
		return nil, err
	}
	return session, nil
}
