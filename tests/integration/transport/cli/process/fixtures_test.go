package process_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func waitForStatus(
	t testing.TB,
	baseURL string,
	timeout time.Duration,
	accept func(factoryapi.StatusResponse) bool,
) factoryapi.StatusResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.StatusResponse
	var lastErr error
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			func() {
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					lastErr = fmt.Errorf("status = %d", response.StatusCode)
					return
				}
				lastErr = json.NewDecoder(response.Body).Decode(&last)
			}()
			if lastErr == nil && accept(last) {
				return last
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for status at %s: last=%#v error=%v", endpoint, last, lastErr)
			return last
		}
	}
}

const (
	stdinRunWorkTypeName      = "prompt-task"
	stdinRunWorkstationName   = "process-prompt"
	stdinRunWorkerName        = "mock-worker"
	stdinRunFactoryConfigName = "stdin-run-process"
)

func writeStdinRunFactory(t testing.TB, workDir string) string {
	t.Helper()

	factoryDir := filepath.Join(workDir, "stdin-run-factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create stdin run factory directory: %v", err)
	}

	factoryJSON := fmt.Sprintf(`{
  "name": %q,
  "workTypes": [{
    "name": %q,
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": %q}],
  "workstations": [{
    "name": %q,
    "worker": %q,
    "inputs": [{"workType": %q, "state": "init"}],
    "outputs": [{"workType": %q, "state": "complete"}],
    "onFailure": [{"workType": %q, "state": "failed"}]
  }]
}`,
		stdinRunFactoryConfigName,
		stdinRunWorkTypeName,
		stdinRunWorkerName,
		stdinRunWorkstationName,
		stdinRunWorkerName,
		stdinRunWorkTypeName,
		stdinRunWorkTypeName,
		stdinRunWorkTypeName,
	)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write stdin run factory.json: %v", err)
	}

	workstationPath := filepath.Join(factoryDir, "workstations", stdinRunWorkstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		t.Fatalf("create stdin run workstation directory: %v", err)
	}
	workstationConfig := "---\ntype: MODEL_WORKSTATION\n---\nProcess {{ (index .Inputs 0).Payload }}.\n"
	if err := os.WriteFile(workstationPath, []byte(workstationConfig), 0o644); err != nil {
		t.Fatalf("write stdin run workstation config: %v", err)
	}

	return factoryPath
}

func writeStdinRunDefaultMockWorkers(t testing.TB) string {
	t.Helper()

	data, err := json.MarshalIndent(workers.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal stdin run mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "stdin-run-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write stdin run mock workers: %v", err)
	}
	return path
}

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "processor"}],
  "workstations": [{
    "name": "process",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "worker": "processor"
  }]
}`

func writeIdleCurrentFactory(t testing.TB, workingDirectory string) {
	t.Helper()

	factoryDirectory := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	path := filepath.Join(factoryDirectory, "factory.json")
	if err := os.WriteFile(path, []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}
}

func waitForDashboardURL(
	t testing.TB,
	lines <-chan string,
	scanErr <-chan error,
	stderr *bytes.Buffer,
	timeout time.Duration,
) string {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line := <-lines:
			if target, ok := strings.CutPrefix(line, "Dashboard URL: "); ok {
				return target
			}
		case err := <-scanErr:
			t.Fatalf("root process exited before readiness: %v; stderr=%q", err, stderr.String())
		case <-timer.C:
			t.Fatalf("timed out waiting for root process readiness; stderr=%q", stderr.String())
		}
	}
}

func interruptAndAssertCancellationExit(t testing.TB, command *builtcliacceptance.Command, waitTimeout time.Duration) {
	t.Helper()
	command.Cancel()
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		if err != nil && !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("canceled root process exit = %v, want clean cancellation", err)
		}
	case <-time.After(waitTimeout):
		t.Fatalf("canceled root process did not exit within %s", waitTimeout)
	}
}

func waitForScannerCompletion(t testing.TB, scanErr <-chan error, role string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-scanErr:
		// exec.Cmd.Wait closes a StdoutPipe after the child exits. Depending on
		// scheduling, the scanner can observe that terminal close as fs.ErrClosed
		// instead of EOF after an intentional cancellation.
		if err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("%s stdout scanner failed: %v", role, err)
		}
	case <-timer.C:
		t.Fatalf("%s stdout scanner did not finish within %s", role, timeout)
	}
}

func startRootProcessServerCommand(
	t testing.TB,
	session *builtcliacceptance.Session,
	harness *builtcliacceptance.Harness,
) (*builtcliacceptance.Command, <-chan string, <-chan error, *bytes.Buffer) {
	t.Helper()

	writeIdleCurrentFactory(t, session.WorkDir)

	args := append([]string{}, session.ServerFlags()...)
	args = append(args, "server")

	command := harness.Command(args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start root process server: %v", err)
	}

	lines := make(chan string, 128)
	scanErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanErr <- scanner.Err()
	}()
	return command, lines, scanErr, &stderr
}
