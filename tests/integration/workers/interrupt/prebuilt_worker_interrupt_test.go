package interrupt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	prebuiltArtifactEnvironment         = "INFINITE_YOU_INTEGRATION_BINARY"
	prebuiltArtifactFallbackEnvironment = "INFINITE_YOU_PREBUILT_ARTIFACT"
	prebuiltArtifactRequiredEnvironment = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	interruptFactorySessionID           = factorysessions.DefaultSessionID
	interruptSourceWorkerSessionID      = "fr-a6-source"
	interruptSuccessorWorkerSessionID   = "fr-a6-successor"
	interruptSourceRequestID            = "fr-a6-source-request"
	interruptSourceDispatchID           = "fr-a6-source-dispatch"
	interruptRequestID                  = "fr-a6-interrupt"
	interruptWorkID                     = "fr-a6-work"
	interruptModel                      = "fr-a6-model"
	interruptReplacementMessage         = "fr-a6 replacement"
	interruptIntegrationTimeout         = 90 * time.Second
)

// TestPrebuiltWorkerInterruptStopsExactChildAndAdmitsOneSuccessor proves the
// public Worker Session interrupt contract through one already-built CLI,
// one real daemon process, and one local provider child. The source provider
// child publishes its session identity and then remains live until the server
// cancels and joins it. The successor is allowed to start only after the
// source child is gone, so the ordering assertion is against process evidence,
// not a mock cancellation callback.
func TestPrebuiltWorkerInterruptStopsExactChildAndAdmitsOneSuccessor(t *testing.T) {
	binaryPath := resolvePrebuiltArtifact(t)
	ctx, cancel := context.WithTimeout(t.Context(), interruptIntegrationTimeout)
	defer cancel()

	factoryDir := t.TempDir()
	workspaceDir := filepath.Join(factoryDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("create execution workspace: %v", err)
	}
	writeInterruptFactory(t, factoryDir)
	stateDir := filepath.Join(t.TempDir(), "provider-state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create provider state directory: %v", err)
	}
	providerDir := filepath.Join(t.TempDir(), "provider-bin")
	writeInterruptProvider(t, providerDir)
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve integration listener: %v", err)
	}
	serverURL := "http://127.0.0.1:" + strconv.Itoa(port)
	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create isolated home: %v", err)
	}
	env := interruptIntegrationEnvironment(homeDir, providerDir, stateDir)
	daemon := startInterruptDaemon(t, ctx, binaryPath, factoryDir, serverURL, env)

	waitForInterruptStatus(t, ctx, daemon, serverURL)
	execution := interruptExecutionDocument(workspaceDir)
	invoke := runInterruptBinary(t, ctx, binaryPath, factoryDir, env,
		"--remote", "--server", serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", interruptSourceRequestID,
		"--worker-session-id", interruptSourceWorkerSessionID,
		"--dispatch-id", interruptSourceDispatchID,
		"--execution", execution,
		"--retry-max-attempts", "1",
		"--async",
	)
	assertInterruptInvokeAccepted(t, invoke)
	t.Logf("prebuilt invoke stdout=%q stderr=%q", invoke.stdout, invoke.stderr)

	sourcePID := waitForInterruptPIDOrFailure(t, ctx, daemon, serverURL, filepath.Join(stateDir, "source.pid"))
	waitForInterruptMarker(t, ctx, filepath.Join(stateDir, "source.started"))
	waitForInterruptProviderSession(t, ctx, serverURL, interruptSourceWorkerSessionID, "fr-a6-source-thread")
	interruptResponse := postInterrupt(t, ctx, serverURL)
	if !interruptResponse.Accepted || interruptResponse.RequestId != interruptRequestID ||
		interruptResponse.SourceWorkerSessionId != interruptSourceWorkerSessionID ||
		interruptResponse.SuccessorWorkerSessionId != interruptSuccessorWorkerSessionID {
		t.Fatalf("interrupt response = %#v, want accepted exact source/successor identities", interruptResponse)
	}
	if interruptResponse.Source.State != factoryapi.WorkerSessionInterruptSnapshotState("CANCELED") {
		t.Fatalf("interrupt source snapshot = %#v, want CANCELED", interruptResponse.Source)
	}
	if interruptResponse.Successor.State != factoryapi.WorkerSessionInterruptSnapshotState("STARTING") &&
		interruptResponse.Successor.State != factoryapi.WorkerSessionInterruptSnapshotState("RUNNING") {
		t.Fatalf("interrupt successor snapshot = %#v, want admitted active state", interruptResponse.Successor)
	}

	waitForInterruptMarker(t, ctx, filepath.Join(stateDir, "successor.started"))
	sourceAlive, err := interruptProcessAlive(sourcePID)
	if err != nil {
		t.Fatalf("check source provider PID %d after successor admission: %v", sourcePID, err)
	}
	if sourceAlive {
		t.Fatalf("source provider PID %d remained alive when successor start marker appeared", sourcePID)
	}
	successorPID := waitForInterruptPID(t, ctx, filepath.Join(stateDir, "successor.pid"))
	waitForInterruptMarker(t, ctx, filepath.Join(stateDir, "successor.completed"))
	waitForInterruptProcessExit(t, ctx, successorPID)
	if count, err := os.ReadFile(filepath.Join(stateDir, "count")); err != nil || strings.TrimSpace(string(count)) != "2" {
		t.Fatalf("provider invocation count = %q, %v; want exactly source plus one successor", count, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "unexpected.marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected provider invocation marker error = %v, want absent", err)
	}

	source := waitForInterruptWorkerState(t, ctx, serverURL, stateDir, daemon, interruptSourceWorkerSessionID, "CANCELED")
	successor := waitForInterruptWorkerState(t, ctx, serverURL, stateDir, daemon, interruptSuccessorWorkerSessionID, "COMPLETED")
	if source.WorkerSessionId != interruptSourceWorkerSessionID || successor.WorkerSessionId != interruptSuccessorWorkerSessionID {
		t.Fatalf("worker observations = source:%#v successor:%#v, want exact identities", source, successor)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "late.marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("late source effect marker error = %v, want absent after joined cancellation", err)
	}
	assertInterruptWorkNotAdvanced(t, ctx, serverURL)

	stopInterruptDaemon(t, binaryPath, factoryDir, serverURL, env, daemon)
	assertInterruptPortAvailable(t, port)
}

