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

// TestCLISuccessorAfterHardKillReachesRuntime proves the actual process
// boundary around startup. It records the listener readiness observation,
// force-kills the predecessor, and starts a successor with the same isolated
// HOME, factory, and persisted backend scope. The test deliberately inventories
// the isolated home rather than assuming that backend scope identity is a lock.
func TestCLISuccessorAfterHardKillReachesRuntime(t *testing.T) {
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
	predecessorURL := predecessor.waitForDashboardURL(t, "predecessor")
	if predecessorURL != session.ServerURL+"/dashboard/ui" {
		t.Fatalf("predecessor readiness URL = %q, want %q", predecessorURL, session.ServerURL+"/dashboard/ui")
	}

	if !operatorsettings.IsLocalBackendScopeID(predecessorScope) {
		t.Fatalf("predecessor persisted backendScopeID = %q, want local scope", predecessorScope)
	}

	if err := predecessor.stop(); err != nil {
		t.Fatalf("hard-kill predecessor: %v; stdout=%q stderr=%q process=%s", err, predecessor.stdoutText(), predecessor.stderrText(), predecessor.processState())
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
	t.Logf("isolated HOME after hard-killing predecessor: files=%d ownership_candidates=%v", len(retainedFiles), ownershipCandidates)

	successor := startHardKillCLIProcess(t, binaryPath, session, args...)
	t.Cleanup(func() { _ = successor.stop() })
	successorURL := successor.waitForDashboardURL(t, "successor")
	if successorURL != predecessorURL {
		t.Fatalf("successor readiness URL = %q, predecessor URL = %q", successorURL, predecessorURL)
	}
	if got := readPersistedBackendScopeID(t, session); got != predecessorScope {
		t.Fatalf("successor changed persisted backendScopeID to %q, want predecessor scope %q", got, predecessorScope)
	}
	if err := successor.stop(); err != nil {
		t.Fatalf("stop successor: %v; stdout=%q stderr=%q process=%s", err, successor.stdoutText(), successor.stderrText(), successor.processState())
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
		"--with-mock-workers",
	)
}

type hardKillCLIProcess struct {
	command   *exec.Cmd
	ready     chan string
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
		ready:     make(chan string, 1),
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
			if target, ok := strings.CutPrefix(line, "Dashboard URL: "); ok {
				select {
				case process.ready <- target:
				default:
				}
			}
		}
		process.stdoutErr <- scanner.Err()
	}()
	return process
}

func (process *hardKillCLIProcess) waitForDashboardURL(t testing.TB, role string) string {
	t.Helper()
	timer := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer timer.Stop()
	select {
	case target := <-process.ready:
		return target
	case err := <-process.stdoutErr:
		t.Fatalf("%s exited before dashboard readiness: scanner=%v stdout=%q stderr=%q process=%s", role, err, process.stdoutText(), process.stderrText(), process.processState())
	case <-timer.C:
		t.Fatalf("timed out waiting for %s dashboard readiness: stdout=%q stderr=%q process=%s", role, process.stdoutText(), process.stderrText(), process.processState())
	}
	return ""
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
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
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
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("observe pre-runtime operator config %s: %v", path, err)
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out observing pre-runtime operator config %s", path)
		}
	}
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
