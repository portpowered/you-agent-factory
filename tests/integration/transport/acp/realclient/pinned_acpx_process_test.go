package realclient_test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	processTreeHelperModeEnv  = "INFINITE_YOU_ACPX_PROCESS_TREE_HELPER_MODE"
	processTreePIDPathEnv     = "INFINITE_YOU_ACPX_PROCESS_TREE_PID_PATH"
	processTreeParentMode     = "parent"
	processTreeParentExitMode = "parent-exit-nonzero"
	processTreeDescendantMode = "descendant"
)

func TestRunBoundedCommandTerminatesScenarioDescendants(t *testing.T) {
	if runProcessTreeHelper(t) {
		return
	}
	started := time.Now()
	defer func() {
		t.Logf("real ACP evidence witness: property=timeout-process-tree-descendant-cleanup duration=%s", time.Since(started))
	}()
	pid := runTimedOutProcessTreeScenario(t)
	waitForProcessExit(t, pid)
}

func TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit(t *testing.T) {
	if runProcessTreeHelper(t) {
		return
	}
	started := time.Now()
	defer func() {
		t.Logf("real ACP evidence witness: property=non-zero-process-tree-descendant-cleanup duration=%s", time.Since(started))
	}()
	pidPath := filepath.Join(t.TempDir(), "descendant-pid")
	_, err := runBoundedCommandWithTimeout(
		t.TempDir(),
		allowlistedEnvironment(map[string]string{
			processTreeHelperModeEnv: processTreeParentExitMode,
			processTreePIDPathEnv:    pidPath,
		}),
		"terminate-process-tree-after-non-zero-exit",
		time.Second,
		os.Args[0],
		"-test.run=^TestRunBoundedCommandTerminatesDescendantsAfterNonZeroExit$",
	)
	if err == nil || !strings.Contains(err.Error(), "non-zero exit") {
		if err == nil {
			t.Fatal("real ACP evidence cleanup failed: non-zero scenario did not report non-zero exit")
		}
		t.Fatalf("real ACP evidence cleanup failed: non-zero scenario returned safe classification %q", err.Error())
	}
	waitForProcessExit(t, readProcessTreeDescendantPID(t, pidPath))
}

func runProcessTreeHelper(t *testing.T) bool {
	switch os.Getenv(processTreeHelperModeEnv) {
	case processTreeParentMode:
		runProcessTreeParent(t, false)
		return true
	case processTreeParentExitMode:
		runProcessTreeParent(t, true)
		return true
	case processTreeDescendantMode:
		fmt.Fprintln(os.Stdout, os.Getpid())
		for {
			time.Sleep(time.Hour)
		}
	}
	return false
}

func runTimedOutProcessTreeScenario(t *testing.T) int {
	t.Helper()
	pidPath := filepath.Join(t.TempDir(), "descendant-pid")
	_, err := runBoundedCommandWithTimeout(
		t.TempDir(),
		allowlistedEnvironment(map[string]string{
			processTreeHelperModeEnv: processTreeParentMode,
			processTreePIDPathEnv:    pidPath,
		}),
		"terminate-process-tree",
		time.Second,
		os.Args[0],
		"-test.run=^TestRunBoundedCommandTerminatesScenarioDescendants$",
	)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		if err == nil {
			t.Fatal("real ACP evidence cleanup failed: timeout scenario did not report timeout")
		}
		t.Fatalf("real ACP evidence cleanup failed: timeout scenario returned safe classification %q", err.Error())
	}
	return readProcessTreeDescendantPID(t, pidPath)
}

func readProcessTreeDescendantPID(t *testing.T, pidPath string) int {
	t.Helper()
	payload, readErr := os.ReadFile(pidPath)
	if readErr != nil {
		t.Fatal("real ACP evidence cleanup failed: scenario did not start descendant")
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
	if parseErr != nil || pid <= 0 {
		t.Fatal("real ACP evidence cleanup failed: scenario reported invalid descendant identity")
	}
	return pid
}

func runProcessTreeParent(t *testing.T, exitNonZero bool) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRunBoundedCommandTerminatesScenarioDescendants$")
	command.Env = allowlistedEnvironment(map[string]string{
		processTreeHelperModeEnv: processTreeDescendantMode,
		processTreePIDPathEnv:    os.Getenv(processTreePIDPathEnv),
	})
	command.Stderr = io.Discard
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal("real ACP evidence cleanup failed: capture descendant identity")
	}
	if err := command.Start(); err != nil {
		t.Fatal("real ACP evidence cleanup failed: start descendant")
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatal("real ACP evidence cleanup failed: read descendant identity")
	}
	if err := os.WriteFile(os.Getenv(processTreePIDPathEnv), []byte(strings.TrimSpace(line)), 0o600); err != nil {
		t.Fatal("real ACP evidence cleanup failed: retain descendant identity")
	}
	if exitNonZero {
		os.Exit(1)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func waitForProcessExit(t *testing.T, pid int) {
	t.Helper()
	// The descendant is re-parented when the OS kills its owner, so the test
	// cannot call Wait on it. Polling its recorded PID is the deterministic OS
	// observation that proves tree cleanup without using an arbitrary sleep.
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		exited, err := processHasExited(pid)
		if err != nil {
			t.Fatal("real ACP evidence cleanup failed: inspect descendant state")
		}
		if exited {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("real ACP evidence cleanup failed: scenario-owned descendant remained active")
		case <-ticker.C:
		}
	}
}
