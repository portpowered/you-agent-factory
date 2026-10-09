package interrupt_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// I-T4 consumes the build lane's artifact and reconstructs the selected host
// over the same profile to observe committed stop facts without live handles.
func TestExactStopRecoveryPrebuilt(t *testing.T) {
	binary := resolvePrebuiltArtifact(t)
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%x fixture=exact-stop-v1 platform=%s/%s", binary, sha256.Sum256(content), runtime.GOOS, runtime.GOARCH)
	for _, action := range []string{"cancel", "terminate"} {
		t.Run("live-"+action, func(t *testing.T) {
			t.Parallel()
			fixture := startExactStopFixture(t, binary)
			fixture.invoke(t, "source")
			fixture.invoke(t, "sibling")
			fixture.assertLive(t, "source")
			fixture.assertLive(t, "sibling")
			fixture.stop(t, "source", action, "APPLIED")
			fixture.assertJoined(t, "source")
			fixture.assertTerminal(t, "source", action)
			for _, repeat := range []string{"cancel", "terminate"} {
				fixture.stop(t, "source", repeat, "NOOP")
				fixture.assertTerminal(t, "source", action)
			}
			fixture.assertLive(t, "sibling")
			fixture.stop(t, "sibling", "terminate", "APPLIED")
			fixture.assertJoined(t, "sibling")
			fixture.assertTerminal(t, "sibling", "terminate")
			fixture.restart(t)
			fixture.assertTerminal(t, "source", action)
			fixture.assertTerminal(t, "sibling", "terminate")
			fixture.assertCLIObservation(t, "source", action)
			for _, repeat := range []string{"cancel", "terminate"} {
				fixture.stop(t, "source", repeat, "NOOP")
				fixture.assertTerminal(t, "source", action)
			}
			fixture.assertJoined(t, "source")
			fixture.assertJoined(t, "sibling")
			stopInterruptDaemon(t, binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
			assertInterruptPortAvailable(t, fixture.port)
		})
	}
	t.Run("partial-admission-restart", func(t *testing.T) {
		t.Parallel()
		assertPartialInterruptRecovery(t, binary, "")
	})
	for _, fault := range []string{"intent", "torn-tail"} {
		t.Run(fault+"-restart", func(t *testing.T) {
			t.Parallel()
			assertPartialInterruptRecovery(t, binary, fault)
		})
	}
	t.Run("host-crash-without-terminal", func(t *testing.T) {
		t.Parallel()
		fixture := startExactStopFixture(t, binary)
		fixture.invoke(t, "source")
		fixture.assertLive(t, "source")
		t.Cleanup(func() { cleanupExactStopChild(fixture.pids["source"]) })
		fixture.assertIncompletePrefix(t)
		fixture.crashHost(t)
		fixture.daemon = startInterruptDaemon(t, fixture.ctx, binary, fixture.dir, fixture.url, fixture.env)
		waitForInterruptStatus(t, fixture.ctx, fixture.daemon, fixture.url)
		fixture.assertUnownedStopRefusal(t)
		fixture.assertIncompletePrefix(t)
		observation, status, err := getInterruptWorker(fixture.ctx, http.DefaultClient, fixture.url, "exact-source")
		if err != nil || status != http.StatusOK || string(observation.State) != "FAILED" ||
			observation.TerminalCause == nil || string(*observation.TerminalCause) != "OWNER_LOST" ||
			observation.Failure == nil || string(observation.Failure.Kind) != "PROCESS_GONE" {
			t.Fatalf("dead supervisor classification missing: observation=%+v HTTP %d error=%v", observation, status, err)
		}
		fixture.restart(t)
		replayed, _, err := getInterruptWorker(fixture.ctx, http.DefaultClient, fixture.url, "exact-source")
		if err != nil || !reflect.DeepEqual(observation, replayed) {
			t.Fatalf("owner loss changed after another boot: before=%+v after=%+v error=%v", observation, replayed, err)
		}
		stopInterruptDaemon(t, binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
		assertInterruptPortAvailable(t, fixture.port)
	})
}

// Kill only the fixture's host, allowing the production owner to disappear
// without delivering a terminal callback. Child cleanup is test-owned and
// deliberately happens after all public recovery assertions.
func (fixture *exactStopFixture) crashHost(t *testing.T) {
	t.Helper()
	if err := fixture.daemon.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.daemon.done:
	case <-time.After(30 * time.Second):
		t.Fatal("crashed host did not exit")
	}
	fixture.daemon.mu.Lock()
	fixture.daemon.stopped = true
	fixture.daemon.mu.Unlock()
	assertInterruptPortAvailable(t, fixture.port)
}

func cleanupExactStopChild(pid int) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
		return
	}
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
	}
}

