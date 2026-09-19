package managed_process_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
)

const (
	helperPathEnvironment  = "YOU_MODELS_MANAGED_PROCESS_HELPER"
	helperSHAEnvironment   = "YOU_MODELS_MANAGED_PROCESS_HELPER_SHA256"
	managedProcessWaitTime = 15 * time.Second
	managedProcessPollTime = 10 * time.Millisecond

	naturalNonzeroOutputBytes = 70 << 10
	forcedStopStdout          = "managed-process forced-stop stdout\n"
	forcedStopStderr          = "managed-process forced-stop stderr\n"
)

// TestManagedProcessNaturalNonzeroUsesTheProductionBoundary is I01: the
// already-built helper exits nonzero after producing more than the retained
// tail limit on both streams. The test starts it only through managedchild.
func TestManagedProcessNaturalNonzeroUsesTheProductionBoundary(t *testing.T) {
	helper := requireManagedProcessHelper(t)
	process, err := managedchild.Start(context.Background(), managedchild.Spec{
		Command:         helper,
		Args:            []string{"--case", "natural-nonzero"},
		OutputTailLimit: managedchild.DefaultOutputTailLimit,
	})
	if err != nil {
		t.Fatalf("managedchild.Start(natural-nonzero) = %v", err)
	}
	registerManagedProcessCleanup(t, process)

	const waiters = 4
	waitResults := waitForManagedProcess(t, process, waiters)
	firstError := waitResults[0]
	if firstError == nil {
		t.Fatal("natural-nonzero Wait() = nil, want an exit error")
	}
	for index, waitErr := range waitResults {
		if !sameManagedProcessError(waitErr, firstError) {
			t.Fatalf("Wait() observer %d error = %v, want the exact shared error %v", index, waitErr, firstError)
		}
	}
	if exitCode, ok := managedProcessExitCode(firstError); !ok || exitCode != 7 {
		t.Fatalf("natural-nonzero exit error = %v, want an ExitError with code 7", firstError)
	}

	snapshot := requireManagedProcessSnapshot(t, process)
	assertManagedProcessStream(t, "stdout", snapshot.Stdout, bytes.Repeat([]byte{'o'}, naturalNonzeroOutputBytes))
	assertManagedProcessStream(t, "stderr", snapshot.Stderr, bytes.Repeat([]byte{'e'}, naturalNonzeroOutputBytes))
	if snapshot.ExitClass != managedchild.ExitClassNonzero || !snapshot.ExitCodeKnown || snapshot.ExitCode != 7 {
		t.Fatalf("natural-nonzero snapshot = %#v, want NONZERO_EXIT/7", snapshot)
	}
	waitForManagedProcessExit(t, process.PID())

	t.Logf(
		"MANAGED-PROCESS-EVIDENCE case=I01 helper=%s helperSHA256=%s pid=%d exitClass=%s exitCode=%d stdoutBytes=%d stdoutSHA256=%s stdoutTruncated=%t stderrBytes=%d stderrSHA256=%s stderrTruncated=%t survivors=0",
		helper, os.Getenv(helperSHAEnvironment), process.PID(), snapshot.ExitClass, snapshot.ExitCode,
		snapshot.Stdout.Bytes, snapshot.Stdout.SHA256, snapshot.Stdout.Truncated,
		snapshot.Stderr.Bytes, snapshot.Stderr.SHA256, snapshot.Stderr.Truncated,
	)
}

