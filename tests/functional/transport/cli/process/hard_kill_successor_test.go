package process_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

const (
	hardKillSuccessorReadinessTimeout = 15 * time.Second
	hardKillProcessExitTimeout        = 5 * time.Second
)

// TestCLISuccessorAfterHardKillReportsPreRuntimeStagingContention proves the
// actual process boundary around startup. It observes the first durable
// startup checkpoint and packaged-factory staging acquisition, force-kills the
// predecessor at that boundary, and starts a successor with the same isolated
// HOME, factory, and persisted backend scope. The test deliberately observes
// the packaged-factory staging resource rather than assuming that backend
// scope identity is a lock.
func TestCLISuccessorAfterHardKillReportsPreRuntimeStagingContention(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)
	writeIdleCurrentFactory(t, session.WorkDir)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, harness.RepoRoot)
	args := hardKillSuccessorArgs(session)

	predecessor := startHardKillCLIProcess(t, binaryPath, session, args...)
	t.Cleanup(func() { _ = predecessor.stop() })
	// The config file is the first durable startup checkpoint owned by the
	// operator-settings path. Observe it before waiting for the runtime
	// listener, while keeping its identity separate from any ownership claim.
	predecessorScope := waitForPersistedBackendScopeID(t, session)
	if !operatorsettings.IsLocalBackendScopeID(predecessorScope) {
		t.Fatalf("predecessor persisted backendScopeID = %q, want local scope", predecessorScope)
	}
	stagingPath, releasePredecessor := waitForPreRuntimeStagingPath(t, session, predecessor.command.Process.Pid)
	t.Cleanup(releasePredecessor)
	if err := predecessor.stop(); err != nil {
		t.Fatalf("hard-kill predecessor: %v; stdout=%q stderr=%q process=%s", err, predecessor.stdoutText(), predecessor.stderrText(), predecessor.processState())
	}
	t.Logf("predecessor acquired pre-runtime packaged-factory staging resource and was hard-killed: %s", stagingPath)
	if _, err := os.Stat(stagingPath); err != nil {
		t.Fatalf("hard-killed predecessor did not retain staged resource %s: %v", stagingPath, err)
	}
	retainedFiles := listRegularFiles(t, session.HomeDir)
	ownershipCandidates := make([]string, 0)
	for _, path := range retainedFiles {
		name := strings.ToLower(filepath.Base(path))
		if strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".lease") || strings.HasSuffix(name, ".pid") {
			ownershipCandidates = append(ownershipCandidates, path)
			t.Errorf("hard-killed predecessor left ownership-looking file %q", path)
		}
	}
	if len(retainedFiles) <= 20 {
		t.Logf("isolated HOME after hard-killing predecessor: files=%d paths=%v ownership_candidates=%v", len(retainedFiles), retainedFiles, ownershipCandidates)
	} else {
		t.Logf("isolated HOME after hard-killing predecessor: files=%d ownership_candidates=%v", len(retainedFiles), ownershipCandidates)
	}

	successor := startHardKillCLIProcess(t, binaryPath, session, args...)
	t.Cleanup(func() { _ = successor.stop() })
	successorErr := successor.waitForBoundedFailure(t, "successor")
	if successorErr == nil {
		t.Fatal("successor returned success after retained pre-runtime staging contention")
	}
	if stdout := successor.stdoutText(); strings.Contains(stdout, "Dashboard URL:") {
		t.Fatalf("successor reached runtime despite retained staging contention: stdout=%q stderr=%q", stdout, successor.stderrText())
	}
	for _, want := range []string{
		stagingPath,
		"outcome=indeterminate-contention",
		"owner_liveness=indeterminate",
		fmt.Sprintf("owner_pid=%d", predecessor.command.Process.Pid),
		"verify no you process is still installing",
		"remove only " + stagingPath,
	} {
		if !strings.Contains(successor.stderrText(), want) {
			t.Fatalf("successor stderr = %q, want %q; process=%s", successor.stderrText(), want, successor.processState())
		}
	}
	if got := readPersistedBackendScopeID(t, session); got != predecessorScope {
		t.Fatalf("successor changed persisted backendScopeID to %q, want predecessor scope %q", got, predecessorScope)
	}
	if _, err := os.Stat(stagingPath); err != nil {
		t.Fatalf("successor removed retained staging resource %s: %v", stagingPath, err)
	}
}

func hardKillSuccessorArgs(session *builtcliacceptance.Session) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	return append(args,
		"run",
		"--dir", "factory",
		"--continuously",
		"--with-server",
		"--no-record",
	)
}

type hardKillCLIProcess struct {
	command   *exec.Cmd
	stdout    processOutput
	stderr    processOutput
	stdoutErr chan error
	waited    bool
}

func startHardKillCLIProcess(t testing.TB, binaryPath string, session *builtcliacceptance.Session, args ...string) *hardKillCLIProcess {
	t.Helper()

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open hard-kill predecessor stdout: %v", err)
	}
	process := &hardKillCLIProcess{
		command:   command,
		stdoutErr: make(chan error, 1),
	}
	command.Stderr = lockedProcessWriter{output: &process.stderr}
	if err := command.Start(); err != nil {
		t.Fatalf("start hard-kill CLI process: %v", err)
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			process.stdout.append([]byte(line + "\n"))
		}
		process.stdoutErr <- scanner.Err()
	}()
	return process
}