type interruptDaemon struct {
	cmd     *exec.Cmd
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
	stopped bool
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
}

func resolvePrebuiltArtifact(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(prebuiltArtifactEnvironment))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(prebuiltArtifactFallbackEnvironment))
	}
	if path == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(prebuiltArtifactRequiredEnvironment)), "1") {
			t.Fatalf("%s or %s is required; the integration test never builds the CLI", prebuiltArtifactEnvironment, prebuiltArtifactFallbackEnvironment)
		}
		t.Skip("prebuilt integration artifact unavailable; set INFINITE_YOU_INTEGRATION_BINARY")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt integration artifact %q: %v", path, err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		t.Fatalf("stat prebuilt integration artifact %q: %v", absPath, err)
	}
	if info.IsDir() {
		t.Fatalf("prebuilt integration artifact %q is a directory", absPath)
	}
	return absPath
}

func interruptIntegrationEnvironment(homeDir, providerDir, stateDir string) []string {
	env := builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	env = setInterruptEnvironment(env, "FR_A6_STATE", stateDir)
	return setInterruptEnvironment(env, "PATH", providerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func setInterruptEnvironment(env []string, key, value string) []string {
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, key+"="+value)
}

func startInterruptDaemon(t *testing.T, ctx context.Context, binaryPath, factoryDir, serverURL string, env []string) *interruptDaemon {
	t.Helper()
	command := exec.CommandContext(ctx, binaryPath,
		"run", "--dir", factoryDir, "--continuously", "--with-server",
		"--listen", strings.TrimPrefix(serverURL, "http://"), "--no-record", "--quiet",
	)
	command.Dir = factoryDir
	command.Env = append([]string(nil), env...)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start prebuilt factory daemon: %v", err)
	}
	daemon := &interruptDaemon{cmd: command, done: make(chan struct{}), stdout: stdout, stderr: stderr}
	go func() {
		err := command.Wait()
		daemon.mu.Lock()
		daemon.waitErr = err
		daemon.mu.Unlock()
		close(daemon.done)
	}()
	t.Cleanup(func() { cleanupInterruptDaemon(daemon) })
	return daemon
}

