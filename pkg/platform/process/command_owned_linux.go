//go:build linux

package process

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func (tree *commandProcessTree) ownedControl(done <-chan struct{}, clock Clock, procFS fs.FS) *ownedCommandControl {
	if tree == nil || tree.pgid <= 0 || procFS == nil {
		return nil
	}
	return &ownedCommandControl{done: done, stop: func(ctx context.Context) (bool, error) {
		return tree.forceKillAndJoin(ctx, clock, procFS)
	}}
}

// WNOWAIT observes exit without releasing the leader's PID/group identity.
// Expiry serializes with a claimed force before os/exec is allowed to reap it.
func waitForOwnedCommandExit(cmd *exec.Cmd, control *ownedCommandControl) {
	if control == nil {
		return
	}
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, cmd.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if !errors.Is(err, unix.EINTR) {
			break
		}
	}
	// Errors also revoke the capability before any fallback reap.
	control.expire()
}

func (tree *commandProcessTree) forceKillAndJoin(ctx context.Context, clock Clock, procFS fs.FS) (bool, error) {
	var info unix.Siginfo
	if err := unix.Waitid(unix.P_PID, tree.pgid, &info, unix.WEXITED|unix.WNOWAIT|unix.WNOHANG, nil); err != nil {
		return false, err
	}
	if info.Signo != 0 {
		return false, nil // Natural completion won; the retained zombie is not live.
	}
	if err := unix.Kill(-tree.pgid, unix.SIGKILL); err != nil {
		return false, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		running, err := ownedLinuxGroupRunning(procFS, tree.pgid)
		if err != nil {
			return false, err
		}
		if !running {
			tree.forced.Store(true)
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-clock.After(10 * time.Millisecond):
		}
	}
}

// /proc is the Linux process boundary. A killed zombie cannot execute or fork;
// reaping descendant zombies belongs to their parent/init, not this runner.
// Unreadable or malformed process facts never become a successful tree join.
func ownedLinuxGroupRunning(procFS fs.FS, pgid int) (bool, error) {
	entries, err := fs.ReadDir(procFS, ".")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		stat, err := fs.ReadFile(procFS, entry.Name()+"/stat")
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, unix.ESRCH) {
			// A process may be reaped between enumeration, open and read.
			// procfs reports ESRCH when an already-open stat loses its task.
			continue
		}
		if err != nil {
			return false, err
		}
		running, err := linuxStatGroupRunning(string(stat), pgid)
		if err != nil || running {
			return running, err
		}
	}
	return false, nil
}

func linuxStatGroupRunning(stat string, pgid int) (bool, error) {
	end := strings.LastIndex(stat, ")")
	if end < 0 {
		return false, errors.New("invalid Linux process stat command")
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 3 {
		return false, errors.New("incomplete Linux process stat")
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return false, fmt.Errorf("invalid Linux process group: %w", err)
	}
	return group == pgid && fields[0] != "Z" && fields[0] != "X", nil
}