func (process *hardKillCLIProcess) waitForBoundedFailure(t testing.TB, role string) error {
	t.Helper()
	waitResult := make(chan error, 1)
	go func() { waitResult <- process.command.Wait() }()
	timer := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-waitResult:
		process.waited = true
	case <-timer.C:
		if process.command.Process != nil && process.command.ProcessState == nil {
			_ = process.command.Process.Kill()
		}
		waitErr = <-waitResult
		process.waited = true
		t.Fatalf("%s did not return a bounded failure: stdout=%q stderr=%q process=%s", role, process.stdoutText(), process.stderrText(), process.processState())
	}
	select {
	case scanErr := <-process.stdoutErr:
		if scanErr != nil {
			t.Fatalf("%s stdout scanner failed: %v; stdout=%q stderr=%q process=%s", role, scanErr, process.stdoutText(), process.stderrText(), process.processState())
		}
	case <-time.After(hardKillProcessExitTimeout):
		t.Fatalf("%s stdout scanner did not finish within %s: stdout=%q stderr=%q process=%s", role, hardKillProcessExitTimeout, process.stdoutText(), process.stderrText(), process.processState())
	}
	if waitErr == nil {
		t.Fatalf("%s returned success; stdout=%q stderr=%q process=%s", role, process.stdoutText(), process.stderrText(), process.processState())
	}
	return waitErr
}

func (process *hardKillCLIProcess) stop() error {
	if process == nil || process.command == nil {
		return nil
	}
	if process.waited {
		return nil
	}
	if process.command.Process != nil && process.command.ProcessState == nil {
		if err := process.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill process: %w", err)
		}
	}
	waitResult := make(chan error, 1)
	go func() { waitResult <- process.command.Wait() }()
	timer := time.NewTimer(hardKillProcessExitTimeout)
	defer timer.Stop()
	select {
	case <-waitResult:
		process.waited = true
	case <-timer.C:
		return fmt.Errorf("process did not exit within %s", hardKillProcessExitTimeout)
	}
	select {
	case <-process.stdoutErr:
	case <-time.After(hardKillProcessExitTimeout):
		return fmt.Errorf("stdout scanner did not finish within %s", hardKillProcessExitTimeout)
	}
	return nil
}

func (process *hardKillCLIProcess) stdoutText() string { return process.stdout.String() }

func (process *hardKillCLIProcess) stderrText() string { return process.stderr.String() }

func (process *hardKillCLIProcess) processState() string {
	if process == nil || process.command == nil || process.command.ProcessState == nil {
		return "running-or-unwaited"
	}
	return process.command.ProcessState.String()
}

type processOutput struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (output *processOutput) append(data []byte) {
	output.mu.Lock()
	defer output.mu.Unlock()
	_, _ = output.data.Write(data)
}

func (output *processOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.data.String()
}

type lockedProcessWriter struct{ output *processOutput }

func (writer lockedProcessWriter) Write(data []byte) (int, error) {
	writer.output.append(data)
	return len(data), nil
}

func readPersistedBackendScopeID(t testing.TB, session *builtcliacceptance.Session) string {
	t.Helper()
	path := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted operator config %s: %v", path, err)
	}
	var config struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode persisted operator config %s: %v; data=%q", path, err, data)
	}
	return strings.TrimSpace(config.BackendScopeID)
}

func waitForPersistedBackendScopeID(t testing.TB, session *builtcliacceptance.Session) string {
	t.Helper()
	path := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	deadline := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer deadline.Stop()
	// The predecessor is a separately built CLI process, so its durable
	// operator-settings write cannot be observed through BuildProcess or an
	// injected edge. Poll this filesystem checkpoint only to synchronize the
	// OS-process-boundary proof before inspecting the packaged-factory resource.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastReadErr error
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			var config struct {
				BackendScopeID string `json:"backendScopeID"`
			}
			if err := json.Unmarshal(data, &config); err != nil {
				t.Fatalf("decode pre-runtime operator config %s: %v; data=%q", path, err, data)
			}
			if scopeID := strings.TrimSpace(config.BackendScopeID); scopeID != "" {
				return scopeID
			}
			t.Fatalf("pre-runtime operator config %s has empty backendScopeID; data=%q", path, data)
		}
		lastReadErr = err

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out observing pre-runtime operator config %s; last read error: %v", path, lastReadErr)
		}
	}
}

func waitForPreRuntimeStagingPath(t testing.TB, session *builtcliacceptance.Session, predecessorPID int) (string, func()) {
	t.Helper()
	root := filepath.Join(session.HomeDir, ".you-agent-factory", "factories")
	deadline := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer deadline.Stop()
	// The packaged installer and its exclusive directory reservation run in the
	// child process and expose no parent-process event or injectable edge. Stop
	// the child before inspecting the isolated filesystem. This is the
	// acquisition barrier: once the owner directory is observed, the child
	// cannot reach its normal release path before the parent hard-kills it. If
	// no owner is present, resume the child and retry; the deadline is only a
	// failure guard for a missing acquisition.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		resume, err := suspendHardKillProcess(predecessorPID)
		if err != nil {
			t.Fatalf("suspend predecessor before observing pre-runtime staging: %v", err)
		}
		path, err := findPreRuntimeStagingPath(root)
		if err != nil {
			_ = resume()
			t.Fatalf("observe pre-runtime packaged-factory staging under %s: %v", root, err)
		}
		if path != "" {
			return path, func() { _ = resume() }
		}
		if err := resume(); err != nil {
			t.Fatalf("resume predecessor after observing no pre-runtime staging: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out observing pre-runtime packaged-factory staging under %s", root)
		}
	}
}

func findPreRuntimeStagingPath(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || !entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") && strings.HasSuffix(entry.Name(), ".staging-owner") {
			if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				return nil
			} else if statErr != nil {
				return statErr
			}
			found = path
			return filepath.SkipDir
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return found, err
}

func listRegularFiles(t testing.TB, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, relative)
		return nil
	})
	if err != nil {
		t.Fatalf("inventory isolated HOME %s: %v", root, err)
	}
	return files
}