func waitForInterruptStatus(t *testing.T, ctx context.Context, daemon *interruptDaemon, serverURL string) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for {
		status, err := getInterruptJSON[factoryapi.StatusResponse](ctx, client, serverURL+"/status")
		if err == nil && status.FactoryState == "RUNNING" {
			return
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("factory state=%q runtime status=%q", status.FactoryState, status.RuntimeStatus)
		}
		select {
		case <-daemon.done:
			t.Fatalf("prebuilt factory daemon exited before readiness: %v\nstdout=%s\nstderr=%s", daemon.waitError(), daemon.stdout.String(), daemon.stderr.String())
		case <-ctx.Done():
			t.Fatalf("wait for prebuilt factory readiness: %v (last error: %v)\nstdout=%s\nstderr=%s", ctx.Err(), lastErr, daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
		}
	}
}

func (daemon *interruptDaemon) waitError() error {
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	return daemon.waitErr
}

type interruptBinaryResult struct {
	stdout string
	stderr string
	err    error
}

func runInterruptBinary(t *testing.T, ctx context.Context, binaryPath, workingDirectory string, env []string, args ...string) interruptBinaryResult {
	t.Helper()
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = workingDirectory
	command.Env = append([]string(nil), env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return interruptBinaryResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func assertInterruptInvokeAccepted(t *testing.T, result interruptBinaryResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("prebuilt worker-session invoke: %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.stdout)), &fields); err != nil {
		t.Fatalf("decode prebuilt invoke response: %v; stdout=%q stderr=%q", err, result.stdout, result.stderr)
	}
	var accepted bool
	var requestID, workerSessionID string
	if err := json.Unmarshal(fields["accepted"], &accepted); err != nil {
		t.Fatalf("decode invoke accepted field: %v; response=%s", err, result.stdout)
	}
	if err := json.Unmarshal(fields["requestId"], &requestID); err != nil {
		t.Fatalf("decode invoke request ID field: %v; response=%s", err, result.stdout)
	}
	if err := json.Unmarshal(fields["workerSessionId"], &workerSessionID); err != nil {
		t.Fatalf("decode invoke Worker Session ID field: %v; response=%s", err, result.stdout)
	}
	if !accepted || requestID != interruptSourceRequestID || workerSessionID != interruptSourceWorkerSessionID {
		t.Fatalf("prebuilt invoke response = %#v, want accepted exact source identity", fields)
	}
}

func postInterrupt(t *testing.T, ctx context.Context, serverURL string) factoryapi.WorkerSessionInterruptResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.WorkerSessionInterruptRequest{
		RequestId: interruptRequestID, SuccessorWorkerSessionId: interruptSuccessorWorkerSessionID,
		ReplacementMessage: interruptReplacementMessage,
	})
	if err != nil {
		t.Fatalf("marshal public interrupt request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions/" + url.PathEscape(interruptSourceWorkerSessionID) + "/interrupt"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create public interrupt request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST public interrupt: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read public interrupt response: %v", err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("public interrupt status = %d body=%s", response.StatusCode, body)
	}
	var decoded factoryapi.WorkerSessionInterruptResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode public interrupt response: %v body=%s", err, body)
	}
	return decoded
}

