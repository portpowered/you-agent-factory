package cancel_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	deadline := time.NewTimer(120 * time.Second)
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
		grandchildPID, grandchildErr := readPositivePID(filepath.Join(attemptDir, "grandchild.pid"))
		ready, readyErr := os.ReadFile(filepath.Join(attemptDir, "ready"))
		childStarted, childStartedErr := os.ReadFile(filepath.Join(attemptDir, "child.started"))
		lateOutputReady, lateOutputErr := os.ReadFile(filepath.Join(attemptDir, "late-output.ready"))
		if rootErr != nil || childErr != nil || grandchildErr != nil || readyErr != nil || childStartedErr != nil || lateOutputErr != nil ||
			strings.TrimSpace(string(ready)) != "ready" || strings.TrimSpace(string(childStarted)) != "started" ||
			strings.TrimSpace(string(lateOutputReady)) != "ready" || containsPID(excludedRootPIDs, rootPID) {
			lastReadiness = fmt.Sprintf("attempt=%s root=%v child=%v ready=%v child_started=%v late_output=%v", entry.Name(), rootErr, childErr, readyErr, childStartedErr, lateOutputErr)
			continue
		}
		pids, err := processTreePIDs(rootPID)
		if err == nil && containsPID(pids, childPID) && containsPID(pids, grandchildPID) {
			return workerProcessTree{WorkID: workID, RootPID: rootPID, ChildPID: childPID, GrandchildPID: grandchildPID, PIDs: pids}, ""
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
	parent, err = processParentPID(tree.GrandchildPID)
	if err != nil || parent != tree.ChildPID {
		t.Fatalf("Work %q process ancestry grandchild %d parent=%d, error=%v, want child %d", tree.WorkID, tree.GrandchildPID, parent, err, tree.ChildPID)
	}
}

func sampleCancelProcessTrees(t *testing.T, ctx context.Context, target, unrelated workerProcessTree, appliedAt time.Time) []cancelProcessSample {
	t.Helper()
	samples := make([]cancelProcessSample, 0, int(cancelProcessBound/cancelProcessSampleInterval)+1)
	// A process identity that was observed absent has exited for good. The OS
	// may hand the same numeric PID to an unrelated process on a busy host, so
	// later presence of a retired PID is PID reuse, not survival of the target.
	retired := make(map[int]struct{}, len(target.PIDs))
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
		samples = append(samples, retireExitedTargetProcesses(sample, target, retired))
	}
	return samples
}

// retireExitedTargetProcesses masks target PIDs that were already observed
// absent in an earlier sample so a reused PID cannot read as a surviving
// target. It records each newly absent target PID as retired.
func retireExitedTargetProcesses(sample cancelProcessSample, target workerProcessTree, retired map[int]struct{}) cancelProcessSample {
	present := make([]int, 0, len(sample.TargetPresent))
	for _, pid := range sample.TargetPresent {
		if _, gone := retired[pid]; !gone {
			present = append(present, pid)
		}
	}
	sample.TargetPresent = present
	if _, rootGone := retired[target.RootPID]; rootGone {
		sample.TargetDescendants = nil
	}
	alive := make(map[int]struct{}, len(sample.TargetPresent))
	for _, pid := range sample.TargetPresent {
		alive[pid] = struct{}{}
	}
	for _, pid := range target.PIDs {
		if _, ok := alive[pid]; !ok {
			retired[pid] = struct{}{}
		}
	}
	return sample
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
	case <-time.After(90 * time.Second):
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
	case <-time.After(60 * time.Second):
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

// This prebuilt witness crosses native ownership and public archived recovery.
// Factory dispatch replay remains a distinct retained requirement.
func TestPrebuiltWorkerSessionForceDirectTreeAndArchive(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("owned force capability requires Windows Job or Linux retained leader")
	}
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			proveNativeForceTreeAndArchive(t, provider)
		})
	}
}