func (fixture *exactStopFixture) assertIncompletePrefix(t *testing.T) {
	t.Helper()
	logs, err := getInterruptJSON[factoryapi.WorkerSessionLogPage](fixture.ctx, http.DefaultClient, fixture.url+"/worker-sessions/exact-source/logs")
	if err != nil || logs.Health != factoryapi.INCOMPLETE || logs.CommittedPosition == 0 {
		t.Fatalf("crash prefix fabricated a terminal: logs=%#v error=%v", logs, err)
	}
}

func (fixture *exactStopFixture) assertUnownedStopRefusal(t *testing.T) {
	t.Helper()
	for _, action := range []string{"cancel", "terminate"} {
		request, err := http.NewRequestWithContext(fixture.ctx, http.MethodPost, fixture.url+"/worker-sessions/exact-source/"+action, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var result factoryapi.ErrorResponse
		if readErr != nil || json.Unmarshal(body, &result) != nil || response.StatusCode != http.StatusServiceUnavailable || string(result.Code) != "WORKER_SESSION_CONTROL_FAILED" {
			t.Fatalf("unowned %s fabricated stop/join: status=%d error=%v body=%s", action, response.StatusCode, readErr, body)
		}
		cliResult := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
			"--remote", "--server", fixture.url, "--json", "worker-sessions", action, "exact-source")
		if cliResult.err == nil || !strings.Contains(cliResult.stdout+cliResult.stderr, "WORKER_SESSION_CONTROL_FAILED") {
			t.Fatalf("unowned CLI %s disagrees: error=%v stdout=%s stderr=%s", action, cliResult.err, cliResult.stdout, cliResult.stderr)
		}
		t.Logf("crashed-host %s refuses without claiming APPLIED/NOOP: HTTP %d %s", action, response.StatusCode, body)
	}
}

func (fixture *exactStopFixture) restart(t *testing.T) {
	t.Helper()
	stopInterruptDaemon(t, fixture.binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
	assertInterruptPortAvailable(t, fixture.port)
	fixture.daemon = startInterruptDaemon(t, fixture.ctx, fixture.binary, fixture.dir, fixture.url, fixture.env)
	waitForInterruptStatus(t, fixture.ctx, fixture.daemon, fixture.url)
	t.Log("selected host restarted over the same isolated profile and recording directory")
}

func (fixture *exactStopFixture) assertCLIObservation(t *testing.T, name, action string) {
	t.Helper()
	result := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", "show", "--worker-session-id", "exact-"+name)
	var observation factoryapi.WorkerSessionObservation
	state, cause := "CANCELED", "OPERATOR_CANCEL"
	if action == "terminate" {
		state, cause = "TERMINATED", "OPERATOR_TERMINATE"
	}
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &observation) != nil ||
		observation.WorkerSessionId != "exact-"+name || string(observation.State) != state ||
		observation.TerminalCause == nil || string(*observation.TerminalCause) != cause {
		t.Fatalf("recovered CLI show %s: error=%v stdout=%s stderr=%s", name, result.err, result.stdout, result.stderr)
	}
	t.Logf("recovered selected-host CLI show exact-%s: %s", name, strings.TrimSpace(result.stdout))
}

type exactStopFixture struct {
	ctx        context.Context
	binary     string
	dir        string
	state      string
	url        string
	port       int
	env        []string
	daemon     *interruptDaemon
	pids       map[string]int
	dispatches map[string]string
}

func startExactStopFixture(t *testing.T, binary string) *exactStopFixture {
	t.Helper()
	return startExactStopProviderFixture(t, binary, exactStopProviderPowerShell, exactStopProviderShell)
}