func waitForInterruptWorkerState(t *testing.T, ctx context.Context, serverURL, stateDir string, daemon *interruptDaemon, workerSessionID, wantState string) factoryapi.WorkerSessionObservation {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.WorkerSessionObservation
	var lastErr error
	for {
		observation, status, err := getInterruptWorker(ctx, client, serverURL, workerSessionID)
		if err == nil {
			last = observation
			if status == http.StatusOK && string(observation.State) == wantState {
				return observation
			}
			if status == http.StatusOK && (observation.State == factoryapi.WorkerSessionObservationState("FAILED") ||
				observation.State == factoryapi.WorkerSessionObservationState("CANCELED") ||
				observation.State == factoryapi.WorkerSessionObservationState("TERMINATED")) && wantState == "COMPLETED" {
				encoded, marshalErr := json.Marshal(observation)
				t.Fatalf("Worker Session %q reached terminal state %q while waiting for %q: observation=%s marshal error=%v\nprovider evidence=%s\ndaemon stdout=%s\ndaemon stderr=%s", workerSessionID, observation.State, wantState, encoded, marshalErr, interruptProviderEvidence(stateDir), daemon.stdout.String(), daemon.stderr.String())
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for Worker Session %q state %q: %v; last=%#v error=%v", workerSessionID, wantState, ctx.Err(), last, lastErr)
		case <-ticker.C:
		}
	}
}

func getInterruptWorker(ctx context.Context, client *http.Client, serverURL, workerSessionID string) (factoryapi.WorkerSessionObservation, int, error) {
	endpoint := strings.TrimSuffix(serverURL, "/") + "/worker-sessions/" + url.PathEscape(workerSessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, 0, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.WorkerSessionObservation{}, response.StatusCode, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.WorkerSessionObservation{}, response.StatusCode, fmt.Errorf("GET Worker Session %q returned HTTP %d: %s", workerSessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var observation factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(body, &observation); err != nil {
		return factoryapi.WorkerSessionObservation{}, response.StatusCode, err
	}
	return observation, response.StatusCode, nil
}

func assertInterruptWorkNotAdvanced(t *testing.T, ctx context.Context, serverURL string) {
	t.Helper()
	work, err := getInterruptJSON[factoryapi.ListWorkResponse](ctx, &http.Client{Timeout: 2 * time.Second},
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+url.PathEscape(interruptFactorySessionID)+"/work")
	if err != nil {
		t.Fatalf("GET public Work after interrupt: %v", err)
	}
	for _, item := range work.Results {
		if item.WorkId != nil && *item.WorkId == interruptWorkID {
			t.Fatalf("interrupt exposed direct-invocation Work %q: %#v", interruptWorkID, item)
		}
	}
}

func getInterruptJSON[T any](ctx context.Context, client *http.Client, endpoint string) (T, error) {
	var result T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return result, readErr
		}
		return result, fmt.Errorf("GET %s returned HTTP %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func waitForInterruptPID(t *testing.T, ctx context.Context, path string) int {
	t.Helper()
	content := waitForInterruptFile(t, ctx, path)
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || pid <= 0 {
		t.Fatalf("provider PID marker %q = %q: %v", path, content, err)
	}
	return pid
}

func waitForInterruptPIDOrFailure(t *testing.T, ctx context.Context, daemon *interruptDaemon, serverURL, path string) int {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if content, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(content)) > 0 {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(content)))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("provider PID marker %q = %q: %v", path, content, parseErr)
			}
			return pid
		}
		observation, status, err := getInterruptWorker(ctx, client, serverURL, interruptSourceWorkerSessionID)
		if err == nil && status == http.StatusOK {
			switch string(observation.State) {
			case "FAILED", "COMPLETED", "CANCELED", "TERMINATED":
				encoded, marshalErr := json.Marshal(observation)
				t.Fatalf("provider start marker %q was not produced; source state=%q observation=%s marshal error=%v\nprovider evidence=%s\ndaemon stdout=%s\ndaemon stderr=%s", path, observation.State, encoded, marshalErr, interruptProviderEvidence(filepath.Dir(path)), daemon.stdout.String(), daemon.stderr.String())
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for provider PID marker %q: %v", path, ctx.Err())
		case <-ticker.C:
		}
	}
}

func interruptProviderEvidence(directory string) string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Sprintf("<read directory failed: %v>", err)
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		content, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			parts = append(parts, entry.Name()+"=<read failed>")
			continue
		}
		parts = append(parts, entry.Name()+"="+strings.TrimSpace(string(content)))
	}
	return strings.Join(parts, ", ")
}

func waitForInterruptMarker(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	_ = waitForInterruptFile(t, ctx, path)
}