// TestManagedProcessForcedStopCleansTheObservedTree is I02: the already-built
// helper creates one descendant that inherits both output pipes, then blocks.
// Cancellation and repeated Stop calls race with multiple Wait observers.
func TestManagedProcessForcedStopCleansTheObservedTree(t *testing.T) {
	helper := requireManagedProcessHelper(t)
	root := t.TempDir()
	pidPath := filepath.Join(root, "descendant.pid")
	readyPath := filepath.Join(root, "descendant.ready")
	ctx, cancel := context.WithCancel(context.Background())
	process, err := managedchild.Start(ctx, managedchild.Spec{
		Command: helper,
		Args: []string{
			"--case", "forced-stop",
			"--pid-file", pidPath,
			"--ready-file", readyPath,
		},
		OutputTailLimit: managedchild.DefaultOutputTailLimit,
	})
	if err != nil {
		cancel()
		t.Fatalf("managedchild.Start(forced-stop) = %v", err)
	}
	registerManagedProcessCleanup(t, process)
	t.Cleanup(cancel)

	descendantPID := waitForManagedProcessPID(t, pidPath)
	if !helperProcessRunning(descendantPID) {
		t.Fatalf("forced-stop descendant PID %d was not running at readiness", descendantPID)
	}

	const waiters = 8
	waitResults := make(chan error, waiters)
	for index := 0; index < waiters; index++ {
		go func() { waitResults <- process.Wait() }()
	}
	stopResults := make(chan error, waiters)
	cancel()
	for index := 0; index < waiters; index++ {
		go func() { stopResults <- process.Stop(context.Background()) }()
	}
	for index := 0; index < waiters; index++ {
		if stopErr := receiveManagedProcessResult(t, stopResults); stopErr != nil {
			t.Fatalf("concurrent Stop() observer %d = %v, want nil", index, stopErr)
		}
	}

	waitErrors := make([]error, 0, waiters)
	for index := 0; index < waiters; index++ {
		waitErrors = append(waitErrors, receiveManagedProcessResult(t, waitResults))
	}
	firstError := waitErrors[0]
	if firstError == nil {
		t.Fatal("forced-stop Wait() = nil, want a terminal process error")
	}
	for index, waitErr := range waitErrors {
		if !sameManagedProcessError(waitErr, firstError) {
			t.Fatalf("forced-stop Wait() observer %d error = %v, want the exact shared error %v", index, waitErr, firstError)
		}
	}

	snapshot := requireManagedProcessSnapshot(t, process)
	assertManagedProcessStream(t, "stdout", snapshot.Stdout, []byte(forcedStopStdout))
	assertManagedProcessStream(t, "stderr", snapshot.Stderr, []byte(forcedStopStderr))
	if snapshot.ExitClass != managedchild.ExitClassNonzero {
		t.Fatalf("forced-stop snapshot exit class = %s, want NONZERO_EXIT", snapshot.ExitClass)
	}
	if exitCode, ok := managedProcessExitCode(firstError); ok && snapshot.ExitCodeKnown && snapshot.ExitCode != exitCode {
		t.Fatalf("forced-stop snapshot exit code = %d, Wait() exit code = %d", snapshot.ExitCode, exitCode)
	}
	waitForManagedProcessExit(t, process.PID())
	waitForManagedProcessExit(t, descendantPID)
	if repeatedErr := process.Stop(context.Background()); repeatedErr != nil {
		t.Fatalf("repeated Stop() = %v, want nil", repeatedErr)
	}

	t.Logf(
		"MANAGED-PROCESS-EVIDENCE case=I02 helper=%s helperSHA256=%s pid=%d descendantPID=%d exitClass=%s exitCode=%d exitCodeKnown=%t stdoutBytes=%d stdoutSHA256=%s stderrBytes=%d stderrSHA256=%s survivors=0",
		helper, os.Getenv(helperSHAEnvironment), process.PID(), descendantPID, snapshot.ExitClass, snapshot.ExitCode, snapshot.ExitCodeKnown,
		snapshot.Stdout.Bytes, snapshot.Stdout.SHA256, snapshot.Stderr.Bytes, snapshot.Stderr.SHA256,
	)
}

func requireManagedProcessHelper(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(helperPathEnvironment))
	expectedSHA256 := strings.TrimSpace(os.Getenv(helperSHAEnvironment))
	if path == "" || expectedSHA256 == "" {
		t.Fatalf("managed-process integration requires %s and %s from the Make-owned helper target", helperPathEnvironment, helperSHAEnvironment)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("managed-process helper path %q is not absolute", path)
	}
	if expectedSHA256 != strings.ToLower(expectedSHA256) || len(expectedSHA256) != sha256.Size*2 {
		t.Fatalf("managed-process helper SHA-256 %q is not lowercase hexadecimal", expectedSHA256)
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		t.Fatalf("managed-process helper SHA-256 %q is invalid: %v", expectedSHA256, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat managed-process helper %q: %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		t.Fatalf("managed-process helper %q is not a nonempty regular file", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read managed-process helper %q: %v", path, err)
	}
	actual := sha256.Sum256(body)
	actualSHA256 := hex.EncodeToString(actual[:])
	if actualSHA256 != expectedSHA256 {
		t.Fatalf("managed-process helper SHA-256 = %s, want Make-owned digest %s", actualSHA256, expectedSHA256)
	}
	t.Logf("MANAGED-PROCESS-HELPER path=%s bytes=%d sha256=%s", path, info.Size(), actualSHA256)
	return path
}

func registerManagedProcessCleanup(t *testing.T, process *managedchild.Process) {
	t.Helper()
	t.Cleanup(func() {
		cleanupContext, cancel := context.WithTimeout(context.Background(), managedProcessWaitTime)
		defer cancel()
		_ = process.Stop(cleanupContext)
	})
}

func waitForManagedProcess(t *testing.T, process *managedchild.Process, count int) []error {
	t.Helper()
	results := make(chan error, count)
	for index := 0; index < count; index++ {
		go func() { results <- process.Wait() }()
	}
	waitErrors := make([]error, 0, count)
	for index := 0; index < count; index++ {
		waitErrors = append(waitErrors, receiveManagedProcessResult(t, results))
	}
	return waitErrors
}

func receiveManagedProcessResult(t *testing.T, results <-chan error) error {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(managedProcessWaitTime):
		t.Fatal("managed-process observer did not reach terminal state before the safety deadline")
		return nil
	}
}

func waitForManagedProcessPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.NewTimer(managedProcessWaitTime)
	defer deadline.Stop()
	// The helper's PID file is the only cross-process readiness signal exposed
	// by this intentionally small artifact. The ticker is a safety ceiling and
	// returns immediately after the observable file is atomically published.
	ticker := time.NewTicker(managedProcessPollTime)
	defer ticker.Stop()
	var lastErr error
	for {
		body, err := os.ReadFile(path)
		if err == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(body)), "%d", &pid); scanErr == nil && pid > 0 {
				return pid
			}
			lastErr = fmt.Errorf("invalid PID contents %q", body)
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for managed-process helper PID file %q: %v", path, lastErr)
			return 0
		}
	}
}

func waitForManagedProcessExit(t *testing.T, pid int) {
	t.Helper()
	if pid <= 0 {
		t.Fatalf("managed-process PID %d is not positive", pid)
	}
	deadline := time.NewTimer(managedProcessWaitTime)
	defer deadline.Stop()
	// Process termination is asynchronous even after the managedchild terminal
	// snapshot is published. Observe the OS process state until it disappears;
	// the deadline is a hang guard, not a performance assertion.
	ticker := time.NewTicker(managedProcessPollTime)
	defer ticker.Stop()
	for helperProcessRunning(pid) {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("managed-process PID %d remained alive after terminalization", pid)
		}
	}
}

func requireManagedProcessSnapshot(t *testing.T, process *managedchild.Process) managedchild.Snapshot {
	t.Helper()
	snapshot, ok := process.Snapshot()
	if !ok {
		t.Fatal("managedchild.Snapshot() = unavailable after all Wait observers completed")
	}
	return snapshot
}

func assertManagedProcessStream(t *testing.T, name string, got managedchild.StreamSnapshot, want []byte) {
	t.Helper()
	digest := sha256.Sum256(want)
	wantSHA256 := hex.EncodeToString(digest[:])
	if got.Bytes != uint64(len(want)) || got.SHA256 != wantSHA256 {
		t.Fatalf("%s snapshot = bytes=%d sha256=%s, want bytes=%d sha256=%s", name, got.Bytes, got.SHA256, len(want), wantSHA256)
	}
	if !got.Truncated {
		if len(want) > managedchild.DefaultOutputTailLimit {
			t.Fatalf("%s snapshot is not marked truncated for %d bytes", name, len(want))
		}
	} else if len(want) <= managedchild.DefaultOutputTailLimit {
		t.Fatalf("%s snapshot is marked truncated for %d bytes", name, len(want))
	}
	if len(got.Tail) > managedchild.DefaultOutputTailLimit {
		t.Fatalf("%s retained tail length = %d, want at most %d", name, len(got.Tail), managedchild.DefaultOutputTailLimit)
	}
	wantTail := want
	if len(wantTail) > managedchild.DefaultOutputTailLimit {
		wantTail = wantTail[len(wantTail)-managedchild.DefaultOutputTailLimit:]
	}
	if !bytes.Equal(got.Tail, wantTail) {
		t.Fatalf("%s retained tail does not match the final bounded bytes", name)
	}
}

type managedProcessExitCoder interface {
	ExitCode() int
}

func managedProcessExitCode(err error) (int, bool) {
	var coded managedProcessExitCoder
	if !errors.As(err, &coded) {
		return 0, false
	}
	return coded.ExitCode(), true
}

func sameManagedProcessError(left, right error) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftValue, rightValue := reflect.ValueOf(left), reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() || leftValue.Kind() != reflect.Pointer {
		return false
	}
	return leftValue.Pointer() == rightValue.Pointer()
}