func startExactStopProviderFixture(t *testing.T, binary, powershell, shell string) *exactStopFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), interruptIntegrationTimeout)
	t.Cleanup(cancel)
	root := t.TempDir()
	dir := filepath.Join(root, "factory")
	state := filepath.Join(root, "state")
	provider := filepath.Join(root, "provider")
	home := filepath.Join(root, "home")
	for _, path := range []string{dir, state, provider, home} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeInterruptFactory(t, dir)
	if runtime.GOOS == "windows" {
		writeInterruptProviderFile(t, filepath.Join(provider, "codex.cmd"), interruptProviderCommand)
		writeInterruptProviderFile(t, filepath.Join(provider, "codex.ps1"), powershell)
	} else {
		writeInterruptProviderFile(t, filepath.Join(provider, "codex"), shell)
	}
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	url := "http://127.0.0.1:" + strconv.Itoa(port)
	env := interruptIntegrationEnvironment(home, provider, state)
	daemon := startInterruptDaemon(t, ctx, binary, dir, url, env)
	waitForInterruptStatus(t, ctx, daemon, url)
	return &exactStopFixture{ctx: ctx, binary: binary, dir: dir, state: state, url: url,
		port: port, env: env, daemon: daemon, pids: make(map[string]int), dispatches: make(map[string]string)}
}

func (fixture *exactStopFixture) invoke(t *testing.T, name string) {
	t.Helper()
	fixture.invokeDispatch(t, name, "exact-"+name+"-dispatch")
}

func (fixture *exactStopFixture) invokeDispatch(t *testing.T, name, dispatch string) {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(interruptExecutionDocument(fixture.dir)), &document); err != nil {
		t.Fatal(err)
	}
	id := "exact-" + name
	document["requestId"] = id + "-request"
	document["workerSessionId"] = id
	execution := document["execution"].(map[string]any)
	execution["userMessage"] = name
	execution["dispatch"].(map[string]any)["dispatchId"] = dispatch
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	result := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", "invoke",
		"--request-id", id+"-request", "--worker-session-id", id, "--dispatch-id", dispatch,
		"--execution", string(encoded), "--retry-max-attempts", "1", "--async")
	var response struct {
		Accepted        bool   `json:"accepted"`
		WorkerSessionId string `json:"workerSessionId"`
	}
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || !response.Accepted || response.WorkerSessionId != id {
		t.Fatalf("invoke %s: error=%v stdout=%s stderr=%s", id, result.err, result.stdout, result.stderr)
	}
	fixture.pids[name] = waitForInterruptPID(t, fixture.ctx, filepath.Join(fixture.state, name+".pid"))
	fixture.dispatches[name] = dispatch
	waitForInterruptMarker(t, fixture.ctx, filepath.Join(fixture.state, name+".started"))
}

func (fixture *exactStopFixture) assertLive(t *testing.T, name string) {
	t.Helper()
	observation := waitForInterruptWorkerState(t, fixture.ctx, fixture.url, fixture.state, fixture.daemon, "exact-"+name, "RUNNING")
	if observation.ProviderSessionAvailable || observation.ProviderSession != nil || observation.TerminalCause != nil {
		t.Fatalf("live no-reference %s: %#v", name, observation)
	}
	alive, err := interruptProcessAlive(fixture.pids[name])
	if err != nil || !alive {
		t.Fatalf("live %s PID %d: alive=%t error=%v", name, fixture.pids[name], alive, err)
	}
}

func (fixture *exactStopFixture) stop(t *testing.T, name, action, outcome string) {
	t.Helper()
	result := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", action, "exact-"+name)
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != outcome || response.DispatchId != fixture.dispatches[name] {
		t.Fatalf("%s %s: error=%v stdout=%s stderr=%s", action, name, result.err, result.stdout, result.stderr)
	}
	t.Logf("selected-host CLI %s exact-%s: %s", action, name, strings.TrimSpace(result.stdout))
}

func (fixture *exactStopFixture) assertJoined(t *testing.T, name string) {
	t.Helper()
	alive, err := interruptProcessAlive(fixture.pids[name])
	if err != nil || alive {
		t.Fatalf("APPLIED returned before child %s PID %d was gone: alive=%t error=%v", name, fixture.pids[name], alive, err)
	}
}

func (fixture *exactStopFixture) assertTerminal(t *testing.T, name, action string) {
	t.Helper()
	state, cause := "CANCELED", "OPERATOR_CANCEL"
	if action == "terminate" {
		state, cause = "TERMINATED", "OPERATOR_TERMINATE"
	}
	observation, status, err := getInterruptWorker(fixture.ctx, http.DefaultClient, fixture.url, "exact-"+name)
	if err != nil || status != http.StatusOK || string(observation.State) != state || observation.TerminalCause == nil || string(*observation.TerminalCause) != cause {
		t.Fatalf("terminal %s: observation=%#v status=%d error=%v want=%s/%s", name, observation, status, err, state, cause)
	}
	t.Logf("HTTP observation exact-%s: state=%s terminalCause=%s", name, state, cause)
}

