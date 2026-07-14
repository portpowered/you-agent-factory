//go:build windows

package agypty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"github.com/portpowered/infinite-you/pkg/workers/process"
	"golang.org/x/sys/windows"
)

func runPlatformSession(ctx context.Context, session *platformSession) (SessionResult, error) {
	if session == nil {
		return SessionResult{}, errors.New("agypty: session is required")
	}
	defer closeSessionPTY(session)

	conpty, ok := session.pty.(*conPTYAllocation)
	if !ok || conpty == nil {
		return SessionResult{}, errors.New("agypty: ConPTY allocation is required")
	}

	proc, reader, err := startConPTYSessionProcess(session.launch, conpty)
	if err != nil {
		return SessionResult{}, err
	}

	result, runErr := executeSessionRun(ctx, session.cfg, reader, proc)
	closeConPTYHostPipes(conpty)
	if proc.winHandle != 0 {
		_ = windows.CloseHandle(windows.Handle(proc.winHandle))
		proc.winHandle = 0
	}
	return result, runErr
}

func startConPTYSessionProcess(launch ProcessLaunch, alloc *conPTYAllocation) (*sessionProcess, io.ReadCloser, error) {
	reader := alloc.OutputPipe()
	if reader == nil {
		return nil, nil, errors.New("agypty: ConPTY output pipe is required")
	}
	if alloc.Handle() == 0 {
		return nil, nil, errors.New("agypty: ConPTY handle is required")
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

	return &sessionProcess{cmd: cmd, tree: tree, winHandle: winHandle}, reader, nil
}

func startConPTYProcess(launch ProcessLaunch, conptyHandle windows.Handle) (*exec.Cmd, uintptr, error) {
	if len(launch.Argv) == 0 {
		return nil, 0, errors.New("agypty: argv is required")
	}

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, 0, err
	}
	defer attrList.Delete()

	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(conptyHandle),
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

func sessionProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	event, err := windows.WaitForSingleObject(process, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}

func terminateSessionTestProcess(pid int) {
	if pid <= 0 {
		return
	}
	process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	_ = windows.TerminateProcess(process, 1)
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

func closeConPTYHostPipes(alloc *conPTYAllocation) {
	if alloc == nil {
		return
	}
	if input := alloc.InputPipe(); input != nil {
		_ = input.Close()
	}
	if output := alloc.OutputPipe(); output != nil {
		_ = output.Close()
	}
}
