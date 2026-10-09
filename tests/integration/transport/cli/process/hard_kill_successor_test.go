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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	hardKillSuccessorReadinessTimeout = 90 * time.Second
	hardKillProcessExitTimeout        = 30 * time.Second
	preRuntimeStagingOwnerMetadata    = ".owner.json"
)

type hardKillProcessControl struct {
	resume    func() error
	terminate func() error
}

// TestCLISuccessorAfterHardKillRecoversAbandonedStaging proves the
// actual process boundary around startup. It observes the first durable
// startup checkpoint and packaged-factory staging acquisition, force-kills the
// predecessor at that boundary, and starts a successor with the same isolated
// HOME, factory, and persisted backend scope. The test deliberately observes
// the packaged-factory staging resource rather than assuming that backend
// scope identity is a lock. The successor must reclaim abandoned ownership,
// reach public readiness, preserve that scope and stop through the public CLI.
func TestCLISuccessorAfterHardKillRecoversAbandonedStaging(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)
	writeIdleCurrentFactory(t, session.WorkDir)

	binaryPath := quietShutdownArtifact(t)
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
	stagingPath, predecessorControl := waitForPreRuntimeStagingPath(t, session, predecessor.command.Process.Pid)
	t.Cleanup(func() { _ = predecessorControl.resume() })
	if err := predecessor.stopWith(predecessorControl.terminate); err != nil {
		t.Fatalf("hard-kill predecessor: %v; stdout=%q stderr=%q process=%s", err, predecessor.stdoutText(), predecessor.stderrText(), predecessor.processState())
	}
	_ = predecessorControl.resume()
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
	assertHardKillSuccessorRecovery(t, binaryPath, session, successor, stagingPath, predecessorScope)
}

func assertHardKillSuccessorRecovery(
	t *testing.T,
	binaryPath string,
	session *builtcliacceptance.Session,
	successor *hardKillCLIProcess,
	stagingPath, predecessorScope string,
) {
	t.Helper()
	successor.waitForReadiness(t)
	// This compiled child owns the listener; public status polling crosses the
	// real process boundary and cannot be replaced by an in-process event.
	waitForStatus(t, session.ServerURL, hardKillSuccessorReadinessTimeout, func(status factoryapi.StatusResponse) bool {
		return status.FactoryState == "RUNNING"
	})
	if got := readPersistedBackendScopeID(t, session); got != predecessorScope {
		t.Fatalf("successor changed persisted backendScopeID to %q, want predecessor scope %q", got, predecessorScope)
	}
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successor did not release abandoned staging resource %s: %v", stagingPath, err)
	}
	if got := listStagingOwnershipResources(t, session.HomeDir); len(got) != 0 {
		t.Fatalf("successor retained staging ownership resources after readiness: %v", got)
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), hardKillProcessExitTimeout)
	defer cancel()
	result, err := runBuiltYouBinary(stopCtx, binaryPath, session, "--server", session.ServerURL, "server", "stop")
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("public successor shutdown: %v; result=%#v", err, result)
	}
	if err, exited := successor.waitForExit(hardKillProcessExitTimeout); !exited || err != nil {
		t.Fatalf("successor did not exit cleanly after public shutdown: exited=%v err=%v stderr=%q", exited, err, successor.stderrText())
	}
	if err, scanned := successor.waitForScanner(hardKillProcessExitTimeout); !scanned || (err != nil && !errors.Is(err, os.ErrClosed)) {
		t.Fatalf("successor stdout scanner: scanned=%v err=%v", scanned, err)
	}
	t.Logf("hard-killed owner recovered: public readiness, preserved backend scope, lease release and public shutdown; staging=%s", stagingPath)
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
	command *exec.Cmd
	stdout  processOutput
	stderr  processOutput

	waitOnce sync.Once
	waitDone chan struct{}
	waitErr  error

	ready    chan struct{}
	scanDone chan struct{}
	scanErr  error
	mu       sync.Mutex
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
		command:  command,
		waitDone: make(chan struct{}),
		scanDone: make(chan struct{}),
		ready:    make(chan struct{}, 1),
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
			if strings.HasPrefix(line, "Dashboard URL: ") {
				select {
				case process.ready <- struct{}{}:
				default:
				}
			}
		}
		process.mu.Lock()
		process.scanErr = scanner.Err()
		process.mu.Unlock()
		close(process.scanDone)
	}()
	return process
}

func (process *hardKillCLIProcess) waitForReadiness(t testing.TB) {
	t.Helper()
	timer := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer timer.Stop()
	for {
		select {
		case <-process.ready:
			return
		case <-process.scanDone:
			t.Fatalf("successor exited before readiness: stdout=%q stderr=%q", process.stdoutText(), process.stderrText())
		case <-timer.C:
			t.Fatalf("successor did not reach readiness: stdout=%q stderr=%q", process.stdoutText(), process.stderrText())
		}
	}
}

func (process *hardKillCLIProcess) stop() error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return nil
	}
	return process.stopWith(process.command.Process.Kill)
}

