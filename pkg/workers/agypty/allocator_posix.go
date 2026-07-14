//go:build linux || darwin

package agypty

import (
	"context"
	"os"

	"github.com/creack/pty"
)

// posixPTYOpener opens a POSIX master/slave PTY pair via openpty semantics.
// Tests inject a mock opener to exercise allocation seams without a live Agy binary.
type posixPTYOpener func() (master, slave *os.File, err error)

// POSIXPTYAllocator allocates a POSIX openpty master/slave pair for supervised Agy children.
type POSIXPTYAllocator struct {
	Open posixPTYOpener
}

// NewPOSIXPTYAllocator returns a POSIX PTY allocator that uses openpty via creack/pty.
func NewPOSIXPTYAllocator() *POSIXPTYAllocator {
	return &POSIXPTYAllocator{Open: pty.Open}
}

// Allocate implements PTYAllocator.
func (a *POSIXPTYAllocator) Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
	if err := checkAllocateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateProcessLaunch(launch); err != nil {
		return nil, err
	}
	opener := a.Open
	if opener == nil {
		opener = pty.Open
	}

	master, slave, err := opener()
	if err != nil {
		return nil, wrapPTYAllocationFailure(err)
	}

	allocation := &posixPTYAllocation{
		master: master,
		slave:  slave,
	}
	return newPlatformSession(launch, normalizeSessionConfig(cfg), PTYKindPOSIX, allocation)
}

type posixPTYAllocation struct {
	master *os.File
	slave  *os.File
}

func (p *posixPTYAllocation) Close() error {
	var firstErr error
	if p.master != nil {
		if err := p.master.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.master = nil
	}
	if p.slave != nil {
		if err := p.slave.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.slave = nil
	}
	return firstErr
}

// Master returns the POSIX PTY master fd used for host-side reads.
func (p *posixPTYAllocation) Master() *os.File {
	if p == nil {
		return nil
	}
	return p.master
}

// Slave returns the POSIX PTY slave TTY attached to the supervised child.
func (p *posixPTYAllocation) Slave() *os.File {
	if p == nil {
		return nil
	}
	return p.slave
}