func proveNativeForceTreeAndArchive(t *testing.T, provider string) {
	t.Helper()
	binary := resolveCancelArtifact(t)
	ctx, cancel := context.WithTimeout(t.Context(), 110*time.Second)
	defer cancel()
	fixture := writeNativeForceFixture(t, provider)
	hash, err := fileSHA256(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%s OS=%s/%s fixture=direct-force-tree-v1", binary, hash, runtime.GOOS, runtime.GOARCH)
	daemon := startCancelDaemon(t, ctx, binary, fixture)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("native force daemon stdout=%s stderr=%s", daemon.stdout.String(), daemon.stderr.String())
		}
	})
	session := waitForCancelFactorySession(t, ctx, fixture.serverURL, daemon)
	source := invokeNativeForceFixture(t, ctx, binary, fixture, session, provider, "source")
	sourceTree := waitForFixtureProcessTree(t, ctx, fixture.stateDir, "source")
	registerFailedTreeCleanup(t, sourceTree)
	assertObservedTreeAncestry(t, sourceTree)
	invokeNativeForceFixture(t, ctx, binary, fixture, session, provider, "sibling")
	siblingTree := waitForFixtureProcessTree(t, ctx, fixture.stateDir, "sibling")
	registerFailedTreeCleanup(t, siblingTree)
	assertObservedTreeAncestry(t, siblingTree)
	if runtime.GOOS == "windows" {
		// Include the native .cmd launcher as well as the PowerShell tree.
		parent, err := processParentPID(sourceTree.RootPID)
		if err != nil || parent == 0 {
			t.Fatalf("resolve live provider launcher ancestry: %v", err)
		}
		sourceTree.PIDs = append(sourceTree.PIDs, parent)
	}
	assertNativeStaleForceRefused(t, ctx, binary, fixture, source)
	assertNativeTreesLive(t, sourceTree, siblingTree)
	result := forceNativeFixture(t, ctx, binary, fixture, source)
	sample, err := captureProcessSample(sourceTree, siblingTree, time.Now())
	if err != nil || len(sample.TargetPresent) != 0 || !reflect.DeepEqual(sample.UnrelatedPresent, siblingTree.PIDs) {
		t.Fatalf("APPLIED did not join exact tree with live sibling: sample=%+v error=%v", sample, err)
	}
	assertForcedNativeArchive(t, ctx, fixture, source)
	if retry := forceNativeFixture(t, ctx, binary, fixture, source); !reflect.DeepEqual(result, retry) {
		t.Fatalf("committed retry changed: original=%+v retry=%+v", result, retry)
	}
	assertNativeTerminalForceNoop(t, ctx, binary, fixture, source)
	assertNativeTreesLive(t, siblingTree)
	assertNativeLaunchCount(t, fixture, 2)
	stopCancelDaemon(t, binary, fixture, daemon)
	daemon = startCancelDaemon(t, ctx, binary, fixture)
	waitForCancelFactorySession(t, ctx, fixture.serverURL, daemon)
	assertForcedNativeArchive(t, ctx, fixture, source)
	if retry := forceNativeFixture(t, ctx, binary, fixture, source); !reflect.DeepEqual(result, retry) {
		t.Fatalf("restart retry changed: original=%+v retry=%+v", result, retry)
	}
	assertNativeLaunchCount(t, fixture, 2)
	stopCancelDaemon(t, binary, fixture, daemon)
	t.Log("PASS: direct force joined native launcher/root/child/grandchild before APPLIED; sibling survived; archive and committed retry survived joined host restart")
}

func assertNativeLaunchCount(t *testing.T, fixture cancelFixture, want int) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixture.stateDir, "native-launches"))
	if err != nil || len(strings.Fields(string(content))) != want {
		t.Fatalf("provider launches = %q, error=%v; want exactly %d with no force/restart retry", content, err, want)
	}
}

func invokeNativeForceFixture(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, session, provider, name string) factoryapi.WorkerSessionObservation {
	t.Helper()
	id := "force-" + name
	document := map[string]any{"execution": map[string]any{
		"factorySessionId": session, "workstationName": "__provider_invocation__",
		"dispatch":   map[string]any{"dispatchId": id + "-dispatch", "workstationName": "__provider_invocation__", "workerType": "process-worker"},
		"workerType": "process-worker", "runnerId": provider, "executorProvider": provider, "modelProvider": provider, "model": "force-fixture",
		"workingDirectory": fixture.factoryDir, "workingDirectoryAuthored": true, "userMessage": name,
	}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", id+"-invoke", "--worker-session-id", id, "--dispatch-id", id+"-dispatch", "--execution", string(encoded), "--retry-max-attempts", "1", "--async")
	if result.err != nil {
		t.Fatalf("invoke native fixture: %+v", result)
	}
	waitForFixtureProcessTree(t, ctx, fixture.stateDir, name)
	observation, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+id)
	if err != nil || string(observation.State) != "RUNNING" || observation.ProviderSession != nil {
		t.Fatalf("native fixture before force: observation=%+v error=%v", observation, err)
	}
	return observation
}

func assertNativeTreesLive(t *testing.T, trees ...workerProcessTree) {
	t.Helper()
	for _, tree := range trees {
		for _, pid := range tree.PIDs {
			present, err := processPIDPresent(pid)
			if err != nil || !present {
				t.Fatalf("refused control affected live %s process %d: present=%t error=%v", tree.WorkID, pid, present, err)
			}
		}
	}
}

func assertNativeStaleForceRefused(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-stale", "--expected-attempt-id", observation.AttemptId+"-stale")
	var response factoryapi.ErrorResponse
	if result.err == nil || json.Unmarshal([]byte(result.stderr), &response) != nil || string(response.Code) != "WORKER_SESSION_CONTROL_CONFLICT" {
		t.Fatalf("stale force must return typed conflict: result=%+v response=%+v", result, response)
	}
}

func assertNativeTerminalForceNoop(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-expired", "--expected-attempt-id", observation.AttemptId)
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != "NOOP" || string(response.State) != "TERMINATED" {
		t.Fatalf("terminal force must be a no-op: result=%+v response=%+v", result, response)
	}
	assertForcedNativeArchive(t, ctx, fixture, observation)
}

func forceNativeFixture(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) factoryapi.WorkerSessionControlResponse {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-kill", "--expected-attempt-id", observation.AttemptId)
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != "APPLIED" || string(response.State) != "TERMINATED" || response.Forced == nil || !*response.Forced {
		current, readErr := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+observation.WorkerSessionId)
		t.Logf("force failure observation=%+v error=%v", current, readErr)
		t.Fatalf("native force: result=%+v response=%+v", result, response)
	}
	return response
}

func assertForcedNativeArchive(t *testing.T, ctx context.Context, fixture cancelFixture, original factoryapi.WorkerSessionObservation) {
	t.Helper()
	observation, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+original.WorkerSessionId)
	if err != nil || string(observation.State) != "TERMINATED" || observation.TerminalCause == nil || string(*observation.TerminalCause) != "OPERATOR_KILL" || observation.AttemptId != original.AttemptId {
		t.Fatalf("force terminal archive: observation=%+v error=%v", observation, err)
	}
}
