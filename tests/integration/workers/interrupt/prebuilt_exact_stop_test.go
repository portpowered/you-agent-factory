package interrupt_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This first I-T4 cell consumes the build lane's artifact. Restart/interrupt
// recovery is a separate cell owned by story 003, not inferred from live stops.
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
			stopInterruptDaemon(t, binary, fixture.dir, fixture.url, fixture.env, fixture.daemon)
			assertInterruptPortAvailable(t, fixture.port)
		})
	}
}

type exactStopFixture struct {
	ctx    context.Context
	binary string
	dir    string
	state  string
	url    string
	port   int
	env    []string
	daemon *interruptDaemon
	pids   map[string]int
}

func startExactStopFixture(t *testing.T, binary string) *exactStopFixture {
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
		writeInterruptProviderFile(t, filepath.Join(provider, "codex.ps1"), exactStopProviderPowerShell)
	} else {
		writeInterruptProviderFile(t, filepath.Join(provider, "codex"), exactStopProviderShell)
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
		port: port, env: env, daemon: daemon, pids: make(map[string]int)}
}

func (fixture *exactStopFixture) invoke(t *testing.T, name string) {
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
	execution["dispatch"].(map[string]any)["dispatchId"] = id + "-dispatch"
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	result := runInterruptBinary(t, fixture.ctx, fixture.binary, fixture.dir, fixture.env,
		"--remote", "--server", fixture.url, "--json", "worker-sessions", "invoke",
		"--request-id", id+"-request", "--worker-session-id", id, "--dispatch-id", id+"-dispatch",
		"--execution", string(encoded), "--retry-max-attempts", "1", "--async")
	var response struct {
		Accepted        bool   `json:"accepted"`
		WorkerSessionId string `json:"workerSessionId"`
	}
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || !response.Accepted || response.WorkerSessionId != id {
		t.Fatalf("invoke %s: error=%v stdout=%s stderr=%s", id, result.err, result.stdout, result.stderr)
	}
	fixture.pids[name] = waitForInterruptPID(t, fixture.ctx, filepath.Join(fixture.state, name+".pid"))
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
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != outcome || response.DispatchId != "exact-"+name+"-dispatch" {
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
