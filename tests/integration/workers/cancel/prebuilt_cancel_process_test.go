package cancel_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fixtureProcessTreeWatcher struct {
	t       *testing.T
	watcher *fsnotify.Watcher
	workDir string
	watched map[string]struct{}
}

func (watcher *fixtureProcessTreeWatcher) add(path string) {
	watcher.t.Helper()
	path = filepath.Clean(path)
	if _, exists := watcher.watched[path]; exists {
		return
	}
	if err := watcher.watcher.Add(path); err != nil {
		watcher.t.Fatalf("watch process fixture directory %q: %v", path, err)
	}
	watcher.watched[path] = struct{}{}
}

func (watcher *fixtureProcessTreeWatcher) addExistingAttempts() {
	watcher.t.Helper()
	entries, err := os.ReadDir(watcher.workDir)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		watcher.t.Fatalf("read process fixture Work directory %q: %v", watcher.workDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "run-") {
			watcher.add(filepath.Join(watcher.workDir, entry.Name()))
		}
	}
}

func (watcher *fixtureProcessTreeWatcher) observe(event fsnotify.Event) {
	watcher.t.Helper()
	if filepath.Clean(event.Name) == watcher.workDir && event.Op&fsnotify.Create != 0 {
		watcher.add(watcher.workDir)
		watcher.addExistingAttempts()
	}
	if strings.HasPrefix(filepath.Base(event.Name), "run-") && event.Op&fsnotify.Create != 0 && directoryExists(event.Name) {
		watcher.add(event.Name)
	}
}

func waitForFixtureProcessTree(
	t *testing.T,
	ctx context.Context,
	stateDir, workID string,
	excludedRootPIDs ...int,
) workerProcessTree {
	t.Helper()
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("watch process fixture readiness: %v", err)
	}
	defer fsWatcher.Close()
	watcher := fixtureProcessTreeWatcher{
		t: t, watcher: fsWatcher, workDir: filepath.Clean(filepath.Join(stateDir, safePathSegment(workID))),
		watched: make(map[string]struct{}),
	}
	watcher.add(stateDir)
	if directoryExists(watcher.workDir) {
		watcher.add(watcher.workDir)
		watcher.addExistingAttempts()
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	var lastReadiness string
	for {
		tree, diagnostic := readyFixtureProcessTree(watcher.workDir, workID, excludedRootPIDs)
		if tree.RootPID > 0 {
			return tree
		}
		lastReadiness = diagnostic
		select {
		case <-ctx.Done():
			t.Fatalf("wait for real worker process tree for Work %q excluding roots %v: %v (%s)", workID, excludedRootPIDs, ctx.Err(), lastReadiness)
		case <-deadline.C:
			t.Fatalf("timed out waiting for real worker process tree for Work %q excluding roots %v (%s)", workID, excludedRootPIDs, lastReadiness)
		case event, ok := <-fsWatcher.Events:
			if !ok {
				t.Fatalf("process fixture readiness watcher closed for Work %q", workID)
			}
			watcher.observe(event)
		case watcherErr, ok := <-fsWatcher.Errors:
			if !ok {
				t.Fatalf("process fixture readiness watcher errors closed for Work %q", workID)
			}
			t.Fatalf("watch process fixture readiness for Work %q: %v", workID, watcherErr)
		}
	}
}

func readyFixtureProcessTree(workDir, workID string, excludedRootPIDs []int) (workerProcessTree, string) {
	entries, err := os.ReadDir(workDir)
	if errors.Is(err, fs.ErrNotExist) {
		return workerProcessTree{}, "Work process directory is not ready"
	}
	if err != nil {
		return workerProcessTree{}, fmt.Sprintf("read process fixture Work directory: %v", err)
	}
	lastReadiness := "no ready process attempt"
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "run-") {
			continue
		}
		attemptDir := filepath.Join(workDir, entry.Name())
		rootPID, rootErr := readPositivePID(filepath.Join(attemptDir, "root.pid"))
		childPID, childErr := readPositivePID(filepath.Join(attemptDir, "child.pid"))
		ready, readyErr := os.ReadFile(filepath.Join(attemptDir, "ready"))
		childStarted, childStartedErr := os.ReadFile(filepath.Join(attemptDir, "child.started"))
		lateOutputReady, lateOutputErr := os.ReadFile(filepath.Join(attemptDir, "late-output.ready"))
		if rootErr != nil || childErr != nil || readyErr != nil || childStartedErr != nil || lateOutputErr != nil ||
			strings.TrimSpace(string(ready)) != "ready" || strings.TrimSpace(string(childStarted)) != "started" ||
			strings.TrimSpace(string(lateOutputReady)) != "ready" || containsPID(excludedRootPIDs, rootPID) {
			lastReadiness = fmt.Sprintf("attempt=%s root=%v child=%v ready=%v child_started=%v late_output=%v", entry.Name(), rootErr, childErr, readyErr, childStartedErr, lateOutputErr)
			continue
		}
		pids, err := processTreePIDs(rootPID)
		if err == nil && containsPID(pids, childPID) {
			return workerProcessTree{WorkID: workID, RootPID: rootPID, ChildPID: childPID, PIDs: pids}, ""
		}
		if err != nil {
			lastReadiness = fmt.Sprintf("attempt=%s process snapshot: %v", entry.Name(), err)
		}
	}
	return workerProcessTree{}, lastReadiness
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func assertObservedTreeAncestry(t *testing.T, tree workerProcessTree) {
	t.Helper()
	parent, err := processParentPID(tree.ChildPID)
	if err != nil {
		t.Fatalf("read process ancestry for Work %q child %d: %v", tree.WorkID, tree.ChildPID, err)
	}
	if parent != tree.RootPID {
		t.Fatalf("Work %q process ancestry child %d parent=%d, want worker root %d", tree.WorkID, tree.ChildPID, parent, tree.RootPID)
	}
}