// A sibling owns the future dispatch without owning the reserved successor ID.
// The source must join before continuation detects this admission conflict.
func assertPartialInterruptRecovery(t *testing.T, binary, fault string) {
	t.Helper()
	thread := `{"type":"thread.started","thread_id":"exact-source-thread"}`
	ps := strings.Replace(exactStopProviderPowerShell, "[Console]::Out.Flush()", "[Console]::WriteLine('"+thread+"')\n[Console]::Out.Flush()", 1)
	sh := strings.Replace(exactStopProviderShell, "printf '%s' \"$$\"", "printf '%s\\n' '"+thread+"'\nprintf '%s' \"$$\"", 1)
	ps = strings.Replace(ps, "$name = \"source\"", "if (Test-Path -LiteralPath \"$state\\sibling.started\") { Set-Content -LiteralPath \"$state\\unexpected.marker\" -Value \"third invocation\"; exit 1 }\n$name = \"source\"", 1)
	sh = strings.Replace(sh, "name=source", "if [ -f \"$state/sibling.started\" ]; then printf unexpected > \"$state/unexpected.marker\"; exit 1; fi\nname=source", 1)
	fixture := startExactStopProviderFixture(t, binary, ps, sh)
	fixture.invoke(t, "source")
	waitForInterruptProviderSession(t, fixture.ctx, fixture.url, "exact-source", "exact-source-thread")
	fixture.invokeDispatch(t, "sibling", "exact-source-dispatch/continue/exact-successor")
	first := fixture.partialInterrupt(t)
	fixture.assertJoined(t, "source")
	fixture.assertTerminal(t, "source", "cancel")
	fixture.assertSiblingAlive(t)
	if repeated := fixture.partialInterrupt(t); !reflect.DeepEqual(first, repeated) {
		t.Fatalf("live partial replay changed: first=%#v replay=%#v", first, repeated)
	}
	fixture.stop(t, "sibling", "terminate", "APPLIED")
	fixture.assertJoined(t, "sibling")
	if fault != "" {
		stopInterruptDaemon(t, binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
		fixture.retainInterruptIntent(t, fault == "torn-tail")
		fixture.daemon = startInterruptDaemon(t, fixture.ctx, binary, fixture.dir, fixture.url, fixture.env)
		waitForInterruptStatus(t, fixture.ctx, fixture.daemon, fixture.url)
		fixture.assertIncompleteInterruptRecovery(t, fault)
		return
	}
	fixture.restart(t)
	if repeated := fixture.partialInterrupt(t); !reflect.DeepEqual(first, repeated) {
		t.Fatalf("reconstructed partial replay changed: first=%#v replay=%#v", first, repeated)
	}
	fixture.assertTerminal(t, "source", "cancel")
	fixture.assertCLIObservation(t, "source", "cancel")
	fixture.assertPartialCLI(t, first)
	_, status, _ := getInterruptWorker(fixture.ctx, http.DefaultClient, fixture.url, "exact-successor")
	if status != http.StatusNotFound {
		t.Fatalf("failed successor became inspectable after restart: HTTP %d", status)
	}
	fixture.assertJoined(t, "source")
	fixture.assertJoined(t, "sibling")
	if _, err := os.Stat(filepath.Join(fixture.state, "unexpected.marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial replay executed another provider: %v", err)
	}
	stopInterruptDaemon(t, binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
	assertInterruptPortAvailable(t, fixture.port)
	t.Log("delivered restart replays exact SUCCESSOR_ADMISSION failure and OPERATOR_CANCEL without successor execution")
}

// Crash-window fault injection retains an actual synced public-operation INTENT
// and an unterminated next row. Later callback facts are deliberately absent:
// the reconstructed host must not infer them from this test's child cleanup.
func (fixture *exactStopFixture) retainInterruptIntent(t *testing.T, torn bool) {
	t.Helper()
	matched := 0
	err := filepath.WalkDir(fixture.dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".worker.jsonl") {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		offset := 0
		for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
			offset += len(line)
			var row struct {
				ControlOperation struct {
					Operation struct {
						RequestID string `json:"requestId"`
						Phase     string `json:"phase"`
					} `json:"operation"`
				} `json:"controlOperation"`
			}
			if json.Unmarshal(line, &row) == nil && row.ControlOperation.Operation.RequestID == "exact-partial" && row.ControlOperation.Operation.Phase == "INTENT" {
				matched++
				prefix := bytes.Clone(data[:offset])
				if torn {
					prefix = append(prefix, []byte(`{"version":2,"kind":"control-operation"`)...)
				}
				return os.WriteFile(path, prefix, 0o600)
			}
		}
		return nil
	})
	if err != nil || matched != 1 {
		t.Fatalf("retain one synced INTENT with torn tail: matched=%d error=%v", matched, err)
	}
}