func waitForInterruptFile(t *testing.T, ctx context.Context, path string) []byte {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		content, err := os.ReadFile(path)
		if err == nil && len(bytes.TrimSpace(content)) > 0 {
			return content
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for provider evidence %q: %v (last read error: %v)", path, ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

func waitForInterruptProcessExit(t *testing.T, ctx context.Context, pid int) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		alive, err := interruptProcessAlive(pid)
		if err != nil {
			t.Fatalf("check provider PID %d after completion: %v", pid, err)
		}
		if !alive {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for provider PID %d to exit: %v", pid, ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForInterruptProviderSession(t *testing.T, ctx context.Context, serverURL, workerSessionID, providerSessionID string) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		observation, status, err := getInterruptWorker(ctx, client, serverURL, workerSessionID)
		if err == nil && status == http.StatusOK {
			if observation.ProviderSessionAvailable && observation.ProviderSession != nil &&
				observation.ProviderSession.Provider == "codex" &&
				observation.ProviderSession.Kind == "session_id" &&
				observation.ProviderSession.Id == providerSessionID {
				return
			}
			if observation.State == factoryapi.WorkerSessionObservationState("FAILED") ||
				observation.State == factoryapi.WorkerSessionObservationState("CANCELED") ||
				observation.State == factoryapi.WorkerSessionObservationState("TERMINATED") {
				t.Fatalf("Worker Session %q reached terminal state %q before provider session %q was observed", workerSessionID, observation.State, providerSessionID)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for public provider session %q on Worker Session %q: %v", providerSessionID, workerSessionID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func stopInterruptDaemon(t *testing.T, binaryPath, factoryDir, serverURL string, env []string, daemon *interruptDaemon) {
	t.Helper()
	if daemon == nil {
		return
	}
	result := runInterruptBinary(t, context.Background(), binaryPath, factoryDir, env,
		"--server", serverURL, "server", "stop")
	if result.err != nil {
		t.Fatalf("public server stop: %v\nstdout=%s\nstderr=%s", result.err, result.stdout, result.stderr)
	}
	waitInterruptDaemonExit(t, daemon, 20*time.Second)
	daemon.mu.Lock()
	daemon.stopped = true
	daemon.mu.Unlock()
}

func waitInterruptDaemonExit(t *testing.T, daemon *interruptDaemon, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-daemon.done:
		if err := daemon.waitError(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("prebuilt factory daemon exit after public stop = %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
		}
	case <-timer.C:
		t.Fatalf("prebuilt factory daemon did not exit after public stop\nstdout=%s\nstderr=%s", daemon.stdout.String(), daemon.stderr.String())
	}
}

func assertInterruptPortAvailable(t *testing.T, port int) {
	t.Helper()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("interrupt daemon port %d remained bound after stop: %v", port, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close interrupt daemon port %d availability probe: %v", port, err)
	}
}

func cleanupInterruptDaemon(daemon *interruptDaemon) {
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

func interruptProcessAlive(pid int) (bool, error) {
	if runtime.GOOS == "windows" {
		output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").CombinedOutput()
		if err != nil {
			return false, err
		}
		return strings.Contains(string(output), "\""+strconv.Itoa(pid)+"\""), nil
	}
	command := exec.Command("kill", "-0", strconv.Itoa(pid))
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func interruptExecutionDocument(workspaceDir string) string {
	document := map[string]any{
		"requestId":       interruptSourceRequestID,
		"workerSessionId": interruptSourceWorkerSessionID,
		"execution": map[string]any{
			"factorySessionId": interruptFactorySessionID,
			"workstationName":  workers.ProviderInvocationRoute,
			"dispatch": map[string]any{
				"dispatchId":      interruptSourceDispatchID,
				"workstationName": workers.ProviderInvocationRoute,
				"workerType":      "processor",
				"execution": map[string]any{
					"workIds": []string{interruptWorkID},
				},
			},
			"workerType":               "processor",
			"runnerId":                 "codex",
			"executorProvider":         "codex",
			"modelProvider":            "codex",
			"model":                    interruptModel,
			"workingDirectory":         workspaceDir,
			"workingDirectoryAuthored": true,
			"userMessage":              "fr-a6 source",
		},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(fmt.Sprintf("marshal integration execution document: %v", err))
	}
	return string(encoded)
}

func writeInterruptFactory(t *testing.T, factoryDir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(interruptFactoryJSON), 0o600); err != nil {
		t.Fatalf("write integration Current Factory: %v", err)
	}
}

const interruptFactoryJSON = `{
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

func writeInterruptProvider(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create integration provider directory: %v", err)
	}
	if runtime.GOOS == "windows" {
		writeInterruptProviderFile(t, filepath.Join(directory, "codex.ps1"), interruptProviderPowerShell)
		writeInterruptProviderFile(t, filepath.Join(directory, "codex.cmd"), interruptProviderCommand)
		return
	}
	path := filepath.Join(directory, "codex")
	writeInterruptProviderFile(t, path, interruptProviderShell)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("make integration provider executable: %v", err)
	}
}

func writeInterruptProviderFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write integration provider fixture %q: %v", path, err)
	}
}

const interruptProviderCommand = `@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0codex.ps1"
if errorlevel 1 (
  exit /b 1
)
exit /b 0
`

const interruptProviderPowerShell = `$ErrorActionPreference = "Stop"
$state = $env:FR_A6_STATE
Set-Content -Path "$state\ps.started" -Value "started" -NoNewline
$countPath = "$state\count"
$count = 0
if (Test-Path -Path $countPath) {
  $count = [int]([System.IO.File]::ReadAllText($countPath))
}
$count++
Set-Content -LiteralPath $countPath -Value ([string]$count) -NoNewline
$null = [Console]::In.ReadToEnd()
function Emit {
  param([string]$line)
  [Console]::WriteLine($line)
  [Console]::Out.Flush()
}
if ($count -eq 1) {
  Set-Content -Path "$state\source.pid" -Value ([string]$PID) -NoNewline
  Set-Content -Path "$state\source.started" -Value "started" -NoNewline
  Emit '{"type":"thread.started","thread_id":"fr-a6-source-thread"}'
  $waitHandle = New-Object -TypeName System.Threading.ManualResetEvent -ArgumentList $false
  $waitHandle.WaitOne()
  Set-Content -Path "$state\late.marker" -Value "late" -NoNewline
  exit 0
}
if ($count -eq 2) {
  Set-Content -Path "$state\successor.pid" -Value ([string]$PID) -NoNewline
  Set-Content -Path "$state\successor.started" -Value "started" -NoNewline
  Emit '{"type":"thread.started","thread_id":"fr-a6-source-thread"}'
  Emit '{"type":"item.completed","item":{"id":"fr-a6-result","type":"agent_message","text":"fr-a6 successor"}}'
  Emit '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
  Set-Content -Path "$state\successor.completed" -Value "completed" -NoNewline
  exit 0
}
Set-Content -Path "$state\unexpected.marker" -Value ([string]$count) -NoNewline
exit 1
`

const interruptProviderShell = `#!/bin/sh
set -eu
state=${FR_A6_STATE:?FR_A6_STATE is required}
count_path="$state/count"
count=0
if [ -f "$count_path" ]; then
  count=$(cat "$count_path")
fi
count=$((count + 1))
printf '%s' "$count" > "$count_path"
if [ "$count" -eq 1 ]; then
  printf '%s' "$$" > "$state/source.pid"
  printf '%s' started > "$state/source.started"
  printf '%s\n' '{"type":"thread.started","thread_id":"fr-a6-source-thread"}'
  exec tail -f /dev/null
  printf '%s' late > "$state/late.marker"
  exit 0
fi
if [ "$count" -eq 2 ]; then
  printf '%s' "$$" > "$state/successor.pid"
  printf '%s' started > "$state/successor.started"
  printf '%s\n' '{"type":"thread.started","thread_id":"fr-a6-source-thread"}'
  printf '%s\n' '{"type":"item.completed","item":{"id":"fr-a6-result","type":"agent_message","text":"fr-a6 successor"}}'
  printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
  printf '%s' completed > "$state/successor.completed"
  exit 0
fi
printf '%s' "$count" > "$state/unexpected.marker"
exit 1
`