func sampleCancelProcessTrees(t *testing.T, ctx context.Context, target, unrelated workerProcessTree, appliedAt time.Time) []cancelProcessSample {
	t.Helper()
	samples := make([]cancelProcessSample, 0, int(cancelProcessBound/cancelProcessSampleInterval)+1)
	for index := 0; index <= int(cancelProcessBound/cancelProcessSampleInterval); index++ {
		if index > 0 {
			deadline := appliedAt.Add(time.Duration(index) * cancelProcessSampleInterval)
			timer := time.NewTimer(time.Until(deadline))
			select {
			case <-ctx.Done():
				t.Fatalf("process sample %d canceled: %v", index, ctx.Err())
			case <-timer.C:
			}
		}
		sample, err := captureProcessSample(target, unrelated, appliedAt)
		if err != nil {
			t.Fatalf("capture process sample %d: %v", index, err)
		}
		samples = append(samples, sample)
	}
	return samples
}

func stopCancelDaemon(t *testing.T, binaryPath string, fixture cancelFixture, daemon *cancelDaemon) {
	t.Helper()
	result := runCancelServerStopCLI(context.Background(), binaryPath, fixture)
	if result.err != nil {
		t.Fatalf("public Factory server stop: %v; stdout=%s stderr=%s", result.err, result.stdout, result.stderr)
	}
	select {
	case <-daemon.done:
		if err := daemon.waitError(); err != nil {
			t.Fatalf("prebuilt Factory daemon exit after public stop: %v; stdout=%s stderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("prebuilt Factory daemon did not exit after public stop; stdout=%s stderr=%s", daemon.stdout.String(), daemon.stderr.String())
	}
	daemon.mu.Lock()
	daemon.stopped = true
	daemon.mu.Unlock()
}

func runCancelServerStopCLI(ctx context.Context, binaryPath string, fixture cancelFixture) cancelCommandResult {
	command := exec.CommandContext(ctx, binaryPath, "--server", fixture.serverURL, "server", "stop")
	command.Dir = fixture.factoryDir
	command.Env = append([]string(nil), fixture.environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return cancelCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func cancelPortAvailabilityError(port int) error {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("Factory listener port %d remains bound after public stop: %w", port, err)
	}
	if err := listener.Close(); err != nil {
		return fmt.Errorf("close listener availability probe on port %d: %w", port, err)
	}
	return nil
}

func cleanupCancelDaemon(daemon *cancelDaemon) {
	if daemon == nil {
		return
	}
	daemon.mu.Lock()
	if daemon.stopped || daemon.cmd == nil || daemon.cmd.Process == nil {
		daemon.mu.Unlock()
		return
	}
	daemon.stopped = true
	daemon.mu.Unlock()
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(daemon.cmd.Process.Pid), "/T", "/F").Run()
	} else {
		_ = daemon.cmd.Process.Kill()
	}
	select {
	case <-daemon.done:
	case <-time.After(10 * time.Second):
	}
}

func (daemon *cancelDaemon) waitError() error {
	if daemon == nil {
		return nil
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	return daemon.waitErr
}

func captureProcessSample(target, unrelated workerProcessTree, appliedAt time.Time) (cancelProcessSample, error) {
	targetDescendants, err := processTreePIDs(target.RootPID)
	if err != nil {
		return cancelProcessSample{}, fmt.Errorf("inspect target process tree: %w", err)
	}
	unrelatedDescendants, err := processTreePIDs(unrelated.RootPID)
	if err != nil {
		return cancelProcessSample{}, fmt.Errorf("inspect unrelated process tree: %w", err)
	}
	targetAlive, err := processPIDsPresent(target.PIDs)
	if err != nil {
		return cancelProcessSample{}, fmt.Errorf("inspect target process identities: %w", err)
	}
	unrelatedAlive, err := processPIDsPresent(unrelated.PIDs)
	if err != nil {
		return cancelProcessSample{}, fmt.Errorf("inspect unrelated process identities: %w", err)
	}
	return cancelProcessSample{
		AfterAPIMillis:       time.Since(appliedAt).Milliseconds(),
		TargetPIDs:           append([]int(nil), target.PIDs...),
		TargetPresent:        targetAlive,
		TargetDescendants:    targetDescendants,
		UnrelatedPIDs:        append([]int(nil), unrelated.PIDs...),
		UnrelatedPresent:     unrelatedAlive,
		UnrelatedDescendants: unrelatedDescendants,
	}, nil
}

func processPIDsPresent(pids []int) ([]int, error) {
	present := make([]int, 0, len(pids))
	for _, pid := range pids {
		alive, err := processPIDPresent(pid)
		if err != nil {
			return nil, err
		}
		if alive {
			present = append(present, pid)
		}
	}
	sort.Ints(present)
	return present, nil
}

func allTargetSamplesAbsent(samples []cancelProcessSample) bool {
	if len(samples) != int(cancelProcessBound/cancelProcessSampleInterval)+1 {
		return false
	}
	for _, sample := range samples {
		if len(sample.TargetPresent) != 0 || len(sample.TargetDescendants) != 0 {
			return false
		}
	}
	return true
}

func firstTargetAbsenceMillis(samples []cancelProcessSample) int64 {
	for _, sample := range samples {
		if len(sample.TargetPresent) == 0 && len(sample.TargetDescendants) == 0 {
			return sample.AfterAPIMillis
		}
	}
	return -1
}

func allUnrelatedSamplesUnchanged(samples []cancelProcessSample, want []int) bool {
	if len(samples) != int(cancelProcessBound/cancelProcessSampleInterval)+1 {
		return false
	}
	want = append([]int(nil), want...)
	sort.Ints(want)
	for _, sample := range samples {
		alive := append([]int(nil), sample.UnrelatedPresent...)
		descendants := append([]int(nil), sample.UnrelatedDescendants...)
		sort.Ints(alive)
		sort.Ints(descendants)
		if !reflect.DeepEqual(alive, want) || !reflect.DeepEqual(descendants, want) {
			return false
		}
	}
	return true
}

func validCancelSampleSchedule(samples []cancelProcessSample) bool {
	if len(samples) != int(cancelProcessBound/cancelProcessSampleInterval)+1 {
		return false
	}
	tolerance := int64(500)
	for index, sample := range samples {
		expected := int64(index) * cancelProcessSampleInterval.Milliseconds()
		if sample.AfterAPIMillis < expected || sample.AfterAPIMillis-expected > tolerance {
			return false
		}
		if index > 0 {
			interval := sample.AfterAPIMillis - samples[index-1].AfterAPIMillis
			if interval < 500 || interval > 1500 {
				return false
			}
		}
	}
	return true
}

func collectCancelCleanupCensus(targets, unrelated []workerProcessTree, daemonPID, port int) (cancelCleanupCensus, error) {
	var census cancelCleanupCensus
	var err error
	census.TargetPIDsRemaining, err = currentCancelTreesPIDs(targets)
	if err != nil {
		return census, fmt.Errorf("inspect target tree after cleanup: %w", err)
	}
	census.UnrelatedPIDsRemaining, err = currentCancelTreesPIDs(unrelated)
	if err != nil {
		return census, fmt.Errorf("inspect unrelated tree after cleanup: %w", err)
	}
	census.DaemonPIDRemaining, err = processPIDPresent(daemonPID)
	if err != nil {
		return census, fmt.Errorf("inspect daemon process after cleanup: %w", err)
	}
	err = cancelPortAvailabilityError(port)
	census.ListenerAvailable = err == nil
	if err != nil {
		return census, err
	}
	return census, nil
}

func currentCancelTreesPIDs(trees []workerProcessTree) ([]int, error) {
	set := make(map[int]struct{})
	for _, tree := range trees {
		pids, err := currentCancelTreePIDs(tree)
		if err != nil {
			return nil, err
		}
		for _, pid := range pids {
			set[pid] = struct{}{}
		}
	}
	result := make([]int, 0, len(set))
	for pid := range set {
		result = append(result, pid)
	}
	sort.Ints(result)
	return result, nil
}

func currentCancelTreePIDs(tree workerProcessTree) ([]int, error) {
	descendants, err := processTreePIDs(tree.RootPID)
	if err != nil {
		return nil, err
	}
	known, err := processPIDsPresent(tree.PIDs)
	if err != nil {
		return nil, err
	}
	set := make(map[int]struct{}, len(descendants)+len(known))
	for _, pid := range descendants {
		set[pid] = struct{}{}
	}
	for _, pid := range known {
		set[pid] = struct{}{}
	}
	pids := make([]int, 0, len(set))
	for pid := range set {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids, nil
}

func safePathSegment(value string) string {
	var segment strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-' {
			segment.WriteRune(char)
		} else {
			segment.WriteByte('_')
		}
	}
	return segment.String()
}

func readPositivePID(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || pid <= 0 {
		if err == nil {
			err = fmt.Errorf("PID must be positive, got %d", pid)
		}
		return 0, err
	}
	return pid, nil
}

func containsPID(pids []int, want int) bool {
	for _, pid := range pids {
		if pid == want {
			return true
		}
	}
	return false
}

func fileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}