func (fixture *exactStopFixture) assertIncompleteInterruptRecovery(t *testing.T, fault string) {
	t.Helper()
	wantStatus, wantCode := http.StatusServiceUnavailable, "WORKER_SESSION_INTERRUPT_ADMISSION_FAILED"
	if fault == "torn-tail" {
		wantStatus, wantCode = http.StatusInternalServerError, "INTERNAL_ERROR"
	}
	for range 2 {
		payload := []byte(`{"requestId":"exact-partial","successorWorkerSessionId":"exact-successor","replacementMessage":"replacement"}`)
		request, err := http.NewRequestWithContext(fixture.ctx, http.MethodPost, fixture.url+"/worker-sessions/exact-source/interrupt", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		var result factoryapi.WorkerSessionInterruptError
		if readErr != nil || json.Unmarshal(body, &result) != nil || response.StatusCode != wantStatus ||
			result.Code != wantCode || result.Phase != "VALIDATION" || result.Successor != nil || result.Source != nil {
			t.Fatalf("INTENT/torn-tail retry fabricated effects: status=%d error=%v body=%s", response.StatusCode, readErr, body)
		}
		t.Logf("reconstructed INTENT/torn-tail HTTP %d: %s", response.StatusCode, body)
	}
	fixture.assertIncompleteCLIAndLogs(t, wantCode)
	fixture.assertJoined(t, "source")
	fixture.assertJoined(t, "sibling")
	fixture.assertUnadmittedSuccessor(t, fault)
	if _, err := os.Stat(filepath.Join(fixture.state, "unexpected.marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete intent executed provider: %v", err)
	}
	stopInterruptDaemon(t, fixture.binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
	assertInterruptPortAvailable(t, fixture.port)
}

func (fixture *exactStopFixture) assertUnadmittedSuccessor(t *testing.T, fault string) {
	t.Helper()
	request, err := http.NewRequestWithContext(fixture.ctx, http.MethodGet, fixture.url+"/worker-sessions/exact-successor", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	wantStatus, wantCode := http.StatusNotFound, factoryapi.ErrorResponseCode("NOT_FOUND")
	if fault == "torn-tail" {
		// Incomplete archived membership cannot prove absence. This error alone
		// does not prove that the successor was never admitted to the live host.
		wantStatus, wantCode = http.StatusInternalServerError, "PROJECTION_UNAVAILABLE"
	}
	if response.StatusCode != wantStatus || result.Code != wantCode {
		t.Fatalf("incomplete intent successor lookup: HTTP %d error=%#v want=%d/%s", response.StatusCode, result, wantStatus, wantCode)
	}
	live, err := getInterruptJSON[factoryapi.ListWorkerSessionsResponse](fixture.ctx, http.DefaultClient,
		fixture.url+"/worker-sessions?history=active&maxResults=100")
	if err != nil {
		t.Fatal(err)
	}
	if live.PaginationContext != nil && live.PaginationContext.NextToken != nil {
		t.Fatal("live Worker Session inventory is incomplete")
	}
	for _, session := range live.Sessions {
		if session.WorkerSessionId == "exact-successor" {
			t.Fatalf("incomplete intent admitted live successor: %#v", session)
		}
	}
	t.Logf("successor detail HTTP %d/%s; exact-successor absent from live Worker Sessions", response.StatusCode, result.Code)
}

func (fixture *exactStopFixture) assertIncompleteCLIAndLogs(t *testing.T, wantCode string) {
	t.Helper()
	cliResult := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", "interrupt", "exact-source",
		"--request-id", "exact-partial", "--successor-worker-session-id", "exact-successor",
		"--replacement-message", "replacement", "--async")
	if cliResult.err == nil || !strings.Contains(cliResult.stdout+cliResult.stderr, wantCode) ||
		!strings.Contains(cliResult.stdout+cliResult.stderr, "VALIDATION") {
		t.Fatalf("incomplete replay CLI error disagrees: error=%v stdout=%s stderr=%s", cliResult.err, cliResult.stdout, cliResult.stderr)
	}
	logs, err := getInterruptJSON[factoryapi.WorkerSessionLogPage](fixture.ctx, http.DefaultClient, fixture.url+"/worker-sessions/exact-source/logs")
	if err != nil || logs.Health != factoryapi.INCOMPLETE || logs.CommittedPosition == 0 {
		t.Fatalf("incomplete capture prefix: logs=%#v error=%v", logs, err)
	}
}

func (fixture *exactStopFixture) assertSiblingAlive(t *testing.T) {
	t.Helper()
	alive, err := interruptProcessAlive(fixture.pids["sibling"])
	if err != nil || !alive {
		t.Fatalf("partial interruption affected sibling: alive=%t error=%v", alive, err)
	}
}

func (fixture *exactStopFixture) partialInterrupt(t *testing.T) factoryapi.WorkerSessionInterruptError {
	t.Helper()
	payload := []byte(`{"requestId":"exact-partial","successorWorkerSessionId":"exact-successor","replacementMessage":"replacement"}`)
	request, err := http.NewRequestWithContext(fixture.ctx, http.MethodPost, fixture.url+"/worker-sessions/exact-source/interrupt", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	var result factoryapi.WorkerSessionInterruptError
	if err != nil || json.Unmarshal(body, &result) != nil || response.StatusCode != http.StatusServiceUnavailable || result.Code != "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED" ||
		result.Phase != "SUCCESSOR_ADMISSION" || result.Source == nil || result.Source.State != "CANCELED" || result.Successor != nil ||
		result.RequestId == nil || *result.RequestId != "exact-partial" || result.SuccessorWorkerSessionId == nil || *result.SuccessorWorkerSessionId != "exact-successor" {
		t.Fatalf("partial interrupt: status=%d error=%v body=%s", response.StatusCode, err, body)
	}
	t.Logf("partial interrupt HTTP %d: %s", response.StatusCode, body)
	return result
}

func (fixture *exactStopFixture) assertPartialCLI(t *testing.T, expected factoryapi.WorkerSessionInterruptError) {
	t.Helper()
	result := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", "interrupt", "exact-source",
		"--request-id", "exact-partial", "--successor-worker-session-id", "exact-successor",
		"--replacement-message", "replacement", "--async")
	if result.err == nil || !strings.Contains(result.stderr+result.stdout, expected.Code) || !strings.Contains(result.stderr+result.stdout, "SUCCESSOR_ADMISSION") {
		t.Fatalf("reconstructed CLI partial failure: error=%v stdout=%s stderr=%s", result.err, result.stdout, result.stderr)
	}
}

// The provider deliberately emits no thread/session ID. Its first two calls
// remain blocked in real child processes; public controls must stop only one.
const exactStopProviderPowerShell = `$ErrorActionPreference = "Stop"
$state = $env:FR_A6_STATE
$name = "source"
if (Test-Path -LiteralPath "$state\source.started") { $name = "sibling" }
$null = [Console]::In.ReadToEnd()
[Console]::WriteLine('{"type":"item.completed","item":{"id":"progress","type":"agent_message","text":"captured progress"}}')
[Console]::Out.Flush()
Set-Content -LiteralPath "$state\$name.pid" -Value ([string]$PID) -NoNewline
Set-Content -LiteralPath "$state\$name.started" -Value "started" -NoNewline
$waitHandle = New-Object -TypeName System.Threading.ManualResetEvent -ArgumentList $false
$null = $waitHandle.WaitOne()
`

const exactStopProviderShell = `#!/bin/sh
set -eu
state=${FR_A6_STATE:?FR_A6_STATE is required}
name=source
if [ -f "$state/source.started" ]; then name=sibling; fi
printf '%s\n' '{"type":"item.completed","item":{"id":"progress","type":"agent_message","text":"captured progress"}}'
printf '%s' "$$" > "$state/$name.pid"
printf '%s' started > "$state/$name.started"
exec tail -f /dev/null
`
