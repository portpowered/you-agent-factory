//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package platform_conformance

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
)

func TestPortableControlledRunnerRecoversAfterExecutableWriterCloses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux denies executing an inode held open for writing")
	}
	fixture := newControlledFixture(t, controlledHelperModeSuccess)
	runner := mustControlledRunner(t)
	starts := 0
	runner.starter = func(command *exec.Cmd) error {
		starts++
		if starts != 1 {
			return command.Start()
		}
		writer, err := os.OpenFile(command.Path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		busy := command.Start()
		closeErr := writer.Close()
		if !errors.Is(busy, syscall.ETXTBSY) || closeErr != nil {
			t.Fatalf("write-held executable start=%v close=%v, want ETXTBSY", busy, closeErr)
		}
		return busy
	}
	testControlledSuccess(t, runner, fixture)
	if starts != 2 {
		t.Fatalf("recovered run starts=%d, want busy attempt then successful start", starts)
	}
}

// controlledProcessTree is one process group created exclusively for a
// controlled attempt. Negative-pgid signals cannot reach an unrelated peer
// process because the group is made before the child starts.
type controlledProcessTree struct {
	pgid int
}

func configureControlledProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachControlledProcessTree(command *exec.Cmd) (controlledProcessTree, error) {
	if command == nil || command.Process == nil || command.Process.Pid <= 0 {
		return controlledProcessTree{}, errors.New("controlled process has no attachable pid")
	}
	return controlledProcessTree{pgid: command.Process.Pid}, nil
}

func terminateControlledProcessTree(command *exec.Cmd, tree controlledProcessTree) error {
	return signalControlledProcessGroup(command, tree, syscall.SIGKILL)
}

func requestControlledProcessTreeStop(command *exec.Cmd, tree controlledProcessTree) error {
	return signalControlledProcessGroup(command, tree, syscall.SIGINT)
}

func signalControlledProcessGroup(command *exec.Cmd, tree controlledProcessTree, signal syscall.Signal) error {
	pgid := tree.pgid
	if pgid <= 0 && command != nil && command.Process != nil {
		pgid = command.Process.Pid
	}
	if pgid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pgid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func closeControlledProcessTree(controlledProcessTree, bool) {}

func controlledProcessTreeAlive(tree controlledProcessTree) bool {
	err := probeControlledProcessGroup(tree)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// probeControlledProcessGroup sends only the read-only zero signal to the
// attached PGID. It deliberately has no fallback to the direct PID, so
// release evidence cannot widen ownership to an unrelated process.
func probeControlledProcessGroup(tree controlledProcessTree) error {
	if tree.pgid <= 0 {
		return syscall.ESRCH
	}
	return syscall.Kill(-tree.pgid, 0)
}
