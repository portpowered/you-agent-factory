//go:build windows

package agypty

import (
	"context"
	"os"

	"golang.org/x/sys/windows"
)

// conPTYOpener allocates a Windows ConPTY pseudo-console pair.
// Tests inject a mock opener to exercise allocation seams without a live Agy binary.
type conPTYOpener func() (*conPTYAllocation, error)

// WindowsConPTYAllocator allocates a ConPTY pseudo-console for supervised Agy children.
type WindowsConPTYAllocator struct {
	Open conPTYOpener
}

// NewWindowsConPTYAllocator returns a ConPTY allocator using CreatePseudoConsole.
func NewWindowsConPTYAllocator() *WindowsConPTYAllocator {
	return &WindowsConPTYAllocator{Open: allocateConPTY}
}

// Allocate implements PTYAllocator.
func (a *WindowsConPTYAllocator) Allocate(ctx context.Context, launch ProcessLaunch, cfg SessionConfig) (PTYSession, error) {
	if err := checkAllocateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateProcessLaunch(launch); err != nil {
		return nil, err
	}
	opener := a.Open
	if opener == nil {
		opener = allocateConPTY
	}

	allocation, err := opener()
	if err != nil {
		return nil, wrapPTYAllocationFailure(err)
	}
	return newPlatformSession(launch, normalizeSessionConfig(cfg), PTYKindConPTY, allocation)
}

type conPTYAllocation struct {
	handle  windows.Handle
	inPipe  *os.File
	outPipe *os.File
	ptyIn   *os.File
	ptyOut  *os.File
}

func allocateConPTY() (*conPTYAllocation, error) {
	ptyIn, inPipeOurs, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	outPipeOurs, ptyOut, err := os.Pipe()
	if err != nil {
		_ = ptyIn.Close()
		_ = inPipeOurs.Close()
		return nil, err
	}

	var handle windows.Handle
	coord := windows.Coord{X: 80, Y: 25}
	if err := windows.CreatePseudoConsole(coord, windows.Handle(ptyIn.Fd()), windows.Handle(ptyOut.Fd()), 0, &handle); err != nil {
		_ = ptyIn.Close()
		_ = inPipeOurs.Close()
		_ = outPipeOurs.Close()
		_ = ptyOut.Close()
		return nil, err
	}

	return &conPTYAllocation{
		handle:  handle,
		inPipe:  inPipeOurs,
		outPipe: outPipeOurs,
		ptyIn:   ptyIn,
		ptyOut:  ptyOut,
	}, nil
}

func (c *conPTYAllocation) Close() error {
	var firstErr error
	if c.inPipe != nil {
		if err := c.inPipe.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.inPipe = nil
	}
	if c.outPipe != nil {
		if err := c.outPipe.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.outPipe = nil
	}
	if c.ptyIn != nil {
		if err := c.ptyIn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.ptyIn = nil
	}
	if c.ptyOut != nil {
		if err := c.ptyOut.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		c.ptyOut = nil
	}
	if c.handle != 0 {
		windows.ClosePseudoConsole(c.handle)
		c.handle = 0
	}
	return firstErr
}

// Handle returns the ConPTY pseudo-console handle passed to PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE.
func (c *conPTYAllocation) Handle() windows.Handle {
	if c == nil {
		return 0
	}
	return c.handle
}

// InputPipe returns the host-side pipe used to write stdin to the ConPTY session.
func (c *conPTYAllocation) InputPipe() *os.File {
	if c == nil {
		return nil
	}
	return c.inPipe
}

// OutputPipe returns the host-side pipe used to read stdout/stderr from the ConPTY session.
func (c *conPTYAllocation) OutputPipe() *os.File {
	if c == nil {
		return nil
	}
	return c.outPipe
}
