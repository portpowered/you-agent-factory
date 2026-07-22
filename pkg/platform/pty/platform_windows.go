//go:build windows

package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	"golang.org/x/sys/windows"
)

// conPTYOpener allocates a Windows ConPTY pseudo-console pair.
// Tests inject a mock opener to exercise allocation without a live child process.
type conPTYOpener func() (*conPTYAllocation, error)

// WindowsHost owns only native ConPTY handles and subprocess mechanics.
type WindowsHost struct {
	Open conPTYOpener
}

// NewHost returns the policy-free PTY host compiled for this platform.
func NewHost() Host { return &WindowsHost{Open: allocateConPTY} }

// Allocate opens an opaque ConPTY allocation.
func (a *WindowsHost) Allocate(ctx context.Context) (Allocation, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if a == nil {
		return nil, errors.New("pty: Windows host is required")
	}
	opener := a.Open
	if opener == nil {
		return nil, errors.New("pty: ConPTY opener is required")
	}
	return opener()
}

type conPTYAllocation struct {
	handle  windows.Handle
	inPipe  *os.File
	outPipe *os.File
	ptyIn   *os.File
	ptyOut  *os.File
}

func (*conPTYAllocation) Kind() Kind { return KindConPTY }

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

// Start attaches the supplied launch description to the allocated ConPTY.
func (h *WindowsHost) Start(launch ProcessLaunch, native Allocation) (Process, io.ReadCloser, error) {
	alloc, ok := native.(*conPTYAllocation)
	if !ok || alloc == nil {
		return nil, nil, errors.New("pty: ConPTY allocation is required")
	}
	reader := alloc.OutputPipe()
	if reader == nil {
		return nil, nil, errors.New("pty: ConPTY output pipe is required")
	}
	if alloc.Handle() == 0 {
		return nil, nil, errors.New("pty: ConPTY handle is required")
	}

	cmd, winHandle, err := startConPTYProcess(launch, alloc.Handle())
	if err != nil {
		return nil, nil, err
	}
	closeConPTYSpawnHandles(alloc)

	tree, err := process.AttachSubprocessTree(cmd)
	if err != nil {
		_ = process.TerminateSubprocessTree(cmd, tree)
		_ = cmd.Wait()
		if winHandle != 0 {
			_ = windows.CloseHandle(windows.Handle(winHandle))
		}
		return nil, nil, err
	}

	return &windowsProcess{cmd: cmd, tree: tree, winHandle: winHandle}, reader, nil
}

func startConPTYProcess(launch ProcessLaunch, conptyHandle windows.Handle) (*exec.Cmd, uintptr, error) {
	if len(launch.Argv) == 0 {
		return nil, 0, errors.New("pty: argv is required")
	}

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, 0, err
	}
	defer attrList.Delete()

	// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE expects the HPCON value itself in
	// lpValue, not the address of the handle. Reinterpret the handle-sized value
	// without a uintptr-to-pointer conversion so go vet can distinguish this
	// Win32 handle ABI from Go pointer arithmetic.
	conptyAttribute := *(*unsafe.Pointer)(unsafe.Pointer(&conptyHandle))
	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		conptyAttribute,
		unsafe.Sizeof(conptyHandle),
	); err != nil {
		return nil, 0, err
	}

	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		},
		ProcThreadAttributeList: attrList.List(),
	}

	cmdLine := windows.ComposeCommandLine(launch.Argv)
	cmdLineUTF16, err := windows.UTF16FromString(cmdLine)
	if err != nil {
		return nil, 0, err
	}
	cmdLineMutable := append([]uint16(nil), cmdLineUTF16...)

	var cwd *uint16
	if strings.TrimSpace(launch.WorkDir) != "" {
		cwd, err = windows.UTF16PtrFromString(launch.WorkDir)
		if err != nil {
			return nil, 0, err
		}
	}

	envBlock, err := utf16EnvBlock(launch.Env)
	if err != nil {
		return nil, 0, err
	}

	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_NO_WINDOW)
	if envBlock != nil {
		flags |= windows.CREATE_UNICODE_ENVIRONMENT
	}
	pi := new(windows.ProcessInformation)
	err = windows.CreateProcess(
		nil,
		&cmdLineMutable[0],
		nil,
		nil,
		false,
		flags,
		envBlock,
		cwd,
		&si.StartupInfo,
		pi,
	)
	if err != nil {
		return nil, 0, err
	}
	_ = windows.CloseHandle(pi.Thread)

	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		_ = windows.CloseHandle(pi.Process)
		return nil, 0, err
	}

	return &exec.Cmd{Process: proc}, uintptr(pi.Process), nil
}

func utf16EnvBlock(env []string) (*uint16, error) {
	if len(env) == 0 {
		return nil, nil
	}
	var utf16Block []uint16
	for _, entry := range env {
		entryUTF16, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		utf16Block = append(utf16Block, entryUTF16...)
	}
	utf16Block = append(utf16Block, 0)
	return &utf16Block[0], nil
}

type windowsProcess struct {
	cmd       *exec.Cmd
	tree      process.SubprocessTree
	winHandle uintptr
	exitCode  int
}

func (p *windowsProcess) Wait() error {
	if p == nil {
		return errors.New("pty: process is not started")
	}
	if p.winHandle != 0 {
		event, err := windows.WaitForSingleObject(windows.Handle(p.winHandle), windows.INFINITE)
		if err != nil {
			return err
		}
		if event != windows.WAIT_OBJECT_0 {
			return errors.New("pty: unexpected wait result")
		}
		var exitCode uint32
		if err := windows.GetExitCodeProcess(windows.Handle(p.winHandle), &exitCode); err != nil {
			return err
		}
		p.exitCode = int(exitCode)
		return nil
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("pty: process is not started")
	}
	err := p.cmd.Wait()
	if err == nil {
		return nil
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		p.exitCode = exitErr.ExitCode()
		return nil
	}
	return err
}

func (p *windowsProcess) Terminate() error {
	if p == nil || p.cmd == nil {
		return nil
	}
	return process.TerminateSubprocessTree(p.cmd, p.tree)
}

func (p *windowsProcess) Close() {
	if p == nil {
		return
	}
	if p.cmd != nil {
		process.CloseSubprocessTree(p.cmd, p.tree)
	}
	if p.winHandle != 0 {
		_ = windows.CloseHandle(windows.Handle(p.winHandle))
		p.winHandle = 0
	}
}

func (p *windowsProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *windowsProcess) ExitCode() int {
	if p == nil {
		return -1
	}
	return p.exitCode
}

func closeConPTYSpawnHandles(alloc *conPTYAllocation) {
	if alloc == nil {
		return
	}
	if alloc.ptyIn != nil {
		_ = alloc.ptyIn.Close()
		alloc.ptyIn = nil
	}
	if alloc.ptyOut != nil {
		_ = alloc.ptyOut.Close()
		alloc.ptyOut = nil
	}
}