func (process *hardKillCLIProcess) stopWith(terminate func() error) error {
	if process == nil || process.command == nil {
		return nil
	}
	terminated, err := process.killWith(terminate)
	if err != nil {
		return err
	}
	if _, exited := process.waitForExit(hardKillProcessExitTimeout); !exited {
		return fmt.Errorf("process did not exit within %s", hardKillProcessExitTimeout)
	}
	scanErr, scanned := process.waitForScanner(hardKillProcessExitTimeout)
	if !scanned {
		return fmt.Errorf("stdout scanner did not finish within %s", hardKillProcessExitTimeout)
	}
	// Cmd.Wait closes a StdoutPipe after reaping the child. When this helper's
	// intentional termination wins that ordering race, the scanner can observe
	// the terminal descriptor close as fs.ErrClosed instead of EOF. The process
	// has already been reaped here, so accept only that expected terminal error;
	// every other scanner error remains actionable.
	if scanErr != nil && !(terminated && errors.Is(scanErr, os.ErrClosed)) {
		return fmt.Errorf("stdout scanner: %w", scanErr)
	}
	return nil
}

func (process *hardKillCLIProcess) killWith(terminate func() error) (bool, error) {
	if process.command.Process == nil || process.command.ProcessState != nil {
		return false, nil
	}
	if err := terminate(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return false, fmt.Errorf("kill process: %w", err)
	}
	return true, nil
}

func (process *hardKillCLIProcess) waitForExit(timeout time.Duration) (error, bool) {
	process.waitOnce.Do(func() {
		go func() {
			err := process.command.Wait()
			process.mu.Lock()
			process.waitErr = err
			process.mu.Unlock()
			close(process.waitDone)
		}()
	})
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-process.waitDone:
		process.mu.Lock()
		err := process.waitErr
		process.mu.Unlock()
		return err, true
	case <-timer.C:
		return nil, false
	}
}

func (process *hardKillCLIProcess) waitForScanner(timeout time.Duration) (error, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-process.scanDone:
		process.mu.Lock()
		err := process.scanErr
		process.mu.Unlock()
		return err, true
	case <-timer.C:
		return nil, false
	}
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

func waitForPreRuntimeStagingPath(t testing.TB, session *builtcliacceptance.Session, predecessorPID int) (string, hardKillProcessControl) {
	t.Helper()
	root := filepath.Join(session.HomeDir, ".you-agent-factory", "factories")
	deadline := time.NewTimer(hardKillSuccessorReadinessTimeout)
	defer deadline.Stop()
	// The packaged installer and its exclusive directory reservation run in the
	// child process and expose no parent-process event or injectable edge. First
	// wait for the complete readable owner record. Then suspend the child and
	// verify that exact ownership directory remains before returning the kill
	// checkpoint; if the child released it in the suspend gap, resume and wait
	// for the next complete record. A Windows writer can expose the directory
	// while retaining the metadata handle, so a sharing violation is an
	// incomplete checkpoint rather than a failed successor observation. Once the
	// final path check succeeds, the child cannot reach its normal release path
	// before the parent hard-kills it; the deadline is only a failure guard for a
	// missing acquisition.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		path, err := findPreRuntimeStagingPath(root, predecessorPID)
		if err != nil {
			if !isPreRuntimeStagingMetadataUnavailable(err) {
				t.Fatalf("observe complete pre-runtime packaged-factory staging under %s: %v", root, err)
			}
			path = ""
		}
		if path != "" {
			control, err := suspendHardKillProcess(predecessorPID)
			if err != nil {
				t.Fatalf("suspend predecessor before observing pre-runtime staging: %v", err)
			}
			_, statErr := os.Stat(path)
			if statErr == nil || isPreRuntimeStagingMetadataUnavailable(statErr) {
				return path, control
			}
			if !errors.Is(statErr, os.ErrNotExist) {
				_ = control.resume()
				t.Fatalf("verify pre-runtime packaged-factory staging under %s: %v", root, statErr)
			}
			if err := control.resume(); err != nil {
				t.Fatalf("resume predecessor after staging release before suspension: %v", err)
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out observing pre-runtime packaged-factory staging under %s", root)
		}
	}
}

func findPreRuntimeStagingPath(root string, predecessorPID int) (string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".staging-owner") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		metadata, err := os.ReadFile(filepath.Join(path, preRuntimeStagingOwnerMetadata))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		var owner struct {
			PID int `json:"pid"`
		}
		if err := json.Unmarshal(metadata, &owner); err != nil {
			return "", fmt.Errorf("decode staging owner metadata %s: %w", path, err)
		}
		if owner.PID != predecessorPID {
			return "", fmt.Errorf("staging owner metadata %s has PID %d, want predecessor PID %d", path, owner.PID, predecessorPID)
		}
		return path, nil
	}
	return "", nil
}

func listStagingOwnershipResources(t testing.TB, root string) []string {
	t.Helper()
	var resources []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := strings.ToLower(entry.Name())
		metadataName := strings.ToLower(preRuntimeStagingOwnerMetadata)
		if (entry.IsDir() && strings.HasSuffix(name, ".staging-owner")) ||
			(!entry.IsDir() && (name == metadataName || name == metadataName+".tmp")) {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			resources = append(resources, relative)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inventory staging ownership resources under %s: %v", root, err)
	}
	return resources
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
